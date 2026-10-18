package application

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"proxy-start/internal/domain"
)

type retryBackend struct {
	mu            sync.Mutex
	shutdownCalls int
	failNext      bool
	sessions      []domain.ForcedSession
}

func (b *retryBackend) Apply(_ context.Context, sessions []domain.ForcedSession) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sessions = cloneSessionsForTest(sessions)
	return nil
}

func (b *retryBackend) Shutdown(context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.shutdownCalls++
	if b.failNext {
		b.failNext = false
		return errors.New("temporary shutdown failure")
	}
	b.sessions = nil
	return nil
}

func (b *retryBackend) Status(context.Context) domain.BackendStatus { return domain.BackendStatus{} }
func (b *retryBackend) Install(context.Context) error               { return nil }

func cloneSessionsForTest(sessions []domain.ForcedSession) []domain.ForcedSession {
	result := make([]domain.ForcedSession, len(sessions))
	for i, session := range sessions {
		result[i] = session
		result[i].ExecutablePaths = append([]string(nil), session.ExecutablePaths...)
	}
	return result
}

func lifecycleDesktop(t *testing.T, settings domain.Settings, resolver AppResolver, processes ProcessController, backend ForcedBackend) *Desktop {
	t.Helper()
	desktop, err := NewDesktop(context.Background(), Dependencies{
		Store:       &memorySettings{settings: settings},
		Credentials: memoryCredentials{},
		Resolver:    resolver,
		Processes:   processes,
		Probe:       verifiedProbe{},
		Backend:     backend,
		Adapters:    []NativeAdapter{testAdapter{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = desktop.Shutdown(ctx)
	})
	return desktop
}

func TestFailedStartRetriesForcedCleanup(t *testing.T) {
	settings := domain.DefaultSettings()
	settings.Apps = []domain.AppEntry{{ID: "app", Name: "unknown", Identity: domain.AppIdentity{Name: "unknown", Executable: "test.exe"}}}
	backend := &retryBackend{failNext: true}
	desktop := lifecycleDesktop(t, settings, fakeResolver{}, &fakeProcesses{fail: true}, backend)

	if err := desktop.Start(context.Background(), "app"); err == nil {
		t.Fatal("expected process start failure")
	}
	state := desktop.Snapshot().Runtimes["app"]
	if !state.CleanupPending || !state.ProxyApplied || len(desktop.launcher.forced.sessions) != 1 {
		t.Fatalf("failed start did not preserve pending cleanup: %#v", state)
	}

	desktop.launcher.poll(context.Background())
	state = desktop.Snapshot().Runtimes["app"]
	if state.CleanupPending || state.ProxyApplied || len(desktop.launcher.forced.sessions) != 0 {
		t.Fatalf("pending cleanup was not retried: %#v", state)
	}
}

type relatedProcesses struct {
	mu          sync.Mutex
	rootAlive   bool
	helperAlive bool
}

// Keep the fixture portable: filepath.Base must see the same separator on
// every test host. Windows-specific identity normalization remains covered by
// catalog_settings_test.go.
var relatedHelperPath = filepath.Join("fixtures", "helper.exe")

func (p *relatedProcesses) Find(_ context.Context, identity domain.AppIdentity) ([]domain.ProcessHandle, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	name := strings.ToLower(filepath.Base(identity.Executable))
	if name == "test.exe" && p.rootAlive {
		return []domain.ProcessHandle{{PID: 101, Executable: "test.exe", StartedAt: time.Unix(1, 0)}}, nil
	}
	if name == "helper.exe" && p.helperAlive {
		return []domain.ProcessHandle{{PID: 102, Executable: relatedHelperPath, StartedAt: time.Unix(2, 0)}}, nil
	}
	return nil, nil
}

func (p *relatedProcesses) Start(context.Context, domain.LaunchSpec) (domain.ProcessHandle, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rootAlive = true
	return domain.ProcessHandle{PID: 101, Executable: "test.exe", StartedAt: time.Unix(1, 0)}, nil
}

func (p *relatedProcesses) Stop(_ context.Context, _ domain.AppIdentity, _ []domain.ProcessHandle) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rootAlive = false
	p.helperAlive = false
	return nil
}

func (p *relatedProcesses) Alive(_ context.Context, handle domain.ProcessHandle) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if handle.PID == 101 {
		return p.rootAlive
	}
	return handle.PID == 102 && p.helperAlive
}

