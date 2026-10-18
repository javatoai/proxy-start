package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"proxy-start/internal/application"
	"proxy-start/internal/domain"
)

// platformCapabilities is the single UI boundary for platform-specific
// availability. Native views consume the immutable application snapshot and
// never inspect runtime.GOOS themselves.
func (m *model) platformCapabilities() application.PlatformCapabilities {
	p := m.snapshot.Platform
	// Existing headless fixtures and older callers omit Platform. Keep those
	// renders compatible with the original Windows product behavior.
	if strings.TrimSpace(p.Name) == "" {
		return application.WindowsCapabilities()
	}
	return p
}

func (m *model) uiFontFamily() string {
	if font := strings.TrimSpace(m.platformCapabilities().UIFontFamily); font != "" {
		return font
	}
	return desktopUIFont
}

func nativeAdapterSummary(platform application.PlatformCapabilities) string {
	for _, id := range platform.NativeAdapterIDs {
		if strings.EqualFold(strings.TrimSpace(id), "codex") {
			return "原生适配 Codex"
		}
	}
	return "当前无可用原生适配"
}

func backendPresentationForPlatform(backend domain.BackendStatus, platform application.PlatformCapabilities, t *ui.Theme) (label, summary string, color ui.Color) {
	if !platform.ForceProxySupported {
		label = "当前平台不可用"
		summary = strings.TrimSpace(platform.ForceProxyUnavailableReason)
		if summary == "" {
			summary = "当前平台暂不支持按程序强制代理"
		}
		return label, summary, t.TextMuted
	}
	return backendPresentation(backend, t)
}
