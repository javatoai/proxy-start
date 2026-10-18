// Package application defines use cases and the small interfaces they consume.
package application

import (
	"context"
	"proxy-start/internal/domain"
)

type SettingsStore interface {
	Load(context.Context) (domain.Settings, error)
	Save(context.Context, domain.Settings) error
}

type CredentialStore interface {
	Read(context.Context, string) (string, error)
	Write(context.Context, string, string) error
	Delete(context.Context, string) error
}

type AppResolver interface {
	Resolve(context.Context, string) (domain.AppIdentity, error)
	Installed(context.Context) ([]domain.AppIdentity, error)
}

// AppIconProvider returns encoded PNG bytes without exposing native shell APIs
// or filesystem reads to the UI.
type AppIconProvider interface {
	Icon(context.Context, domain.AppIdentity) ([]byte, error)
}

type ProcessController interface {
	Find(context.Context, domain.AppIdentity) ([]domain.ProcessHandle, error)
	Start(context.Context, domain.LaunchSpec) (domain.ProcessHandle, error)
	Stop(context.Context, domain.AppIdentity, []domain.ProcessHandle) error
	Alive(context.Context, domain.ProcessHandle) bool
}

type ProxyProbe interface {
	Test(context.Context, domain.ProxyEndpoint) (domain.ProbeResult, error)
}

// ConsoleSpec describes the environment to inject into a newly opened shell.
// Environment values are runtime-only; callers must never persist or log them.
type ConsoleSpec struct {
	ProfileName      string
	Environment      map[string]string
	UnsetEnvironment []string
}

// ConsoleLauncher opens an independent interactive shell with the supplied
// environment. The launcher owns only the new shell process; it must not
// modify the parent process environment or terminate existing shells.
type ConsoleLauncher interface {
	Open(context.Context, ConsoleSpec) error
}

type NativeAdapter interface {
	ID() string
	Match(domain.AppIdentity) bool
	Supports(domain.ProxyEndpoint) bool
	Plan(domain.AppIdentity, domain.ProxyEndpoint) (domain.LaunchSpec, error)
}

type ForcedBackend interface {
	Apply(context.Context, []domain.ForcedSession) error
	Shutdown(context.Context) error
	Status(context.Context) domain.BackendStatus
	Install(context.Context) error
}
