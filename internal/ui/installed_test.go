package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	nativeui "github.com/egoist/mygo/ui"
	"proxy-start/internal/domain"
)

func installedFixture() *model {
	m := newModel(nil)
	m.snapshot.Settings = domain.DefaultSettings()
	m.snapshot.Settings.Theme = "light"
	m.installed.open = true
	m.installed.items = []domain.AppIdentity{
		{Name: "Proxy Start", Executable: `C:\Apps\Proxy Start\proxy-start.exe`},
		{Name: "Google Chrome", Executable: `C:\Program Files\Google\Chrome\Application\chrome.exe`},
		{Name: "微信", Executable: `C:\Apps\Weixin\Weixin.exe`, PackageName: "Tencent.WeChat", PackageFamily: "Tencent.WeChat_test", ApplicationID: "Chat", AUMID: "Tencent.WeChat_test!Chat"},
	}
	return m
}

func TestInstalledSearchMatchesNamePathAndPackageIdentity(t *testing.T) {
	items := installedFixture().installed.items
	for _, tt := range []struct{ query, name string }{
		{"  CHROME  ", "Google Chrome"},
		{`Google\Chrome\Application`, "Google Chrome"},
		{"google chrome", "Google Chrome"},
		{"微信", "微信"},
		{"tencent.wechat", "微信"},
		{"wechat_test!chat", "微信"},
		{"google wechat", ""},
	} {
		result := filteredInstalledApps(items, tt.query)
		if tt.name == "" {
			if len(result) != 0 {
				t.Fatalf("%q unexpectedly matched %v", tt.query, result)
			}
		} else if len(result) != 1 || result[0].Name != tt.name {
			t.Fatalf("%q matched %v, want %q", tt.query, result, tt.name)
		}
	}
	if len(filteredInstalledApps(items, "  ")) != len(items) {
		t.Fatal("blank query removed applications")
	}
}

func TestInstalledPickerSearchClearAndNoResultsAtSupportedSizes(t *testing.T) {
	for _, size := range [][2]int{{1164, 721}, {804, 521}} {
		m := installedFixture()
		m.appSearch = "launch query is independent"
		tester := nativeui.NewTester(m.view, size[0], size[1])
		if err := tester.Click("搜索已安装应用"); err != nil {
			t.Fatal(err)
		}
		tester.Type("CHROME")
		if !tester.HasText("添加 Google Chrome") || tester.HasText("添加 微信") || !tester.HasText("找到 1 个") {
			t.Fatalf("search did not filter picker: %v", tester.Texts())
		}
		button, ok := tester.Find("添加 Google Chrome")
		if !ok || button.X < 0 || button.X+button.W > float32(size[0]) || button.Y < 0 || button.Y+button.H > float32(size[1]) {
			t.Fatalf("add action is clipped at %v: %+v", size, button)
		}
		if err := tester.Click("清空已安装应用搜索"); err != nil {
			t.Fatal(err)
		}
		if !tester.HasText("添加 微信") || m.appSearch != "launch query is independent" {
			t.Fatal("clear failed or changed the launch-page query")
		}
		if err := tester.Click("搜索已安装应用"); err != nil {
			t.Fatal(err)
		}
		tester.Type("missing app")
		if !tester.HasText("没有匹配的应用") || !tester.HasText("找到 0 个") {
			t.Fatalf("missing no-result feedback: %v", tester.Texts())
		}
		if err := tester.Click("清空无结果搜索"); err != nil {
			t.Fatal(err)
		}
		if !tester.HasText("添加 Google Chrome") {
			t.Fatal("no-result recovery did not restore list")
		}
	}
}

