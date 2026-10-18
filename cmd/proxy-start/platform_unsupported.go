//go:build !windows && !darwin

package main

import "errors"

func newPlatformServices(string) (platformServices, error) {
	return platformServices{}, errors.New("Proxy Start 当前仅提供 Windows 与 macOS 客户端")
}
