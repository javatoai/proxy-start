package ui

import (
	"testing"
	"time"
)

func TestSuccessExpiresButErrorsRemainAvailable(t *testing.T) {
	m := newModel(nil)
	m.setStatus("代理配置已保存", noticeSuccess)
	message, _, active := m.currentStatus(m.statusUntil.Add(time.Second))
	if active || message != "就绪" {
		t.Fatal("success message did not expire")
	}
	m.setStatus("代理测试失败", noticeError)
	message, kind, active := m.currentStatus(time.Now().Add(time.Hour))
	if !active || message != "代理测试失败" || kind != noticeError {
		t.Fatal("error disappeared without dismissal")
	}
}

func TestStatusVersionsOnlyAdvanceForNewFeedback(t *testing.T) {
	m := newModel(nil)
	if m.statusVersion != 0 || m.toastVersion != 0 {
		t.Fatal("new model has unexpected toast version")
	}
	m.setStatus("正在保存…", noticePending)
	if m.statusVersion != 1 {
		t.Fatalf("status version = %d, want 1", m.statusVersion)
	}
	m.setStatus("正在保存…", noticePending)
	if m.statusVersion != 2 {
		t.Fatalf("status version = %d, want 2", m.statusVersion)
	}
}
