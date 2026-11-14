//go:build windows

package windows

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// Assign only a newly created test subprocess to a non-breakaway job. Never
// change the runner's or the current Codex process's job membership.
func TestProcessControllerStartsWithinRestrictedJob(t *testing.T) {
	if os.Getenv("PROXY_START_WINDOWS_INTEGRATION") != "1" {
		t.Skip("set PROXY_START_WINDOWS_INTEGRATION=1")
	}
	if ready := os.Getenv("PROXY_START_JOB_TEST_READY"); ready != "" {
		deadline := time.Now().Add(5 * time.Second)
		for {
			if _, err := os.Stat(ready); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("test subprocess was not assigned to a job")
			}
			time.Sleep(10 * time.Millisecond)
		}
		controller, _, handle := ownedSleepingProcess(t)
		if !controller.Alive(context.Background(), handle) {
			t.Fatal("target did not survive startup within restricted job")
		}
		return
	}
	job, _, err := kernel32.NewProc("CreateJobObjectW").Call(0, 0)
	if job == 0 {
		t.Fatal(err)
	}
	defer closeHandle.Call(job)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(t.TempDir(), "assigned")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestProcessControllerStartsWithinRestrictedJob$", "-test.v")
	cmd.Env = append(os.Environ(), "PROXY_START_JOB_TEST_READY="+ready)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	handle, _, err := openProcess.Call(0x0100|processTerminate, 0, uintptr(cmd.Process.Pid))
	if handle == 0 {
		t.Fatal(err)
	}
	assigned, _, assignErr := kernel32.NewProc("AssignProcessToJobObject").Call(job, handle)
	closeHandle.Call(handle)
	if assigned == 0 {
		t.Fatal(assignErr)
	}
	if err := os.WriteFile(ready, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("start from restricted job failed: %v\n%s", err, output.String())
	}
}
