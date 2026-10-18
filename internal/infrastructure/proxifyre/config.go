package proxifyre

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"sort"
	"strings"

	"proxy-start/internal/domain"
)

// Bridge is the intentionally small contract needed by the backend. The
// concrete HTTP bridge lives in its own infrastructure package so the
// interception backend remains independent of transport implementation.
type Bridge interface {
	Start(context.Context) (string, error)
	Close() error
}

// BridgeFactory is injected by the composition root. SOCKS5 profiles do not
// require a bridge; HTTP profiles must provide one before they can be routed
// through ProxiFyre's SOCKS5-only engine.
type BridgeFactory func(domain.ProxyEndpoint) Bridge

type proxyStartConfig struct {
	LogLevel  string      `json:"logLevel"`
	BypassLAN bool        `json:"bypassLan"`
	Proxies   []proxyRule `json:"proxies"`
	Excludes  []string    `json:"excludes"`
}

type proxyRule struct {
	AppNames                 []string `json:"appNames"`
	Socks5ProxyEndpoint      string   `json:"socks5ProxyEndpoint"`
	Username                 string   `json:"username,omitempty"`
	Password                 string   `json:"password,omitempty"`
	Socks5Transport          string   `json:"socks5Transport"`
	SupportedProtocols       []string `json:"supportedProtocols"`
	SupportedAddressFamilies []string `json:"supportedAddressFamilies"`
}

type helperSession struct {
	AppID           string    `json:"appId"`
	ExecutablePaths []string  `json:"executablePaths"`
	Upstream        proxyRule `json:"upstream"`
}

type helperApplyRequest struct {
	Sessions     []helperSession  `json:"sessions"`
	Config       proxyStartConfig `json:"config"`
	ConfigSHA256 string           `json:"configSha256"`
}

func makeConfig(sessions []domain.ForcedSession, exclusions []string, bridges map[string]string) (proxyStartConfig, error) {
	if len(sessions) == 0 {
		return proxyStartConfig{LogLevel: "Error", BypassLAN: false, Proxies: []proxyRule{}, Excludes: cleanExclusions(exclusions)}, nil
	}
	rules := make([]proxyRule, 0, len(sessions))
	for _, session := range sessions {
		if err := validateSession(session); err != nil {
			return proxyStartConfig{}, err
		}
		endpoint, err := endpointFor(session.Proxy, bridges)
		if err != nil {
			return proxyStartConfig{}, err
		}
		names := explicitNames(session.ExecutablePaths)
		// ProxiFyre's rule matcher accepts executable names and path fragments.
		// We deliberately emit only explicit values; the catch-all name "" would
		// route applications the user never selected.
		username, password := session.Proxy.Username, session.Proxy.Password
		if session.Proxy.Protocol == domain.ProtocolHTTP {
			// The HTTP bridge terminates credentials. ProxiFyre connects to
			// that loopback SOCKS5 listener without reusing upstream HTTP auth.
			username, password = "", ""
		}
		rules = append(rules, proxyRule{
			AppNames:            names,
			Socks5ProxyEndpoint: endpoint,
			Username:            username,
			Password:            password,
			Socks5Transport:     "TCP",
			// ProxiFyre's engine needs both protocol entries for a complete
			// per-app rule. An HTTP upstream bridge intentionally rejects UDP;
			// direct SOCKS5 can relay both protocols.
			SupportedProtocols:       []string{"TCP", "UDP"},
			SupportedAddressFamilies: []string{"IPv4", "IPv6"},
		})
	}
	return proxyStartConfig{LogLevel: "Info", BypassLAN: false, Proxies: rules, Excludes: cleanExclusions(exclusions)}, nil
}

func makeHelperSessions(sessions []domain.ForcedSession, bridges map[string]string) ([]helperSession, error) {
	result := make([]helperSession, 0, len(sessions))
	for _, session := range sessions {
		if err := validateSession(session); err != nil {
			return nil, err
		}
		endpoint, err := endpointFor(session.Proxy, bridges)
		if err != nil {
			return nil, err
		}
		username, password := session.Proxy.Username, session.Proxy.Password
		if session.Proxy.Protocol == domain.ProtocolHTTP {
			username, password = "", ""
		}
		result = append(result, helperSession{
			AppID:           session.AppID,
			ExecutablePaths: explicitPaths(session.ExecutablePaths),
			Upstream: proxyRule{
				AppNames:                 explicitNames(session.ExecutablePaths),
				Socks5ProxyEndpoint:      endpoint,
				Username:                 username,
				Password:                 password,
				Socks5Transport:          "TCP",
				SupportedProtocols:       []string{"TCP", "UDP"},
				SupportedAddressFamilies: []string{"IPv4", "IPv6"},
			},
		})
	}
	return result, nil
}

func validateSession(session domain.ForcedSession) error {
	if strings.TrimSpace(session.AppID) == "" {
		return fmt.Errorf("%w: app id is empty", ErrInvalidSession)
	}
	if len(explicitPaths(session.ExecutablePaths)) == 0 {
		return fmt.Errorf("%w: %s has no executable path", ErrInvalidSession, session.AppID)
	}
	if session.Proxy.Protocol != domain.ProtocolSOCKS5 && session.Proxy.Protocol != domain.ProtocolHTTP {
		return fmt.Errorf("%w: %s requires a concrete HTTP or SOCKS5 proxy", ErrInvalidSession, session.AppID)
	}
	if strings.TrimSpace(session.Proxy.Host) == "" || session.Proxy.Port < 1 || session.Proxy.Port > 65535 {
		return fmt.Errorf("%w: %s has an invalid proxy endpoint", ErrInvalidSession, session.AppID)
	}
	return nil
}

func endpointFor(endpoint domain.ProxyEndpoint, bridges map[string]string) (string, error) {
	if endpoint.Protocol == domain.ProtocolSOCKS5 {
		return endpoint.Address(), nil
	}
	key := endpointKey(endpoint)
	if address := strings.TrimSpace(bridges[key]); address != "" {
		return address, nil
	}
	return "", fmt.Errorf("%w: HTTP profile %s has no local SOCKS5 bridge", ErrInvalidSession, endpoint.Address())
}

func endpointKey(endpoint domain.ProxyEndpoint) string {
	return fmt.Sprintf("%s|%s|%d|%s|%s", endpoint.Protocol, strings.ToLower(endpoint.Host), endpoint.Port, endpoint.Username, endpoint.Password)
}

func explicitPaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	result := make([]string, 0, len(paths))
	for _, value := range paths {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		value = filepath.Clean(value)
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func explicitNames(paths []string) []string {
	// Full paths are intentionally used exclusively. Bare executable names can
	// match an unrelated application in another directory; an empty name would
	// become ProxiFyre's global catch-all rule.
	return explicitPaths(paths)
}

func cleanExclusions(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(filepath.Clean(value))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, filepath.Clean(value))
	}
	sort.Strings(result)
	return result
}

func marshalConfig(config proxyStartConfig) ([]byte, string, error) {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, "", err
	}
	data = append(data, '\n')
	digest := sha256.Sum256(data)
	return data, hex.EncodeToString(digest[:]), nil
}

func validProxyEndpoint(endpoint string) bool {
	host, port, err := net.SplitHostPort(endpoint)
	return err == nil && strings.TrimSpace(host) != "" && port != ""
}
