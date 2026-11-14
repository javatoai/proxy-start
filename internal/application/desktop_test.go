package application

import (
	"context"
	"errors"
	"net"
	"proxy-start/internal/domain"
	"strconv"
	"sync"
	"testing"
	"time"
)

type memoryStore struct {
	settings domain.Settings
	err      error
}

func (s *memoryStore) Load(context.Context) (domain.Settings, error) { return s.settings, nil }
func (s *memoryStore) Save(_ context.Context, v domain.Settings) error {
	if s.err != nil {
		return s.err
	}
	s.settings = v
	return nil
}

type fakeResolver struct {
	identity domain.AppIdentity
	err      error
}

func (r *fakeResolver) Discover(context.Context) (domain.AppIdentity, error) {
	return r.identity, r.err
}

type fakeProcesses struct {
	mu                      sync.Mutex
	handles                 []domain.ProcessHandle
	infos                   map[int]domain.ProcessInfo
	finds, starts, inspects int
	findError               error
	startError              error
	stops                   int
	stopError               error
	exitOnStopError         bool
	stopIdentity            domain.AppIdentity
	spec                    domain.LaunchSpec
}

func (p *fakeProcesses) Find(context.Context, domain.AppIdentity) ([]domain.ProcessHandle, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.finds++
	return append([]domain.ProcessHandle(nil), p.handles...), p.findError
}
func (p *fakeProcesses) Start(_ context.Context, spec domain.LaunchSpec) (domain.ProcessHandle, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.starts++
	if p.startError != nil {
		return domain.ProcessHandle{}, p.startError
	}
	p.spec = spec
	handle := domain.ProcessHandle{PID: 90, StartedAt: time.Now(), Executable: spec.Identity.Executable}
	p.handles = []domain.ProcessHandle{handle}
	return handle, nil
}
func (p *fakeProcesses) Inspect(_ context.Context, h domain.ProcessHandle) (domain.ProcessInfo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.inspects++
	return p.infos[h.PID], nil
}
func (p *fakeProcesses) Stop(_ context.Context, identity domain.AppIdentity, _ []domain.ProcessHandle) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stops++
	p.stopIdentity = identity
	if p.stopError != nil {
		if p.exitOnStopError {
			p.handles = nil
		}
		return p.stopError
	}
	p.handles = nil
	return nil
}

type fakeConsole struct {
	calls int
	spec  ConsoleSpec
}

func (c *fakeConsole) Open(_ context.Context, spec ConsoleSpec) error {
	c.calls++
	c.spec = spec
	return nil
}

func codexFixture() domain.AppIdentity {
	return domain.AppIdentity{PackageName: "OpenAI.Codex", Executable: "C:/Apps/Codex.exe"}
}
func desktopFixture(t *testing.T, p *fakeProcesses, r *fakeResolver, s *memoryStore, c *fakeConsole) *Desktop {
	t.Helper()
	d, err := NewDesktop(context.Background(), Dependencies{Store: s, Resolver: r, Processes: p, Console: c, CheckPort: func(context.Context, int) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := d.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Shutdown(context.Background()) })
	return d
}

func TestAlreadyRunningIsObservedWithoutLaunchingOrChangingIt(t *testing.T) {
	p := &fakeProcesses{handles: []domain.ProcessHandle{{PID: 1}, {PID: 2, ParentPID: 1}}, infos: map[int]domain.ProcessInfo{
		1: {Readable: true, Arguments: []string{"Codex.exe", "--proxy-server=http://127.0.0.1:8000"}},
		2: {Readable: true, Arguments: []string{"Codex.exe", "--type=renderer"}},
	}}
	d := desktopFixture(t, p, &fakeResolver{identity: codexFixture()}, &memoryStore{settings: domain.DefaultSettings()}, &fakeConsole{})
	if d.Snapshot().State != "running" || d.Snapshot().Proxy.Address != "http://127.0.0.1:8000" {
		t.Fatalf("snapshot: %+v", d.Snapshot())
	}
	if err := d.SaveProxyPort(context.Background(), 9000); err != nil {
		t.Fatal(err)
	}
	if err := d.StartCodex(context.Background()); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("start: %v", err)
	}
	if p.starts != 0 || p.inspects != 1 {
		t.Fatalf("starts %d inspections %d", p.starts, p.inspects)
	}
	if d.Snapshot().Proxy.Address != "http://127.0.0.1:8000" {
		t.Fatal("editing port changed running observation")
	}
}

