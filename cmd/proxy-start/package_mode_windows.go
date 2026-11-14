//go:build windows

package main

import platform "proxy-start/internal/platform/windows"

func runPackagedMode(args []string) (bool, error) {
	return platform.RunPackagedLaunch(args)
}
