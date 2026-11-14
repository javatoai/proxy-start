package native

import (
	"errors"
	"proxy-start/internal/domain"
	"strings"
)

func IsCodex(identity domain.AppIdentity) bool {
	name := strings.ToLower(strings.TrimSpace(identity.PackageName))
	path := strings.ToLower(strings.ReplaceAll(identity.Executable, "\\", "/"))
	base := path[strings.LastIndex(path, "/")+1:]
	if name == "com.openai.codex" {
		return base == "codex" && strings.Contains(path, ".app/contents/macos/")
	}
	for _, value := range []string{identity.PackageName, identity.PackageFamily, identity.AUMID} {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "openai.codex" || strings.HasPrefix(value, "openai.codex_") || strings.HasPrefix(value, "openai.codex!") {
			return base == "codex.exe" || base == "chatgpt.exe" || base == "codex-code-mode-host.exe"
		}
	}
	return false
}

func PlanCodex(identity domain.AppIdentity, port int) (domain.LaunchSpec, error) {
	if !IsCodex(identity) {
		return domain.LaunchSpec{}, errors.New("只支持 Codex 桌面程序")
	}
	if err := domain.ValidatePort(port); err != nil {
		return domain.LaunchSpec{}, err
	}
	var args []string
	for i := 0; i < len(identity.Arguments); i++ {
		arg := identity.Arguments[i]
		flag, _, equal := strings.Cut(arg, "=")
		switch flag {
		case "--proxy-server", "--proxy-bypass-list", "--proxy-pac-url":
			if !equal && i+1 < len(identity.Arguments) && !strings.HasPrefix(identity.Arguments[i+1], "--") {
				i++
			}
		case "--no-proxy-server", "--proxy-auto-detect":
		default:
			args = append(args, arg)
		}
	}
	proxy := domain.ProxyURL(port)
	args = append(args, "--proxy-server="+proxy)
	identity.Arguments = append([]string(nil), identity.Arguments...)
	return domain.LaunchSpec{Identity: identity, Arguments: args,
		Environment:      map[string]string{"HTTP_PROXY": proxy, "HTTPS_PROXY": proxy, "ALL_PROXY": proxy, "http_proxy": proxy, "https_proxy": proxy, "all_proxy": proxy},
		UnsetEnvironment: []string{"NO_PROXY", "no_proxy"}}, nil
}
