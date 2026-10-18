package ui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	nativeui "github.com/egoist/mygo/ui"
	"proxy-start/internal/application"
	"proxy-start/internal/domain"
)

func TestProbePresentationKeepsSummaryShort(t *testing.T) {
	tests := []struct {
		name    string
		probe   domain.ProbeResult
		status  string
		summary string
	}{
		{
			name:    "verified protocols",
			probe:   domain.ProbeResult{HTTP: true, SOCKS5: true, HTTPVerified: true, SOCKS5Verified: true},
			status:  "代理连接已验证",
			summary: "已验证 HTTP · SOCKS5",
		},
		{
			name:    "identified only",
			probe:   domain.ProbeResult{HTTP: true, SOCKS5: true},
			status:  "已识别代理协议",
			summary: "已识别 HTTP · SOCKS5，连接未验证",
		},
		{
			name:    "udp only",
			probe:   domain.ProbeResult{UDPVerified: true},
			status:  "部分验证",
			summary: "已验证 UDP",
		},
		{
			name:    "failed without protocol",
			probe:   domain.ProbeResult{CheckedAt: time.Unix(1, 0), Detail: "连接失败: connection refused"},
			status:  "连接未通过验证",
			summary: "请检查地址、端口或认证信息",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := probeCardStatus(tt.probe); got != tt.status {
				t.Fatalf("probeCardStatus() = %q, want %q", got, tt.status)
			}
			if got := probeCardSummary(tt.probe); got != tt.summary {
				t.Fatalf("probeCardSummary() = %q, want %q", got, tt.summary)
			}
		})
	}
}

func TestProbePendingHidesPreviousResult(t *testing.T) {
	m := newModel(nil)
	m.page = pageProxySettings
	m.proxyModalOpen = true
	m.snapshot = application.Snapshot{Settings: domain.Settings{
		Profiles: []domain.ProxyProfile{{ID: "local", Name: "本机 Clash", Protocol: domain.ProtocolAuto, Host: "127.0.0.1", Port: 7897}},
	}}
	m.probes["local"] = domain.ProbeResult{HTTP: true, HTTPVerified: true, TCPVerified: true, Detail: "旧的成功结果"}
	m.busy["probe:local"] = true
	tester := nativeui.NewTester(m.view, 1080, 720)
	if !tester.HasText("正在测试代理连接…") || !tester.HasText("正在验证 TCP 连接") {
		t.Fatalf("pending probe summary missing: %v", tester.Texts())
	}
	if tester.HasText("旧的成功结果") || tester.HasText("查看详情") {
		t.Fatalf("pending probe should hide previous result: %v", tester.Texts())
	}
}