func TestNoCodexDoesNotBlockProxyConsole(t *testing.T) {
	console := &fakeConsole{}
	d := desktopFixture(t, &fakeProcesses{}, &fakeResolver{err: ErrNotFound}, &memoryStore{settings: domain.DefaultSettings()}, console)
	if d.Snapshot().ScanError == "" {
		t.Fatal("missing scan error")
	}
	if err := d.OpenProxyConsole(context.Background()); err != nil {
		t.Fatal(err)
	}
	if console.calls != 1 || console.spec.Environment["HTTPS_PROXY"] != "http://127.0.0.1:7897" {
		t.Fatal("console did not receive shared port")
	}
	if len(console.spec.UnsetEnvironment) != 2 {
		t.Fatal("inherited bypass not cleared")
	}
}

func TestLaunchReResolvesCodexAndRefusesNewlyAppearedProcess(t *testing.T) {
	p := &fakeProcesses{}
	r := &fakeResolver{identity: codexFixture()}
	d := desktopFixture(t, p, r, &memoryStore{settings: domain.DefaultSettings()}, &fakeConsole{})
	r.identity.Executable = "C:/Apps/updated/Codex.exe"
	d.deps.CheckPort = func(context.Context, int) error {
		p.handles = []domain.ProcessHandle{{PID: 11}}
		return nil
	}
	if err := d.StartCodex(context.Background()); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("race check: %v", err)
	}
	if p.starts != 0 {
		t.Fatal("duplicate launch")
	}
	if d.Snapshot().Identity.Executable != r.identity.Executable {
		t.Fatal("stale executable")
	}
}

func TestLaunchUsesSharedProxyAndShutdownLeavesChildAlone(t *testing.T) {
	p := &fakeProcesses{}
	d := desktopFixture(t, p, &fakeResolver{identity: codexFixture()}, &memoryStore{settings: domain.DefaultSettings()}, &fakeConsole{})
	if err := d.SaveProxyPort(context.Background(), 8800); err != nil {
		t.Fatal(err)
	}
	if err := d.StartCodex(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.starts != 1 || p.spec.Environment["HTTP_PROXY"] != "http://127.0.0.1:8800" {
		t.Fatal("wrong launch environment")
	}
	if err := d.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(p.handles) != 1 {
		t.Fatal("shutdown changed child")
	}
	if err := d.StartCodex(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("operation after shutdown: %v", err)
	}
}

func TestInspectionCacheDoesNotSurvivePIDReuse(t *testing.T) {
	p := &fakeProcesses{handles: []domain.ProcessHandle{{PID: 1, StartedAt: time.Unix(1, 0)}}, infos: map[int]domain.ProcessInfo{1: {Readable: true}}}
	d := desktopFixture(t, p, &fakeResolver{identity: codexFixture()}, &memoryStore{settings: domain.DefaultSettings()}, &fakeConsole{})
	p.handles[0].StartedAt = time.Unix(2, 0)
	p.infos[1] = domain.ProcessInfo{Readable: true, Arguments: []string{"--proxy-server=http://localhost:8000"}}
	if err := d.ScanCodex(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.inspects != 2 || d.Snapshot().Proxy.Kind != "explicit" {
		t.Fatal("stale PID evidence reused")
	}
}

func TestSaveAndObservationFailuresDoNotPublishFalseState(t *testing.T) {
	store := &memoryStore{settings: domain.DefaultSettings(), err: errors.New("disk full")}
	p := &fakeProcesses{findError: errors.New("access denied")}
	d := desktopFixture(t, p, &fakeResolver{identity: codexFixture()}, store, &fakeConsole{})
	if err := d.SaveProxyPort(context.Background(), 1234); err == nil {
		t.Fatal("save failure hidden")
	}
	if d.Snapshot().Settings.ProxyPort != 7897 || d.Snapshot().State != "unknown" {
		t.Fatal("uncommitted or unobserved state published")
	}
	if err := d.StartCodex(context.Background()); err == nil || p.starts != 0 {
		t.Fatal("launch after process-query failure")
	}
}

func TestLocalPortCheck(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, portString, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(portString)
	if err := CheckLocalPort(context.Background(), port); err != nil {
		t.Fatal(err)
	}
	for _, port := range []int{0, -1, 65536} {
		if err := CheckLocalPort(context.Background(), port); err == nil {
			t.Fatalf("invalid port %d", port)
		}
	}
	_ = listener.Close()
	if err := CheckLocalPort(context.Background(), port); err == nil {
		t.Fatal("closed port accepted")
	}
}
