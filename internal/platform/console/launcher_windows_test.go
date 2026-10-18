//go:build windows

package console

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"proxy-start/internal/application"
	"proxy-start/internal/domain"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Opt-in because it starts a short-lived PowerShell process. It only inspects
// a process created by this test and never opens the user's interactive shell.
func TestWindowsChildEnvironmentIntegration(t *testing.T) {
	if os.Getenv("PROXY_START_CONSOLE_REAL") != "1" {
		t.Skip("set PROXY_START_CONSOLE_REAL=1 to run the Windows process check")
	}
	powershell, err := powershellPath()
	if err != nil {
		t.Fatal(err)
	}
	fixturePassword := "fixture-password-never-argv"
	proxyEnv, unset, err := application.BuildProxyEnvironment(domain.ProxyEndpoint{Host: "127.0.0.1", Port: 7897, Username: "fixture-user", Password: fixturePassword}, domain.ProtocolHTTP)
	if err != nil {
		t.Fatal(err)
	}
	proxyEnv["PROXY_START_CONSOLE_REAL_VALUE"] = "child-only"
	parentHTTPProxy := os.Getenv("HTTP_PROXY")
	cmd := exec.Command(powershell, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", "Write-Output ($env:PROXY_START_CONSOLE_REAL_VALUE + '|' + $env:HTTP_PROXY + '|' + $env:HTTPS_PROXY + '|' + $env:ALL_PROXY + '|' + [string]$env:NO_PROXY + '|' + [string]$env:no_proxy)")
	args := strings.Join(cmd.Args, " ")
	if strings.Contains(args, fixturePassword) {
		t.Fatal("fixture password leaked into child arguments")
	}
	cmd.Env = childEnvironment(proxyEnv, unset)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("child process failed: %v", err)
	}
	fields := strings.SplitN(strings.TrimSpace(string(output)), "|", 6)
	if len(fields) != 6 || fields[0] != "child-only" || fields[1] != proxyEnv["HTTP_PROXY"] || fields[2] != proxyEnv["HTTPS_PROXY"] || fields[3] != proxyEnv["ALL_PROXY"] || fields[4] != "" || fields[5] != "" {
		t.Fatalf("child did not receive injected environment: %q", output)
	}
	if os.Getenv("HTTP_PROXY") != parentHTTPProxy {
		t.Fatal("parent process environment was modified")
	}
}

