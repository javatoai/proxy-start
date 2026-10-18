package ui

import (
	"strings"

	"github.com/egoist/mygo"
	nativeui "github.com/egoist/mygo/ui"
)

const desktopUIFont = "Microsoft YaHei UI, Segoe UI"

func (m *model) view(c *nativeui.Context) {
	m.applyTheme(c)
	if c.Shortcut(nativeui.Cmd, nativeui.KeyQ) {
		mygo.App.Quit()
		return
	}
	t := c.Theme()
	nativeui.Row(c).Fill().Background(t.Background).Font(m.uiFontFamily()).Children(func() {
		m.sidebar(c)
		nativeui.Column(c).Grow(1).MinWidth(0).Fill().Children(func() {
			nativeui.Box(c).Grow(1).MinHeight(0).FillWidth().Children(func() {
				switch m.page {
				case pageProxySettings:
					m.proxySettingsPage(c)
				case pageSystemSettings:
					m.systemSettingsPage(c)
				default:
					m.launchPage(c)
				}
			})
		})
	})
	if m.installed.open {
		m.installedDialog(c)
	}
	if m.proxyModalOpen {
		m.profileModal(c)
	}
	if m.deleteModalOpen {
		m.deleteProfileModal(c)
	}
	if m.toastDetailOpen {
		m.toastDetailModal(c)
	}
	m.operationToast(c)
}

func (m *model) applyTheme(c *nativeui.Context) {
	var theme *nativeui.Theme
	switch strings.ToLower(m.snapshot.Settings.Theme) {
	case "light":
		theme = nativeui.LightTheme()
	case "dark":
		theme = nativeui.DarkTheme()
	case "system", "":
		// MyGO resets Context.Theme from the host's current appearance at
		// the beginning of every frame. Copy it before applying product
		// colors so SetTheme also updates the root text style.
		copy := *c.Theme()
		theme = &copy
	default:
		copy := *c.Theme()
		theme = &copy
	}
	t := theme
	// Keep the product accent stable across Windows accent preferences so
	// the selected navigation and primary actions retain the intended blue.
	if t.Dark {
		t.Background = nativeui.Hex("#171D26")
		t.Surface = nativeui.Hex("#232B37")
		t.SurfaceHover = nativeui.Hex("#2C3746")
		t.SurfacePressed = nativeui.Hex("#364255")
		t.Border = nativeui.Hex("#394453")
		t.Text = nativeui.Hex("#EDF2F7")
		t.TextMuted = nativeui.Hex("#A8B3C2")
		t.Success = nativeui.Hex("#63D69A")
		t.Warning = nativeui.Hex("#F4B75E")
		t.Danger = nativeui.Hex("#FF8A8F")
		t.Accent = nativeui.Hex("#2C6DE8")
		t.AccentHover = nativeui.Hex("#2563D9")
		t.AccentPressed = nativeui.Hex("#1D4ED8")
		t.Selection = nativeui.RGBA(74, 140, 255, 0.34)
		t.Focus = nativeui.RGBA(104, 160, 255, 0.62)
	} else {
		t.Background = nativeui.Hex("#FFFFFF")
		t.Surface = nativeui.Hex("#F3F5F7")
		t.SurfaceHover = nativeui.Hex("#E9EDF2")
		t.SurfacePressed = nativeui.Hex("#DEE5ED")
		t.Border = nativeui.Hex("#DCE1E8")
		t.Text = nativeui.Hex("#202938")
		t.TextMuted = nativeui.Hex("#667085")
		t.Success = nativeui.Hex("#177B46")
		t.Warning = nativeui.Hex("#A85A08")
		t.Danger = nativeui.Hex("#C53B3F")
		t.Accent = nativeui.Hex("#2C6DE8")
		t.AccentHover = nativeui.Hex("#2563D9")
		t.AccentPressed = nativeui.Hex("#1D4ED8")
		t.Selection = nativeui.RGBA(52, 120, 246, 0.23)
		t.Focus = nativeui.RGBA(52, 120, 246, 0.52)
	}
	t.AccentText = nativeui.Hex("#FFFFFF")
	t.Font = m.uiFontFamily()
	t.FontSize = 14
	t.Radius = 8
	c.SetTheme(t)
}

func (m *model) sidebar(c *nativeui.Context) {
	t := c.Theme()
	sidebar := nativeui.Hex("#EDF2F6")
	if t.Dark {
		sidebar = t.Surface
	}
	nativeui.Column(c).Width(214).FillHeight().Padding(24, 16, 20, 16).Gap(20).Background(sidebar).Children(func() {
		nativeui.Row(c).Gap(10).AlignItems(nativeui.Center).Children(func() {
			m.brandBadge(c)
			nativeui.Column(c).Gap(3).Children(func() {
				nativeui.Text(c, "Proxy Start").FontSize(18).FontWeight(600).SingleLine()
				nativeui.Text(c, "按应用连接代理").FontSize(12).TextColor(t.TextMuted).SingleLine()
			})
		})
		m.sidebarDivider(c)
		nativeui.Column(c).Gap(5).Children(func() {
			m.navButton(c, "应用", pageLaunch, appsIcon)
			m.navButton(c, "代理设置", pageProxySettings, settingsIcon)
			m.navButton(c, "系统设置", pageSystemSettings, settingsIcon)
		})
	})
}

func (m *model) sidebarDivider(c *nativeui.Context) {
	nativeui.Box(c).Height(1).FillWidth().Background(c.Theme().Border)
}

func (m *model) navButton(c *nativeui.Context, label string, page int, icon *nativeui.SVG) {
	t := c.Theme()
	button := nativeui.ButtonBase(c).FillWidth().Height(40).Padding(0, 12).Radius(8).Label(label).TextColor(t.Text)
	if page == m.page {
		button.Background(t.Accent).TextColor(t.AccentText)
	} else if button.Hovered() {
		button.Background(t.SurfaceHover)
	}
	button.Children(func() {
		nativeui.Row(c).FillWidth().Gap(10).AlignItems(nativeui.Center).Children(func() {
			nativeui.Icon(c, icon).Size(17, 17)
			nativeui.Text(c, label).FontSize(14).FontWeight(500)
		})
	})
	if button.Clicked() {
		m.page = page
	}
}
