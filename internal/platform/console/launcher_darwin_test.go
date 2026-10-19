//go:build darwin

package console

import (
	"context"
	"errors"
	"os"
	"proxy-start/internal/application"
	"strings"
	"testing"
	"time"
)

func applicationSpecForTest() application.ConsoleSpec {
	return application.ConsoleSpec{ProfileName: "fixture", Environment: map[string]string{"HTTP_PROXY": "http://user:fixture-password@example.test:8080"}, UnsetEnvironment: []string{"NO_PROXY", "no_proxy"}}
}

func TestDarwinCommandScriptCarriesBypassEnvironment(t *testing.T) {
	spec := application.ConsoleSpec{
		ProfileName: "fixture",
		Environment: map[string]string{
			"HTTP_PROXY": "http://127.0.0.1:7897",
			"NO_PROXY":   "example.com,api.example",
			"no_proxy":   "example.com,api.example",
		},
	}
	script := commandScript(spec)
	for _, expected := range []string{
		"export 'NO_PROXY'='example.com,api.example'",
		"export 'no_proxy'='example.com,api.example'",
		"export 'HTTP_PROXY'='http://127.0.0.1:7897'",
	} {
		if !strings.Contains(script, expected) {
			t.Fatalf("Terminal script missing %q: %s", expected, script)
		}
	}
	if strings.Contains(script, "unset 'NO_PROXY'") || strings.Contains(script, "unset 'no_proxy'") {
		t.Fatal("non-empty bypass script unexpectedly unsets bypass variables")
	}
}

func TestDarwinLauncherCleansScriptWhenTerminalFails(t *testing.T) {
	var script string
	launcher := &Launcher{timeout: time.Second, open: func(_ context.Context, path string) error {
		script = path
		return errors.New("fake open failure")
	}}
	if err := launcher.Open(context.Background(), applicationSpecForTest()); err == nil {
		t.Fatal("Terminal failure was reported as success")
	}
	if script == "" {
		t.Fatal("test opener did not receive a script path")
	}
	if _, err := os.Stat(script); !os.IsNotExist(err) {
		t.Fatalf("failed launch left credentials script behind: %v", err)
	}
}

func TestDarwinLauncherCleansScriptOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var script string
	launcher := &Launcher{timeout: time.Second, open: func(_ context.Context, path string) error {
		script = path
		cancel()
		return nil
	}}
	if err := launcher.Open(ctx, applicationSpecForTest()); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation was not propagated: %v", err)
	}
	if _, err := os.Stat(script); !os.IsNotExist(err) {
		t.Fatalf("cancelled launch left credentials script behind: %v", err)
	}
}

func TestDarwinLauncherTimesOutAndCleansUnreadScript(t *testing.T) {
	var script string
	launcher := &Launcher{timeout: 5 * time.Millisecond, open: func(_ context.Context, path string) error {
		script = path
		return nil
	}}
	if err := launcher.Open(context.Background(), applicationSpecForTest()); err == nil {
		t.Fatal("unread Terminal script was reported as success")
	}
	if _, err := os.Stat(script); !os.IsNotExist(err) {
		t.Fatalf("timed-out launch left credentials script behind: %v", err)
	}
}
