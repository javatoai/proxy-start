package ui

import (
	"bytes"
	"context"
	"errors"
	nativeui "github.com/egoist/mygo/ui"
	"image/png"
	"os"
	"path/filepath"
	"proxy-start/internal/application"
	"proxy-start/internal/domain"
	"strings"
	"sync"
	"testing"
	"time"
)

func fixtureModel() *model {
	m := newModel(nil)
	m.snapshot = application.Snapshot{Settings: domain.DefaultSettings(), State: "running",
		Identity: domain.AppIdentity{PackageName: "OpenAI.Codex", Executable: "C:/Program Files/WindowsApps/OpenAI.Codex_26.1106.6628.0_x64__test/app/Codex.exe"},
		Proxy:    domain.ProxyObservation{Kind: "explicit", Address: "http://127.0.0.1:7897"}, Platform: application.WindowsCapabilities()}
	return m
}

func TestCompactSurfaceContainsOnlyCodexAndConsoleActions(t *testing.T) {
	m := fixtureModel()
	tester := nativeui.NewTester(m.view, 460, 420)
	for _, text := range []string{"已启动", "重启", "停止"} {
		if !tester.HasText(text) {
			t.Fatalf("missing %q", text)
		}
	}
	for _, text := range []string{"打开代理控制台", "Proxy Start", "代理端口", "127.0.0.1 :", "显式代理：http://127.0.0.1:7897", "默认 7897，使用 Clash 的 HTTP / mixed 端口。", "操作已完成", "添加程序", "代理设置", "系统设置", "Claude", "结束", "重新扫描", "新增代理"} {
		if tester.HasText(text) {
			t.Fatalf("obsolete control %q", text)
		}
	}
	if tester.HasText("启动 Codex") {
		t.Fatal("running Codex has a launch action")
	}
}

func TestControlsReflectRunningState(t *testing.T) {
	m := fixtureModel()
	tester := nativeui.NewTester(m.view, 460, 420)
	if !m.canStop() || !tester.HasText("重启") || !tester.HasText("停止") {
		t.Fatal("running controls missing")
	}
	m.snapshot.State = "stopped"
	tester.Frame()
	if m.canStop() || !tester.HasText("启动") {
		t.Fatal("stopped controls incorrect")
	}
	if _, ok := tester.Find("刷新 Codex 状态"); !ok {
		t.Fatal("circle is not a refresh control")
	}
	m.snapshot.ScanError, m.snapshot.ScanNotFound, m.snapshot.Identity = "access denied", false, domain.AppIdentity{}
	m.snapshot.State = "unknown"
	if m.codexStatus() != "状态待确认" {
		t.Fatal("scan failure shown as not installed")
	}
	m.snapshot.ScanNotFound = true
	if m.codexStatus() != "未找到" {
		t.Fatal("missing application not identified")
	}
}

func TestPortValidationIsLocalAndDoesNotLaunch(t *testing.T) {
	m := fixtureModel()
	for _, port := range []string{"", "a", "0", "65536", "-1"} {
		m.port = port
		m.withPort(nil)
		if !m.failed || m.busy {
			t.Fatalf("bad port %q", port)
		}
	}
}

