package application

import (
	"fmt"
	"net/url"
	"proxy-start/internal/domain"
	"strings"
)

// BuildProxyEnvironment creates proxy variables for a child shell. It is
// intentionally pure so platform launchers can be tested without opening a
// terminal. Empty policies clear inherited bypass settings in the child shell;
// non-empty policies replace them with the validated domain list.
func BuildProxyEnvironment(endpoint domain.ProxyEndpoint, protocol domain.Protocol, bypass domain.BypassPolicy) (map[string]string, []string, error) {
	if protocol != domain.ProtocolHTTP && protocol != domain.ProtocolSOCKS5 {
		return nil, nil, fmt.Errorf("控制台需要明确的 HTTP 或 SOCKS5 协议")
	}
	scheme := "http"
	if protocol == domain.ProtocolSOCKS5 {
		scheme = "socks5h"
	}
	proxyURL := url.URL{Scheme: scheme, Host: endpoint.Address()}
	if endpoint.Username != "" || endpoint.Password != "" {
		proxyURL.User = url.UserPassword(endpoint.Username, endpoint.Password)
	}
	value := proxyURL.String()
	environment := map[string]string{
		"HTTP_PROXY":  value,
		"HTTPS_PROXY": value,
		"ALL_PROXY":   value,
		"http_proxy":  value,
		"https_proxy": value,
		"all_proxy":   value,
	}
	if bypass.Empty() {
		return environment, []string{"NO_PROXY", "no_proxy"}, nil
	}
	// Keep the bypass value in both casing variants. The child launcher removes
	// inherited values only when the policy is empty, so these explicit values
	// survive on Windows and Unix shells alike.
	bypassValue := strings.Join(bypass.Domains(), ",")
	environment["NO_PROXY"] = bypassValue
	environment["no_proxy"] = bypassValue
	return environment, nil, nil
}
