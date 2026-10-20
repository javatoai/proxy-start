package native

import (
	"fmt"
	"strings"

	"proxy-start/internal/application"
	"proxy-start/internal/domain"
)

const claudeWindowsPackageFamily = "Claude_pzs8sxrjxfjjc"

// ClaudeAdapter supports the Windows Store version of Claude Desktop.
// The resolver supplies its stable package identity and current executable path.
type ClaudeAdapter struct{}

// NewClaude returns the native adapter for Windows Store Claude Desktop.
func NewClaude() application.NativeAdapter { return &ClaudeAdapter{} }

func (ClaudeAdapter) ID() string { return "claude" }

func (ClaudeAdapter) Match(identity domain.AppIdentity) bool {
	if executableName(identity.Executable) != "claude.exe" {
		return false
	}
	// A display name or executable basename alone is not package evidence.
	// Prefer the explicit family so a conflicting AUMID cannot override it.
	family := strings.TrimSpace(identity.PackageFamily)
	if family == "" {
		var applicationID string
		var found bool
		family, applicationID, found = strings.Cut(strings.TrimSpace(identity.AUMID), "!")
		if !found || strings.TrimSpace(applicationID) == "" {
			return false
		}
	}
	return strings.EqualFold(family, claudeWindowsPackageFamily)
}

func (ClaudeAdapter) Supports(proxy domain.ProxyEndpoint, _ domain.BypassPolicy) bool {
	return validEndpoint(proxy) && effectiveProtocol(proxy.Protocol) == domain.ProtocolHTTP && noCredentials(proxy)
}

func (a ClaudeAdapter) Plan(identity domain.AppIdentity, proxy domain.ProxyEndpoint, bypass domain.BypassPolicy) (domain.LaunchSpec, error) {
	if !a.Match(identity) {
		return domain.LaunchSpec{}, fmt.Errorf("native Claude adapter does not match %q", identity.Executable)
	}
	if !a.Supports(proxy, bypass) {
		return domain.LaunchSpec{}, fmt.Errorf("native Claude supports only unauthenticated HTTP proxies")
	}
	spec, err := planChromiumHTTP(identity, proxy, bypass)
	if err != nil {
		return domain.LaunchSpec{}, err
	}
	// Store Claude supports direct desktop launch, as used by its standalone
	// launcher. Package activation creates a fresh environment and discards
	// the bootstrap variables needed by the packaged PowerShell launch path.
	spec.DirectLaunch = true
	return spec, nil
}
