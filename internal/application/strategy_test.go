package application

import (
	"proxy-start/internal/domain"
	"testing"
)

type testAdapter struct{ auth bool }

func (testAdapter) ID() string                      { return "test-native" }
func (testAdapter) Match(i domain.AppIdentity) bool { return i.Name == "known" }
func (a testAdapter) Supports(p domain.ProxyEndpoint) bool {
	return p.Protocol == domain.ProtocolHTTP && (a.auth || p.Password == "")
}
func (testAdapter) Plan(i domain.AppIdentity, p domain.ProxyEndpoint) (domain.LaunchSpec, error) {
	return domain.LaunchSpec{Identity: i, Arguments: []string{"native"}}, nil
}

func TestStrategyNativeUsesVerifiedHTTPOnMixedPort(t *testing.T) {
	s := NewStrategySelector(testAdapter{})
	plan, err := s.Select(domain.AppIdentity{Name: "known"}, domain.ProxyEndpoint{Protocol: domain.ProtocolAuto}, domain.ProbeResult{HTTP: true, SOCKS5: true, HTTPVerified: true, SOCKS5Verified: true})
	if err != nil || plan.Mode != domain.ModeNative || plan.Proxy.Protocol != domain.ProtocolHTTP {
		t.Fatalf("native selection: %#v, %v", plan, err)
	}
}

func TestStrategyFallbackPrefersSOCKSAndPreservesArguments(t *testing.T) {
	s := NewStrategySelector(testAdapter{})
	identity := domain.AppIdentity{Name: "unknown", Arguments: []string{"a b"}}
	plan, err := s.Select(identity, domain.ProxyEndpoint{Protocol: domain.ProtocolAuto}, domain.ProbeResult{HTTPVerified: true, SOCKS5Verified: true})
	if err != nil || plan.Mode != domain.ModeForced || plan.Proxy.Protocol != domain.ProtocolSOCKS5 {
		t.Fatalf("forced selection: %#v, %v", plan, err)
	}
	plan.Spec.Arguments[0] = "changed"
	if identity.Arguments[0] != "a b" {
		t.Fatal("selection mutated stored arguments")
	}
}

func TestStrategyAuthAndFailedCONNECTDoNotMasqueradeAsNative(t *testing.T) {
	s := NewStrategySelector(testAdapter{})
	plan, err := s.Select(domain.AppIdentity{Name: "known"}, domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP, Password: "secret"}, domain.ProbeResult{HTTP: true, HTTPVerified: true})
	if err != nil || plan.Mode != domain.ModeForced {
		t.Fatalf("auth fallback: %#v, %v", plan, err)
	}
	_, err = s.Select(domain.AppIdentity{Name: "known"}, domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP}, domain.ProbeResult{HTTP: true})
	if err == nil {
		t.Fatal("recognized protocol without a verified CONNECT must not launch")
	}
}
