package ui

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/egoist/mygo/ui"
	"proxy-start/internal/application"
	"proxy-start/internal/domain"
)

// TestNativeVisualReviewScreenshots writes deterministic, native MyGO frames
// for the bounded visual review. All rows and proxy endpoints are synthetic
// in-memory fixtures; the images contain no product or user data.
func TestNativeVisualReviewScreenshots(t *testing.T) {
	root := projectRoot(t)
	reviewDir := filepath.Join(root, ".impeccable", "review")
	if err := os.MkdirAll(reviewDir, 0o755); err != nil {
		t.Fatal(err)
	}

	launch := newModel(nil)
	launch.snapshot = application.Snapshot{
		Settings: domain.Settings{
			Theme:          "light",
			DefaultProxyID: "local",
			Profiles: []domain.ProxyProfile{
				{ID: "local", Name: "本机 Clash", Protocol: domain.ProtocolAuto, Host: "127.0.0.1", Port: 7897},
				{ID: "remote", Name: "远程 SOCKS5", Protocol: domain.ProtocolSOCKS5, Host: "proxy.example.com", Port: 1080},
			},
			Apps: []domain.AppEntry{
				{ID: "codex", Name: "Codex", Source: "fixture", Identity: domain.AppIdentity{Name: "Codex", Executable: `C:\\Apps\\Codex\\Codex.exe`}, ProxyID: "local"},
				{ID: "chrome", Name: "Google Chrome", Source: "fixture", Identity: domain.AppIdentity{Name: "Google Chrome", Executable: `C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe`}, ProxyID: "remote"},
			},
		},
		Runtimes: map[string]domain.AppRuntime{
			"codex":  {AppID: "codex", State: domain.StateRunning, Mode: domain.ModeNative, AdapterID: "codex", ProxyApplied: true, ConnectionVerified: true},
			"chrome": {AppID: "chrome", State: domain.StateStopped, Mode: domain.ModeForced, Detail: "等待启动后应用强制代理"},
		},
		Backend: domain.BackendStatus{Installed: true, Ready: true, Running: true, Detail: "ProxiFyre 组件已就绪（评审夹具）"},
	}
	writeNativePNG(t, filepath.Join(reviewDir, "proxy-start-launch-light-1080x720.png"), ui.NewTester(launch.view, 1080, 720).Image())
	writeNativePNG(t, filepath.Join(reviewDir, "polish-launch-light-1632x971.png"), ui.NewTester(launch.view, 1632, 971).Image())
	writeNativePNG(t, filepath.Join(reviewDir, "polish-launch-light-1180x760.png"), ui.NewTester(launch.view, 1180, 760).Image())

	minimum := ui.NewTester(launch.view, 1080, 720)
	minimum.SetSize(820, 560)
	writeNativePNG(t, filepath.Join(reviewDir, "proxy-start-launch-light-min-820x560.png"), minimum.Image())

	selectedLarge := newModel(nil)
	selectedLarge.snapshot = launch.snapshot
	selectedLarge.selectedAppID = "codex"
	writeNativePNG(t, filepath.Join(reviewDir, "proxy-start-launch-selected-light-1180x760.png"), ui.NewTester(selectedLarge.view, 1180, 760).Image())

	selectedSmall := newModel(nil)
	selectedSmall.snapshot = launch.snapshot
	selectedSmall.selectedAppID = "codex"
	selectedSmallTester := ui.NewTester(selectedSmall.view, 1180, 760)
	selectedSmallTester.SetSize(820, 560)
	writeNativePNG(t, filepath.Join(reviewDir, "proxy-start-launch-selected-light-820x560.png"), selectedSmallTester.Image())

	settings := newModel(nil)
	settings.page = pageSystemSettings
	settings.snapshot = application.Snapshot{
		Settings: domain.Settings{
			Theme:          "dark",
			DefaultProxyID: "remote",
			Profiles:       []domain.ProxyProfile{{ID: "remote", Name: "远程 SOCKS5", Protocol: domain.ProtocolSOCKS5, Host: "proxy.example.com", Port: 1080, Username: "team"}},
		},
		Backend: domain.BackendStatus{Installed: false, Detail: "尚未安装强制代理组件"},
	}
	writeNativePNG(t, filepath.Join(reviewDir, "proxy-start-settings-dark-1080x720.png"), ui.NewTester(settings.view, 1080, 720).Image())

	settingsSmall := ui.NewTester(settings.view, 1080, 720)
	settingsSmall.SetSize(820, 560)
	writeNativePNG(t, filepath.Join(reviewDir, "proxy-start-settings-dark-820x560.png"), settingsSmall.Image())
	settingsSmall.Scroll(730, 400, 0, 700)
	writeNativePNG(t, filepath.Join(reviewDir, "polish-settings-dark-scrolled-820x560.png"), settingsSmall.Image())

	settingsLight := newModel(nil)
	settingsLight.page = pageSystemSettings
	settingsLight.snapshot = settings.snapshot
	settingsLight.snapshot.Settings.Theme = "light"
	settingsLight.snapshot.Settings.Profiles = []domain.ProxyProfile{
		{ID: "local", Name: "本机 Clash", Protocol: domain.ProtocolAuto, Host: "127.0.0.1", Port: 7897},
		{ID: "remote", Name: "远程团队代理 · 新加坡工作网络", Protocol: domain.ProtocolSOCKS5, Host: "proxy.example.com", Port: 1080, Username: "team"},
	}
	settingsLight.profileID = "remote"
	writeNativePNG(t, filepath.Join(reviewDir, "polish-settings-light-1632x971.png"), ui.NewTester(settingsLight.view, 1632, 971).Image())
	writeNativePNG(t, filepath.Join(reviewDir, "polish-settings-light-1180x760.png"), ui.NewTester(settingsLight.view, 1180, 760).Image())

	empty := newModel(nil)
	empty.snapshot.Settings = domain.DefaultSettings()
	empty.snapshot.Settings.Theme = "light"
	writeNativePNG(t, filepath.Join(reviewDir, "polish-launch-empty-1180x760.png"), ui.NewTester(empty.view, 1180, 760).Image())
}