func TestPollKeepsForcedLeaseWhileRelatedHelperIsAlive(t *testing.T) {
	settings := domain.DefaultSettings()
	settings.Apps = []domain.AppEntry{{ID: "app", Name: "unknown", Identity: domain.AppIdentity{Name: "unknown", Executable: "test.exe"}, RelatedExecutables: []string{relatedHelperPath}}}
	processes := &relatedProcesses{}
	backend := &fakeBackend{}
	desktop := lifecycleDesktop(t, settings, fakeResolver{}, processes, backend)
	if err := desktop.Start(context.Background(), "app"); err != nil {
		t.Fatal(err)
	}
	processes.mu.Lock()
	processes.rootAlive = false
	processes.helperAlive = true
	processes.mu.Unlock()

	desktop.launcher.poll(context.Background())
	state := desktop.Snapshot().Runtimes["app"]
	if state.State != domain.StateRunning || len(desktop.launcher.forced.sessions) != 1 {
		t.Fatalf("related helper was treated as stopped: %#v", state)
	}

	processes.mu.Lock()
	processes.helperAlive = false
	processes.mu.Unlock()
	desktop.launcher.poll(context.Background())
	state = desktop.Snapshot().Runtimes["app"]
	if state.State != domain.StateStopped || len(desktop.launcher.forced.sessions) != 0 {
		t.Fatalf("lease was not released after helper exit: %#v", state)
	}
}

type refreshedResolver struct{}

func (refreshedResolver) Resolve(context.Context, string) (domain.AppIdentity, error) {
	return domain.AppIdentity{Name: "unknown", Executable: "fresh.exe"}, nil
}
func (refreshedResolver) Installed(context.Context) ([]domain.AppIdentity, error) { return nil, nil }

type identityProcesses struct {
	mu      sync.Mutex
	running bool
	started string
	stopped string
}

func (p *identityProcesses) Find(context.Context, domain.AppIdentity) ([]domain.ProcessHandle, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.running {
		return []domain.ProcessHandle{{PID: 111, Executable: p.started, StartedAt: time.Unix(3, 0)}}, nil
	}
	return nil, nil
}
func (p *identityProcesses) Start(_ context.Context, spec domain.LaunchSpec) (domain.ProcessHandle, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.running = true
	p.started = spec.Identity.Executable
	return domain.ProcessHandle{PID: 111, Executable: spec.Identity.Executable, StartedAt: time.Unix(3, 0)}, nil
}
func (p *identityProcesses) Stop(_ context.Context, identity domain.AppIdentity, _ []domain.ProcessHandle) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopped = identity.Executable
	p.running = false
	return nil
}
func (p *identityProcesses) Alive(context.Context, domain.ProcessHandle) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.running
}

func TestLifecycleUsesRefreshedSourceIdentity(t *testing.T) {
	settings := domain.DefaultSettings()
	settings.Apps = []domain.AppEntry{{ID: "app", Name: "unknown", Source: "shortcut.lnk", Identity: domain.AppIdentity{Name: "unknown", Executable: "old.exe"}}}
	processes := &identityProcesses{}
	desktop := lifecycleDesktop(t, settings, refreshedResolver{}, processes, &fakeBackend{})
	if err := desktop.Start(context.Background(), "app"); err != nil {
		t.Fatal(err)
	}
	state := desktop.Snapshot().Runtimes["app"]
	if state.EffectiveIdentity.Executable != "fresh.exe" {
		t.Fatalf("runtime did not retain refreshed identity: %#v", state.EffectiveIdentity)
	}
	if err := desktop.Stop(context.Background(), "app"); err != nil {
		t.Fatal(err)
	}
	if processes.stopped != "fresh.exe" {
		t.Fatalf("stop used persisted identity %q instead of refreshed source", processes.stopped)
	}
}

func TestConcurrentShutdownIsIdempotent(t *testing.T) {
	settings := domain.DefaultSettings()
	settings.Apps = []domain.AppEntry{{ID: "app", Name: "unknown", Identity: domain.AppIdentity{Name: "unknown", Executable: "test.exe"}}}
	backend := &fakeBackend{}
	desktop := lifecycleDesktop(t, settings, fakeResolver{}, &fakeProcesses{}, backend)
	if err := desktop.Start(context.Background(), "app"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = desktop.Shutdown(context.Background())
		}()
	}
	wg.Wait()
	if backend.shutdowns != 1 {
		t.Fatalf("concurrent shutdown called backend %d times", backend.shutdowns)
	}
}
