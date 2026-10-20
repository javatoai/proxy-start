package native

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"proxy-start/internal/application"
	"proxy-start/internal/domain"
)

func claudeIdentity() domain.AppIdentity {
	return domain.AppIdentity{
		Name:             "Claude",
		Executable:       `C:\Program Files\WindowsApps\Claude_1.40609.0.0_x64__pzs8sxrjxfjjc\app\Claude.exe`,
		WorkingDirectory: `C:\Program Files\WindowsApps\Claude_1.40609.0.0_x64__pzs8sxrjxfjjc\app`,
		PackageName:      "Claude",
		PackageFamily:    claudeWindowsPackageFamily,
		ApplicationID:    "Claude",
		AUMID:            claudeWindowsPackageFamily + "!Claude",
	}
}

func TestClaudeMatchRequiresStoreFamilyAndExecutable(t *testing.T) {
	for _, test := range []struct {
		name     string
		identity domain.AppIdentity
		want     bool
	}{
		{"installed Store app", claudeIdentity(), true},
		{"updated version", domain.AppIdentity{Executable: `C:\Program Files\WindowsApps\Claude_2.0.0.0_x64__pzs8sxrjxfjjc\app\claude.exe`, PackageFamily: claudeWindowsPackageFamily}, true},
		{"AUMID identity", domain.AppIdentity{Executable: `C:\Apps\Claude.exe`, AUMID: claudeWindowsPackageFamily + "!Claude"}, true},
		{"case and whitespace", domain.AppIdentity{Executable: `C:\Apps\CLAUDE.EXE`, PackageFamily: " CLAUDE_PZS8SXRJXFJJC "}, true},
		{"display name only", domain.AppIdentity{Name: "Claude", Executable: `C:\Apps\claude.exe`}, false},
		{"package name only", domain.AppIdentity{PackageName: "Claude", Executable: `C:\Apps\claude.exe`}, false},
		{"wrong publisher", domain.AppIdentity{Executable: `C:\Apps\claude.exe`, PackageFamily: "Claude_otherpublisher"}, false},
		{"near miss family", domain.AppIdentity{Executable: `C:\Apps\claude.exe`, PackageFamily: "ClaudePreview_pzs8sxrjxfjjc"}, false},
		{"unrelated executable", domain.AppIdentity{Executable: `C:\Apps\helper.exe`, PackageFamily: claudeWindowsPackageFamily}, false},
		{"missing AUMID app", domain.AppIdentity{Executable: `C:\Apps\claude.exe`, AUMID: claudeWindowsPackageFamily + "!"}, false},
		{"family is not an AUMID", domain.AppIdentity{Executable: `C:\Apps\claude.exe`, AUMID: claudeWindowsPackageFamily}, false},
		{"conflicting family", domain.AppIdentity{Executable: `C:\Apps\claude.exe`, PackageFamily: "Other_pzs8sxrjxfjjc", AUMID: claudeWindowsPackageFamily + "!Claude"}, false},
		{"macOS bundle", domain.AppIdentity{Executable: "/Applications/Claude.app/Contents/MacOS/Claude", PackageName: "com.anthropic.claudefordesktop"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := NewClaude().Match(test.identity); got != test.want {
				t.Fatalf("Match() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestClaudeSupportsOnlyValidUnauthenticatedHTTP(t *testing.T) {
	policy, err := domain.NewBypassPolicy([]string{"example.com"})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name     string
		endpoint domain.ProxyEndpoint
		want     bool
	}{
		{"HTTP", domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP, Host: "127.0.0.1", Port: 7897}, true},
		{"auto", domain.ProxyEndpoint{Protocol: domain.ProtocolAuto, Host: "127.0.0.1", Port: 7897}, true},
		{"SOCKS5", domain.ProxyEndpoint{Protocol: domain.ProtocolSOCKS5, Host: "127.0.0.1", Port: 7897}, false},
		{"username", domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP, Host: "127.0.0.1", Port: 7897, Username: "user"}, false},
		{"password", domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP, Host: "127.0.0.1", Port: 7897, Password: "fixture"}, false},
		{"empty host", domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP, Port: 7897}, false},
		{"URL as host", domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP, Host: "http://proxy", Port: 7897}, false},
		{"zero port", domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP, Host: "127.0.0.1"}, false},
		{"port out of range", domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP, Host: "127.0.0.1", Port: 65536}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			adapter := NewClaude()
			if got := adapter.Supports(test.endpoint, policy); got != test.want {
				t.Fatalf("Supports() = %v, want %v", got, test.want)
			}
			if _, err := adapter.Plan(claudeIdentity(), test.endpoint, policy); (err == nil) != test.want {
				t.Fatalf("Plan() error = %v, supported = %v", err, test.want)
			}
		})
	}
	if _, err := NewClaude().Plan(domain.AppIdentity{Executable: `C:\Apps\claude.exe`}, domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP, Host: "127.0.0.1", Port: 7897}, policy); err == nil {
		t.Fatal("Plan must reject an identity without Store package evidence")
	}
}

