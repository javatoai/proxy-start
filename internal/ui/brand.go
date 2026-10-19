package ui

import (
	nativeui "github.com/egoist/mygo/ui"
	"proxy-start/assets"
)

// The sidebar renders the same SVG from which native PNG/ICO resources are
// generated. There is no second, hand-maintained drawing of the mark.
var brandMark = nativeui.MustParseSVG(assets.LogoSVG())

func brandIconPNG() []byte { return assets.IconPNG() }

func (m *model) brandBadge(c *nativeui.Context) {
	nativeui.Image(c, brandMark).Size(32, 32).Fit(nativeui.Contain).Label("Proxy Start")
}
