//go:build darwin

package main

import (
	"runtime"

	"proxy-start/internal/application"
	"proxy-start/internal/infrastructure/unsupportedproxy"
	"proxy-start/internal/platform/console"
	platform "proxy-start/internal/platform/darwin"
)

func newPlatformServices(string) (platformServices, error) {
	capabilities := application.MacOSCapabilities()
	return platformServices{
		resolver:     platform.NewResolver(),
		processes:    platform.NewProcessController(),
		icons:        platform.NewIconProvider(),
		backend:      unsupportedproxy.New(capabilities.ForceProxyUnavailableReason),
		console:      console.NewLauncher(),
		capabilities: capabilities,
		target:       "darwin-" + runtime.GOARCH,
	}, nil
}