func TestNativeInteractionNavigationAndDetails(t *testing.T) {
	m := newModel(nil)
	m.snapshot = application.Snapshot{
		Settings: domain.Settings{
			Theme:    "light",
			Profiles: []domain.ProxyProfile{{ID: "local", Name: "本机 Clash", Protocol: domain.ProtocolAuto, Host: "127.0.0.1", Port: 7897}},
			Apps:     []domain.AppEntry{{ID: "codex", Name: "Codex", Identity: domain.AppIdentity{Name: "Codex", Executable: `C:\\Apps\\Codex.exe`}, ProxyID: "local"}},
		},
		Backend: domain.BackendStatus{Installed: true, Ready: true},
	}
	tester := ui.NewTester(m.view, 1080, 720)
	if err := tester.Click("Codex"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("关联程序") || !tester.HasText("保存程序设置") {
		t.Fatalf("details editor did not open selected=%q: %v", m.selectedAppID, tester.Texts())
	}
	if err := tester.Click("代理设置"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("代理列表") || !tester.HasText("打开控制台") {
		t.Fatalf("settings navigation failed: %v", tester.Texts())
	}
	if err := tester.Click("编辑代理 本机 Clash"); err != nil {
		t.Fatal(err)
	}
	if m.profileID != "local" {
		t.Fatalf("profile selection did not update model: %q", m.profileID)
	}
}

func TestSystemThemeRestoresHostAppearance(t *testing.T) {
	m := newModel(nil)
	m.snapshot.Settings.Theme = "dark"
	tester := ui.NewTester(m.view, 820, 560)
	dark := tester.Image().At(10, 10)

	// The headless host is explicitly light for this check. Switching the
	// model back to system must let MyGO's frame theme take effect again.
	tester.SetDark(false)
	m.snapshot.Settings.Theme = "system"
	tester.Frame()
	light := tester.Image().At(10, 10)
	dr, _, _, _ := dark.RGBA()
	lr, _, _, _ := light.RGBA()
	if light == dark || lr <= dr {
		t.Fatalf("system theme did not restore host appearance: dark=%v light=%v", dark, light)
	}
}

func projectRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("project root not found")
		}
		dir = parent
	}
}

func writeNativePNG(t *testing.T, path string, frame *image.RGBA) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, frame); err != nil {
		t.Fatal(err)
	}
}
