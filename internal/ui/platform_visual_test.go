package ui

import (
	"path/filepath"
	"testing"

	nativeui "github.com/egoist/mygo/ui"
	"proxy-start/internal/application"
	"proxy-start/internal/domain"
)

// TestMacOSPlatformVisualReviewScreenshots keeps the macOS capability state
// visible in native light, dark, system-default, and minimum-size frames.
// These are deterministic headless frames; no macOS runtime is required.
func TestMacOSPlatformVisualReviewScreenshots(t *testing.T) {
	reviewDir := filepath.Join(projectRoot(t), ".impeccable", "review")
	cases := []struct {
		name       string
		theme      string
		width      int
		height     int
		scrollDown bool
	}{
		{name: "macos-light", theme: "light", width: 1180, height: 760},
		{name: "macos-dark", theme: "dark", width: 1180, height: 760},
		{name: "macos-default", theme: "", width: 1180, height: 760},
		{name: "macos-minimum", theme: "light", width: 820, height: 560, scrollDown: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newModel(nil)
			m.page = pageSystemSettings
			m.profileID = "local"
			m.snapshot = application.Snapshot{
				Settings: domain.Settings{
					Theme:          tc.theme,
					DefaultProxyID: "local",
					Profiles: []domain.ProxyProfile{{
						ID: "local", Name: "本机 Clash", Protocol: domain.ProtocolAuto,
						Host: "127.0.0.1", Port: 7897,
					}},
				},
				Backend:  domain.BackendStatus{Installed: true, Ready: true, Running: true},
				Platform: application.MacOSCapabilities(),
			}
			tester := nativeui.NewTester(m.view, tc.width, tc.height)
			if tc.name == "macos-default" {
				tester.SetDark(false)
			}
			if !tester.HasText("当前平台不可用") || !tester.HasText("原生适配 Codex") || !tester.HasText("不可用") {
				t.Fatalf("macOS capability state missing: %v", tester.Texts())
			}
			if tc.scrollDown {
				tester.Scroll(730, 400, 0, 700)
			}
			writeNativePNG(t, filepath.Join(reviewDir, tc.name+".png"), tester.Image())
		})
	}
}
