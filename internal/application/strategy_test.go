package application

import (
	"errors"
	"proxy-start/internal/domain"
	"testing"
)

func TestStrategyRefusesForcedFallbackWithBypass(t *testing.T) {
	bypass, err := domain.NewBypassPolicy([]string{"direct.example"})
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		name     string
		identity domain.AppIdentity
		endpoint domain.ProxyEndpoint
	}{
		{"unknown", domain.AppIdentity{Name: "unknown"}, domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP}},
		{"authenticated native", domain.AppIdentity{Name: "known"}, domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP, Password: "fixture"}},
		{"SOCKS5 native", domain.AppIdentity{Name: "known"}, domain.ProxyEndpoint{Protocol: domain.ProtocolSOCKS5}},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			_, err := NewStrategySelector(testAdapter{}).Select(fixture.identity, fixture.endpoint, domain.ProbeResult{HTTPVerified: true, SOCKS5Verified: true}, bypass)
			if !errors.Is(err, ErrUnsupportedBypass) {
				t.Fatalf("unsupported bypass fell back: %v", err)
			}
		})
	}
	plan, err := NewStrategySelector(testAdapter{}).Select(domain.AppIdentity{Name: "known"}, domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP}, domain.ProbeResult{HTTPVerified: true}, bypass)
	if err != nil || plan.Mode != domain.ModeNative {
		t.Fatalf("supported bypass did not select native: %#v, %v", plan, err)
	}
}

type testAdapter struct{ auth bool }

func (testAdapter) ID() string                      { return "test-native" }
func (testAdapter) Match(i domain.AppIdentity) bool { return i.Name == "known" }
func (a testAdapter) Supports(p domain.ProxyEndpoint, _ domain.BypassPolicy) bool {
	return p.Protocol == domain.ProtocolHTTP && (a.auth || p.Password == "")
}
func (testAdapter) Plan(i domain.AppIdentity, p domain.ProxyEndpoint, _ domain.BypassPolicy) (domain.LaunchSpec, error) {
	return domain.LaunchSpec{Identity: i, Arguments: []string{"native"}}, nil
}

func TestStrategyNativeUsesVerifiedHTTPOnMixedPort(t *testing.T) {
	s := NewStrategySelector(testAdapter{})
	plan, err := s.Select(domain.AppIdentity{Name: "known"}, domain.ProxyEndpoint{Protocol: domain.ProtocolAuto}, domain.ProbeResult{HTTP: true, SOCKS5: true, HTTPVerified: true, SOCKS5Verified: true}, domain.BypassPolicy{})
	if err != nil || plan.Mode != domain.ModeNative || plan.Proxy.Protocol != domain.ProtocolHTTP {
		t.Fatalf("native selection: %#v, %v", plan, err)
	}
}

func TestStrategyFallbackPrefersSOCKSAndPreservesArguments(t *testing.T) {
	s := NewStrategySelector(testAdapter{})
	identity := domain.AppIdentity{Name: "unknown", Arguments: []string{"a b"}}
	plan, err := s.Select(identity, domain.ProxyEndpoint{Protocol: domain.ProtocolAuto}, domain.ProbeResult{HTTPVerified: true, SOCKS5Verified: true}, domain.BypassPolicy{})
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
	plan, err := s.Select(domain.AppIdentity{Name: "known"}, domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP, Password: "secret"}, domain.ProbeResult{HTTP: true, HTTPVerified: true}, domain.BypassPolicy{})
	if err != nil || plan.Mode != domain.ModeForced {
		t.Fatalf("auth fallback: %#v, %v", plan, err)
	}
	_, err = s.Select(domain.AppIdentity{Name: "known"}, domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP}, domain.ProbeResult{HTTP: true}, domain.BypassPolicy{})
	if err == nil {
		t.Fatal("recognized protocol without a verified CONNECT must not launch")
	}
}
