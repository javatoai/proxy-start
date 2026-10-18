package ui

import (
	"strings"
	"time"

	nativeui "github.com/egoist/mygo/ui"
)

type noticeKind uint8

const (
	noticeNeutral noticeKind = iota
	noticePending
	noticeSuccess
	noticeError
)

// Operation feedback has one reserved location across pages. Success messages
// expire without moving the form; errors remain available until dismissed.
func (m *model) setStatus(message string, kind noticeKind) {
	m.status, m.statusKind, m.statusUntil = message, kind, time.Time{}
	m.statusVersion++
	if kind == noticeSuccess {
		m.statusUntil = time.Now().Add(4 * time.Second)
	}
}

func (m *model) currentStatus(now time.Time) (string, noticeKind, bool) {
	if m.status == "" || (!m.statusUntil.IsZero() && !now.Before(m.statusUntil)) {
		return "就绪", noticeNeutral, false
	}
	return m.status, m.statusKind, true
}

// operationToast renders one shared top-right notification. The application
// state changes once per operation and the version guard prevents queuing the
// same native toast on every frame.
func (m *model) operationToast(c *nativeui.Context) {
	message, kind, active := m.currentStatus(time.Now())
	if m.statusVersion != m.toastVersion {
		m.toastVersion = m.statusVersion
		if m.toastID != "" {
			c.CloseToast(m.toastID)
			m.toastID = ""
		}
		if active {
			typeName := "info"
			if kind == noticeSuccess {
				typeName = "success"
			} else if kind == noticeError {
				typeName = "error"
			}
			timeout := 4 * time.Second
			if kind == noticePending || kind == noticeError {
				timeout = -1
			}
			toast := nativeui.Toast{ID: "proxy-start-operation", Title: toastSummary(message), Description: "", Type: typeName, Timeout: timeout}
			if kind == noticeError {
				m.toastDetail = message
				toast.Description = "操作未完成，可查看完整错误"
				toast.Action = "查看详情"
				toast.OnAction = func() {
					m.toastDetailOpen = true
				}
			}
			m.toastID = c.AddToast(toast)
		}
	}
	nativeui.ToastViewportBase(c, func(viewport nativeui.Element, toasts []nativeui.Toast) {
		// Leave the 88px page header clear; the 16px inset places the first
		// toast at roughly 104px while retaining the top-right treatment.
		viewport.Top(88).Padding(16).AlignItems(nativeui.End).Justify(nativeui.Start).Gap(8)
		for _, item := range toasts {
			toast := nativeui.ToastBase(c, item)
			root := toast.Root.Row().Gap(12).Padding(10, 14).Radius(8).MaxWidth(420).Background(c.Theme().Surface).Border(1, c.Theme().Border)
			root.Children(func() {
				nativeui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
					nativeui.Text(c, item.Title).FontSize(13).FontWeight(600).TextColor(c.Theme().Text).Wrap()
					if item.Description != "" {
						nativeui.Text(c, item.Description).FontSize(12).TextColor(c.Theme().TextMuted).Wrap()
					}
				})
				if item.Type == "error" {
					action := toast.ActionButton().Height(28).Padding(0, 8).Radius(6).Label("查看完整错误")
					if action.Hovered() {
						action.Background(c.Theme().SurfaceHover)
					}
					action.Children(func() {
						nativeui.Text(c, item.Action).FontSize(12).FontWeight(600).TextColor(c.Theme().Accent).SingleLine()
					})
					close := toast.CloseButton().Size(24, 24).Radius(6).Label("关闭错误提示").Tooltip("关闭提示")
					if close.Hovered() {
						close.Background(c.Theme().SurfaceHover)
					}
					close.Children(func() { nativeui.Icon(c, clearIcon).Size(12, 12).TextColor(c.Theme().TextMuted).Center() })
				}
			})
		}
	})
}

func toastSummary(message string) string {
	message = strings.TrimSpace(strings.ReplaceAll(message, "\r\n", "\n"))
	message = strings.Join(strings.Fields(message), " ")
	// Keep the compact notification to about three lines at the native
	// 420px toast width. The detail modal carries the complete message.
	const maxRunes = 48
	runes := []rune(message)
	if len(runes) <= maxRunes {
		return message
	}
	return string(runes[:maxRunes-1]) + "…"
}

func completedMessage(key string) string {
	switch {
	case key == "theme":
		return "主题已更新"
	case key == "hide-system-apps":
		return "应用列表显示设置已保存"
	case key == "profile-save":
		return "代理配置已保存"
	case key == "profile-default":
		return "默认代理已更新"
	case key == "profile-delete":
		return "代理已删除"
	case strings.HasPrefix(key, "profile-console:"):
		return "已打开代理控制台"
	case key == "profile-credential-clear":
		return "已清除保存的认证信息"
	case strings.HasPrefix(key, "add:"):
		return "程序已添加"
	case strings.HasPrefix(key, "remove:"):
		return "程序已移除"
	case strings.HasPrefix(key, "save:"):
		return "程序设置已保存"
	case strings.HasPrefix(key, "start:"):
		return "程序已启动"
	case strings.HasPrefix(key, "stop:"):
		return "程序已结束"
	case strings.HasPrefix(key, "restart:"):
		return "程序已重启"
	default:
		return "操作已完成"
	}
}
