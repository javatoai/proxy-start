package application

import (
	"context"
	"errors"
	"proxy-start/internal/domain"
	"strings"
	"sync"
	"testing"
)

type recordingConsole struct {
	mu    sync.Mutex
	spec  ConsoleSpec
	err   error
	calls int
}

func (c *recordingConsole) Open(_ context.Context, spec ConsoleSpec) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.spec = ConsoleSpec{ProfileName: spec.ProfileName, Environment: cloneStringMap(spec.Environment), UnsetEnvironment: append([]string(nil), spec.UnsetEnvironment...)}
	c.calls++
	return c.err
}

func cloneStringMap(input map[string]string) map[string]string {
	result := make(map[string]string, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}

func emptyBypassPolicy(t *testing.T) domain.BypassPolicy {
	t.Helper()
	policy, err := domain.NewBypassPolicy(nil)
	if err != nil {
		t.Fatalf("empty bypass policy: %v", err)
	}
	return policy
}

func TestBuildProxyEnvironmentEscapesCredentialsAndIPv6(t *testing.T) {
	env, unset, err := BuildProxyEnvironment(domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP, Host: "2001:db8::7", Port: 8080, Username: "alice@example.com", Password: "p@ss:word"}, domain.ProtocolHTTP, emptyBypassPolicy(t))
	if err != nil {
		t.Fatal(err)
	}
	want := "http://alice%40example.com:p%40ss%3Aword@[2001:db8::7]:8080"
	if env["HTTP_PROXY"] != want || env["https_proxy"] != want || env["ALL_PROXY"] != want {
		t.Fatalf("proxy URL was not encoded correctly: %#v", env)
	}
	if strings.Join(unset, ",") != "NO_PROXY,no_proxy" {
		t.Fatalf("unexpected inherited bypass variables: %#v", unset)
	}
}

func TestBuildProxyEnvironmentUsesSocks5hForRemoteDNS(t *testing.T) {
	env, _, err := BuildProxyEnvironment(domain.ProxyEndpoint{Host: "proxy.example", Port: 1080}, domain.ProtocolSOCKS5, emptyBypassPolicy(t))
	if err != nil {
		t.Fatal(err)
	}
	if env["ALL_PROXY"] != "socks5h://proxy.example:1080" || env["http_proxy"] != env["ALL_PROXY"] {
		t.Fatalf("SOCKS5 environment was not shared across proxy variables: %#v", env)
	}
}

func TestBuildProxyEnvironmentInjectsBypassWithoutUnsettingIt(t *testing.T) {
	policy, err := domain.NewBypassPolicy([]string{"Example.com", "api.example"})
	if err != nil {
		t.Fatal(err)
	}
	env, unset, err := BuildProxyEnvironment(domain.ProxyEndpoint{Host: "proxy.example", Port: 1080}, domain.ProtocolHTTP, policy)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join(policy.Domains(), ",")
	if env["NO_PROXY"] != want || env["no_proxy"] != env["NO_PROXY"] {
		t.Fatalf("unexpected bypass environment: %#v", env)
	}
	if len(unset) != 0 {
		t.Fatalf("bypass variables were unexpectedly unset: %#v", unset)
	}
}

func TestOpenProxyConsoleResolvesAutoAndUsesCredentials(t *testing.T) {
	settings := domain.DefaultSettings()
	settings.Profiles[0].Protocol = domain.ProtocolAuto
	settings.Profiles[0].CredentialRef = "proxy-start/local-clash"
	settings.Profiles[0].Username = "u"
	store := &memorySettings{settings: settings}
	console := &recordingConsole{}
	backend := &fakeBackend{}
	desktop, err := NewDesktop(context.Background(), Dependencies{
		Store: &memorySettings{settings: settings}, Credentials: memoryCredentials{}, Resolver: fakeResolver{}, Processes: &fakeProcesses{}, Probe: verifiedProbe{}, Backend: backend, Console: console,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = desktop.Shutdown(context.Background()) })
	if err := desktop.OpenProxyConsole(context.Background(), "local-clash"); err != nil {
		t.Fatal(err)
	}
	console.mu.Lock()
	spec := console.spec
	calls := console.calls
	console.mu.Unlock()
	if calls != 1 || spec.ProfileName != "本机 Clash" {
		t.Fatalf("console was not opened with profile metadata: calls=%d spec=%#v", calls, spec)
	}
	if spec.Environment["HTTP_PROXY"] != "http://u:secret@127.0.0.1:7897" {
		t.Fatalf("auto detection did not prefer the verified HTTP endpoint: %#v", spec.Environment)
	}
	if len(spec.UnsetEnvironment) != 2 {
		t.Fatalf("console inherited bypass variables: %#v", spec.UnsetEnvironment)
	}
	_ = store
}

