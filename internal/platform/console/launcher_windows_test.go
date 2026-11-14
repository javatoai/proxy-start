//go:build windows

package console

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"proxy-start/internal/application"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestConsoleChildEnvironmentAndParentIsolation(t *testing.T) {
	t.Setenv("HTTP_PROXY", "parent-proxy")
	t.Setenv("NO_PROXY", "inherited-bypass")
	environment, unset := application.BuildProxyEnvironment(7897)
	path := filepath.Join(t.TempDir(), "child.json")
	environment["PROXY_START_TEST_OUTPUT"] = path
	script := `$data = @{http=$env:HTTP_PROXY;https=$env:HTTPS_PROXY;all=$env:ALL_PROXY;bypass=$env:NO_PROXY}; [IO.File]::WriteAllText($env:PROXY_START_TEST_OUTPUT,($data | ConvertTo-Json -Compress))`
	launcher := &Launcher{commandArgs: []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", script}, waitForExit: true, hideWindow: true}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := launcher.Open(ctx, application.ConsoleSpec{Environment: environment, UnsetEnvironment: unset}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]string
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"http", "https", "all"} {
		if result[key] != "http://127.0.0.1:7897" {
			t.Fatal(key, result)
		}
	}
	if result["bypass"] != "" || os.Getenv("HTTP_PROXY") != "parent-proxy" || os.Getenv("NO_PROXY") != "inherited-bypass" {
		t.Fatal("proxy environment escaped the child")
	}
}

func TestConsoleCurlUsesProxyAndUnreachableProxyFails(t *testing.T) {
	curl := filepath.Join(os.Getenv("SystemRoot"), "System32", "curl.exe")
	if _, err := os.Stat(curl); err != nil {
		t.Skip("Windows curl.exe is unavailable")
	}
	var requests atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Host != "proxy-start.invalid" {
			t.Errorf("request was not sent as a proxy request: %s", r.URL)
		}
		requests.Add(1)
		_, _ = w.Write([]byte("proxy-start-ok"))
	}))
	defer proxy.Close()
	_, portString, _ := net.SplitHostPort(strings.TrimPrefix(proxy.URL, "http://"))
	port, _ := strconv.Atoi(portString)
	run := func(port int) (string, error) {
		environment, unset := application.BuildProxyEnvironment(port)
		path := filepath.Join(t.TempDir(), "response.txt")
		environment["PROXY_START_TEST_OUTPUT"], environment["PROXY_START_TEST_CURL"] = path, curl
		script := `$result = & $env:PROXY_START_TEST_CURL --silent --show-error --fail --max-time 3 http://proxy-start.invalid/probe 2>$null; $code=$LASTEXITCODE; [IO.File]::WriteAllText($env:PROXY_START_TEST_OUTPUT,($result -join '')); exit $code`
		launcher := &Launcher{commandArgs: []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", script}, waitForExit: true, hideWindow: true}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		err := launcher.Open(ctx, application.ConsoleSpec{Environment: environment, UnsetEnvironment: unset})
		data, _ := os.ReadFile(path)
		return string(data), err
	}
	if data, err := run(port); err != nil || data != "proxy-start-ok" || requests.Load() != 1 {
		t.Fatalf("proxy request: %q %v, count %d", data, err, requests.Load())
	}
	unused, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, closedString, _ := net.SplitHostPort(unused.Addr().String())
	closedPort, _ := strconv.Atoi(closedString)
	_ = unused.Close()
	if _, err := run(closedPort); err == nil {
		t.Fatal("request succeeded with unavailable proxy")
	}
}