func TestCompactVisualFrames(t *testing.T) {
	dir := filepath.Join("..", "..", ".impeccable", "review", "compact")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, size := range [][2]int{{460, 420}, {380, 360}} {
		m := fixtureModel()
		tester := nativeui.NewTester(m.view, size[0], size[1])
		name := "light"
		if size[0] == 380 {
			name += "-minimum"
		}
		file, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(file, tester.Image()); err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	m := fixtureModel()
	m.snapshot.Scanning, m.snapshot.Identity = true, domain.AppIdentity{}
	if !nativeui.NewTester(m.view, 380, 360).HasText("扫描中") {
		t.Fatal("missing scanning state")
	}
}

func TestSettingsGearOpensPortDialogAndCancelDiscardsDraft(t *testing.T) {
	m := fixtureModel()
	tester := nativeui.NewTester(m.view, 460, 420)
	if err := tester.Click("打开设置"); err != nil {
		t.Fatal(err)
	}
	if !m.settingsOpen || !tester.HasText("代理端口") || !tester.HasText("保存") || !tester.HasText("打开代理控制台") {
		t.Fatal("gear did not open settings")
	}
	m.portDraft = "65536"
	if err := tester.Click("保存设置"); err != nil {
		t.Fatal(err)
	}
	if m.settingsError == "" || !m.settingsOpen {
		t.Fatal("invalid port dismissed the dialog")
	}
	m.portDraft = "9000"
	if err := tester.Click("取消"); err != nil {
		t.Fatal(err)
	}
	if m.settingsOpen || m.port != "7897" {
		t.Fatal("cancel changed the configured port")
	}
	if err := tester.Click("打开设置"); err != nil {
		t.Fatal(err)
	}
	if m.portDraft != "7897" || m.settingsError != "" {
		t.Fatal("reopening reused cancelled draft")
	}
	dir := filepath.Join("..", "..", ".impeccable", "review", "compact")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(dir, "settings-modal.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, tester.Image()); err != nil {
		t.Fatal(err)
	}
}

type uiSettingsStore struct {
	settings domain.Settings
	err      error
}

func (s *uiSettingsStore) Load(context.Context) (domain.Settings, error) { return s.settings, nil }
func (s *uiSettingsStore) Save(_ context.Context, settings domain.Settings) error {
	if s.err != nil {
		return s.err
	}
	s.settings = settings
	return nil
}

type inertPlatform struct{}

func (inertPlatform) Stop(context.Context, domain.AppIdentity, []domain.ProcessHandle) error {
	return nil
}

type settingsConsole struct {
	calls int
	proxy string
	err   error
}

func (c *settingsConsole) Open(_ context.Context, spec application.ConsoleSpec) error {
	c.calls++
	c.proxy = spec.Environment["HTTP_PROXY"]
	return c.err
}

func TestConsoleOpensFromSettingsUsingTheEnteredPort(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "opened", true: "console failure"}[fail], func(t *testing.T) {
			store := &uiSettingsStore{settings: domain.DefaultSettings()}
			console := &settingsConsole{}
			if fail {
				console.err = errors.New("cannot open console")
			}
			d, err := application.NewDesktop(context.Background(), application.Dependencies{
				Store: store, Resolver: inertPlatform{}, Processes: inertPlatform{}, Console: console, CheckPort: func(context.Context, int) error { return nil },
			})
			if err != nil {
				t.Fatal(err)
			}
			defer d.Shutdown(context.Background())
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := d.WaitReady(ctx); err != nil {
				t.Fatal(err)
			}
			m := newModel(d)
			updates := make(chan func(), 1)
			m.postUpdate = func(update func()) { updates <- update }
			tester := nativeui.NewTester(m.view, 460, 420)
			if tester.HasText("打开代理控制台") {
				t.Fatal("console action is still on the home surface")
			}
			if err := tester.Click("打开设置"); err != nil {
				t.Fatal(err)
			}
			m.portDraft = "9000"
			if err := tester.Click("打开代理控制台"); err != nil {
				t.Fatal(err)
			}
			select {
			case update := <-updates:
				update()
			case <-ctx.Done():
				t.Fatal("console action did not finish")
			}
			if console.calls != 1 || console.proxy != "http://127.0.0.1:9000" || m.port != "9000" {
				t.Fatal("console ignored the settings port")
			}
			if fail {
				if !m.settingsOpen || m.settingsError == "" {
					t.Fatal("console error was hidden")
				}
			} else if m.settingsOpen {
				t.Fatal("successful action left settings open")
			}
		})
	}
}

func (inertPlatform) Discover(context.Context) (domain.AppIdentity, error) {
	return domain.AppIdentity{}, application.ErrNotFound
}
func (inertPlatform) Find(context.Context, domain.AppIdentity) ([]domain.ProcessHandle, error) {
	return nil, nil
}
func (inertPlatform) Start(context.Context, domain.LaunchSpec) (domain.ProcessHandle, error) {
	return domain.ProcessHandle{}, errors.New("not used")
}
func (inertPlatform) Inspect(context.Context, domain.ProcessHandle) (domain.ProcessInfo, error) {
	return domain.ProcessInfo{}, nil
}
func (inertPlatform) Open(context.Context, application.ConsoleSpec) error { return nil }

