package ui

import (
	"path/filepath"
	"strings"
	"testing"

	nativeui "github.com/egoist/mygo/ui"
	"proxy-start/internal/application"
	"proxy-start/internal/domain"
)

func TestLongInspectorKeepsActionsVisibleAndBodyScrollable(t *testing.T) {
	cases := []struct {
		name    string
		related []string
		error   string
	}{
		{name: "long-path"},
		{name: "related", related: []string{`C:\Applications\Application Suite\Helpers\renderer.exe`, `C:\Applications\Application Suite\Helpers\network.exe`, `C:\Applications\Application Suite\Helpers\last-related.exe`}},
		{name: "error", error: "测试错误：" + strings.Repeat("代理连接失败，请检查网络与认证信息。", 12)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newModel(nil)
			m.snapshot = application.Snapshot{
				Settings: domain.Settings{Theme: "light", Apps: []domain.AppEntry{{ID: "app", Name: "Example Application", Identity: domain.AppIdentity{Executable: `C:\Program Files\WindowsApps\Example.Application_26.1013.4894.0_x64__application\` + strings.Repeat("Long Installation Directory\\", 6) + "app.exe"}, RelatedExecutables: tc.related}}},
				Runtimes: map[string]domain.AppRuntime{"app": {State: domain.StateFailed, Error: tc.error}},
			}
			m.selectedAppID = "app"
			// Windows non-client chrome leaves a 804x521 content area inside
			// the supported 820x560 outer window on the verification machine.
			tester := nativeui.NewTester(m.view, 804, 521)
			for _, label := range []string{"保存程序设置", "移除程序"} {
				assertVisible := func() {
					r, ok := tester.Find(label)
					if !ok || r.Y < 0 || r.Y+r.H > 521 || r.X < 0 || r.X+r.W > 804 {
						t.Fatalf("%s is outside the minimum window: %+v", label, r)
					}
				}
				assertVisible()
				tester.Scroll(700, 330, 0, 10000)
				assertVisible()
			}
			writeNativePNG(t, filepath.Join(projectRoot(t), ".impeccable", "review", "polish-inspector-"+tc.name+"-scrolled-804x521.png"), tester.Image())
		})
	}
}
