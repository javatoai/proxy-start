package application

import (
	"net/url"
	"proxy-start/internal/domain"
	"strings"
)

func IsChromiumChild(args []string) bool {
	for _, arg := range args {
		if arg == "--type" || strings.HasPrefix(arg, "--type=") {
			return true
		}
	}
	return false
}

// ObserveProxy reports launch evidence, never a claim about actual routing.
func ObserveProxy(info domain.ProcessInfo) domain.ProxyObservation {
	if !info.Readable {
		return domain.ProxyObservation{Kind: "unknown"}
	}
	result := domain.ProxyObservation{Kind: "unspecified"}
	signals := 0
	for index, arg := range info.Arguments {
		flag, value, hasValue := strings.Cut(arg, "=")
		if !hasValue && index+1 < len(info.Arguments) {
			value = info.Arguments[index+1]
		}
		switch flag {
		case "--proxy-server":
			signals++
			if value == "" || strings.HasPrefix(value, "--") {
				return domain.ProxyObservation{Kind: "unknown"}
			}
			u, err := url.Parse(value)
			if err == nil && u.Host != "" && (u.Scheme == "http" || u.Scheme == "https" || u.Scheme == "socks5") {
				result = domain.ProxyObservation{Kind: "explicit", Address: u.Scheme + "://" + u.Host}
			} else {
				result = domain.ProxyObservation{Kind: "explicit", Address: "已设置自定义代理"}
			}
		case "--no-proxy-server":
			signals++
			result = domain.ProxyObservation{Kind: "disabled"}
		case "--proxy-pac-url", "--proxy-auto-detect":
			signals++
			result = domain.ProxyObservation{Kind: "automatic"}
		}
	}
	if signals > 1 {
		return domain.ProxyObservation{Kind: "unknown", Detail: "检测到多项代理设置"}
	}
	return result
}

func CombineProxyObservations(observations []domain.ProxyObservation) domain.ProxyObservation {
	if len(observations) == 0 {
		return domain.ProxyObservation{Kind: "unknown"}
	}
	result := observations[0]
	for _, observation := range observations[1:] {
		if observation.Kind != result.Kind || observation.Address != result.Address {
			return domain.ProxyObservation{Kind: "unknown", Detail: "多个实例的代理信息不一致"}
		}
	}
	return result
}