func TestProfileAuthSectionCollapsesWithoutDroppingDraft(t *testing.T) {
	m := detailsFixtureModel("light", domain.ProbeResult{})
	tester := nativeui.NewTester(m.view, 1180, 760)
	if tester.HasText("用户名") || tester.HasText("密码") {
		t.Fatalf("optional auth should start collapsed: %v", tester.Texts())
	}
	if err := tester.Click("展开认证（可选）"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("用户名") || !tester.HasText("密码") || !tester.HasText("收起") {
		t.Fatalf("optional auth did not expand: %v", tester.Texts())
	}
	m.profileDraft.username = "alice"
	m.profileDraft.password = "secret"
	tester.Frame()
	if err := tester.Click("收起认证（可选）"); err != nil {
		t.Fatal(err)
	}
	if tester.HasText("用户名") || tester.HasText("密码") {
		t.Fatalf("optional auth did not collapse: %v", tester.Texts())
	}
	if m.profileDraft.username != "alice" || m.profileDraft.password != "secret" {
		t.Fatalf("collapsing optional auth dropped draft values: username=%q password=%q", m.profileDraft.username, m.profileDraft.password)
	}
}

func TestProfileAuthSectionExpandsForSavedCredential(t *testing.T) {
	m := newModel(nil)
	m.page = pageProxySettings
	m.proxyModalOpen = true
	m.snapshot = application.Snapshot{Settings: domain.Settings{
		Profiles: []domain.ProxyProfile{{ID: "local", Name: "本机 Clash", Protocol: domain.ProtocolAuto, Host: "127.0.0.1", Port: 7897, CredentialRef: "proxy-start/local"}},
	}}
	m.profileID = "local"
	tester := nativeui.NewTester(m.view, 1180, 760)
	if !tester.HasText("用户名") || !tester.HasText("密码") || !tester.HasText("收起") {
		t.Fatalf("saved credential should auto-expand auth section: %v", tester.Texts())
	}
}

func TestSettingsDetailsLayoutAndScreenshots(t *testing.T) {
	root := projectRoot(t)
	dir := filepath.Join(root, ".impeccable", "review", "details-v3")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, scenario := range []struct {
		name   string
		status string
		kind   noticeKind
		busy   bool
		probe  domain.ProbeResult
	}{
		{name: "success", status: "代理配置已保存", kind: noticeSuccess, probe: domain.ProbeResult{HTTP: true, SOCKS5: true, HTTPVerified: true, SOCKS5Verified: true, TCPVerified: true, Detail: "HTTP: CONNECT 已验证 (200 Connection established); SOCKS5: CONNECT 已验证"}},
		{name: "pending", status: "正在测试代理连接…", kind: noticePending, busy: true, probe: domain.ProbeResult{HTTP: true, HTTPVerified: true, TCPVerified: true, Detail: "上一次结果应隐藏"}},
		{name: "error", status: "代理测试失败：连接被拒绝", kind: noticeError, probe: domain.ProbeResult{CheckedAt: time.Unix(1, 0), Detail: "连接失败: connection refused"}},
	} {
		for _, theme := range []string{"light", "dark"} {
			m := detailsFixtureModel(theme, scenario.probe)
			m.setStatus(scenario.status, scenario.kind)
			m.busy["probe:local"] = scenario.busy
			for _, size := range []struct {
				label string
				w     int
				h     int
			}{{"1632x971", 1632, 971}, {"1180x760", 1180, 760}} {
				tester := nativeui.NewTester(m.view, size.w, size.h)
				path := filepath.Join(dir, scenario.name+"-"+theme+"-"+size.label+".png")
				writeNativePNG(t, path, tester.Image())
			}
			if theme == "dark" {
				tester := nativeui.NewTester(m.view, 804, 521)
				tester.Scroll(720, 430, 0, 1600)
				path := filepath.Join(dir, scenario.name+"-dark-804x521-scrolled.png")
				writeNativePNG(t, path, tester.Image())
				if _, ok := tester.Find("保存代理配置"); !ok {
					t.Fatalf("%s: save action not reachable after minimum-size scroll", scenario.name)
				}
				if _, ok := tester.Find("删除代理"); !ok {
					t.Fatalf("%s: delete action not reachable after minimum-size scroll", scenario.name)
				}
			}
		}
	}

	// The header action keeps its existing accessible label while showing the
	// compact visual copy. A wide editor keeps save and delete on one row.
	m := detailsFixtureModel("light", domain.ProbeResult{})
	tester := nativeui.NewTester(m.view, 1632, 971)
	tester.Key(0, nativeui.KeyEscape)
	if err := tester.Click("新增代理"); err != nil {
		t.Fatal(err)
	}
	if !m.profileNew {
		t.Fatal("new proxy action did not enter create mode")
	}
	m.profileNew = false
	m.profileID = "local"
	m.loadProfileDraft(m.snapshot.Settings.Profiles[0])
	tester.Frame()
	save, saveOK := tester.Find("保存代理配置")
	remove, removeOK := tester.Find("删除代理")
	if !saveOK || !removeOK || absFloat(save.Y-remove.Y) > 1 {
		t.Fatalf("save/delete actions should share one row: save=%+v delete=%+v", save, remove)
	}
}

func detailsFixtureModel(theme string, probe domain.ProbeResult) *model {
	m := newModel(nil)
	m.page = pageProxySettings
	m.proxyModalOpen = true
	m.snapshot = application.Snapshot{Settings: domain.Settings{
		Theme:    theme,
		Profiles: []domain.ProxyProfile{{ID: "local", Name: "本机 Clash", Protocol: domain.ProtocolAuto, Host: "127.0.0.1", Port: 7897}},
	}, Backend: domain.BackendStatus{Installed: false, Detail: "尚未安装强制代理组件"}}
	m.profileID = "local"
	m.probes["local"] = probe
	return m
}

func absFloat(value float32) float32 {
	if value < 0 {
		return -value
	}
	return value
}

func TestProbeDetailStaysCollapsedUntilRequested(t *testing.T) {
	m := newModel(nil)
	m.page = pageProxySettings
	m.proxyModalOpen = true
	m.snapshot = application.Snapshot{Settings: domain.Settings{
		Profiles: []domain.ProxyProfile{{ID: "local", Name: "本机 Clash", Protocol: domain.ProtocolAuto, Host: "127.0.0.1", Port: 7897}},
	}}
	m.probes["local"] = domain.ProbeResult{
		HTTP:         true,
		HTTPVerified: true,
		TCPVerified:  true,
		Detail:       "HTTP: CONNECT 已验证 (200 Connection established)",
	}
	tester := nativeui.NewTester(m.view, 1080, 720)
	if !tester.HasText("代理连接已验证") || !tester.HasText("已验证 TCP · HTTP") {
		t.Fatalf("probe summary missing: %v", tester.Texts())
	}
	if tester.HasText("HTTP: CONNECT 已验证") {
		t.Fatalf("raw probe detail should start collapsed: %v", tester.Texts())
	}
	if err := tester.Click("查看详情"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("HTTP: CONNECT 已验证") || !tester.HasText("收起详情") {
		t.Fatalf("probe detail did not open: %v", tester.Texts())
	}
}
