//go:build windows

package main

import (
	"proxy-start/internal/application"
	"proxy-start/internal/platform/console"
	platform "proxy-start/internal/platform/windows"
	"runtime"
)

func newPlatformServices(string) (platformServices, error) {
	return platformServices{resolver: platform.NewResolver(), processes: platform.NewProcessController(), console: console.NewLauncher(),
		capabilities: application.WindowsCapabilities(), target: "windows-" + runtime.GOARCH}, nil
}
