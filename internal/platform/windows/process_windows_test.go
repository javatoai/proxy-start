//go:build windows

package windows

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"proxy-start/internal/domain"
)

// Run with PROXY_START_WINDOWS_INTEGRATION=1 on a disposable Windows host.
// The child is a temporary PowerShell helper and is always terminated by the
// identity-checked Stop path; no installed user application is touched.
func TestProcessControllerSafeChildLifecycle(t *testing.T) {
	if os.Getenv("PROXY_START_WINDOWS_INTEGRATION") != "1" {
		t.Skip("set PROXY_START_WINDOWS_INTEGRATION=1 to run the Windows process helper test")
	}
	executable := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	controller := NewProcessController()
	envOutput := filepath.Join(t.TempDir(), "inherited-env.txt")
	launchCtx, cancelLaunch := context.WithTimeout(context.Background(), 15*time.Second)
	handle, err := controller.Start(launchCtx, domain.LaunchSpec{
		Identity: domain.AppIdentity{Executable: executable},
		Arguments: []string{
			"-NoLogo", "-NoProfile", "-NonInteractive", "-Command",
			"[IO.File]::WriteAllText($env:PROXY_START_TEST_OUTPUT, $env:PROXY_START_TEST_ENV); Start-Sleep -Seconds 30",
		},
		Environment: map[string]string{
			"PROXY_START_TEST_ENV":    "inherited",
			"PROXY_START_TEST_OUTPUT": envOutput,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if value, readErr := os.ReadFile(envOutput); readErr == nil {
			if string(value) != "inherited" {
				t.Fatalf("child environment = %q, want inherited", string(value))
			}
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, err := os.Stat(envOutput); err != nil {
		t.Fatalf("child did not write inherited environment: %v", err)
	}
	// A launch context controls preparation only; canceling it must not tear
	// down the detached child.
	cancelLaunch()
	if !controller.Alive(context.Background(), handle) {
		t.Fatalf("new child PID %d was not reported alive", handle.PID)
	}
	stopCtx, cancelStop := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelStop()
	if err := controller.Stop(stopCtx, domain.AppIdentity{Executable: executable}, []domain.ProcessHandle{handle}); err != nil {
		t.Fatal(err)
	}
	if controller.Alive(context.Background(), handle) {
		t.Fatalf("child PID %d is still alive after Stop", handle.PID)
	}
}
