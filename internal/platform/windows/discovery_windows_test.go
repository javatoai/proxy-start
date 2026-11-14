//go:build windows

package windows

import (
	"context"
	"errors"
	"os"
	"proxy-start/internal/adapters/native"
	"proxy-start/internal/application"
	"testing"
	"time"
)

// This opt-in test only reads the installed package and existing processes.
func TestInstalledCodexDiscoveryAndMainProcessInspection(t *testing.T) {
	if os.Getenv("PROXY_START_WINDOWS_INTEGRATION") != "1" {
		t.Skip("set PROXY_START_WINDOWS_INTEGRATION=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	identity, err := NewResolver().Discover(ctx)
	if errors.Is(err, application.ErrNotFound) {
		t.Skip("Codex is not installed")
	}
	if err != nil {
		t.Fatal(err)
	}
	if !native.IsCodex(identity) {
		t.Fatal("unverified Codex identity")
	}
	controller := NewProcessController()
	handles, err := controller.Find(ctx, identity)
	if err != nil {
		t.Fatal(err)
	}
	pids := make(map[int]bool)
	for _, handle := range handles {
		pids[handle.PID] = true
	}
	for _, handle := range handles {
		if pids[handle.ParentPID] {
			continue
		}
		info, err := controller.Inspect(ctx, handle)
		if err != nil {
			t.Fatal(err)
		}
		if !info.Readable {
			t.Fatal("main process argv is inaccessible")
		}
		observation := application.ObserveProxy(info)
		t.Logf("Codex main PID %d: evidence=%s endpoint=%s", handle.PID, observation.Kind, observation.Address)
	}
}
