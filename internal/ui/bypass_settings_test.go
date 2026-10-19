package ui

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	nativeui "github.com/egoist/mygo/ui"
	"proxy-start/internal/application"
	"proxy-start/internal/domain"
)

type uiBypassSettingsStore struct {
	mu       sync.Mutex
	settings domain.Settings
}

func (s *uiBypassSettingsStore) Load(context.Context) (domain.Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settings, nil
}

func (s *uiBypassSettingsStore) Save(_ context.Context, settings domain.Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings = settings
	return nil
}

type uiBypassCredentials struct{}

func (uiBypassCredentials) Read(context.Context, string) (string, error) {
	return "", errors.New("no credential")
}
func (uiBypassCredentials) Write(context.Context, string, string) error { return nil }
func (uiBypassCredentials) Delete(context.Context, string) error        { return nil }

type uiBypassResolver struct{}

func (uiBypassResolver) Resolve(_ context.Context, executable string) (domain.AppIdentity, error) {
	return domain.AppIdentity{Name: executable, Executable: executable}, nil
}
func (uiBypassResolver) Installed(context.Context) ([]domain.AppIdentity, error) { return nil, nil }

type uiBypassProcesses struct{}

func (uiBypassProcesses) Find(context.Context, domain.AppIdentity) ([]domain.ProcessHandle, error) {
	return nil, nil
}
func (uiBypassProcesses) Start(context.Context, domain.LaunchSpec) (domain.ProcessHandle, error) {
	return domain.ProcessHandle{}, nil
}
func (uiBypassProcesses) Stop(context.Context, domain.AppIdentity, []domain.ProcessHandle) error {
	return nil
}
func (uiBypassProcesses) Alive(context.Context, domain.ProcessHandle) bool { return false }

type uiBypassProbe struct{}

func (uiBypassProbe) Test(context.Context, domain.ProxyEndpoint) (domain.ProbeResult, error) {
	return domain.ProbeResult{}, nil
}

type uiBypassBackend struct{}

func (uiBypassBackend) Apply(context.Context, []domain.ForcedSession) error { return nil }
func (uiBypassBackend) Shutdown(context.Context) error                      { return nil }
func (uiBypassBackend) Status(context.Context) domain.BackendStatus         { return domain.BackendStatus{} }
func (uiBypassBackend) Install(context.Context) error                       { return nil }

func bypassEditorFixture() (*model, *nativeui.Tester) {
	m := newModel(nil)
	m.page = pageProxySettings
	m.proxyModalOpen = true
	m.profileNew = true
	m.profileDraft = profileDraft{
		protocol: string(domain.ProtocolHTTP),
		host:     "127.0.0.1",
		port:     "7897",
	}
	m.snapshot = application.Snapshot{Settings: domain.Settings{Theme: "light"}}
	return m, nativeui.NewTester(m.view, 820, 560)
}

func TestBypassEditorRejectsURLAndKeepsModalOpen(t *testing.T) {
	m, tester := bypassEditorFixture()
	m.profileDraft.name = "本机代理"
	m.profileDraft.bypassDomains = "https://example.com"
	if err := tester.Click("保存代理配置"); err != nil {
		t.Fatal(err)
	}
	if !m.proxyModalOpen || !tester.HasText("不含 URL") {
		t.Fatalf("invalid bypass input did not stay in editor: modal=%v texts=%v", m.proxyModalOpen, tester.Texts())
	}
}

func TestBypassEditorClearsValidationErrorWhenEdited(t *testing.T) {
	m, tester := bypassEditorFixture()
	m.profileDraft.name = "本机代理"
	m.profileDraft.bypassDomains = "https://example.com"
	if err := tester.Click("保存代理配置"); err != nil {
		t.Fatal(err)
	}
	if m.profileDraft.bypassError == "" {
		t.Fatal("invalid input did not set inline error")
	}

	// Locate the label in the rendered editor, then click inside the textarea
	// immediately below it. This keeps the interaction tied to the layout
	// rather than a hard-coded viewport coordinate.
	label, ok := tester.Find("绕过代理的域名")
	if !ok {
		t.Fatal("bypass field label disappeared after validation")
	}
	tester.ClickAt(label.X+label.W/2, label.Y+label.H+14)
	if active, _ := tester.TextInputActive(); !active {
		t.Fatal("bypass textarea did not receive focus after validation")
	}
	// Cmd maps to Ctrl on Windows and Command on macOS, matching the
	// platform-native select-all shortcut used by MyGO's text editor.
	tester.Key(nativeui.Cmd, nativeui.KeyA)
	tester.Type("example.com")
	if !m.proxyModalOpen || m.profileDraft.bypassDomains != "example.com" || m.profileDraft.bypassError != "" {
		t.Fatalf("editing bypass input did not clear validation state: modal=%v draft=%q error=%q", m.proxyModalOpen, m.profileDraft.bypassDomains, m.profileDraft.bypassError)
	}
}

