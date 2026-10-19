package application

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"proxy-start/internal/domain"
)

type bypassRecordingAdapter struct {
	testAdapter
	policy domain.BypassPolicy
}

func (a *bypassRecordingAdapter) Plan(identity domain.AppIdentity, endpoint domain.ProxyEndpoint, policy domain.BypassPolicy) (domain.LaunchSpec, error) {
	a.policy = policy
	return a.testAdapter.Plan(identity, endpoint, policy)
}

func TestSavedBypassReachesNativeAdapterAndProxyConsole(t *testing.T) {
	settings := domain.DefaultSettings()
	settings.Profiles[0].BypassDomains = []string{"Example.com", "api.example.test"}
	settings.Apps = []domain.AppEntry{{ID: "app", Identity: domain.AppIdentity{Name: "known", Executable: "test.exe"}}}
	adapter := &bypassRecordingAdapter{}
	console := &recordingConsole{}
	desktop, err := NewDesktop(context.Background(), Dependencies{
		Store: &memorySettings{settings: settings}, Credentials: memoryCredentials{}, Resolver: fakeResolver{},
		Processes: &fakeProcesses{}, Probe: verifiedProbe{}, Backend: &fakeBackend{},
		Adapters: []NativeAdapter{adapter}, Console: console,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = desktop.Shutdown(context.Background()) })
	if err := desktop.OpenProxyConsole(context.Background(), "local-clash"); err != nil {
		t.Fatal(err)
	}
	if err := desktop.Start(context.Background(), "app"); err != nil {
		t.Fatal(err)
	}
	want := "example.com,api.example.test"
	if got := strings.Join(adapter.policy.Domains(), ","); got != want {
		t.Fatalf("native adapter received bypass %q, want %q", got, want)
	}
	console.mu.Lock()
	defer console.mu.Unlock()
	if console.spec.Environment["NO_PROXY"] != want || console.spec.Environment["no_proxy"] != want || len(console.spec.UnsetEnvironment) != 0 {
		t.Fatalf("console did not receive the saved bypass: %#v", console.spec)
	}
	state := desktop.Snapshot().Runtimes["app"]
	if state.Mode != domain.ModeNative || !state.ProxyApplied || state.ConnectionVerified {
		t.Fatalf("native bypass application was reported as verified traffic: %#v", state)
	}
}

