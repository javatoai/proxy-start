// Package native contains pure launch adapters for applications that expose a
// proxy setting of their own.  The package deliberately does not inspect the
// machine or start processes: platform process controllers consume the launch
// specifications produced here.
package native

import (
	"fmt"
	"strings"

	"proxy-start/internal/application"
	"proxy-start/internal/domain"
)

// CodexAdapter launches the packaged OpenAI Codex desktop application with
// the proxy settings understood by its Chromium/Node runtime.
//
// The adapter is intentionally stateless.  In particular, package discovery
// and process activation stay in the Windows resolver and ProcessController.
type CodexAdapter struct{}

// NewCodex returns the native adapter for the packaged OpenAI Codex desktop
// application.
func NewCodex() application.NativeAdapter { return &CodexAdapter{} }

func (CodexAdapter) ID() string { return "codex" }

func (CodexAdapter) Match(identity domain.AppIdentity) bool {
	return hasCodexPackage(identity)
}

// Supports accepts an automatically detected profile as HTTP. Clash's mixed
// port is exposed to applications as an HTTP CONNECT endpoint by the native
// launcher; SOCKS5 (and all authenticated profiles) remain unsupported until
// the packaged child-process stack is verified. Bypass rules are supported by
// Chromium's command-line switch and the matching child environment.
func (a CodexAdapter) Supports(proxy domain.ProxyEndpoint, _ domain.BypassPolicy) bool {
	return validEndpoint(proxy) && effectiveProtocol(proxy.Protocol) == domain.ProtocolHTTP && noCredentials(proxy)
}

func (a CodexAdapter) Plan(identity domain.AppIdentity, proxy domain.ProxyEndpoint, bypass domain.BypassPolicy) (domain.LaunchSpec, error) {
	if !a.Match(identity) {
		return domain.LaunchSpec{}, fmt.Errorf("native Codex adapter does not match %q", identity.Executable)
	}
	if !a.Supports(proxy, bypass) {
		return domain.LaunchSpec{}, fmt.Errorf("native Codex supports only unauthenticated HTTP proxies")
	}

	// A packaged Codex launch is activated by the platform ProcessController.
	// We only provide the bootstrap's effective command-line and environment;
	// this package never invokes PowerShell or an AppX API.
	identityCopy := cloneIdentity(identity)
	args := withProxyServer(identity.Arguments, proxyURL(proxy))
	args = withProxyBypassList(args, bypass)
	environment, unsetEnvironment, err := application.BuildProxyEnvironment(proxy, effectiveProtocol(proxy.Protocol), bypass)
	if err != nil {
		return domain.LaunchSpec{}, err
	}
	return domain.LaunchSpec{
		Identity:         identityCopy,
		Arguments:        args,
		Environment:      environment,
		UnsetEnvironment: unsetEnvironment,
	}, nil
}

func hasCodexPackage(identity domain.AppIdentity) bool {
	if isCodexWindowsPackageValue(identity.PackageName) ||
		isCodexWindowsPackageValue(identity.PackageFamily) ||
		isCodexWindowsPackageValue(identity.AUMID) {
		return isKnownWindowsCodexExecutable(identity.Executable)
	}
	// macOS does not expose AppX family/AUMID fields. Its resolver supplies
	// the signed bundle identifier in PackageName instead. Requiring both the
	// exact bundle id and an app-bundle executable path prevents a display name
	// or a basename from impersonating Codex.
	return strings.EqualFold(strings.TrimSpace(identity.PackageName), macCodexBundleID) && isKnownMacCodexExecutable(identity.Executable)
}

// macCodexBundleID is the signed bundle identifier used by Codex.app. The
// value is also declared by OpenAI's official macOS launcher:
// https://github.com/openai/codex/blob/main/codex-rs/cli/src/desktop_app/mac.rs
const macCodexBundleID = "com.openai.codex"

func isCodexWindowsPackageValue(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return false
	}
	// PackageName may be either the short name (OpenAI.Codex) or a full
	// package name (OpenAI.Codex_<version>_<arch>...).  AUMID includes the
	// family followed by !ApplicationId, so the same prefix is safe there.
	return value == "openai.codex" || strings.HasPrefix(value, "openai.codex_") || strings.HasPrefix(value, "openai.codex!")
}