func TestInstalledVirtualListSearchRestoresFirstResultAfterScrolling(t *testing.T) {
	m := installedFixture()
	m.installed.items = nil
	for i := 0; i < 1000; i++ {
		m.installed.items = append(m.installed.items, domain.AppIdentity{Name: fmt.Sprintf("Application %04d", i), Executable: fmt.Sprintf(`C:\Applications\%04d\app.exe`, i)})
	}
	tester := nativeui.NewTester(m.view, 804, 521)
	rows := 0
	for _, text := range tester.Texts() {
		if strings.HasPrefix(text, "添加 Application") {
			rows++
		}
	}
	if rows == 0 || rows > 20 {
		t.Fatalf("virtual list built %d rows for 1000 applications", rows)
	}
	m.installed.list.ScrollToEnd()
	tester.Frame()
	if err := tester.Click("搜索已安装应用"); err != nil {
		t.Fatal(err)
	}
	tester.Type("0999")
	if !tester.HasText("添加 Application 0999") || !tester.HasText("找到 1 个") {
		t.Fatal("filter from list end lost the result")
	}
	if err := tester.Click("清空已安装应用搜索"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("添加 Application 0000") {
		t.Fatal("clearing the filter did not return to the list beginning")
	}
}

func TestClosingInstalledPickerCancelsWorkAndPreservesCache(t *testing.T) {
	m := installedFixture()
	ctx, cancel := context.WithCancel(context.Background())
	m.installed.ctx, m.installed.cancel = ctx, cancel
	m.installed.loading = true
	generation := m.installed.generation
	tester := nativeui.NewTester(m.view, 804, 521)
	if err := tester.Click("关闭应用列表"); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() != context.Canceled || m.installed.open || m.installed.loading || m.installed.generation <= generation {
		t.Fatal("closing did not cancel the current picker session")
	}
}

func TestInstalledIconBytesReachCacheThroughDispatchedCallback(t *testing.T) {
	m := installedFixture()
	identity := m.installed.items[0]
	key := installedIdentityKey(identity)
	uiQueue := make(chan func(), 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if !submitInstalledIconRequest(ctx, m.installed.iconSlots, identity,
		func(context.Context, domain.AppIdentity) ([]byte, error) { return brandIconPNG(), nil },
		func(fn func()) { uiQueue <- fn },
		func(data []byte, err error) { m.acceptInstalledIcon(ctx, m.installed.generation, key, data, err) },
	) {
		t.Fatal("icon request was not accepted")
	}
	select {
	case callback := <-uiQueue:
		if m.installed.icons[key] != nil {
			t.Fatal("worker mutated UI cache before dispatch")
		}
		callback()
	case <-time.After(time.Second):
		t.Fatal("icon callback was not dispatched")
	}
	if m.installed.icons[key] == nil {
		t.Fatal("PNG bytes were not decoded into the UI cache")
	}
	tester := nativeui.NewTester(m.view, 804, 521)
	if !tester.HasText("Proxy Start 图标") {
		t.Fatal("cached bitmap was not used by the installed row")
	}
	delete(m.installed.icons, key)
	oldGeneration := m.installed.generation
	m.closeInstalled()
	m.installed.open = true
	m.acceptInstalledIcon(ctx, oldGeneration, key, brandIconPNG(), nil)
	if m.installed.icons[key] != nil {
		t.Fatal("old picker request repopulated a reopened picker")
	}
	cancel()
	m.acceptInstalledIcon(ctx, m.installed.generation, key, brandIconPNG(), nil)
	if m.installed.icons[key] != nil {
		t.Fatal("cancelled icon request updated the UI cache")
	}
}

func TestInstalledPickerEscapeClosesAndSearchHasInitialFocus(t *testing.T) {
	m := installedFixture()
	tester := nativeui.NewTester(m.view, 804, 521)
	if !tester.Focused("搜索已安装应用") {
		t.Fatal("picker did not focus the search input")
	}
	tester.Type("Chrome")
	if !tester.HasText("添加 Google Chrome") || tester.HasText("添加 微信") {
		t.Fatal("initial typing did not search the picker")
	}
	tester.Key(0, nativeui.KeyEscape)
	if m.installed.open {
		t.Fatal("Escape did not close the picker")
	}
}

func TestInstalledPickerVisualStates(t *testing.T) {
	dir := filepath.Join(projectRoot(t), ".impeccable", "review", "installed-picker")
	// writeNativePNG expects the output directory to exist.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, theme := range []string{"light", "dark"} {
		for _, size := range [][2]int{{1164, 721}, {804, 521}} {
			m := installedFixture()
			m.snapshot.Settings.Theme = theme
			bitmap, err := nativeui.DecodeBitmap(brandIconPNG())
			if err != nil {
				t.Fatal(err)
			}
			m.installed.icons[installedIdentityKey(m.installed.items[0])] = bitmap
			m.installed.items[0].Name = "Proxy Start · " + strings.Repeat("Long Application Name ", 5)
			file := fmt.Sprintf("%s-%dx%d.png", theme, size[0], size[1])
			writeNativePNG(t, filepath.Join(dir, file), nativeui.NewTester(m.view, size[0], size[1]).Image())
		}
	}
	for _, state := range []string{"loading", "error", "empty", "no-results"} {
		m := installedFixture()
		switch state {
		case "loading":
			m.installed.loading = true
		case "error":
			m.installed.err = "当前无法读取系统应用列表，请稍后重试"
		case "empty":
			m.installed.items = nil
		case "no-results":
			m.installed.query = "未安装的软件"
		}
		writeNativePNG(t, filepath.Join(dir, state+".png"), nativeui.NewTester(m.view, 804, 521).Image())
	}
}
