//go:build darwin

package main

import (
	"proxy-start/internal/application"
	"proxy-start/internal/platform/console"
	platform "proxy-start/internal/platform/darwin"
	"runtime"
)

func newPlatformServices(string) (platformServices, error) {
	return platformServices{resolver: platform.NewResolver(), processes: platform.NewProcessController(), console: console.NewLauncher(),
		capabilities: application.MacOSCapabilities(), target: "darwin-" + runtime.GOARCH}, nil
}
