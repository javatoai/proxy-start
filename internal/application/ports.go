package application

import (
	"context"
	"proxy-start/internal/domain"
)

type SettingsStore interface {
	Load(context.Context) (domain.Settings, error)
	Save(context.Context, domain.Settings) error
}
type CodexResolver interface {
	Discover(context.Context) (domain.AppIdentity, error)
}
type ProcessController interface {
	Find(context.Context, domain.AppIdentity) ([]domain.ProcessHandle, error)
	Start(context.Context, domain.LaunchSpec) (domain.ProcessHandle, error)
	Inspect(context.Context, domain.ProcessHandle) (domain.ProcessInfo, error)
	Stop(context.Context, domain.AppIdentity, []domain.ProcessHandle) error
}
type ConsoleSpec struct {
	Environment      map[string]string
	UnsetEnvironment []string
}
type ConsoleLauncher interface {
	Open(context.Context, ConsoleSpec) error
}
type PlatformCapabilities struct{ Name, UIFontFamily, ConsoleName string }

func WindowsCapabilities() PlatformCapabilities {
	return PlatformCapabilities{Name: "Windows", UIFontFamily: "Microsoft YaHei UI, Segoe UI", ConsoleName: "PowerShell"}
}
func MacOSCapabilities() PlatformCapabilities {
	return PlatformCapabilities{Name: "macOS", UIFontFamily: "system-ui", ConsoleName: "Terminal"}
}
