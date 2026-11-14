// Package proxystart exposes immutable product metadata shared by the entry
// point, UI and release scripts.
package proxystart

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var productVersion string

//go:embed THIRD-PARTY-NOTICES.txt
var thirdPartyNotices string

// ThirdPartyNotices returns license notices bundled into the executable.
func ThirdPartyNotices() string { return thirdPartyNotices }

// Version returns the version embedded from the release's single source.
func Version() string { return strings.TrimSpace(productVersion) }
