package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"proxy-start/internal/application"
	"proxy-start/internal/domain"
)

func TestInstalledInputPrefersPackageIdentity(t *testing.T) {
	tests := []struct {
		name string
		app  domain.AppIdentity
		want string
	}{
		{
			name: "aumid before executable",
			app:  domain.AppIdentity{AUMID: "OpenAI.Codex_abc!Codex", PackageName: "OpenAI.Codex", Executable: `C:\\Codex\\Codex.exe`},
			want: "package:OpenAI.Codex_abc!Codex",
		},
		{
			name: "package name before family",
			app:  domain.AppIdentity{PackageName: "OpenAI.Codex", PackageFamily: "OpenAI.Codex_abc", Executable: `C:\\Codex\\Codex.exe`},
			want: "package:OpenAI.Codex",
		},
		{
			name: "classic executable fallback",
			app:  domain.AppIdentity{Executable: `C:\\Apps\\Chrome\\chrome.exe`},
			want: `C:\\Apps\\Chrome\\chrome.exe`,
		},
		{
			name: "macOS bundle uses executable path",
			app:  domain.AppIdentity{AUMID: "bundle:com.openai.codex", Executable: "/Applications/Codex.app/Contents/MacOS/Codex"},
			want: "/Applications/Codex.app/Contents/MacOS/Codex",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := installedInput(tt.app); got != tt.want {
				t.Fatalf("installedInput() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRuntimeFailureStateIsVisible(t *testing.T) {
	if got := runtimeLabel(domain.AppRuntime{State: domain.StateFailed}); got != "失败" {
		t.Fatalf("runtimeLabel(failed) = %q, want 失败", got)
	}
}

func TestProxySelectionKeepsDefaultInheritance(t *testing.T) {
	m := newModel(nil)
	m.snapshot = application.Snapshot{Settings: domain.Settings{
		DefaultProxyID: "local",
		Profiles:       []domain.ProxyProfile{{ID: "local", Name: "本机", Protocol: domain.ProtocolHTTP, Host: "127.0.0.1", Port: 7897}},
		Apps:           []domain.AppEntry{{ID: "fixture", Name: "Fixture", Identity: domain.AppIdentity{Name: "Fixture"}}},
	}}
	ui.NewTester(m.view, 1080, 720).Image()
	if got := m.drafts["fixture"].proxyID; got != "" {
		t.Fatalf("rendering default-inherited app assigned proxy %q", got)
	}
}

func TestProbeLabelRequiresVerifiedTransport(t *testing.T) {
	tests := []struct {
		name  string
		probe domain.ProbeResult
		want  string
	}{
		{name: "identified only", probe: domain.ProbeResult{HTTP: true}, want: "已识别：HTTP，连接未验证"},
		{name: "http verified", probe: domain.ProbeResult{HTTP: true, HTTPVerified: true}, want: "连接成功：HTTP"},
		{name: "tcp verified", probe: domain.ProbeResult{TCPVerified: true}, want: "连接成功：TCP"},
		{name: "udp only remains unverified", probe: domain.ProbeResult{UDPVerified: true}, want: "UDP 已验证，连接未验证"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := probeLabel(tt.probe); got != tt.want {
				t.Fatalf("probeLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}