func TestSnapshotAndSaveProfileDoNotAliasBypassDomains(t *testing.T) {
	settings := domain.DefaultSettings()
	settings.Profiles[0].BypassDomains = []string{"example.com"}
	store := &memorySettings{settings: settings}
	desktop, err := NewDesktop(context.Background(), Dependencies{
		Store: store, Credentials: memoryCredentials{}, Resolver: fakeResolver{},
		Processes: &fakeProcesses{}, Probe: verifiedProbe{}, Backend: &fakeBackend{},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = desktop.Shutdown(context.Background()) })

	snapshot := desktop.Snapshot()
	snapshot.Settings.Profiles[0].BypassDomains[0] = "changed.example"
	if got := desktop.Snapshot().Settings.Profiles[0].BypassDomains; !reflect.DeepEqual(got, []string{"example.com"}) {
		t.Fatalf("snapshot exposed persisted bypass slice: %#v", got)
	}

	input := []string{"API.Example.com", "api.example.com"}
	saved, err := desktop.SaveProfileResult(context.Background(), domain.ProxyProfile{
		ID: "local-clash", Name: "本机 Clash", Protocol: domain.ProtocolHTTP,
		Host: "127.0.0.1", Port: 7897, BypassDomains: input,
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	input[0] = "changed.example"
	saved.BypassDomains[0] = "changed.example"
	want := []string{"api.example.com"}
	if got := desktop.Snapshot().Settings.Profiles[0].BypassDomains; !reflect.DeepEqual(got, want) {
		t.Fatalf("saved bypass domains were aliased: got %#v, want %#v", got, want)
	}
}

func TestResolveRefreshesStaleProfileAsOneEndpointPolicyCredentialSnapshot(t *testing.T) {
	settings := domain.DefaultSettings()
	settings.Profiles[0].Host = "old.proxy"
	settings.Profiles[0].CredentialRef = "proxy-start/old"
	settings.Profiles[0].BypassDomains = []string{"old.example"}
	store := &toggleSettingsStore{settings: settings}
	credentials := &consistencyCredentials{values: map[string]string{
		"proxy-start/old": "old-secret",
		"proxy-start/new": "new-secret",
	}}
	desktop := newConsistencyDesktop(t, settings, store, credentials)
	stale := settings.Profiles[0]
	if err := desktop.repository.change(context.Background(), func(current *domain.Settings) error {
		current.Profiles[0].Host = "new.proxy"
		current.Profiles[0].CredentialRef = "proxy-start/new"
		current.Profiles[0].BypassDomains = []string{"New.Example", "api.new.example"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	resolved, err := desktop.profiles.resolve(context.Background(), stale)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Profile.Host != "new.proxy" || resolved.Endpoint.Host != "new.proxy" || resolved.Endpoint.Password != "new-secret" {
		t.Fatalf("resolve mixed stale and current profile values: profile=%#v endpoint=%#v", resolved.Profile, resolved.Endpoint)
	}
	if got, want := resolved.Bypass.Domains(), []string{"new.example", "api.new.example"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("resolve returned stale bypass policy: got %#v, want %#v", got, want)
	}
}

func TestForcedStartWithBypassRejectsBeforeProcessOrBackendMutation(t *testing.T) {
	settings := domain.DefaultSettings()
	settings.Profiles[0].BypassDomains = []string{"direct.example"}
	settings.Apps = []domain.AppEntry{{ID: "app", Name: "unknown", Identity: domain.AppIdentity{Name: "unknown", Executable: "test.exe"}}}
	processes := &fakeProcesses{}
	backend := &fakeBackend{}
	desktop := lifecycleDesktop(t, settings, fakeResolver{}, processes, backend)

	err := desktop.Start(context.Background(), "app")
	if !errors.Is(err, ErrUnsupportedBypass) {
		t.Fatalf("forced start error = %v, want ErrUnsupportedBypass", err)
	}
	if processes.starts != 0 || len(backend.applies) != 0 || len(desktop.launcher.forced.sessions) != 0 {
		t.Fatalf("unsupported bypass mutated launch resources: starts=%d applies=%d sessions=%d", processes.starts, len(backend.applies), len(desktop.launcher.forced.sessions))
	}
}

func TestRestartWithNewUnsupportedBypassKeepsExistingProcessAndLease(t *testing.T) {
	settings := domain.DefaultSettings()
	settings.Apps = []domain.AppEntry{{ID: "app", Name: "unknown", Identity: domain.AppIdentity{Name: "unknown", Executable: "test.exe"}}}
	processes := &fakeProcesses{}
	backend := &fakeBackend{}
	desktop := lifecycleDesktop(t, settings, fakeResolver{}, processes, backend)
	if err := desktop.Start(context.Background(), "app"); err != nil {
		t.Fatal(err)
	}
	before := desktop.Snapshot().Runtimes["app"]
	if before.Mode != domain.ModeForced || len(desktop.launcher.forced.sessions) != 1 {
		t.Fatalf("fixture did not establish forced running state: %#v", before)
	}
	if err := desktop.repository.change(context.Background(), func(current *domain.Settings) error {
		current.Profiles[0].BypassDomains = []string{"direct.example"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	err := desktop.Restart(context.Background(), "app")
	if !errors.Is(err, ErrUnsupportedBypass) {
		t.Fatalf("restart error = %v, want ErrUnsupportedBypass", err)
	}
	after := desktop.Snapshot().Runtimes["app"]
	if after.State != domain.StateRunning || after.Mode != domain.ModeForced || !after.ProxyApplied {
		t.Fatalf("restart rejection changed runtime state: before=%#v after=%#v", before, after)
	}
	if processes.stops != 0 || !processes.running || len(desktop.launcher.forced.sessions) != 1 || len(backend.applies) != 1 {
		t.Fatalf("restart rejection disturbed process or lease: stops=%d running=%v sessions=%d applies=%d", processes.stops, processes.running, len(desktop.launcher.forced.sessions), len(backend.applies))
	}
}