func TestClaudePlanReplacesProxyFlagsAndBuildsChildEnvironment(t *testing.T) {
	for _, domains := range [][]string{nil, {"Example.com", "localhost"}} {
		t.Run(strings.Join(domains, ","), func(t *testing.T) {
			identity := claudeIdentity()
			identity.Arguments = []string{
				"--user-data-dir", `C:\Claude Profile`, "--flag",
				"--proxy-server", "http://old:1", "--proxy-server=http://stale:2",
				"--proxy-pac-url", "http://old/pac", "--proxy-bypass-list=old.test",
				"--no-proxy-server", "--proxy-auto-detect",
			}
			original := identity
			original.Arguments = append([]string(nil), identity.Arguments...)
			policy, err := domain.NewBypassPolicy(domains)
			if err != nil {
				t.Fatal(err)
			}
			spec, err := NewClaude().Plan(identity, domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP, Host: "127.0.0.1", Port: 7897}, policy)
			if err != nil {
				t.Fatal(err)
			}
			if !spec.DirectLaunch {
				t.Fatal("Claude must launch directly so its proxy environment survives Store activation")
			}
			wantArgs := []string{"--user-data-dir", `C:\Claude Profile`, "--flag", "--proxy-server=http://127.0.0.1:7897"}
			if !policy.Empty() {
				wantArgs = append(wantArgs, "--proxy-bypass-list=example.com;*.example.com;localhost;*.localhost")
			}
			if !reflect.DeepEqual(spec.Arguments, wantArgs) {
				t.Fatalf("arguments = %#v, want %#v", spec.Arguments, wantArgs)
			}
			wantEnvironment := map[string]string{
				"HTTP_PROXY": "http://127.0.0.1:7897", "HTTPS_PROXY": "http://127.0.0.1:7897", "ALL_PROXY": "http://127.0.0.1:7897",
				"http_proxy": "http://127.0.0.1:7897", "https_proxy": "http://127.0.0.1:7897", "all_proxy": "http://127.0.0.1:7897",
			}
			if policy.Empty() {
				if !reflect.DeepEqual(spec.UnsetEnvironment, []string{"NO_PROXY", "no_proxy"}) {
					t.Fatalf("inherited bypass settings not cleared: %#v", spec.UnsetEnvironment)
				}
			} else {
				wantEnvironment["NO_PROXY"], wantEnvironment["no_proxy"] = "example.com,localhost", "example.com,localhost"
				if len(spec.UnsetEnvironment) != 0 {
					t.Fatalf("explicit bypass settings unexpectedly unset: %#v", spec.UnsetEnvironment)
				}
			}
			if !reflect.DeepEqual(spec.Environment, wantEnvironment) {
				t.Fatalf("environment = %#v, want %#v", spec.Environment, wantEnvironment)
			}
			if !reflect.DeepEqual(identity, original) || !reflect.DeepEqual(spec.Identity, original) {
				t.Fatal("Plan changed the application's identity or original arguments")
			}
			spec.Identity.Arguments[0] = "changed"
			spec.Arguments[0] = "changed"
			if !reflect.DeepEqual(identity, original) {
				t.Fatal("launch plan shares the input argument slice")
			}
		})
	}
}

