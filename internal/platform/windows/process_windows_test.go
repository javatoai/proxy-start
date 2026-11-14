//go:build windows

package windows

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"proxy-start/internal/adapters/native"
	"proxy-start/internal/domain"
)

func TestPackagedLaunchEnvironmentAppliesBypassAfterUnset(t *testing.T) {
	t.Setenv("NO_PROXY", "inherited.invalid")
	environment := packagedEnvironment(packagedLaunchConfig{
		UnsetEnvironment: []string{"NO_PROXY", "no_proxy"},
		Environment:      []packagedEnvironmentValue{{Name: "NO_PROXY", Value: "configured.invalid"}},
	})
	for _, entry := range environment {
		key, value, _ := strings.Cut(entry, "=")
		if strings.EqualFold(key, "NO_PROXY") {
			if value != "configured.invalid" {
				t.Fatal("configured bypass was lost after removing inherited environment")
			}
			return
		}
	}
	t.Fatal("configured bypass is missing")
}

// Exercise the actual Windows PowerShell JSON parser, even in the default
// suite where installed-package integration tests are disabled.
func TestPackagedConfigAcceptsProductionProxyEnvironment(t *testing.T) {
	spec, err := native.PlanCodex(domain.AppIdentity{Executable: `C:\Codex\ChatGPT.exe`, PackageName: "OpenAI.Codex"}, 7897)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(packagedConfig(spec))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output, err := runPowerShellJSON(ctx, `$ErrorActionPreference='Stop'
$config = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($env:PROXY_START_TEST_CONFIG)) | ConvertFrom-Json
@($config.environment) | ConvertTo-Json -Compress`, map[string]string{"PROXY_START_TEST_CONFIG": base64.StdEncoding.EncodeToString(data)})
	if err != nil {
		t.Fatal(err)
	}
	var variables []packagedEnvironmentValue
	if err := json.Unmarshal([]byte(output), &variables); err != nil {
		t.Fatal(err)
	}
	if len(variables) != len(spec.Environment) {
		t.Fatalf("parsed %d environment entries, want %d", len(variables), len(spec.Environment))
	}
	for _, variable := range variables {
		if expected, ok := spec.Environment[variable.Name]; !ok || variable.Value != expected {
			t.Fatalf("proxy variable changed during JSON round trip: %+v", variable)
		}
	}
}

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
