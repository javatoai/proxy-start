package application

import (
	"fmt"
	"proxy-start/internal/domain"
)

// StrategySelector is immutable: application-specific behavior is injected,
// rather than scattered across a launch switch statement.
type StrategySelector struct{ adapters []NativeAdapter }

func NewStrategySelector(adapters ...NativeAdapter) *StrategySelector {
	return &StrategySelector{adapters: append([]NativeAdapter(nil), adapters...)}
}

func (s *StrategySelector) Select(identity domain.AppIdentity, endpoint domain.ProxyEndpoint, cap domain.ProbeResult) (domain.LaunchPlan, error) {
	candidates := make([]domain.ProxyEndpoint, 0, 2)
	if cap.HTTPVerified && endpoint.Protocol != domain.ProtocolSOCKS5 {
		p := endpoint
		p.Protocol = domain.ProtocolHTTP
		candidates = append(candidates, p)
	}
	if cap.SOCKS5Verified && endpoint.Protocol != domain.ProtocolHTTP {
		p := endpoint
		p.Protocol = domain.ProtocolSOCKS5
		candidates = append(candidates, p)
	}
	if len(candidates) == 0 {
		return domain.LaunchPlan{}, fmt.Errorf("代理没有通过协议检测，请检查地址、端口和认证信息")
	}
	reason := "未命中原生适配器，使用按程序强制代理"
	for _, adapter := range s.adapters {
		if !adapter.Match(identity) {
			continue
		}
		reason = "原生适配器不支持当前代理配置，改用强制代理"
		for _, candidate := range candidates {
			if adapter.Supports(candidate) {
				spec, err := adapter.Plan(identity, candidate)
				if err != nil {
					return domain.LaunchPlan{}, err
				}
				return domain.LaunchPlan{Mode: domain.ModeNative, AdapterID: adapter.ID(), Reason: "已应用原生代理启动设置；实际连接尚未验证", Spec: spec, Proxy: candidate}, nil
			}
		}
	}
	// Prefer SOCKS5 for interception when a mixed port offers both protocols.
	proxy := candidates[len(candidates)-1]
	return domain.LaunchPlan{Mode: domain.ModeForced, Reason: reason, Proxy: proxy, Spec: domain.LaunchSpec{Identity: identity, Arguments: append([]string(nil), identity.Arguments...)}}, nil
}
