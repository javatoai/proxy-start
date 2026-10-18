package main

import "proxy-start/internal/application"

type platformServices struct {
	resolver     application.AppResolver
	processes    application.ProcessController
	icons        application.AppIconProvider
	backend      application.ForcedBackend
	console      application.ConsoleLauncher
	capabilities application.PlatformCapabilities
	target       string
}
