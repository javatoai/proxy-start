package ui

import (
	"context"
	"log"
	"time"

	nativeui "github.com/egoist/mygo/ui"
	"proxy-start/internal/domain"
)

// These are authored line icons for navigation and actions. Program icons
// are always loaded from the platform provider; this glyph is only the
// neutral fallback while a provider has no image.
var (
	clearIcon    = nativeui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><path d="m7 7 10 10M17 7 7 17"/></svg>`))
	appsIcon     = nativeui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/></svg>`))
	settingsIcon = nativeui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3v2m0 14v2M3 12h2m14 0h2M5.64 5.64l1.41 1.41m9.9 9.9 1.41 1.41m0-12.72-1.41 1.41m-9.9 9.9-1.41 1.41"/><circle cx="12" cy="12" r="4"/></svg>`))
	searchIcon   = nativeui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><circle cx="10.8" cy="10.8" r="6.5"/><path d="m16 16 4.5 4.5"/></svg>`))
	plusIcon     = nativeui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><path d="M12 5v14M5 12h14"/></svg>`))
	playIcon     = nativeui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="m8 5 11 7-11 7z"/></svg>`))
	stopIcon     = nativeui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="6" y="6" width="12" height="12" rx="1.5"/></svg>`))
	restartIcon  = nativeui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M20 11a8 8 0 0 0-14.7-4L3 10"/><path d="M3 5v5h5M4 13a8 8 0 0 0 14.7 4L21 14"/><path d="M21 19v-5h-5"/></svg>`))
	appGlyph     = nativeui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><rect x="4" y="4" width="16" height="16" rx="3"/><path d="M8 9h8M8 13h5M8 17h8"/></svg>`))
)

func (m *model) appIcon(c *nativeui.Context, entry domain.AppEntry) {
	t := c.Theme()
	if bitmap := m.icons[entry.ID]; bitmap != nil {
		nativeui.Image(c, bitmap).Size(30, 30).Fit(nativeui.Contain).Label(entry.Name + " 图标")
		return
	}
	nativeui.Box(c).Size(30, 30).Radius(7).Background(t.Selection).Children(func() {
		nativeui.Icon(c, appGlyph).Size(18, 18).TextColor(modeColor(t, domain.ModeNative)).Center()
	})
	m.requestIcon(entry)
}

func (m *model) requestIcon(entry domain.AppEntry) {
	if m.desktop == nil || m.win == nil {
		return
	}
	signature := appIconSignature(entry)
	if m.iconLoading[entry.ID] || m.iconAttempts[entry.ID] == signature {
		return
	}
	m.iconAttempts[entry.ID] = signature
	m.iconLoading[entry.ID] = true
	go func(id, signature string) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		data, err := m.desktop.Icon(ctx, id)
		cancel()
		m.win.Update(func() {
			m.iconLoading[id] = false
			if current, ok := m.appByID(id); !ok || appIconSignature(current) != signature {
				return
			}
			if err != nil || len(data) == 0 {
				if err != nil {
					log.Printf("proxy-start: app icon %s: %v", id, err)
				}
				return
			}
			bitmap, decodeErr := nativeui.DecodeBitmap(data)
			if decodeErr != nil {
				log.Printf("proxy-start: decode app icon %s: %v", id, decodeErr)
				return
			}
			m.icons[id] = bitmap
		})
	}(entry.ID, signature)
}

func appIconSignature(entry domain.AppEntry) string {
	return entry.Identity.Executable + "|" + entry.Identity.AUMID + "|" + entry.Identity.PackageName + "|" + entry.Identity.PackageFamily
}
