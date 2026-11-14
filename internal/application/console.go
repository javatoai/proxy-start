package application

import "proxy-start/internal/domain"

// Proxy values belong only to the new child, never to the parent environment.
func BuildProxyEnvironment(port int) (map[string]string, []string) {
	value := domain.ProxyURL(port)
	return map[string]string{
		"HTTP_PROXY": value, "HTTPS_PROXY": value, "ALL_PROXY": value,
		"http_proxy": value, "https_proxy": value, "all_proxy": value,
	}, []string{"NO_PROXY", "no_proxy"}
}