func TestProductionStrategySelectsClaudeNativeHTTPAndHonorsFallback(t *testing.T) {
	bypass, err := domain.NewBypassPolicy([]string{"direct.example"})
	if err != nil {
		t.Fatal(err)
	}
	verified := domain.ProbeResult{HTTPVerified: true, SOCKS5Verified: true}
	for _, test := range []struct {
		name      string
		identity  domain.AppIdentity
		endpoint  domain.ProxyEndpoint
		cap       domain.ProbeResult
		bypass    domain.BypassPolicy
		mode      domain.LaunchMode
		protocol  domain.Protocol
		adapterID string
		wantError bool
	}{
		{"HTTP with bypass", claudeIdentity(), domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP, Host: "127.0.0.1", Port: 7897}, verified, bypass, domain.ModeNative, domain.ProtocolHTTP, "claude", false},
		{"mixed port prefers native HTTP", claudeIdentity(), domain.ProxyEndpoint{Protocol: domain.ProtocolAuto, Host: "127.0.0.1", Port: 7897}, verified, bypass, domain.ModeNative, domain.ProtocolHTTP, "claude", false},
		{"SOCKS5 fallback", claudeIdentity(), domain.ProxyEndpoint{Protocol: domain.ProtocolSOCKS5, Host: "127.0.0.1", Port: 7897}, verified, domain.BypassPolicy{}, domain.ModeForced, domain.ProtocolSOCKS5, "", false},
		{"authenticated HTTP fallback", claudeIdentity(), domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP, Host: "127.0.0.1", Port: 7897, Username: "user"}, verified, domain.BypassPolicy{}, domain.ModeForced, domain.ProtocolHTTP, "", false},
		{"SOCKS5 cannot drop bypass", claudeIdentity(), domain.ProxyEndpoint{Protocol: domain.ProtocolSOCKS5, Host: "127.0.0.1", Port: 7897}, verified, bypass, "", "", "", true},
		{"authenticated HTTP cannot drop bypass", claudeIdentity(), domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP, Host: "127.0.0.1", Port: 7897, Password: "fixture"}, verified, bypass, "", "", "", true},
		{"Codex native regression", domain.AppIdentity{Executable: `C:\Apps\codex.exe`, PackageName: "OpenAI.Codex"}, domain.ProxyEndpoint{Protocol: domain.ProtocolAuto, Host: "127.0.0.1", Port: 7897}, verified, bypass, domain.ModeNative, domain.ProtocolHTTP, "codex", false},
		{"Chrome forced regression", domain.AppIdentity{Executable: `C:\Apps\chrome.exe`}, domain.ProxyEndpoint{Protocol: domain.ProtocolAuto, Host: "127.0.0.1", Port: 7897}, verified, domain.BypassPolicy{}, domain.ModeForced, domain.ProtocolSOCKS5, "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			plan, err := application.NewStrategySelector(Production()...).Select(test.identity, test.endpoint, test.cap, test.bypass)
			if test.wantError {
				if !errors.Is(err, application.ErrUnsupportedBypass) {
					t.Fatalf("error = %v, want unsupported bypass", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if plan.Mode != test.mode || plan.AdapterID != test.adapterID || plan.Proxy.Protocol != test.protocol {
				t.Fatalf("plan mode=%q adapter=%q protocol=%q, want %q %q %q", plan.Mode, plan.AdapterID, plan.Proxy.Protocol, test.mode, test.adapterID, test.protocol)
			}
		})
	}
	if _, err := application.NewStrategySelector(Production()...).Select(claudeIdentity(), domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP, Host: "127.0.0.1", Port: 7897}, domain.ProbeResult{HTTP: true}, bypass); err == nil {
		t.Fatal("recognized but unverified HTTP must not launch Claude")
	}
}
