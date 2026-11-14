package ui

import (
	"context"
	"github.com/egoist/mygo"
	nativeui "github.com/egoist/mygo/ui"
	"log"
	"proxy-start/internal/application"
	"time"
)

type Options struct {
	AutoQuitAfter             time.Duration
	WindowWidth, WindowHeight int
	OnReady                   func(*mygo.Window)
}

func RunWithOptions(desktop *application.Desktop, options Options) error {
	m := newModel(desktop)
	m.options = options
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		m.quitting.Store(true)
		if m.cancel != nil {
			m.cancel()
		}
	})
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.WhenReady(m.ready)
	return mygo.App.Run()
}

func (m *model) ready() {
	width, height := 460, 420
	key := "proxy-start-controls-v2"
	if m.options.AutoQuitAfter > 0 {
		key = ""
		if m.options.WindowWidth >= 380 {
			width = m.options.WindowWidth
		}
		if m.options.WindowHeight >= 360 {
			height = m.options.WindowHeight
		}
	}
	m.win = mygo.NewWindow(mygo.WindowOptions{Title: "Proxy Start", Width: width, Height: height, MinWidth: 380, MinHeight: 360, StateKey: key, Content: nativeui.View(m.view), BackgroundColor: "#ffffff"})
	if err := m.win.SetIcon(brandIconPNG()); err != nil {
		log.Printf("窗口图标：%v", err)
	}
	tray, err := mygo.NewTray(mygo.TrayOptions{Icon: brandIconPNG(), ToolTip: "Proxy Start — 点击打开"})
	if err == nil {
		m.tray = tray
		tray.OnClick(func() { m.win.Show(); m.win.Focus() })
		tray.OnRightClick(func() {
			menu := mygo.NewMenu([]*mygo.MenuItem{
				{Label: "打开 Proxy Start", Click: func(*mygo.MenuItem, *mygo.Window) { m.win.Show(); m.win.Focus() }},
				mygo.Separator(), {Role: mygo.RoleQuit},
			})
			tray.SetMenu(menu)
			tray.PopUpMenu()
			tray.SetMenu(nil)
		})
		mygo.App.OnWillQuit(func(*mygo.QuitEvent) { tray.Destroy() })
	} else {
		log.Printf("托盘不可用：%v", err)
	}
	m.win.OnClose(func(e *mygo.CloseEvent) {
		if !m.quitting.Load() && m.tray != nil {
			e.PreventDefault()
			m.win.Hide()
		}
	})
	if m.options.OnReady != nil {
		m.win.OnReadyToShow(func() { m.options.OnReady(m.win) })
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	go func() {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				snapshot := m.desktop.Snapshot()
				if !m.quitting.Load() {
					m.win.Update(func() { m.snapshot = snapshot })
				}
			}
		}
	}()
	if m.options.AutoQuitAfter > 0 {
		time.AfterFunc(m.options.AutoQuitAfter, mygo.App.Quit)
	}
}
