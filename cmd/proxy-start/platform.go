package main

import "proxy-start/internal/application"

type platformServices struct {
	resolver     application.CodexResolver
	processes    application.ProcessController
	console      application.ConsoleLauncher
	capabilities application.PlatformCapabilities
	target       string
}
