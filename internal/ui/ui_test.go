package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"proxy-start/internal/application"
	"proxy-start/internal/domain"
)

func TestNativeLaunchViewRendersAtMinimumWindow(t *testing.T) {
	m := newModel(nil)
	m.snapshot = application.Snapshot{
		Settings: domain.Settings{
			Theme:          "light",
			DefaultProxyID: "local",
			Profiles: []domain.ProxyProfile{{
				ID: "local", Name: "本机 Clash", Protocol: domain.ProtocolAuto,
				Host: "127.0.0.1", Port: 7897,
			}},
			Apps: []domain.AppEntry{{
				ID: "codex", Name: "Codex",
				Identity: domain.AppIdentity{Name: "Codex", Executable: `C:\Codex\Codex.exe`},
				ProxyID:  "local",
			}},
		},
		Runtimes: map[string]domain.AppRuntime{"codex": {AppID: "codex", State: domain.StateStopped}},
		Backend:  domain.BackendStatus{Installed: true, Ready: true, Detail: "测试组件已就绪"},
	}

	tester := ui.NewTester(m.view, 1080, 720)
	if !tester.HasText("应用") || !tester.HasText("添加程序") || !tester.HasText("本机 Clash") {
		t.Fatalf("launch view text missing: %v", tester.Texts())
	}
	tester.SetSize(820, 560)
	if !tester.HasText("应用") {
		t.Fatalf("minimum window lost launch heading: %v", tester.Texts())
	}
}

func TestNativeSettingsViewRendersProfilesAndBackend(t *testing.T) {
	m := newModel(nil)
	m.page = pageProxySettings
	m.snapshot = application.Snapshot{
		Settings: domain.Settings{
			Theme:          "dark",
			DefaultProxyID: "remote",
			Profiles: []domain.ProxyProfile{{
				ID: "remote", Name: "远程 SOCKS5", Protocol: domain.ProtocolSOCKS5,
				Host: "proxy.example.com", Port: 1080,
			}},
		},
		Backend: domain.BackendStatus{Installed: false, Detail: "尚未安装强制代理组件"},
	}

	tester := ui.NewTester(m.view, 1080, 720)
	if !tester.HasText("代理设置") || !tester.HasText("代理列表") || !tester.HasText("远程 SOCKS5") || tester.HasText("强制代理组件") {
		t.Fatalf("settings view text missing: %v", tester.Texts())
	}
}

func TestNativeSystemSettingsViewRendersBackend(t *testing.T) {
	m := newModel(nil)
	m.page = pageSystemSettings
	m.snapshot = application.Snapshot{Settings: domain.Settings{Theme: "dark"}, Backend: domain.BackendStatus{Installed: false, Detail: "尚未安装强制代理组件"}}
	tester := ui.NewTester(m.view, 1080, 720)
	if !tester.HasText("系统设置") || !tester.HasText("应用列表") || !tester.HasText("强制代理组件") {
		t.Fatalf("system settings view text missing: %v", tester.Texts())
	}
}
