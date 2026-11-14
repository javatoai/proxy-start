//go:build windows

package windows

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"proxy-start/internal/adapters/native"
	"proxy-start/internal/application"
	"proxy-start/internal/domain"
)

func ownedSleepingProcess(t *testing.T) (*ProcessController, domain.AppIdentity, domain.ProcessHandle) {
	t.Helper()
	if os.Getenv("PROXY_START_WINDOWS_INTEGRATION") != "1" {
		t.Skip("set PROXY_START_WINDOWS_INTEGRATION=1")
	}
	controller := NewProcessController()
	identity := domain.AppIdentity{Executable: filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")}
	handle, err := controller.Start(context.Background(), domain.LaunchSpec{Identity: identity, Arguments: []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", "Start-Sleep -Seconds 30"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = controller.Stop(ctx, identity, []domain.ProcessHandle{handle})
	})
	return controller, identity, handle
}

func TestTerminatingAnAlreadyExitedProcessSucceeds(t *testing.T) {
	_, _, handle := ownedSleepingProcess(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	entry := processEntry{pid: uint32(handle.PID), executable: handle.Executable, startedAt: handle.StartedAt}
	if err := terminateAndWait(ctx, entry); err != nil {
		t.Fatal(err)
	}
	if err := terminateAndWait(ctx, entry); err != nil {
		t.Fatalf("already-exited process reported as failure: %v", err)
	}
}

func TestTerminationRechecksCreationTimeBeforeKilling(t *testing.T) {
	controller, _, handle := ownedSleepingProcess(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	entry := processEntry{pid: uint32(handle.PID), executable: handle.Executable, startedAt: handle.StartedAt.Add(-time.Second)}
	if err := terminateAndWait(ctx, entry); err != nil {
		t.Fatal(err)
	}
	if !controller.Alive(ctx, handle) {
		t.Fatal("a stale process identity terminated the new process")
	}
}

// Only a bounded PowerShell fixture is launched inside the installed package.
// Codex itself is never started, stopped or restarted by this test.
func TestPackagedLaunchCarriesConfigWithoutInheritedEnvironment(t *testing.T) {
	if os.Getenv("PROXY_START_WINDOWS_INTEGRATION") != "1" {
		t.Skip("set PROXY_START_WINDOWS_INTEGRATION=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	identity, err := NewResolver().Discover(ctx)
	if errors.Is(err, application.ErrNotFound) {
		t.Skip("Codex package is not installed")
	}
	if err != nil {
		t.Fatal(err)
	}
	spec, err := native.PlanCodex(identity, 7897)
	if err != nil {
		t.Fatal(err)
	}
	// Use the production proxy plan, including both uppercase/lowercase aliases.
	// Replace only the executable and arguments with a harmless bounded fixture.
	identity.Executable = filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	identity.WorkingDirectory = t.TempDir()
	output := filepath.Join(identity.WorkingDirectory, "environment.json")
	marker := "测试 with spaces, 'quotes', $literal and & characters"
	spec.Identity = identity
	spec.Arguments = []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command",
		`Add-Type -MemberDefinition '[System.Runtime.InteropServices.DllImport("kernel32.dll")] public static extern System.IntPtr GetConsoleWindow();' -Name ConsoleInfo -Namespace ProxyStartTest;$value=@{marker=$env:PROXY_START_ROUNDTRIP_VALUE;http=$env:HTTP_PROXY;https=$env:HTTPS_PROXY;all=$env:ALL_PROXY;lowerHttp=$env:http_proxy;lowerHttps=$env:https_proxy;lowerAll=$env:all_proxy;bypass=$env:NO_PROXY;console=[string][ProxyStartTest.ConsoleInfo]::GetConsoleWindow().ToInt64()};[IO.File]::WriteAllText($env:PROXY_START_ROUNDTRIP_OUTPUT,($value|ConvertTo-Json -Compress));Start-Sleep -Seconds 12`}
	spec.Environment["PROXY_START_ROUNDTRIP_VALUE"] = marker
	spec.Environment["PROXY_START_ROUNDTRIP_OUTPUT"] = output
	t.Setenv("NO_PROXY", "must-be-cleared.invalid")
	controller := NewProcessController()
	controller.packagedLauncher = filepath.Join(t.TempDir(), "proxy-start.exe")
	build := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go.exe"), "build", "-trimpath", "-ldflags=-H=windowsgui", "-o", controller.packagedLauncher, "./cmd/proxy-start")
	build.Dir = filepath.Join("..", "..", "..")
	build.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build GUI package launcher: %v\n%s", err, output)
	}
	handle, err := controller.Start(ctx, spec)
	if err != nil {
		t.Fatalf("packaged helper launch: %v", err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = controller.Stop(cleanup, identity, []domain.ProcessHandle{handle})
	})
	deadline := time.Now().Add(4 * time.Second)
	var data []byte
	for time.Now().Before(deadline) {
		data, err = os.ReadFile(output)
		if err == nil && len(data) != 0 {
			break
		}
		time.Sleep(40 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]string
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if result["marker"] != marker || result["bypass"] != "" {
		t.Fatalf("environment did not survive package activation: %+v", result)
	}
	for _, key := range []string{"http", "https", "all", "lowerHttp", "lowerHttps", "lowerAll"} {
		if result[key] != "http://127.0.0.1:7897" {
			t.Fatalf("production proxy plan lost %s: %+v", key, result)
		}
	}
	if family := processPackageFamily(uint32(handle.PID)); family != identity.PackageFamily {
		t.Fatalf("helper package family = %q, want %q", family, identity.PackageFamily)
	}
	if result["console"] != "0" {
		t.Fatalf("background package launch allocated a console window: %s", result["console"])
	}
}