func TestSettingsSavePublishesOnlyPersistedPort(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "save failure"}[fail], func(t *testing.T) {
			store := &uiSettingsStore{settings: domain.DefaultSettings()}
			if fail {
				store.err = errors.New("disk full")
			}
			d, err := application.NewDesktop(context.Background(), application.Dependencies{Store: store, Resolver: inertPlatform{}, Processes: inertPlatform{}, Console: inertPlatform{}})
			if err != nil {
				t.Fatal(err)
			}
			defer d.Shutdown(context.Background())
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := d.WaitReady(ctx); err != nil {
				t.Fatal(err)
			}
			m := newModel(d)
			updates := make(chan func(), 1)
			m.postUpdate = func(update func()) { updates <- update }
			m.openSettings()
			m.portDraft = "9000"
			m.saveSettings()
			select {
			case update := <-updates:
				update()
			case <-ctx.Done():
				t.Fatal("save did not finish")
			}
			if fail {
				if !m.settingsOpen || m.settingsError == "" || m.port != "7897" || store.settings.ProxyPort != 7897 {
					t.Fatal("failed save changed configuration")
				}
			} else if m.settingsOpen || m.port != "9000" || store.settings.ProxyPort != 9000 {
				t.Fatal("saved port did not become active")
			}
		})
	}
}

func TestAlwaysLightEvenWhenSystemIsDark(t *testing.T) {
	m := fixtureModel()
	tester := nativeui.NewTester(func(c *nativeui.Context) { c.SetTheme(nativeui.DarkTheme()); m.view(c) }, 720, 600)
	r, g, b, _ := tester.Image().At(0, 0).RGBA()
	if r != 65535 || g != 65535 || b != 65535 {
		t.Fatal("system dark theme changed the page")
	}
	for _, text := range []string{"外观主题", "浅色", "深色", "跟随系统"} {
		if tester.HasText(text) {
			t.Fatal("theme selector still visible")
		}
	}
}

func TestStoppedCircleVisualFrame(t *testing.T) {
	m := fixtureModel()
	m.snapshot.State = "stopped"
	dir := filepath.Join("..", "..", ".impeccable", "review", "compact")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(dir, "light-stopped.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	tester := nativeui.NewTester(m.view, 460, 420)
	if err := png.Encode(file, tester.Image()); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("启动") || m.canStop() {
		t.Fatal("stopped controls incorrect")
	}
}

type uiLifecyclePlatform struct {
	mu                       sync.Mutex
	handles                  []domain.ProcessHandle
	starts, stops, discovers int
}

func (p *uiLifecyclePlatform) Discover(context.Context) (domain.AppIdentity, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.discovers++
	return domain.AppIdentity{PackageName: "OpenAI.Codex", Executable: "C:/Apps/Codex.exe"}, nil
}
func (p *uiLifecyclePlatform) Find(context.Context, domain.AppIdentity) ([]domain.ProcessHandle, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]domain.ProcessHandle(nil), p.handles...), nil
}
func (p *uiLifecyclePlatform) Inspect(context.Context, domain.ProcessHandle) (domain.ProcessInfo, error) {
	return domain.ProcessInfo{Readable: true}, nil
}
func (p *uiLifecyclePlatform) Start(_ context.Context, spec domain.LaunchSpec) (domain.ProcessHandle, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.starts++
	handle := domain.ProcessHandle{PID: 100 + p.starts, StartedAt: time.Now(), Executable: spec.Identity.Executable}
	p.handles = []domain.ProcessHandle{handle}
	return handle, nil
}
func (p *uiLifecyclePlatform) Stop(context.Context, domain.AppIdentity, []domain.ProcessHandle) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stops++
	p.handles = nil
	return nil
}

