//go:build darwin

package darwin

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"proxy-start/internal/domain"
)

func TestParsePSLineUsesStableLstartFields(t *testing.T) {
	pid, started, ok := parsePSLine("42 Wed Jan  7 11:22:33 2026")
	if !ok || pid != 42 {
		t.Fatalf("parsePSLine = %d, %v, %v", pid, started, ok)
	}
	if started.Location() != time.UTC || started.Year() != 2026 || started.Hour() != 11 {
		t.Fatalf("unexpected start time: %v", started)
	}
	if _, _, ok := parsePSLine("42 invalid"); ok {
		t.Fatal("accepted malformed ps metadata")
	}
}

func TestStartHonorsCancellationBeforeProcessCreation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewProcessController().Start(ctx, domain.LaunchSpec{Identity: domain.AppIdentity{Executable: "/bin/sleep"}, Arguments: []string{"30"}})
	if err == nil {
		t.Fatal("expected cancellation before process creation")
	}
	if err != context.Canceled {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

// This test is opt-in because it exercises macOS libproc and ps on the host.
// It only observes this test process and never launches or stops a target app.
func TestProcessControllerLivePathAndPIDGuard(t *testing.T) {
	if os.Getenv("PROXY_START_DARWIN_INTEGRATION") != "1" {
		t.Skip("set PROXY_START_DARWIN_INTEGRATION=1 to run macOS process integration")
	}
	path, started, err := processMetadata(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	controller := NewProcessController()
	handle := domain.ProcessHandle{PID: os.Getpid(), Executable: path, StartedAt: started}
	if !controller.Alive(context.Background(), handle) {
		t.Fatal("current process was not recognized as alive")
	}
	wrong := handle
	wrong.Executable = filepath.Join(filepath.Dir(path), "different-program")
	if controller.Alive(context.Background(), wrong) {
		t.Fatal("PID guard accepted a different executable path")
	}
}

func TestProcessControllerStartAndStopIntegration(t *testing.T) {
	if os.Getenv("PROXY_START_DARWIN_INTEGRATION") != "1" {
		t.Skip("set PROXY_START_DARWIN_INTEGRATION=1 to run macOS process integration")
	}
	controller := NewProcessController()
	handle, err := controller.Start(context.Background(), domain.LaunchSpec{
		Identity:  domain.AppIdentity{Executable: "/bin/sleep", WorkingDirectory: "/"},
		Arguments: []string{"30"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !controller.Alive(context.Background(), handle) {
		t.Fatal("started helper was not recognized as alive")
	}
	if err := controller.Stop(context.Background(), domain.AppIdentity{Executable: "/bin/sleep"}, []domain.ProcessHandle{handle}); err != nil {
		t.Fatal(err)
	}
	if controller.Alive(context.Background(), handle) {
		t.Fatal("helper remained alive after Stop")
	}
}

func TestProcessControllerReapsShortProcessIntegration(t *testing.T) {
	if os.Getenv("PROXY_START_DARWIN_INTEGRATION") != "1" {
		t.Skip("set PROXY_START_DARWIN_INTEGRATION=1 to run macOS process integration")
	}
	controller := NewProcessController()
	handle, err := controller.Start(context.Background(), domain.LaunchSpec{
		Identity:  domain.AppIdentity{Executable: "/bin/sh", WorkingDirectory: "/"},
		Arguments: []string{"-c", "exit 0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		cmd := exec.Command("/bin/ps", "-p", fmt.Sprint(handle.PID), "-o", "stat=")
		_, psErr := cmd.Output()
		if psErr != nil {
			return
		}
		// A child can briefly be visible as a zombie between exit and the
		// scheduled Wait goroutine. Only a process that remains unreaped for
		// the full bounded interval is a failure.
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("short process PID %d was not reaped", handle.PID)
}