func TestBypassEditorSavesNormalizedDomainsAndRefills(t *testing.T) {
	settings := domain.DefaultSettings()
	desktop, err := application.NewDesktop(context.Background(), application.Dependencies{
		Store:       &uiBypassSettingsStore{settings: settings},
		Credentials: uiBypassCredentials{},
		Resolver:    uiBypassResolver{},
		Processes:   uiBypassProcesses{},
		Probe:       uiBypassProbe{},
		Backend:     uiBypassBackend{},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = desktop.Shutdown(context.Background()) })

	m := newModel(desktop)
	m.page = pageProxySettings
	m.proxyModalOpen = true
	m.profileNew = true
	m.profileEditToken = 1
	m.profileDraft = profileDraft{
		name:          "备用代理",
		protocol:      string(domain.ProtocolHTTP),
		host:          "127.0.0.1",
		port:          "7897",
		bypassDomains: " Example.COM,\napi.example.com,example.com ",
	}
	updates := make(chan func(), 1)
	m.postUpdate = func(update func()) { updates <- update }
	tester := nativeui.NewTester(m.view, 820, 560)
	if err := tester.Click("保存代理配置"); err != nil {
		t.Fatal(err)
	}
	select {
	case update := <-updates:
		// Apply the completion on the test goroutine, matching the native
		// window dispatcher instead of racing the worker against assertions.
		update()
	case <-time.After(time.Second):
		t.Fatal("profile save did not dispatch a completion update")
	}
	if m.proxyModalOpen {
		t.Fatal("successful profile save left editor open")
	}
	profiles := desktop.Snapshot().Settings.Profiles
	var saved domain.ProxyProfile
	for _, profile := range profiles {
		if profile.Name == "备用代理" {
			saved = profile
			break
		}
	}
	if got := strings.Join(saved.BypassDomains, "\n"); got != "example.com\napi.example.com" {
		t.Fatalf("saved domains were not normalized through UI save: %q", got)
	}
	m.openEditProfileModal(saved)
	if got := m.profileDraft.bypassDomains; got != "example.com\napi.example.com" {
		t.Fatalf("saved domains were not refilled into editor: %q", got)
	}
}

func TestBypassCountAndModalSaveRemainVisibleAtCompactSize(t *testing.T) {
	m := newModel(nil)
	m.page = pageProxySettings
	m.snapshot = application.Snapshot{Settings: domain.Settings{
		Theme:    "light",
		Profiles: []domain.ProxyProfile{{ID: "proxy", Name: "本机代理", Protocol: domain.ProtocolHTTP, Host: "127.0.0.1", Port: 7897, BypassDomains: []string{"example.com", "localhost"}}},
	}}
	tester := nativeui.NewTester(m.view, 820, 560)
	if !tester.HasText("绕过 2 个域名") {
		t.Fatalf("compact list did not show bypass count: %v", tester.Texts())
	}
	if err := tester.Click("编辑代理 本机代理"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("绕过代理的域名") {
		t.Fatal("compact modal omitted bypass field")
	}
	tester.Scroll(410, 430, 0, 420)
	for _, label := range []string{"绕过代理的域名", "保存代理配置"} {
		bounds, ok := tester.Find(label)
		if !ok || bounds.X < 0 || bounds.X+bounds.W > 820 || bounds.Y < 0 || bounds.Y+bounds.H > 560 {
			t.Fatalf("compact modal item %q is not reachable: ok=%v bounds=%+v", label, ok, bounds)
		}
	}
	if strings.TrimSpace(m.profileDraft.bypassDomains) == "" {
		t.Fatal("editing existing proxy did not load bypass domains")
	}
}
