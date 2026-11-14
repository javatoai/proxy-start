package ui

import (
	"context"
	"github.com/egoist/mygo"
	"log"
	"proxy-start/internal/application"
	"strconv"
	"sync/atomic"
	"time"
)

type model struct {
	desktop        *application.Desktop
	snapshot       application.Snapshot
	win            *mygo.Window
	tray           *mygo.Tray
	options        Options
	port           string
	busy           bool
	pendingAction  string
	animationStart time.Time
	settingsOpen   bool
	portDraft      string
	settingsError  string
	postUpdate     func(func())
	message        string
	failed         bool
	errorOpen      bool
	quitting       atomic.Bool
	cancel         context.CancelFunc
}

func newModel(desktop *application.Desktop) *model {
	m := &model{desktop: desktop}
	if desktop != nil {
		m.snapshot = desktop.Snapshot()
	}
	if m.snapshot.Settings.ProxyPort == 0 {
		m.snapshot.Settings.ProxyPort = 7897
	}
	m.port = strconv.Itoa(m.snapshot.Settings.ProxyPort)
	return m
}

func (m *model) uiFontFamily() string {
	if m.snapshot.Platform.UIFontFamily != "" {
		return m.snapshot.Platform.UIFontFamily
	}
	return "Microsoft YaHei UI, Segoe UI"
}

func (m *model) run(message string, action func(context.Context) error) {
	m.runWithCompletion(message, action, nil)
}

func (m *model) runWithCompletion(message string, action func(context.Context) error, complete func(error)) {
	if m.busy || m.desktop == nil {
		return
	}
	m.busy, m.failed, m.message = true, false, message
	m.errorOpen = false
	operation := m.pendingAction
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := action(ctx)
		if err != nil {
			log.Printf("操作失败 action=%q: %v", operation, err)
		}
		if m.quitting.Load() {
			return
		}
		update := func() {
			m.busy, m.pendingAction = false, ""
			m.snapshot = m.desktop.Snapshot()
			if err != nil {
				m.message, m.failed = err.Error(), true
			} else {
				m.message = ""
			}
			if complete != nil {
				complete(err)
			}
		}
		if m.postUpdate != nil {
			m.postUpdate(update)
		} else {
			m.win.Update(update)
		}
	}()
}

func (m *model) refreshCodex() {
	if m.busy || m.desktop == nil {
		return
	}
	m.pendingAction = "refresh"
	m.run("", func(ctx context.Context) error {
		started := time.Now()
		err := m.desktop.ScanCodex(ctx)
		if remaining := 650*time.Millisecond - time.Since(started); remaining > 0 {
			timer := time.NewTimer(remaining)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return err
	})
}

func (m *model) runLifecycle(action string) {
	if m.busy || m.desktop == nil {
		return
	}
	m.pendingAction = action
	switch action {
	case "start":
		m.withPort(m.desktop.StartCodex)
	case "restart":
		m.withPort(m.desktop.RestartCodex)
	case "stop":
		m.run("", m.desktop.StopCodex)
	}
	if !m.busy {
		m.pendingAction = ""
	}
}

func (m *model) openSettings() {
	m.portDraft = strconv.Itoa(m.snapshot.Settings.ProxyPort)
	m.settingsError = ""
	m.settingsOpen = true
}

func (m *model) saveSettings() {
	m.applySettings(nil)
}

func (m *model) applySettings(action func(context.Context) error) {
	port, err := strconv.Atoi(m.portDraft)
	if err != nil || port < 1 || port > 65535 {
		m.settingsError = "请输入 1 到 65535 之间的端口号"
		return
	}
	m.settingsError = ""
	m.runWithCompletion("", func(ctx context.Context) error {
		if err := m.desktop.SaveProxyPort(ctx, port); err != nil {
			return err
		}
		if action != nil {
			return action(ctx)
		}
		return nil
	}, func(err error) {
		m.port = strconv.Itoa(m.snapshot.Settings.ProxyPort)
		if err != nil {
			m.settingsError, m.message, m.failed = err.Error(), "", false
			return
		}
		m.settingsOpen = false
	})
}

func (m *model) withPort(action func(context.Context) error) {
	port, err := strconv.Atoi(m.port)
	if err != nil || port < 1 || port > 65535 {
		m.message, m.failed = "请输入 1 到 65535 之间的端口号", true
		return
	}
	m.run("正在处理…", func(ctx context.Context) error {
		if err := m.desktop.SaveProxyPort(ctx, port); err != nil {
			return err
		}
		if action != nil {
			return action(ctx)
		}
		return nil
	})
}