func isKnownWindowsCodexExecutable(path string) bool {
	switch executableName(path) {
	case "chatgpt.exe", "codex.exe", "codex-code-mode-host.exe":
		return true
	default:
		return false
	}
}

func isKnownMacCodexExecutable(path string) bool {
	base := executableName(path)
	// OpenAI's official launcher verifies both Codex.app and the unified
	// ChatGPT.app by bundle identifier, while its signed executable is named
	// Codex. Keep the executable check strict so a generic ChatGPT.app cannot
	// be treated as Codex based on its folder name alone.
	if base != "codex" {
		return false
	}
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(path), `\`, "/"))
	return strings.Contains(normalized, ".app/contents/macos/")
}

func executableName(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	// Use both separators so pure adapter tests and imported Windows paths
	// behave consistently even when the tests run on another host OS.
	if index := strings.LastIndexAny(path, `/\`); index >= 0 {
		path = path[index+1:]
	}
	return strings.ToLower(path)
}

func validEndpoint(proxy domain.ProxyEndpoint) bool {
	if strings.TrimSpace(proxy.Host) == "" || strings.Trim(strings.TrimSpace(proxy.Host), "[]") == "" || strings.ContainsAny(proxy.Host, "/\\\r\n\t ") {
		return false
	}
	return proxy.Port >= 1 && proxy.Port <= 65535
}

func noCredentials(proxy domain.ProxyEndpoint) bool {
	return strings.TrimSpace(proxy.Username) == "" && strings.TrimSpace(proxy.Password) == ""
}

func effectiveProtocol(protocol domain.Protocol) domain.Protocol {
	if protocol == domain.ProtocolAuto {
		return domain.ProtocolHTTP
	}
	return protocol
}

func proxyURL(proxy domain.ProxyEndpoint) string {
	return string(effectiveProtocol(proxy.Protocol)) + "://" + proxy.Address()
}

func cloneIdentity(identity domain.AppIdentity) domain.AppIdentity {
	identity.Arguments = append([]string(nil), identity.Arguments...)
	return identity
}

// withProxyServer removes switches that can override the adapter's selected
// proxy, then appends one canonical switch.  It accepts both --flag=value and
// --flag value forms and leaves every unrelated argument byte-for-byte intact.
func withProxyServer(arguments []string, proxy string) []string {
	clean := make([]string, 0, len(arguments)+1)
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		flag := strings.ToLower(strings.TrimSpace(argument))

		switch {
		case isProxyFlagWithValue(flag):
			if !strings.Contains(argument, "=") && index+1 < len(arguments) && !looksLikeFlag(arguments[index+1]) {
				index++
			}
			continue
		case isProxyFlagWithoutValue(flag):
			continue
		default:
			clean = append(clean, argument)
		}
	}
	return append(clean, "--proxy-server="+proxy)
}

// withProxyBypassList appends one canonical Chromium bypass switch to an
// argument list that has already been cleaned by withProxyServer. Chromium
// uses a semicolon separated list; each domain is expanded with a wildcard so
// the root and all of its subdomains bypass the selected proxy.
func withProxyBypassList(arguments []string, bypass domain.BypassPolicy) []string {
	if bypass.Empty() {
		return arguments
	}
	parts := make([]string, 0, len(bypass.Domains())*2)
	for _, domainName := range bypass.Domains() {
		parts = append(parts, domainName, "*."+domainName)
	}
	return append(arguments, "--proxy-bypass-list="+strings.Join(parts, ";"))
}

func isProxyFlagWithValue(flag string) bool {
	for _, name := range []string{"--proxy-server", "--proxy-pac-url", "--proxy-bypass-list"} {
		if flag == name || strings.HasPrefix(flag, name+"=") {
			return true
		}
	}
	return false
}

func isProxyFlagWithoutValue(flag string) bool {
	for _, name := range []string{"--no-proxy-server", "--proxy-auto-detect"} {
		if flag == name || strings.HasPrefix(flag, name+"=") {
			return true
		}
	}
	return false
}

func looksLikeFlag(value string) bool {
	return strings.HasPrefix(strings.TrimSpace(value), "--")
}