// TestWindowsConsoleProxyNetworkIntegration starts the same launcher used by
// the application, but supplies a test-only commandArgs seam so the child
// PowerShell runs one bounded curl request and exits. The request uses only
// the injected environment variables. A local recording CONNECT forwarder
// sends that request to the user's already-running HTTP proxy on 127.0.0.1:7897.
//
// This is deliberately opt-in: it starts a short-lived child process and
// makes one public HTTPS request through the local proxy. It never changes
// the saved settings or system proxy.
func TestWindowsConsoleProxyNetworkIntegration(t *testing.T) {
	if os.Getenv("PROXY_START_CONSOLE_NETWORK") != "1" {
		t.Skip("set PROXY_START_CONSOLE_NETWORK=1 to run the live console proxy check")
	}
	forwarder, err := newHTTPConnectForwarder("127.0.0.1:7897")
	if err != nil {
		t.Fatal(err)
	}
	defer forwarder.Close()

	resultPath := filepath.Join(t.TempDir(), "curl-result.txt")
	forwarderHost, forwarderPortText, err := net.SplitHostPort(forwarder.Addr())
	if err != nil {
		t.Fatal(err)
	}
	forwarderPort, err := strconv.Atoi(forwarderPortText)
	if err != nil {
		t.Fatal(err)
	}
	proxyEnv, unsetEnvironment, err := application.BuildProxyEnvironment(domain.ProxyEndpoint{Host: forwarderHost, Port: forwarderPort}, domain.ProtocolHTTP)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL := proxyEnv["HTTPS_PROXY"]
	command := networkCheckPowerShell(resultPath)
	launcher := &Launcher{commandArgs: []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", command}, waitForExit: true}
	if err := launcher.Open(context.Background(), application.ConsoleSpec{Environment: proxyEnv, UnsetEnvironment: unsetEnvironment}); err != nil {
		t.Fatal(err)
	}
	status, envValue, code, err := waitForNetworkCheck(resultPath, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if status != 0 {
		t.Fatalf("curl through the console proxy failed: exit=%d http=%s env=%s", status, code, envValue)
	}
	if envValue != proxyURL {
		t.Fatalf("child did not use the injected HTTPS_PROXY: got %q want %q", envValue, proxyURL)
	}
	if code != "200" {
		t.Fatalf("curl did not complete the expected HTTPS request: HTTP status %q", code)
	}
	target := forwarder.WaitForConnect(5 * time.Second)
	if target == "" {
		t.Fatal("the recording forwarder did not observe an HTTP CONNECT request; curl may have bypassed the proxy")
	}
	if target != "example.com:443" {
		t.Fatalf("unexpected CONNECT target %q", target)
	}
	t.Logf("HTTP proxy request: child HTTPS_PROXY=%s; CONNECT=%s; curl_exit=%d; http_status=%s", proxyURL, target, status, code)

	// A closed proxy endpoint is a negative control. If curl ignored the
	// environment and connected directly, it would return a successful status.
	badListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	badAddr := badListener.Addr().String()
	_ = badListener.Close()
	badResultPath := filepath.Join(t.TempDir(), "curl-bad-result.txt")
	badHost, badPortText, err := net.SplitHostPort(badAddr)
	if err != nil {
		t.Fatal(err)
	}
	badPort, err := strconv.Atoi(badPortText)
	if err != nil {
		t.Fatal(err)
	}
	badProxy, badUnsetEnvironment, err := application.BuildProxyEnvironment(domain.ProxyEndpoint{Host: badHost, Port: badPort}, domain.ProtocolHTTP)
	if err != nil {
		t.Fatal(err)
	}
	badCommand := networkCheckPowerShell(badResultPath)
	badLauncher := &Launcher{commandArgs: []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", badCommand}, waitForExit: true}
	if err := badLauncher.Open(context.Background(), application.ConsoleSpec{Environment: badProxy, UnsetEnvironment: badUnsetEnvironment}); err != nil {
		t.Fatal(err)
	}
	badStatus, badEnv, badCode, err := waitForNetworkCheck(badResultPath, 15*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if badStatus == 0 || badCode != "000" || badEnv != badProxy["HTTPS_PROXY"] {
		t.Fatalf("bad proxy control unexpectedly succeeded or did not use env: exit=%d http=%s env=%s", badStatus, badCode, badEnv)
	}
	t.Logf("negative control: child HTTPS_PROXY=%s; curl_exit=%d; http_status=%s", badEnv, badStatus, badCode)

	// The user's mixed Clash/Mihomo port is also exposed as SOCKS5. This
	// branch verifies the other environment form without adding a second test
	// proxy or passing --proxy to curl.
	socksResultPath := filepath.Join(t.TempDir(), "curl-socks-result.txt")
	socksEnvironment, socksUnsetEnvironment, err := application.BuildProxyEnvironment(domain.ProxyEndpoint{Host: "127.0.0.1", Port: 7897}, domain.ProtocolSOCKS5)
	if err != nil {
		t.Fatal(err)
	}
	socksLauncher := &Launcher{commandArgs: []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", networkCheckPowerShell(socksResultPath)}, waitForExit: true}
	if err := socksLauncher.Open(context.Background(), application.ConsoleSpec{Environment: socksEnvironment, UnsetEnvironment: socksUnsetEnvironment}); err != nil {
		t.Fatal(err)
	}
	socksStatus, socksEnv, socksCode, err := waitForNetworkCheck(socksResultPath, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if socksStatus != 0 || socksCode != "200" || socksEnv != socksEnvironment["HTTPS_PROXY"] {
		t.Fatalf("SOCKS5 environment request failed: exit=%d http=%s env=%s", socksStatus, socksCode, socksEnv)
	}
	t.Logf("SOCKS5 environment request: child HTTPS_PROXY=%s; curl_exit=%d; http_status=%s; upstream=127.0.0.1:7897", socksEnv, socksStatus, socksCode)
}

func networkCheckPowerShell(resultPath string) string {
	path := powershellLiteral(resultPath)
	return "$code = & curl.exe -sS --connect-timeout 5 --max-time 20 -o NUL -w '%{http_code}' https://example.com/ 2>$null; $exit = $LASTEXITCODE; [IO.File]::WriteAllText(" + path + ", ($exit.ToString() + '|' + [string]$env:HTTPS_PROXY + '|' + [string]$code))"
}

func powershellLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func waitForNetworkCheck(path string, timeout time.Duration) (int, string, string, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if bytes, err := os.ReadFile(path); err == nil {
			text := strings.TrimPrefix(string(bytes), "\ufeff")
			fields := strings.SplitN(strings.TrimSpace(text), "|", 3)
			if len(fields) == 3 {
				status, err := strconv.Atoi(fields[0])
				if err != nil {
					return 0, "", "", fmt.Errorf("invalid curl exit code %q: %w", fields[0], err)
				}
				return status, fields[1], fields[2], nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return 0, "", "", fmt.Errorf("timed out waiting for PowerShell result %s", path)
}

type httpConnectForwarder struct {
	listener  net.Listener
	upstream  string
	connectCh chan string
	closeOnce sync.Once
	wg        sync.WaitGroup
}

func newHTTPConnectForwarder(upstream string) (*httpConnectForwarder, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	forwarder := &httpConnectForwarder{listener: listener, upstream: upstream, connectCh: make(chan string, 1)}
	forwarder.wg.Add(1)
	go forwarder.acceptLoop()
	return forwarder, nil
}

func (f *httpConnectForwarder) Addr() string { return f.listener.Addr().String() }

func (f *httpConnectForwarder) acceptLoop() {
	defer f.wg.Done()
	for {
		conn, err := f.listener.Accept()
		if err != nil {
			return
		}
		f.wg.Add(1)
		go func() {
			defer f.wg.Done()
			f.handle(conn)
		}()
	}
}

func (f *httpConnectForwarder) handle(client net.Conn) {
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(30 * time.Second))
	reader := bufio.NewReader(client)
	requestHeader, err := readHTTPHeader(reader)
	if err != nil {
		return
	}
	requestLine := strings.SplitN(string(requestHeader), "\r\n", 2)[0]
	requestFields := strings.Fields(requestLine)
	if len(requestFields) < 2 || requestFields[0] != "CONNECT" {
		return
	}
	select {
	case f.connectCh <- requestFields[1]:
	default:
	}

	upstream, err := net.DialTimeout("tcp", f.upstream, 5*time.Second)
	if err != nil {
		return
	}
	defer upstream.Close()
	_ = upstream.SetDeadline(time.Now().Add(30 * time.Second))
	if _, err := upstream.Write(requestHeader); err != nil {
		return
	}
	upstreamReader := bufio.NewReader(upstream)
	responseHeader, err := readHTTPHeader(upstreamReader)
	if err != nil {
		return
	}
	if _, err := client.Write(responseHeader); err != nil {
		return
	}
	responseFields := strings.Fields(strings.SplitN(string(responseHeader), "\r\n", 2)[0])
	if len(responseFields) < 2 || !strings.HasPrefix(responseFields[1], "2") {
		return
	}

	copyDone := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(upstream, reader)
		closeWrite(upstream)
		copyDone <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(client, upstreamReader)
		closeWrite(client)
		copyDone <- struct{}{}
	}()
	<-copyDone
	// One side ending is enough to tear down the short-lived test tunnel;
	// closing both sockets also wakes the other io.Copy so WaitGroup cleanup
	// cannot be held by a keep-alive connection.
	_ = client.Close()
	_ = upstream.Close()
	select {
	case <-copyDone:
	case <-time.After(2 * time.Second):
	}
}

func closeWrite(conn net.Conn) {
	if halfCloser, ok := conn.(interface{ CloseWrite() error }); ok {
		_ = halfCloser.CloseWrite()
	}
}

func readHTTPHeader(reader *bufio.Reader) ([]byte, error) {
	var header []byte
	for {
		line, err := reader.ReadBytes('\n')
		header = append(header, line...)
		if err != nil {
			return nil, err
		}
		if bytes.HasSuffix(header, []byte("\r\n\r\n")) {
			return header, nil
		}
		if len(header) > 64*1024 {
			return nil, fmt.Errorf("HTTP header exceeds 64 KiB")
		}
	}
}

func (f *httpConnectForwarder) WaitForConnect(timeout time.Duration) string {
	select {
	case target := <-f.connectCh:
		return target
	case <-time.After(timeout):
		return ""
	}
}

func (f *httpConnectForwarder) Close() {
	f.closeOnce.Do(func() { _ = f.listener.Close() })
	f.wg.Wait()
}
