package application

import (
	"context"
	"errors"
	"proxy-start/internal/domain"
	"testing"
	"time"
)

func runningFixture(t *testing.T) (*Desktop, *fakeProcesses, *fakeResolver) {
	p := &fakeProcesses{handles: []domain.ProcessHandle{{PID: 1, StartedAt: time.Unix(1, 0), Executable: "C:/Apps/old/Codex.exe"}, {PID: 2, ParentPID: 1}}, infos: map[int]domain.ProcessInfo{1: {Readable: true}}}
	r := &fakeResolver{identity: codexFixture()}
	d := desktopFixture(t, p, r, &memoryStore{settings: domain.DefaultSettings()}, &fakeConsole{})
	return d, p, r
}

func TestRestartValidatesProxyBeforeStopping(t *testing.T) {
	d, p, _ := runningFixture(t)
	d.deps.CheckPort = func(context.Context, int) error { return errors.New("proxy unavailable") }
	if err := d.RestartCodex(context.Background()); err == nil {
		t.Fatal("invalid proxy accepted")
	}
	if p.stops != 0 || p.starts != 0 || d.Snapshot().State != "running" {
		t.Fatal("proxy failure interrupted existing Codex")
	}
}

func TestRestartUsesObservedPathAndFreshProxyPlan(t *testing.T) {
	d, p, r := runningFixture(t)
	r.identity.Executable = "C:/Apps/new/Codex.exe"
	if err := d.SaveProxyPort(context.Background(), 8800); err != nil {
		t.Fatal(err)
	}
	if err := d.RestartCodex(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.stops != 1 || p.starts != 1 || p.stopIdentity.Executable != "C:/Apps/old/Codex.exe" {
		t.Fatalf("stop identity: %+v, stops %d", p.stopIdentity, p.stops)
	}
	if p.spec.Identity.Executable != r.identity.Executable || p.spec.Environment["HTTP_PROXY"] != "http://127.0.0.1:8800" {
		t.Fatal("restart used old configuration")
	}
	if d.Snapshot().State != "running" {
		t.Fatal("restart state not updated")
	}
}

func TestStopWorksWithoutProxyOrInstalledPackageAndIsIdempotent(t *testing.T) {
	d, p, r := runningFixture(t)
	r.err = ErrNotFound
	d.deps.CheckPort = func(context.Context, int) error { t.Fatal("stop checked proxy"); return nil }
	if err := d.StopCodex(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := d.StopCodex(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.stops != 1 || p.starts != 0 || d.Snapshot().State != "stopped" {
		t.Fatal("stop was not idempotent")
	}
}

func TestFailedStopPreventsRestart(t *testing.T) {
	d, p, _ := runningFixture(t)
	p.stopError = errors.New("access denied")
	if err := d.RestartCodex(context.Background()); err == nil {
		t.Fatal("stop failure hidden")
	}
	if p.starts != 0 || d.Snapshot().State != "running" {
		t.Fatal("started a new process after failed stop")
	}
}

func TestFailedLaunchAfterStopDoesNotClaimRunning(t *testing.T) {
	d, p, _ := runningFixture(t)
	p.startError = errors.New("launch failed")
	if err := d.RestartCodex(context.Background()); err == nil {
		t.Fatal("launch failure hidden")
	}
	if d.Snapshot().State != "stopped" {
		t.Fatalf("state = %s", d.Snapshot().State)
	}
}

func TestCancelledLifecycleDoesNotStop(t *testing.T) {
	d, p, _ := runningFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := d.StopCodex(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := d.RestartCodex(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if p.stops != 0 || p.starts != 0 {
		t.Fatal("cancelled operation changed processes")
	}
}

func TestLifecycleAcceptsExitedTargetDespiteCleanupRace(t *testing.T) {
	for _, restart := range []bool{false, true} {
		t.Run(map[bool]string{false: "stop", true: "restart"}[restart], func(t *testing.T) {
			d, p, _ := runningFixture(t)
			p.stopError, p.exitOnStopError = errors.New("child already exited"), true
			var err error
			if restart {
				err = d.RestartCodex(context.Background())
			} else {
				err = d.StopCodex(context.Background())
			}
			if err != nil {
				t.Fatalf("exited target reported as failure: %v", err)
			}
			if restart {
				if p.starts != 1 || d.Snapshot().State != "running" {
					t.Fatal("restart did not continue after confirmed stop")
				}
			} else if d.Snapshot().State != "stopped" {
				t.Fatal("stop state incorrect")
			}
		})
	}
}
