package ui

import (
	"strings"

	nativeui "github.com/egoist/mygo/ui"
	"proxy-start/internal/domain"
)

func (m *model) profileFor(id string) (domain.ProxyProfile, bool) {
	for _, profile := range m.snapshot.Settings.Profiles {
		if profile.ID == id {
			return profile, true
		}
	}
	return domain.ProxyProfile{}, false
}

func launchModeLabel(mode domain.LaunchMode) string {
	if mode == domain.ModeNative {
		return "原生代理"
	}
	if mode == domain.ModeForced {
		return "强制代理"
	}
	return "尚未启动"
}

func modeColor(t *nativeui.Theme, mode domain.LaunchMode) nativeui.Color {
	if mode == domain.ModeNative {
		if t.Dark {
			return nativeui.Hex("#8CB7FF")
		}
		return nativeui.Hex("#245AC2")
	}
	if mode == domain.ModeForced {
		return t.Warning
	}
	return t.TextMuted
}

func runtimeLabel(runtime domain.AppRuntime) string {
	switch runtime.State {
	case domain.StateStarting:
		return "启动中"
	case domain.StateRunning:
		if runtime.ConnectionVerified {
			return "运行中 · 已验证"
		}
		if runtime.ProxyApplied {
			return "运行中 · 已应用"
		}
		return "运行中"
	case domain.StateStopping:
		return "结束中"
	case domain.StateFailed:
		return "失败"
	default:
		return "未运行"
	}
}

func runtimeColor(t *nativeui.Theme, runtime domain.AppRuntime) nativeui.Color {
	switch runtime.State {
	case domain.StateRunning:
		if runtime.ConnectionVerified || runtime.ProxyApplied {
			return t.Success
		}
		return t.Warning
	case domain.StateFailed:
		return t.Danger
	case domain.StateStarting, domain.StateStopping:
		return t.Warning
	default:
		return t.TextMuted
	}
}

func protocolLabel(protocol domain.Protocol) string {
	switch protocol {
	case domain.ProtocolHTTP:
		return "HTTP"
	case domain.ProtocolSOCKS5:
		return "SOCKS5"
	default:
		return "自动检测"
	}
}

func protocolOptionLabel(value string) string {
	return protocolLabel(domain.Protocol(value))
}

func protocolOptionValue(label string) string {
	switch label {
	case "HTTP":
		return string(domain.ProtocolHTTP)
	case "SOCKS5":
		return string(domain.ProtocolSOCKS5)
	default:
		return string(domain.ProtocolAuto)
	}
}

func themeOptionLabel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "light":
		return "浅色"
	case "dark":
		return "深色"
	default:
		return "跟随系统"
	}
}

func themeOptionValue(label string) string {
	switch label {
	case "浅色":
		return "light"
	case "深色":
		return "dark"
	default:
		return "system"
	}
}

func profileOptionLabel(profile domain.ProxyProfile) string {
	return firstNonEmpty(profile.Name, profile.Address())
}

func probeLabel(probe domain.ProbeResult) string {
	identified := make([]string, 0, 2)
	if probe.HTTP {
		identified = append(identified, "HTTP")
	}
	if probe.SOCKS5 {
		identified = append(identified, "SOCKS5")
	}
	verified := make([]string, 0, 4)
	if probe.TCPVerified {
		verified = append(verified, "TCP")
	}
	if probe.HTTPVerified {
		verified = append(verified, "HTTP")
	}
	if probe.SOCKS5Verified {
		verified = append(verified, "SOCKS5")
	}
	if probe.UDPVerified {
		verified = append(verified, "UDP")
	}
	primaryVerified := probe.TCPVerified || probe.HTTPVerified || probe.SOCKS5Verified
	if primaryVerified {
		return "连接成功：" + strings.Join(verified, "、")
	}
	if probe.UDPVerified {
		return "UDP 已验证，连接未验证"
	}
	if len(identified) > 0 {
		return "已识别：" + strings.Join(identified, "、") + "，连接未验证"
	}
	return "连接未验证"
}

func probeColor(t *nativeui.Theme, probe domain.ProbeResult) nativeui.Color {
	if probe.TCPVerified || probe.HTTPVerified || probe.SOCKS5Verified {
		return t.Success
	}
	return t.Warning
}

func installedIdentityDetail(identity domain.AppIdentity) string {
	detail := identity.Executable
	if detail == "" {
		detail = firstNonEmpty(identity.AUMID, identity.PackageFamily, identity.PackageName)
	}
	return detail
}

func installedInput(identity domain.AppIdentity) string {
	// macOS bundle identities carry their stable package identity in AUMID,
	// while the resolver expects a concrete executable path so it can walk up
	// to the .app bundle. Do not turn this into package:bundle:...; that shape
	// is only meaningful for the Windows package resolver.
	if aumid := strings.TrimSpace(identity.AUMID); strings.HasPrefix(strings.ToLower(aumid), "bundle:") {
		if executable := strings.TrimSpace(identity.Executable); executable != "" {
			return executable
		}
	}
	// Store-package identities preserve the package launch path used by the
	// Windows resolver (especially Codex). Falling back to the executable is
	// appropriate for classic Win32 installations without package metadata.
	if packageID := firstNonEmpty(identity.AUMID, identity.PackageName, identity.PackageFamily); packageID != "" {
		return "package:" + packageID
	}
	return strings.TrimSpace(identity.Executable)
}

func detailLabel(open bool) string {
	if open {
		return "收起程序详情"
	}
	return "编辑程序详情"
}

func splitCSV(value string) []string {
	var out []string
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func contains(items []string, wanted string) bool {
	for _, item := range items {
		if item == wanted {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
