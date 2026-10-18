package application

// PlatformCapabilities describes the services wired by the composition root.
// Views consume this snapshot without importing an operating-system package.
type PlatformCapabilities struct {
	Name                        string
	ForceProxySupported         bool
	ForceProxyInstallable       bool
	NativeAdapterIDs            []string
	ForceProxyUnavailableReason string
	AppExtensions               []string
	SystemAppsDescription       string
	DragDescription             string
	UIFontFamily                string
}

func WindowsCapabilities() PlatformCapabilities {
	return PlatformCapabilities{
		Name:                  "Windows",
		ForceProxySupported:   true,
		ForceProxyInstallable: true,
		NativeAdapterIDs:      []string{"codex"},
		AppExtensions:         []string{"exe", "lnk"},
		SystemAppsDescription: "隐藏微软软件及 Windows 系统组件",
		DragDescription:       "拖入 .exe 或快捷方式，也可添加已安装应用",
		UIFontFamily:          "Microsoft YaHei UI, Segoe UI",
	}
}

func MacOSCapabilities() PlatformCapabilities {
	return PlatformCapabilities{
		Name:                        "macOS",
		NativeAdapterIDs:            []string{"codex"},
		ForceProxyUnavailableReason: "macOS 当前仅支持 Codex 原生代理，暂不支持按程序强制代理",
		AppExtensions:               []string{"app"},
		SystemAppsDescription:       "隐藏 Apple、微软软件及系统组件",
		DragDescription:             "拖入 Codex.app，也可从已安装应用中添加",
		UIFontFamily:                "system-ui",
	}
}

func clonePlatform(p PlatformCapabilities) PlatformCapabilities {
	p.NativeAdapterIDs = append([]string(nil), p.NativeAdapterIDs...)
	p.AppExtensions = append([]string(nil), p.AppExtensions...)
	return p
}
