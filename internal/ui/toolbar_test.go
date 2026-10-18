package ui

import (
	"testing"

	nativeui "github.com/egoist/mygo/ui"
	"proxy-start/internal/application"
	"proxy-start/internal/domain"
)

func TestApplicationFilterSupportsPointerKeyboardAndSearch(t *testing.T) {
	m := newModel(nil)
	m.snapshot = application.Snapshot{
		Settings: domain.Settings{Theme: "light", Apps: []domain.AppEntry{
			{ID: "codex", Name: "Codex"}, {ID: "chrome", Name: "Google Chrome"},
		}},
		Runtimes: map[string]domain.AppRuntime{"codex": {State: domain.StateRunning}},
	}
	for _, width := range []int{1180, 820} {
		m.appFilter, m.appSearch = "all", ""
		tester := nativeui.NewTester(m.view, width, 760)
		if err := tester.Click("运行中"); err != nil {
			t.Fatal(err)
		}
		if !tester.HasText("Codex") || tester.HasText("Google Chrome") {
			t.Fatalf("running filter did not restrict rows at width %d: %v", width, tester.Texts())
		}
		tester.Key(0, nativeui.KeyRight)
		if tester.HasText("Codex") || !tester.HasText("Google Chrome") {
			t.Fatalf("arrow key did not select stopped filter: %v", tester.Texts())
		}
		if err := tester.Click("全部"); err != nil {
			t.Fatal(err)
		}
		if err := tester.Click("搜索应用"); err != nil {
			t.Fatal(err)
		}
		tester.Type("Chrome")
		if tester.HasText("Codex") || !tester.HasText("Google Chrome") {
			t.Fatalf("search did not restrict rows: %v", tester.Texts())
		}
		if err := tester.Click("清空搜索"); err != nil {
			t.Fatal(err)
		}
		if !tester.HasText("Codex") || !tester.HasText("Google Chrome") {
			t.Fatalf("clear search did not restore rows: %v", tester.Texts())
		}
	}
}
