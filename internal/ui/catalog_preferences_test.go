package ui

import (
	"os"
	"path/filepath"
	"testing"

	nativeui "github.com/egoist/mygo/ui"
	"proxy-start/internal/domain"
)

func TestAlreadyAddedInstalledEntryIsShownBeforeClickAndUpdatesFromSnapshot(t *testing.T) {
	m := installedFixture()
	identity := m.installed.items[1]
	tester := nativeui.NewTester(m.view, 804, 521)
	if !tester.HasText("添加 Google Chrome") {
		t.Fatal("new entry was not addable")
	}
	// A successful application use case refreshes this snapshot while keeping
	// the picker and its cached catalog open. The row must change immediately.
	m.snapshot.Settings.Apps = []domain.AppEntry{{ID: "existing", Name: "我的浏览器", Identity: identity}}
	tester.Frame()
	_, stillAddable := tester.Find("添加 Google Chrome")
	_, alreadyAdded := tester.Find("已添加 Google Chrome")
	if stillAddable || !alreadyAdded {
		t.Fatalf("added state is stale: %v", tester.Texts())
	}
	if err := tester.Click("已添加 Google Chrome"); err != nil {
		t.Fatal(err)
	}
	if len(m.busy) != 0 || m.status != "" {
		t.Fatal("already-added row attempted another add operation")
	}
}

func TestAddedPackageMatchUsesIdentityInsteadOfDisplayName(t *testing.T) {
	m := installedFixture()
	old := domain.AppIdentity{Name: "OpenAI.Codex", Executable: `C:\Program Files\WindowsApps\OpenAI.Codex_old\app\ChatGPT.exe`, PackageFamily: "OpenAI.Codex_test", ApplicationID: "App", AUMID: "OpenAI.Codex_test!App"}
	current := old
	current.Name = "Codex"
	current.Executable = `C:\Program Files\WindowsApps\OpenAI.Codex_new\app\ChatGPT.exe`
	m.snapshot.Settings.Apps = []domain.AppEntry{{ID: "codex", Name: old.Name, Identity: old}}
	if !m.installedAlreadyAdded(current) {
		t.Fatal("package version or friendly-name change lost the added state")
	}
	other := current
	other.ApplicationID = "DifferentEntry"
	other.AUMID = "OpenAI.Codex_test!DifferentEntry"
	if m.installedAlreadyAdded(other) {
		t.Fatal("different package entry was incorrectly shown as added")
	}
}

func TestCatalogPreferenceAndAddedVisualStates(t *testing.T) {
	dir := filepath.Join(projectRoot(t), ".impeccable", "review", "installed-picker")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, theme := range []string{"light", "dark"} {
		m := installedFixture()
		m.snapshot.Settings.Theme = theme
		m.snapshot.Settings.Apps = []domain.AppEntry{{ID: "chrome", Name: "Google Chrome", Identity: m.installed.items[1]}}
		writeNativePNG(t, filepath.Join(dir, "already-added-"+theme+".png"), nativeui.NewTester(m.view, 804, 521).Image())
		m.installed.open = false
		m.page = pageSystemSettings
		tester := nativeui.NewTester(m.view, 1164, 721)
		if !tester.HasText("隐藏系统软件") || !tester.HasText("隐藏微软软件及 Windows 系统组件") {
			t.Fatal("settings does not expose the catalog preference")
		}
		if domain.DefaultSettings().ShowSystemApps {
			t.Fatal("system software is visible by default")
		}
		writeNativePNG(t, filepath.Join(dir, "settings-filter-"+theme+".png"), tester.Image())
		tester.SetSize(804, 521)
		tester.Scroll(360, 350, 0, 10000)
		toggle, ok := tester.Find("隐藏系统软件")
		if !ok || toggle.Y < 0 || toggle.Y+toggle.H > 521 {
			t.Fatalf("catalog preference is not reachable at minimum size: %+v", toggle)
		}
		writeNativePNG(t, filepath.Join(dir, "settings-filter-"+theme+"-minimum.png"), tester.Image())
	}
}

func TestCatalogPreferenceRemainsAvailableWithoutProxyProfiles(t *testing.T) {
	m := installedFixture()
	m.installed.open = false
	m.page = pageSystemSettings
	m.snapshot.Settings.Profiles = nil
	tester := nativeui.NewTester(m.view, 1164, 721)
	if !tester.HasText("系统设置") || !tester.HasText("隐藏系统软件") {
		t.Fatal("catalog preference disappeared with the last proxy profile")
	}
}
