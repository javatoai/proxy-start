package application

import (
	"context"
	"errors"
	"proxy-start/internal/domain"
	"sync"
	"testing"
	"time"
)

type memorySettings struct {
	mu       sync.Mutex
	settings domain.Settings
}

func (s *memorySettings) Load(context.Context) (domain.Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneSettings(s.settings), nil
}
func (s *memorySettings) Save(_ context.Context, v domain.Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings = cloneSettings(v)
	return nil
}

type memoryCredentials struct{}

func (memoryCredentials) Read(context.Context, string) (string, error) { return "secret", nil }
func (memoryCredentials) Write(context.Context, string, string) error  { return nil }
func (memoryCredentials) Delete(context.Context, string) error         { return nil }

type fakeResolver struct{}

func (fakeResolver) Resolve(_ context.Context, path string) (domain.AppIdentity, error) {
	return domain.AppIdentity{Name: "unknown", Executable: path}, nil
}
func (fakeResolver) Installed(context.Context) ([]domain.AppIdentity, error) { return nil, nil }

type verifiedProbe struct{}

func (verifiedProbe) Test(context.Context, domain.ProxyEndpoint) (domain.ProbeResult, error) {
	return domain.ProbeResult{HTTP: true, SOCKS5: true, HTTPVerified: true, SOCKS5Verified: true, TCPVerified: true}, nil
}

type fakeProcesses struct {
	mu            sync.Mutex
	running       bool
	starts, stops int
	fail          bool
}

func (p *fakeProcesses) Find(context.Context, domain.AppIdentity) ([]domain.ProcessHandle, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.running {
		return []domain.ProcessHandle{{PID: 123, Executable: "test.exe", StartedAt: time.Now()}}, nil
	}
	return nil, nil
}
func (p *fakeProcesses) Start(context.Context, domain.LaunchSpec) (domain.ProcessHandle, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.fail {
		return domain.ProcessHandle{}, errors.New("start failed")
	}
	p.starts++
	p.running = true
	return domain.ProcessHandle{PID: 123, Executable: "test.exe", StartedAt: time.Now()}, nil
}
func (p *fakeProcesses) Stop(context.Context, domain.AppIdentity, []domain.ProcessHandle) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stops++
	p.running = false
	return nil
}
func (p *fakeProcesses) Alive(context.Context, domain.ProcessHandle) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.running
}

func desktopFixture(t *testing.T, name string, processes *fakeProcesses) (*Desktop, *fakeBackend) {
	t.Helper()
	settings := domain.DefaultSettings()
	settings.Apps = []domain.AppEntry{{ID: "app", Name: name, Identity: domain.AppIdentity{Name: name, Executable: "test.exe"}}}
	backend := &fakeBackend{}
	desktop, err := NewDesktop(context.Background(), Dependencies{Store: &memorySettings{settings: settings}, Credentials: memoryCredentials{}, Resolver: fakeResolver{}, Processes: processes, Probe: verifiedProbe{}, Backend: backend, Adapters: []NativeAdapter{testAdapter{}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = desktop.Shutdown(ctx)
	})
	return desktop, backend
}

func TestFullQuitNeverStopsLaunchedApplications(t *testing.T) {
	processes := &fakeProcesses{}
	desktop, backend := desktopFixture(t, "unknown", processes)
	if err := desktop.Start(context.Background(), "app"); err != nil {
		t.Fatal(err)
	}
	if desktop.Snapshot().Runtimes["app"].Mode != domain.ModeForced {
		t.Fatal("unknown app did not use forced mode")
	}
	if err := desktop.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if processes.stops != 0 || !processes.running {
		t.Fatal("full quit killed a target application")
	}
	if backend.shutdowns == 0 {
		t.Fatal("full quit failed to release interception")
	}
	if err := desktop.Start(context.Background(), "app"); !errors.Is(err, ErrClosed) {
		t.Fatalf("launch accepted after quit: %v", err)
	}
}

func TestExistingProcessIsDiscoveredAtStartupWithoutProxy(t *testing.T) {
	processes := &fakeProcesses{running: true}
	desktop, backend := desktopFixture(t, "unknown", processes)
	state := desktop.Snapshot().Runtimes["app"]
	if state.State != domain.StateRunning || state.Detail != ErrAlreadyRunning.Error() {
		t.Fatalf("startup discovery did not report the existing process: %#v", state)
	}
	if state.Mode != "" || state.ProxyApplied || state.ConnectionVerified {
		t.Fatalf("external process was reported as proxied: %#v", state)
	}
	if len(state.Processes) != 1 || processes.starts != 0 || len(backend.applies) != 0 {
		t.Fatalf("startup discovery changed process or proxy state: handles=%d starts=%d applies=%d", len(state.Processes), processes.starts, len(backend.applies))
	}
}

func TestExternalProcessAppearingLaterIsDiscoveredWithoutProxy(t *testing.T) {
	processes := &fakeProcesses{}
	desktop, backend := desktopFixture(t, "unknown", processes)
	processes.mu.Lock()
	processes.running = true
	processes.mu.Unlock()
	desktop.launcher.poll(context.Background())
	state := desktop.Snapshot().Runtimes["app"]
	if state.State != domain.StateRunning || len(state.Processes) != 1 {
		t.Fatalf("poll discovery did not count the external process: %#v", state)
	}
	if state.Mode != "" || state.ProxyApplied || len(backend.applies) != 0 {
		t.Fatalf("poll discovery applied proxy rules: %#v", state)
	}
}

func TestFailedProcessStartReleasesInterception(t *testing.T) {
	desktop, backend := desktopFixture(t, "unknown", &fakeProcesses{fail: true})
	if err := desktop.Start(context.Background(), "app"); err == nil {
		t.Fatal("start failure disappeared")
	}
	if len(desktop.launcher.forced.sessions) != 0 || backend.shutdowns == 0 {
		t.Fatal("failed launch leaked an engine session")
	}
	if desktop.Snapshot().Runtimes["app"].State != domain.StateFailed {
		t.Fatal("failed launch reported as running")
	}
}

func TestDuplicateStartPreservesManagedModeAndDoesNotLaunchTwice(t *testing.T) {
	processes := &fakeProcesses{}
	desktop, _ := desktopFixture(t, "known", processes)
	if err := desktop.Start(context.Background(), "app"); err != nil {
		t.Fatal(err)
	}
	if err := desktop.Start(context.Background(), "app"); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("duplicate launch: %v", err)
	}
	state := desktop.Snapshot().Runtimes["app"]
	if processes.starts != 1 || state.Mode != domain.ModeNative || !state.ProxyApplied || state.ConnectionVerified {
		t.Fatalf("duplicate start damaged managed state: %#v", state)
	}
}

func TestSnapshotCannotMutatePersistedConfiguration(t *testing.T) {
	desktop, _ := desktopFixture(t, "known", &fakeProcesses{})
	snapshot := desktop.Snapshot()
	snapshot.Settings.Profiles[0].Host = "changed"
	snapshot.Settings.Apps[0].Name = "changed"
	actual := desktop.Snapshot()
	if actual.Settings.Profiles[0].Host == "changed" || actual.Settings.Apps[0].Name == "changed" {
		t.Fatal("snapshot exposes mutable settings")
	}
}
