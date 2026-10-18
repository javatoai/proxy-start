package ui

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/egoist/mygo"
	nativeui "github.com/egoist/mygo/ui"
	buildinfo "proxy-start"
	"proxy-start/internal/application"
)

// Options controls optional startup behavior used by native smoke checks.
// Zero values preserve the normal interactive application flow.
type Options struct {
	// AutoQuitAfter asks MyGO to quit after the window is ready. It is useful
	// for bounded native startup checks and is disabled when zero or negative.
	AutoQuitAfter time.Duration
	// InitialPage selects "launch" (the default), "proxy-settings", or
	// "system-settings". The legacy value "settings" still opens proxy
	// settings.
	InitialPage string
	// InitialAppID opens one configured application's inspector in a preview.
	InitialAppID string
	// WindowWidth and WindowHeight override the initial size for previews.
	// Normal interactive windows continue to restore their saved geometry.
	WindowWidth, WindowHeight int
	// OnReady runs once the native window has presented its first frame. The
	// callback receives the window so a smoke harness can inspect its handle.
	OnReady func(*mygo.Window)
}

// Run starts Proxy Start with normal interactive behavior.
func Run(desktop *application.Desktop) error {
	return RunWithOptions(desktop, Options{})
}

// RunWithOptions starts the native MyGO application. Optional settings are
// intentionally limited to startup inspection; they do not seed data or
// change normal runtime behavior.
func RunWithOptions(desktop *application.Desktop, options Options) error {
	if desktop == nil {
		return errors.New("proxy-start: nil desktop facade")
	}
	mygo.App.SetName("Proxy Start")
	mygo.App.SetVersion(buildinfo.Version())
	// A bounded preview must render its own new binary rather than forwarding
	// to a user's older, already-running instance.
	if options.AutoQuitAfter <= 0 && !mygo.App.RequestSingleInstanceLock() {
		return nil
	}

	model := newModel(desktop)
	model.options = options
	model.selectedAppID = options.InitialAppID
	switch strings.ToLower(strings.TrimSpace(options.InitialPage)) {
	case "settings", "proxy-settings":
		model.page = pageProxySettings
	case "system-settings":
		model.page = pageSystemSettings
	}
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		model.quitting.Store(true)
		model.closeInstalled()
		if model.autoQuit != nil {
			model.autoQuit.Stop()
		}
	})
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.WhenReady(model.ready)
	return mygo.App.Run()
}

func (m *model) ready() {
	m.snapshot = m.desktop.Snapshot()
	if m.snapshot.Settings.Theme == "" {
		m.snapshot.Settings.Theme = "system"
	}
	if len(m.snapshot.Settings.Profiles) > 0 {
		m.profileID = m.snapshot.Settings.Profiles[0].ID
	}

	stateKey := "proxy-start-main"
	windowWidth, windowHeight := 1180, 760
	if m.options.AutoQuitAfter > 0 {
		stateKey = ""
		if m.options.WindowWidth >= 820 {
			windowWidth = m.options.WindowWidth
		}
		if m.options.WindowHeight >= 560 {
			windowHeight = m.options.WindowHeight
		}
	}
	m.win = mygo.NewWindow(mygo.WindowOptions{
		Title:           "Proxy Start",
		Width:           windowWidth,
		Height:          windowHeight,
		MinWidth:        820,
		MinHeight:       560,
		StateKey:        stateKey,
		Content:         nativeui.View(m.view),
		BackgroundColor: "#ffffff",
	})
	if err := m.win.SetIcon(brandIconPNG()); err != nil {
		log.Printf("proxy-start: window icon unavailable: %v", err)
	}
	m.win.OnFileDrop(func(e *mygo.FileDropEvent) {
		for _, path := range e.Paths {
			m.addPath(path)
		}
	})
	m.win.OnClose(func(e *mygo.CloseEvent) {
		// A tray-backed window is hidden on close so the running apps remain
		// manageable. If tray creation failed, allow the native close through;
		// otherwise the window would disappear with no way to reopen it.
		if !m.quitting.Load() && m.tray != nil {
			e.PreventDefault()
			m.win.Hide()
		}
	})
	if m.options.OnReady != nil {
		m.win.OnReadyToShow(func() { m.options.OnReady(m.win) })
	}

	tray, err := mygo.NewTray(mygo.TrayOptions{Icon: brandIconPNG(), ToolTip: "Proxy Start — 点击打开"})
	if err == nil {
		m.tray = tray
		tray.OnClick(m.toggleWindow)
		tray.OnRightClick(func() {
			menu := mygo.NewMenu([]*mygo.MenuItem{
				{Label: "打开 Proxy Start", Click: func(*mygo.MenuItem, *mygo.Window) { m.showWindow() }},
				mygo.Separator(),
				{Role: mygo.RoleQuit},
			})
			tray.SetMenu(menu)
			tray.PopUpMenu()
			tray.SetMenu(nil)
		})
		mygo.App.OnWillQuit(func(*mygo.QuitEvent) { tray.Destroy() })
	} else {
		log.Printf("proxy-start: tray unavailable: %v", err)
	}
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
		defer cancel()
		// Shutdown is intentionally a short, independent cleanup operation.
		// Target applications are owned by the user and are not stopped here.
		if err := m.desktop.Shutdown(ctx); err != nil {
			log.Printf("proxy-start: backend shutdown: %v", err)
		}
	})

	go m.refreshLoop()
	if m.options.AutoQuitAfter > 0 {
		m.autoQuit = time.AfterFunc(m.options.AutoQuitAfter, mygo.App.Quit)
	}
}

func (m *model) refreshLoop() {
	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if m.win == nil || m.quitting.Load() {
				return
			}
			snapshot := m.desktop.Snapshot()
			m.win.Update(func() { m.snapshot = snapshot })
		case <-m.refresh:
			return
		}
	}
}

func (m *model) showWindow() {
	if m.win == nil {
		return
	}
	m.win.Show()
	m.win.Focus()
}

func (m *model) toggleWindow() {
	if m.win == nil {
		return
	}
	if m.win.IsVisible() {
		m.win.Hide()
	} else {
		m.showWindow()
	}
}
