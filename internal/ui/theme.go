package ui

import nativeui "github.com/egoist/mygo/ui"

func (m *model) applyTheme(c *nativeui.Context) {
	t := nativeui.LightTheme()
	t.Background = nativeui.Hex("#FFFFFF")
	t.Surface = nativeui.Hex("#F3F5F7")
	t.SurfaceHover = nativeui.Hex("#E9EDF2")
	t.SurfacePressed = nativeui.Hex("#DEE5ED")
	t.Border = nativeui.Hex("#DCE1E8")
	t.Text = nativeui.Hex("#202938")
	t.TextMuted = nativeui.Hex("#667085")
	t.Success = nativeui.Hex("#177B46")
	t.Danger = nativeui.Hex("#C53B3F")
	t.Accent = nativeui.Hex("#2C6DE8")
	t.AccentHover = nativeui.Hex("#2563D9")
	t.AccentPressed = nativeui.Hex("#1D4ED8")
	t.AccentText = nativeui.Hex("#FFFFFF")
	t.Font, t.FontSize, t.Radius = m.uiFontFamily(), 14, 8
	c.SetTheme(t)
}
