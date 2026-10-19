package domain

import (
	"fmt"
	"net"
	"strings"
)

// BypassPolicy is a validated, immutable set of DNS names. Each name denotes
// the host itself and its subdomains. Transport-specific syntax belongs to
// adapters, rather than to this value object. Its zero value is an empty policy.
type BypassPolicy struct {
	domains []string
}

// ParseBypassPolicy parses the text accepted by the proxy editor.
func ParseBypassPolicy(text string) (BypassPolicy, error) {
	return NewBypassPolicy(strings.FieldsFunc(text, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r'
	}))
}

func NewBypassPolicy(domains []string) (BypassPolicy, error) {
	var canonical []string
	seen := make(map[string]struct{}, len(domains))
	for _, value := range domains {
		name := strings.ToLower(strings.TrimSpace(value))
		if name == "" {
			continue
		}
		// An absolute DNS name and its spelling without the final root dot
		// describe the same host. Remove only one dot so malformed names fail.
		name = strings.TrimSuffix(name, ".")
		if !validBypassName(name) {
			return BypassPolicy{}, fmt.Errorf("绕过域名 %q 无效：请填写域名或主机名，不含 URL、端口、通配符或 IP/CIDR", value)
		}
		if _, exists := seen[name]; !exists {
			seen[name] = struct{}{}
			canonical = append(canonical, name)
		}
	}
	return BypassPolicy{domains: canonical}, nil
}

// Domains returns a copy so a configuration snapshot or adapter cannot alter
// the policy used by another launch.
func (p BypassPolicy) Domains() []string { return append([]string(nil), p.domains...) }

func (p BypassPolicy) Empty() bool { return len(p.domains) == 0 }

func validBypassName(name string) bool {
	if name == "" || len(name) > 253 || net.ParseIP(name) != nil {
		return false
	}
	allNumeric := true
	for _, label := range strings.Split(name, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, ch := range label {
			if !(ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '-') {
				return false
			}
			if ch < '0' || ch > '9' {
				allNumeric = false
			}
		}
	}
	// Numeric-only names are interpreted as IP literals by some URL stacks,
	// including shortened IPv4 forms. Do not accept them as DNS bypass rules.
	return !allNumeric
}
