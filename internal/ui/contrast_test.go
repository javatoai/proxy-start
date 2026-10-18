package ui

import (
	"math"
	"testing"

	nativeui "github.com/egoist/mygo/ui"
)

func TestEnabledControlAndSecondaryTextContrast(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		m := newModel(nil)
		m.snapshot.Settings.Theme = appearance
		var theme nativeui.Theme
		nativeui.NewTester(func(c *nativeui.Context) {
			m.applyTheme(c)
			theme = *c.Theme()
		}, 100, 100)
		pairs := []struct {
			name             string
			text, background nativeui.Color
		}{
			{"primary/nav normal", theme.AccentText, theme.Accent},
			{"primary/nav hover", theme.AccentText, theme.AccentHover},
			{"primary/nav pressed", theme.AccentText, theme.AccentPressed},
			{"segmented selection", theme.Text, theme.Background},
			{"secondary text", theme.TextMuted, theme.Surface},
			{"success text", theme.Success, theme.Surface},
			{"warning text", theme.Warning, theme.Surface},
			{"danger action", theme.Danger, theme.Surface},
		}
		for _, pair := range pairs {
			light, dark := relativeLuminance(pair.text), relativeLuminance(pair.background)
			if light < dark {
				light, dark = dark, light
			}
			ratio := (light + 0.05) / (dark + 0.05)
			if ratio < 4.5 {
				t.Errorf("%s %s contrast %.2f:1 is below 4.5:1", appearance, pair.name, ratio)
			}
		}
	}
}

func relativeLuminance(c nativeui.Color) float64 {
	linear := func(component uint8) float64 {
		v := float64(component) / 255
		if v <= 0.04045 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*linear(c.R) + 0.7152*linear(c.G) + 0.0722*linear(c.B)
}
