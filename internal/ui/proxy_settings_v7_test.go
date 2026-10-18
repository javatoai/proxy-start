package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	nativeui "github.com/egoist/mygo/ui"
	"proxy-start/internal/application"
	"proxy-start/internal/domain"
)

func TestProxySettingsListAndModal(t *testing.T) {
	m := newModel(nil)
	m.page = pageProxySettings
	m.snapshot = application.Snapshot{Settings: domain.Settings{Theme: "light", DefaultProxyID: "local", Profiles: []domain.ProxyProfile{{ID: "local", Name: "本机 Clash", Protocol: domain.ProtocolAuto, Host: "127.0.0.1", Port: 7897}}}}
	tester := nativeui.NewTester(m.view, 1180, 760)
	for _, text := range []string{"代理设置", "代理列表", "打开控制台", "编辑", "删除", "默认"} {
		if !tester.HasText(text) {
			t.Fatalf("proxy list missing %q: %v", text, tester.Texts())
		}
	}
	if tester.HasText("强制代理组件") {
		t.Fatal("system settings content leaked into proxy settings")
	}
	if err := tester.Click("新增代理"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("保存代理配置") || !tester.HasText("地址") || !tester.HasText("协议") {
		t.Fatalf("proxy editor modal missing fields: %v", tester.Texts())
	}
	tester.Key(0, nativeui.KeyEscape)
	if tester.HasText("保存代理配置") {
		t.Fatal("Escape did not close proxy editor modal")
	}
	if err := tester.Click("删除代理 本机 Clash"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("确定删除“本机 Clash”吗？") {
		t.Fatalf("delete confirmation missing: %v", tester.Texts())
	}
	if err := tester.Click("取消删除代理"); err != nil {
		t.Fatal(err)
	}
	if m.proxyModalOpen {
		t.Fatal("list delete cancellation reopened an editor")
	}
}

func TestSystemSettingsOwnsGlobalPreferences(t *testing.T) {
	m := newModel(nil)
	m.page = pageSystemSettings
	m.snapshot = application.Snapshot{Settings: domain.Settings{Theme: "dark"}, Backend: domain.BackendStatus{Installed: true, Ready: true}}
	tester := nativeui.NewTester(m.view, 1180, 760)
	for _, text := range []string{"系统设置", "外观", "应用列表", "隐藏系统软件", "强制代理组件"} {
		if !tester.HasText(text) {
			t.Fatalf("system settings missing %q: %v", text, tester.Texts())
		}
	}
}

func TestOperationFeedbackUsesFloatingToast(t *testing.T) {
	m := newModel(nil)
	m.snapshot.Settings.Theme = "light"
	tester := nativeui.NewTester(m.view, 1180, 760)
	m.setStatus("代理配置已保存", noticeSuccess)
	tester.Frame()
	if !tester.HasText("代理配置已保存") || tester.HasText("操作状态：代理配置已保存") {
		t.Fatalf("success feedback did not render as a toast: %v", tester.Texts())
	}
	m.setStatus("操作失败：连接被拒绝", noticeError)
	tester.Frame()
	if err := tester.Click("关闭错误提示"); err != nil {
		t.Fatal(err)
	}
	if tester.HasText("操作失败：连接被拒绝") {
		t.Fatal("error toast did not close")
	}
}

func TestCompactProxyListKeepsActionsVisible(t *testing.T) {
	m := newModel(nil)
	m.page = pageProxySettings
	m.snapshot = application.Snapshot{Settings: domain.Settings{Theme: "light", DefaultProxyID: "local", Profiles: []domain.ProxyProfile{
		{ID: "local", Name: "本机 Clash", Protocol: domain.ProtocolAuto, Host: "127.0.0.1", Port: 7897},
	}}}
	tester := nativeui.NewTester(m.view, 820, 560)
	for _, label := range []string{"打开控制台", "编辑", "删除", "默认", "127.0.0.1:7897"} {
		if !tester.HasText(label) {
			t.Fatalf("compact proxy list missing %q: %v", label, tester.Texts())
		}
	}
	for _, label := range []string{"打开代理控制台", "编辑代理 本机 Clash", "删除代理 本机 Clash"} {
		bounds, ok := tester.Find(label)
		if !ok || bounds.X < 214 || bounds.X+bounds.W > 820 {
			t.Fatalf("compact action %q is clipped: ok=%v bounds=%+v", label, ok, bounds)
		}
	}
}

func TestLongErrorUsesDetailModal(t *testing.T) {
	m := newModel(nil)
	m.snapshot.Settings.Theme = "light"
	long := "操作失败：" + strings.Repeat("代理连接被拒绝；请检查地址、端口和认证信息。", 8)
	m.setStatus(long, noticeError)
	tester := nativeui.NewTester(m.view, 820, 560)
	if !tester.HasText("查看详情") {
		t.Fatalf("long error toast did not expose detail action: %v", tester.Texts())
	}
	if err := tester.Click("查看完整错误"); err != nil {
		if err := tester.Click("查看详情"); err != nil {
			t.Fatal(err)
		}
	}
	if !m.toastDetailOpen || !tester.HasText(long) {
		t.Fatalf("detail modal did not show complete error")
	}
}

func TestDeleteSessionTokenDoesNotClearNewDraft(t *testing.T) {
	m := newModel(nil)
	m.deleteProfileID = "old"
	m.deleteSessionToken = 1
	if !m.deleteSessionCurrent(1, "old") {
		t.Fatal("active delete session was rejected")
	}
	m.deleteSessionToken = 2
	m.deleteProfileID = "new"
	m.profileDraft = profileDraft{id: "new", name: "新代理"}
	if m.deleteSessionCurrent(1, "old") {
		t.Fatal("stale delete callback was allowed to mutate a new session")
	}
	if m.profileDraft.name != "新代理" {
		t.Fatal("new draft was changed by stale delete session")
	}
}

func TestProxySettingsV7ReviewFrames(t *testing.T) {
	dir := filepath.Join(projectRoot(t), ".impeccable", "review", "settings-v7")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, theme := range []string{"light", "dark"} {
		m := newModel(nil)
		m.page = pageProxySettings
		m.snapshot = application.Snapshot{Settings: domain.Settings{Theme: theme, DefaultProxyID: "local", Profiles: []domain.ProxyProfile{
			{ID: "local", Name: "本机 Clash", Protocol: domain.ProtocolAuto, Host: "127.0.0.1", Port: 7897},
			{ID: "remote", Name: "远程工作网络", Protocol: domain.ProtocolSOCKS5, Host: "proxy.example.com", Port: 1080},
		}}}
		tester := nativeui.NewTester(m.view, 1180, 760)
		writeNativePNG(t, filepath.Join(dir, "proxy-list-"+theme+".png"), tester.Image())
		if err := tester.Click("编辑代理 本机 Clash"); err != nil {
			t.Fatal(err)
		}
		writeNativePNG(t, filepath.Join(dir, "proxy-editor-"+theme+".png"), tester.Image())
	}
	for _, theme := range []string{"light", "dark"} {
		compact := newModel(nil)
		compact.page = pageProxySettings
		compact.snapshot = application.Snapshot{Settings: domain.Settings{Theme: theme, DefaultProxyID: "local", Profiles: []domain.ProxyProfile{{ID: "local", Name: "本机 Clash", Protocol: domain.ProtocolAuto, Host: "127.0.0.1", Port: 7897}}}}
		compactTester := nativeui.NewTester(compact.view, 820, 560)
		writeNativePNG(t, filepath.Join(dir, "proxy-list-"+theme+"-minimum.png"), compactTester.Image())
		if theme == "light" {
			if err := compactTester.Click("编辑代理 本机 Clash"); err != nil {
				t.Fatal(err)
			}
			if err := compactTester.Click("展开认证（可选）"); err != nil {
				t.Fatal(err)
			}
			compactTester.Scroll(400, 300, 0, 320)
			writeNativePNG(t, filepath.Join(dir, "proxy-editor-light-minimum-auth.png"), compactTester.Image())
		}
	}
	m := newModel(nil)
	m.page = pageSystemSettings
	m.snapshot = application.Snapshot{Settings: domain.Settings{Theme: "light"}, Backend: domain.BackendStatus{Installed: true, Ready: true}}
	tester := nativeui.NewTester(m.view, 820, 560)
	writeNativePNG(t, filepath.Join(dir, "system-settings-minimum.png"), tester.Image())
	m.setStatus("代理配置已保存", noticeSuccess)
	tester.Frame()
	writeNativePNG(t, filepath.Join(dir, "toast-success-top-right.png"), tester.Image())
	long := "操作失败：" + strings.Repeat("代理连接被拒绝；请检查地址、端口和认证信息。", 8)
	m.setStatus(long, noticeError)
	tester.Frame()
	writeNativePNG(t, filepath.Join(dir, "toast-error-summary.png"), tester.Image())
	if err := tester.Click("查看完整错误"); err != nil {
		t.Fatal(err)
	}
	writeNativePNG(t, filepath.Join(dir, "toast-error-detail.png"), tester.Image())
}
