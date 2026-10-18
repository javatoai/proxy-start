//go:build windows

package main

import (
	"path/filepath"
	"runtime"

	"proxy-start/internal/application"
	"proxy-start/internal/domain"
	"proxy-start/internal/infrastructure/httpbridge"
	"proxy-start/internal/infrastructure/proxifyre"
	"proxy-start/internal/platform/console"
	platform "proxy-start/internal/platform/windows"
)

func newPlatformServices(executable string) (platformServices, error) {
	directory := filepath.Dir(executable)
	backend := proxifyre.NewBackend(proxifyre.Options{
		HelperPath:      filepath.Join(directory, "proxy-start-helper.exe"),
		HelperSHA256:    helperSHA256,
		OwnerExecutable: executable,
		InstallerPath:   filepath.Join(directory, "dependencies", "ProxiFyre-2.6.4-win-x64-setup.exe"),
		BridgeFactory:   func(endpoint domain.ProxyEndpoint) proxifyre.Bridge { return httpbridge.New(endpoint) },
	})
	return platformServices{
		resolver:     platform.NewResolver(),
		processes:    platform.NewProcessController(),
		icons:        platform.NewIconProvider(),
		backend:      backend,
		console:      console.NewLauncher(),
		capabilities: application.WindowsCapabilities(),
		target:       "windows-" + runtime.GOARCH,
	}, nil
}