func TestButtonsAndRefreshRouteToSeparateActions(t *testing.T) {
	platform := &uiLifecyclePlatform{}
	d, err := application.NewDesktop(context.Background(), application.Dependencies{
		Store: &uiSettingsStore{settings: domain.DefaultSettings()}, Resolver: platform, Processes: platform, Console: inertPlatform{}, CheckPort: func(context.Context, int) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Shutdown(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := d.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	m := newModel(d)
	updates := make(chan func(), 1)
	m.postUpdate = func(update func()) { updates <- update }
	tester := nativeui.NewTester(m.view, 460, 420)
	finish := func() {
		select {
		case update := <-updates:
			update()
			tester.Frame()
		case <-ctx.Done():
			t.Fatal("action timed out")
		}
		if m.failed {
			t.Fatal(m.message)
		}
	}
	if err := tester.Click("停止 Codex"); err != nil {
		t.Fatal(err)
	}
	if m.busy || platform.stops != 0 {
		t.Fatal("disabled stop performed an action")
	}
	if err := tester.Click("启动 Codex"); err != nil {
		t.Fatal(err)
	}
	finish()
	if platform.starts != 1 || m.snapshot.State != "running" {
		t.Fatal("start action failed")
	}
	if err := tester.Click("刷新 Codex 状态"); err != nil {
		t.Fatal(err)
	}
	if m.pendingAction != "refresh" {
		t.Fatal("circle did not start refresh")
	}
	finish()
	if platform.starts != 1 || platform.stops != 0 {
		t.Fatal("refresh changed process lifecycle")
	}
	if err := tester.Click("重启 Codex"); err != nil {
		t.Fatal(err)
	}
	finish()
	if platform.starts != 2 || platform.stops != 1 {
		t.Fatal("restart action failed")
	}
	if err := tester.Click("停止 Codex"); err != nil {
		t.Fatal(err)
	}
	finish()
	if platform.stops != 2 || m.snapshot.State != "stopped" || m.canStop() {
		t.Fatal("stop action failed")
	}
}

func TestRefreshAnimationMovesSettlesAndRespectsReducedMotion(t *testing.T) {
	m := fixtureModel()
	m.busy, m.pendingAction = true, "refresh"
	tester := nativeui.NewTester(m.view, 460, 420)
	tester.Advance(200 * time.Millisecond)
	before := append([]byte(nil), tester.Image().Pix...)
	tester.Advance(220 * time.Millisecond)
	if bytes.Equal(before, tester.Image().Pix) {
		t.Fatal("refresh ring did not animate")
	}
	file, err := os.Create(filepath.Join("..", "..", ".impeccable", "review", "compact", "refreshing.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, tester.Image()); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	m.busy, m.pendingAction = false, ""
	tester.Frame()
	tester.Advance(300 * time.Millisecond)
	if tester.FramePending() {
		t.Fatal("animation kept running after refresh")
	}
	m.busy, m.pendingAction = true, "refresh"
	tester.SetPreferences(nativeui.Preferences{ReduceMotion: true, TextScale: 1})
	before = append([]byte(nil), tester.Image().Pix...)
	tester.Advance(300 * time.Millisecond)
	if !bytes.Equal(before, tester.Image().Pix) || tester.FramePending() {
		t.Fatal("reduced-motion preference was ignored")
	}
}

func TestSettingsErrorActionsFitSmallestNativeContentSize(t *testing.T) {
	m := fixtureModel()
	m.openSettings()
	m.settingsError = "请输入 1 到 65535 之间的端口号"
	tester := nativeui.NewTester(m.view, 364, 321)
	for _, label := range []string{"取消", "保存设置", "打开代理控制台"} {
		bounds, ok := tester.Find(label)
		if !ok || bounds.Y < 0 || bounds.Y+bounds.H > 321 {
			t.Fatalf("%s is outside the small window: %+v", label, bounds)
		}
	}
}

func TestLongErrorKeepsLifecycleControlsVisible(t *testing.T) {
	for _, size := range []struct {
		name          string
		width, height int
	}{{"default", 460, 420}, {"minimum", 364, 321}} {
		t.Run(size.name, func(t *testing.T) {
			m := fixtureModel()
			m.failed, m.message = true, strings.Repeat("停止 Codex 失败：PID 12345: Access is denied; ", 100)
			tester := nativeui.NewTester(m.view, size.width, size.height)
			save := func(name string) {
				t.Helper()
				var buffer bytes.Buffer
				if err := png.Encode(&buffer, tester.Image()); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join("..", "..", ".impeccable", "review", "compact", name+"-"+size.name+".png"), buffer.Bytes(), 0600); err != nil {
					t.Fatal(err)
				}
			}
			for _, label := range []string{"重启 Codex", "停止 Codex", "查看错误详情"} {
				bounds, ok := tester.Find(label)
				if !ok || bounds.Y < 0 || bounds.Y+bounds.H > float32(size.height) {
					t.Fatalf("%s moved outside window: %+v", label, bounds)
				}
			}
			save("long-error")
			if err := tester.Click("查看错误详情"); err != nil {
				t.Fatal(err)
			}
			if !tester.HasText("错误详情") {
				t.Fatal("full error is not accessible")
			}
			bounds, ok := tester.Find("关闭错误详情")
			if !ok || bounds.Y < 0 || bounds.Y+bounds.H > float32(size.height) {
				t.Fatal("detail close button is unreachable")
			}
			save("error-details")
			if err := tester.Click("关闭错误详情"); err != nil || m.errorOpen {
				t.Fatal("cannot dismiss error details", err)
			}
		})
	}
}
