package proxifyre

import (
	"strings"
	"testing"

	"proxy-start/internal/domain"
)

func TestMakeConfigEmitsOnlyExplicitRules(t *testing.T) {
	config, err := makeConfig([]domain.ForcedSession{{
		AppID:           "editor",
		ExecutablePaths: []string{`C:\Apps\Editor\editor.exe`},
		Proxy:           domain.ProxyEndpoint{Protocol: domain.ProtocolSOCKS5, Host: "127.0.0.1", Port: 1080},
	}}, []string{`C:\Proxy Start\proxy-start.exe`}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Proxies) != 1 || len(config.Proxies[0].AppNames) == 0 {
		t.Fatalf("expected one explicit rule, got %#v", config.Proxies)
	}
	for _, name := range config.Proxies[0].AppNames {
		if name == "" {
			t.Fatal("config emitted a catch-all app name")
		}
		if !strings.Contains(name, `\`) {
			t.Fatalf("config emitted a non-path app name: %q", name)
		}
	}
	if got := config.Proxies[0].SupportedProtocols; len(got) != 2 || got[0] != "TCP" || got[1] != "UDP" {
		t.Fatalf("unexpected protocol set: %#v", got)
	}
}

func TestHTTPConfigUsesBridgeAndDoesNotForwardCredentials(t *testing.T) {
	session := domain.ForcedSession{
		AppID:           "browser",
		ExecutablePaths: []string{`C:\Apps\Browser\browser.exe`},
		Proxy:           domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP, Host: "proxy.example", Port: 8080, Username: "u", Password: "p"},
	}
	bridges := map[string]string{endpointKey(session.Proxy): "127.0.0.1:40001"}
	config, err := makeConfig([]domain.ForcedSession{session}, nil, bridges)
	if err != nil {
		t.Fatal(err)
	}
	rule := config.Proxies[0]
	if rule.Socks5ProxyEndpoint != "127.0.0.1:40001" {
		t.Fatalf("unexpected bridge endpoint: %s", rule.Socks5ProxyEndpoint)
	}
	if rule.Username != "" || rule.Password != "" {
		t.Fatal("HTTP credentials must terminate at the local bridge")
	}
}

func TestConfigRejectsMissingHTTPBridge(t *testing.T) {
	_, err := makeConfig([]domain.ForcedSession{{
		AppID:           "browser",
		ExecutablePaths: []string{`C:\Apps\Browser\browser.exe`},
		Proxy:           domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP, Host: "proxy.example", Port: 8080},
	}}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "bridge") {
		t.Fatalf("expected missing bridge error, got %v", err)
	}
}

func TestValidateSessionRejectsCatchAllInputs(t *testing.T) {
	if err := validateSession(domain.ForcedSession{AppID: "bad", Proxy: domain.ProxyEndpoint{Protocol: domain.ProtocolSOCKS5, Host: "127.0.0.1", Port: 1080}}); err == nil {
		t.Fatal("expected empty executable list to be rejected")
	}
}

func TestValidateSessionsRejectsOverlappingPathWithDifferentProxy(t *testing.T) {
	path := `C:\Apps\Shared\shared.exe`
	sessions := []domain.ForcedSession{
		{AppID: "one", ExecutablePaths: []string{path}, Proxy: domain.ProxyEndpoint{Protocol: domain.ProtocolSOCKS5, Host: "127.0.0.1", Port: 1080}},
		{AppID: "two", ExecutablePaths: []string{path}, Proxy: domain.ProxyEndpoint{Protocol: domain.ProtocolSOCKS5, Host: "127.0.0.1", Port: 1081}},
	}
	if err := validateSessions(sessions); err == nil || !strings.Contains(err.Error(), "multiple proxy endpoints") {
		t.Fatalf("expected overlapping path rejection, got %v", err)
	}
}
