package ui

import (
	"testing"

	nativeui "github.com/egoist/mygo/ui"
	"proxy-start/internal/application"
	"proxy-start/internal/domain"
)

func TestBackendPresentationUsesHonestStates(t *testing.T) {
	theme := nativeui.LightTheme()
	tests := []struct {
		name    string
		status  domain.BackendStatus
		label   string
		summary string
	}{
		{name: "missing", status: domain.BackendStatus{Detail: "未检测到 ProxiFyre 安装"}, label: "未安装", summary: "尚未安装强制代理组件"},
		{name: "check failed", status: domain.BackendStatus{Detail: "helper exited unexpectedly"}, label: "检查失败", summary: "无法确认组件状态，请查看诊断信息"},
		{name: "installed but not ready", status: domain.BackendStatus{Installed: true, Detail: "ProxiFyre 已停止"}, label: "已安装，未就绪", summary: "组件已安装，但就绪检查尚未通过"},
		{name: "running but not ready", status: domain.BackendStatus{Installed: true, Running: true}, label: "已安装，未就绪", summary: "组件进程正在运行，但就绪检查尚未通过"},
		{name: "ready", status: domain.BackendStatus{Installed: true, Ready: true}, label: "已就绪", summary: "组件已安装并通过就绪检查"},
		{name: "running", status: domain.BackendStatus{Installed: true, Ready: true, Running: true}, label: "运行中", summary: "组件已启动，可为启用强制代理的应用提供服务"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			label, summary, _ := backendPresentation(tt.status, theme)
			if label != tt.label || summary != tt.summary {
				t.Fatalf("backendPresentation() = (%q, %q), want (%q, %q)", label, summary, tt.label, tt.summary)
			}
		})
	}
}

func TestBackendDiagnosticsStayCollapsedUntilRequested(t *testing.T) {
	m := newModel(nil)
	m.page = pageSystemSettings
	m.snapshot = application.Snapshot{
		Settings: domain.Settings{Profiles: []domain.ProxyProfile{{ID: "local", Name: "本机", Protocol: domain.ProtocolAuto, Host: "127.0.0.1", Port: 7897}}},
		Backend:  domain.BackendStatus{Detail: "helper exited unexpectedly"},
	}
	tester := nativeui.NewTester(m.view, 1080, 720)
	if !tester.HasText("检查失败") || !tester.HasText("查看诊断") || tester.HasText("helper exited unexpectedly") {
		t.Fatalf("diagnostic detail should start collapsed: %v", tester.Texts())
	}
	if err := tester.Click("查看诊断"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("helper exited unexpectedly") || !tester.HasText("收起诊断") {
		t.Fatalf("diagnostic detail did not open: %v", tester.Texts())
	}
}
