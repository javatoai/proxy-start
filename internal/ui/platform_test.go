package ui

import (
	"testing"

	nativeui "github.com/egoist/mygo/ui"
	"proxy-start/internal/application"
	"proxy-start/internal/domain"
)

func TestPlatformCapabilitiesDefaultToWindowsForLegacyFixtures(t *testing.T) {
	m := newModel(nil)
	got := m.platformCapabilities()
	if got.Name != "Windows" || got.UIFontFamily != "Microsoft YaHei UI, Segoe UI" {
		t.Fatalf("platformCapabilities() = %+v, want Windows defaults", got)
	}
	if got := m.uiFontFamily(); got != "Microsoft YaHei UI, Segoe UI" {
		t.Fatalf("uiFontFamily() = %q, want Windows default", got)
	}
}

func TestMacOSPlatformPresentationAndFont(t *testing.T) {
	m := newModel(nil)
	m.snapshot.Platform = application.MacOSCapabilities()
	if got := m.uiFontFamily(); got != "system-ui" {
		t.Fatalf("uiFontFamily() = %q, want system-ui", got)
	}
	label, summary, _ := backendPresentationForPlatform(domain.BackendStatus{Installed: true, Ready: true}, m.platformCapabilities(), nativeui.LightTheme())
	if label != "当前平台不可用" {
		t.Fatalf("backend label = %q, want unavailable label", label)
	}
	if summary != application.MacOSCapabilities().ForceProxyUnavailableReason {
		t.Fatalf("backend summary = %q, want macOS reason", summary)
	}
	if got := nativeAdapterSummary(m.platformCapabilities()); got != "原生适配 Codex" {
		t.Fatalf("native adapter summary = %q, want Codex summary", got)
	}
}

func TestProgramDialogOptionsFollowPlatformAppExtensions(t *testing.T) {
	tests := []struct {
		name       string
		platform   application.PlatformCapabilities
		directory  bool
		filterName string
		extensions []string
		message    string
	}{
		{
			name:       "windows",
			platform:   application.WindowsCapabilities(),
			directory:  false,
			filterName: "程序和快捷方式",
			extensions: []string{"exe", "lnk"},
			message:    "拖入 .exe 或快捷方式，也可添加已安装应用",
		},
		{
			name:       "macOS bundle",
			platform:   application.MacOSCapabilities(),
			directory:  false,
			filterName: "应用程序",
			extensions: []string{"app"},
			message:    "拖入 Codex.app，也可从已安装应用中添加",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newModel(nil)
			m.snapshot.Platform = tt.platform
			options := m.programDialogOptions()
			if options.Directory != tt.directory || options.Message != tt.message {
				t.Fatalf("options = %+v, want directory=%v message=%q", options, tt.directory, tt.message)
			}
			if len(options.Filters) != 2 || options.Filters[0].Name != tt.filterName {
				t.Fatalf("filters = %+v, want primary filter %q", options.Filters, tt.filterName)
			}
			if len(options.Filters[0].Extensions) != len(tt.extensions) {
				t.Fatalf("primary extensions = %v, want %v", options.Filters[0].Extensions, tt.extensions)
			}
			for i, want := range tt.extensions {
				if options.Filters[0].Extensions[i] != want {
					t.Fatalf("primary extensions = %v, want %v", options.Filters[0].Extensions, tt.extensions)
				}
			}
			if tt.name == "macOS bundle" && options.TreatPackagesAsDirectories {
				t.Fatal("macOS .app dialog must select the package instead of browsing into it")
			}
		})
	}
}