func TestOpenProxyConsoleRejectsUnverifiedAuto(t *testing.T) {
	settings := domain.DefaultSettings()
	backend := &fakeBackend{}
	console := &recordingConsole{}
	desktop, err := NewDesktop(context.Background(), Dependencies{
		Store: &memorySettings{settings: settings}, Credentials: memoryCredentials{}, Resolver: fakeResolver{}, Processes: &fakeProcesses{}, Probe: unverifiedProbe{}, Backend: backend, Console: console,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = desktop.Shutdown(context.Background()) })
	if err := desktop.OpenProxyConsole(context.Background(), "local-clash"); err == nil {
		t.Fatal("unverified auto proxy was opened")
	}
	console.mu.Lock()
	defer console.mu.Unlock()
	if console.calls != 0 {
		t.Fatal("console launcher was called after failed protocol detection")
	}
}

type unverifiedProbe struct{}

func (unverifiedProbe) Test(context.Context, domain.ProxyEndpoint) (domain.ProbeResult, error) {
	return domain.ProbeResult{HTTP: true, SOCKS5: true}, nil
}

func TestOpenProxyConsoleWithoutPlatformLauncherIsUnavailable(t *testing.T) {
	desktop, _ := desktopFixture(t, "unknown", &fakeProcesses{})
	if err := desktop.OpenProxyConsole(context.Background(), "local-clash"); err == nil || !strings.Contains(err.Error(), "不支持") {
		t.Fatalf("unexpected missing-launcher result: %v", err)
	}
}

func TestOpenProxyConsolePropagatesLauncherError(t *testing.T) {
	console := &recordingConsole{err: errors.New("launch failed")}
	settings := domain.DefaultSettings()
	settings.Profiles[0].Protocol = domain.ProtocolHTTP
	desktop, err := NewDesktop(context.Background(), Dependencies{Store: &memorySettings{settings: settings}, Credentials: memoryCredentials{}, Resolver: fakeResolver{}, Processes: &fakeProcesses{}, Probe: verifiedProbe{}, Backend: &fakeBackend{}, Console: console})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = desktop.Shutdown(context.Background()) })
	if err := desktop.OpenProxyConsole(context.Background(), "local-clash"); err == nil || !strings.Contains(err.Error(), "launch failed") {
		t.Fatalf("launcher error was swallowed: %v", err)
	}
}

func TestSaveProfileResultReturnsCanonicalGeneratedID(t *testing.T) {
	settings := domain.DefaultSettings()
	store := &memorySettings{settings: settings}
	desktop, err := NewDesktop(context.Background(), Dependencies{Store: store, Credentials: memoryCredentials{}, Resolver: fakeResolver{}, Processes: &fakeProcesses{}, Probe: verifiedProbe{}, Backend: &fakeBackend{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = desktop.Shutdown(context.Background()) })
	persisted, err := desktop.SaveProfileResult(context.Background(), domain.ProxyProfile{Name: "备用代理", Protocol: domain.ProtocolHTTP, Host: "127.0.0.1", Port: 8888}, "")
	if err != nil {
		t.Fatal(err)
	}
	if persisted.ID == "" {
		t.Fatal("saved profile did not return generated ID")
	}
	if got := desktop.Snapshot().Settings.Profiles[len(desktop.Snapshot().Settings.Profiles)-1]; got.ID != persisted.ID {
		t.Fatalf("returned profile ID does not identify persisted row: returned=%q persisted=%q", persisted.ID, got.ID)
	}
}
