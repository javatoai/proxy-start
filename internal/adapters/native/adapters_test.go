package native

import (
	"reflect"
	"strings"
	"testing"

	"proxy-start/internal/domain"
)

func emptyBypassPolicy(t *testing.T) domain.BypassPolicy {
	t.Helper()
	policy, err := domain.NewBypassPolicy(nil)
	if err != nil {
		t.Fatalf("empty bypass policy: %v", err)
	}
	return policy
}

func TestCodexMatchRequiresPackageAndKnownExecutable(t *testing.T) {
	adapter := NewCodex()
	tests := []struct {
		name     string
		identity domain.AppIdentity
		want     bool
	}{
		{
			name: "short package name",
			identity: domain.AppIdentity{
				Executable:  `C:\Program Files\WindowsApps\OpenAI.Codex_1\app\ChatGPT.exe`,
				PackageName: "OpenAI.Codex",
			},
			want: true,
		},
		{
			name: "package family",
			identity: domain.AppIdentity{
				Executable:    `C:\Program Files\WindowsApps\OpenAI.Codex_1\app\ChatGPT.exe`,
				PackageFamily: "OpenAI.Codex_123abc_8wekyb3d8bbwe",
			},
			want: true,
		},
		{
			name: "AUMID package identity",
			identity: domain.AppIdentity{
				Executable: `C:\Program Files\WindowsApps\OpenAI.Codex_1\app\codex.exe`,
				AUMID:      "OpenAI.Codex_123abc_8wekyb3d8bbwe!Codex",
			},
			want: true,
		},
		{
			name: "display name is not package evidence",
			identity: domain.AppIdentity{
				Name:       "ChatGPT",
				Executable: `C:\Apps\ChatGPT.exe`,
			},
			want: false,
		},
		{
			name: "known package with unrelated executable",
			identity: domain.AppIdentity{
				Executable:  `C:\Apps\helper.exe`,
				PackageName: "OpenAI.Codex",
			},
			want: false,
		},
		{
			name: "near miss package name",
			identity: domain.AppIdentity{
				Executable:  `C:\Apps\ChatGPT.exe`,
				PackageName: "OpenAI.CodexPreview",
			},
			want: false,
		},
		{
			name: "macOS signed bundle identity",
			identity: domain.AppIdentity{
				Executable:  "/Applications/Codex.app/Contents/MacOS/Codex",
				PackageName: "com.openai.codex",
			},
			want: true,
		},
		{
			name: "macOS unified ChatGPT bundle with Codex executable",
			identity: domain.AppIdentity{
				Executable:  "/Applications/ChatGPT.app/Contents/MacOS/Codex",
				PackageName: "com.openai.codex",
			},
			want: true,
		},
		{
			name: "macOS ChatGPT executable is not enough",
			identity: domain.AppIdentity{
				Executable:  "/Applications/ChatGPT.app/Contents/MacOS/ChatGPT",
				PackageName: "com.openai.codex",
			},
			want: false,
		},
		{
			name: "macOS display name is not bundle evidence",
			identity: domain.AppIdentity{
				Executable: "/Applications/Codex.app/Contents/MacOS/Codex",
				Name:       "Codex",
			},
			want: false,
		},
		{
			name: "macOS bundle id with unrelated executable",
			identity: domain.AppIdentity{
				Executable:  "/Applications/Codex.app/Contents/MacOS/Helper",
				PackageName: "com.openai.codex",
			},
			want: false,
		},
		{
			name: "macOS bundle id in a Windows path",
			identity: domain.AppIdentity{
				Executable:  `C:\\Apps\\codex.exe`,
				PackageName: "com.openai.codex",
			},
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := adapter.Match(test.identity); got != test.want {
				t.Fatalf("Match() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestCodexSupportsAndPlansHTTPWithoutCredentials(t *testing.T) {
	adapter := NewCodex()
	identity := domain.AppIdentity{
		Executable:  `C:\Apps\ChatGPT.exe`,
		PackageName: "OpenAI.Codex",
		Arguments:   []string{"--user-data-dir", "C:\\Codex Profile", "--proxy-server", "http://old:1", "--no-proxy-server", "--flag"},
	}
	original := append([]string(nil), identity.Arguments...)
	proxy := domain.ProxyEndpoint{Protocol: domain.ProtocolAuto, Host: "127.0.0.1", Port: 7897}
	bypass := emptyBypassPolicy(t)

	if !adapter.Supports(proxy, bypass) {
		t.Fatal("Codex should accept an auto-detected mixed HTTP endpoint")
	}
	if adapter.Supports(domain.ProxyEndpoint{Protocol: domain.ProtocolSOCKS5, Host: "127.0.0.1", Port: 7897}, bypass) {
		t.Fatal("Codex must not claim SOCKS5 support")
	}
	if adapter.Supports(domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP, Host: "127.0.0.1", Port: 7897, Username: "user"}, bypass) {
		t.Fatal("Codex must not claim authenticated proxy support")
	}

	spec, err := adapter.Plan(identity, proxy, bypass)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if spec.DirectLaunch {
		t.Fatal("Codex must retain its packaged activation path")
	}
	wantArgs := []string{"--user-data-dir", "C:\\Codex Profile", "--flag", "--proxy-server=http://127.0.0.1:7897"}
	if !reflect.DeepEqual(spec.Arguments, wantArgs) {
		t.Fatalf("arguments = %#v, want %#v", spec.Arguments, wantArgs)
	}
	if !reflect.DeepEqual(identity.Arguments, original) {
		t.Fatalf("Plan mutated input arguments: %#v", identity.Arguments)
	}
	if spec.Environment["HTTP_PROXY"] != "http://127.0.0.1:7897" || spec.Environment["http_proxy"] != "http://127.0.0.1:7897" {
		t.Fatalf("unexpected proxy environment: %#v", spec.Environment)
	}
	for _, key := range []string{"HTTPS_PROXY", "ALL_PROXY", "https_proxy", "all_proxy"} {
		if spec.Environment[key] != "http://127.0.0.1:7897" {
			t.Fatalf("environment[%q] = %q", key, spec.Environment[key])
		}
	}
	if !reflect.DeepEqual(spec.UnsetEnvironment, []string{"NO_PROXY", "no_proxy"}) {
		t.Fatalf("unset environment = %#v", spec.UnsetEnvironment)
	}
	if !reflect.DeepEqual(spec.Identity.Arguments, original) {
		t.Fatalf("Plan changed copied identity arguments: %#v", spec.Identity.Arguments)
	}
}

func TestCodexPlansMacOSBundleWithTheSameProxySpec(t *testing.T) {
	adapter := NewCodex()
	identity := domain.AppIdentity{
		Name:        "Codex",
		Executable:  "/Applications/Codex.app/Contents/MacOS/Codex",
		PackageName: macCodexBundleID,
		Arguments:   []string{"--profile-directory=Default", "--proxy-server", "http://old:1"},
	}
	spec, err := adapter.Plan(identity, domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP, Host: "127.0.0.1", Port: 7897}, emptyBypassPolicy(t))
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	want := []string{"--profile-directory=Default", "--proxy-server=http://127.0.0.1:7897"}
	if !reflect.DeepEqual(spec.Arguments, want) {
		t.Fatalf("arguments = %#v, want %#v", spec.Arguments, want)
	}
	if spec.Identity.PackageName != macCodexBundleID {
		t.Fatalf("identity bundle id = %q, want %q", spec.Identity.PackageName, macCodexBundleID)
	}
}

func TestCodexPlansCanonicalBypassListAndEnvironment(t *testing.T) {
	adapter := NewCodex()
	identity := domain.AppIdentity{
		Executable:  `C:\Apps\ChatGPT.exe`,
		PackageName: "OpenAI.Codex",
		Arguments:   []string{"--proxy-bypass-list", "old.test", "--proxy-bypass-list=stale.test", "--proxy-server=http://old:1"},
	}
	policy, err := domain.NewBypassPolicy([]string{"Example.com", "API.Example"})
	if err != nil {
		t.Fatal(err)
	}
	spec, err := adapter.Plan(identity, domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP, Host: "127.0.0.1", Port: 7897}, policy)
	if err != nil {
		t.Fatal(err)
	}
	wantBypass := "--proxy-bypass-list=" + strings.Join([]string{policy.Domains()[0], "*." + policy.Domains()[0], policy.Domains()[1], "*." + policy.Domains()[1]}, ";")
	count := 0
	for _, arg := range spec.Arguments {
		if strings.HasPrefix(arg, "--proxy-bypass-list=") {
			count++
			if arg != wantBypass {
				t.Fatalf("bypass argument = %q, want %q", arg, wantBypass)
			}
		}
		if arg == "old.test" || arg == "stale.test" {
			t.Fatalf("stale bypass argument remained: %#v", spec.Arguments)
		}
	}
	if count != 1 {
		t.Fatalf("bypass argument count = %d, want 1 (%#v)", count, spec.Arguments)
	}
	if spec.Environment["NO_PROXY"] != strings.Join(policy.Domains(), ",") || spec.Environment["no_proxy"] != spec.Environment["NO_PROXY"] {
		t.Fatalf("bypass environment = %#v", spec.Environment)
	}
	if len(spec.UnsetEnvironment) != 0 {
		t.Fatalf("bypass environment unexpectedly unset: %#v", spec.UnsetEnvironment)
	}
}

func TestProductionRegistryContainsCodexAndClaude(t *testing.T) {
	adapters := Production()
	wantIDs := []string{"codex", "claude"}
	if len(adapters) != len(wantIDs) {
		t.Fatalf("production adapter count = %d, want %d", len(adapters), len(wantIDs))
	}
	for i, adapter := range adapters {
		if adapter.ID() != wantIDs[i] {
			t.Fatalf("production adapter id = %q, want %q", adapter.ID(), wantIDs[i])
		}
		for _, executable := range []string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
		} {
			if adapter.Match(domain.AppIdentity{Executable: executable}) {
				t.Fatalf("%s must use the forced-proxy fallback, matched %s", executable, adapter.ID())
			}
		}
	}
}

func TestPlanRejectsInvalidIdentityOrProxy(t *testing.T) {
	identity := domain.AppIdentity{Executable: `C:\Apps\ChatGPT.exe`, PackageName: "OpenAI.Codex"}
	adapter := NewCodex()
	if _, err := adapter.Plan(identity, domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP, Host: "", Port: 7897}, emptyBypassPolicy(t)); err == nil {
		t.Fatal("Plan should reject an invalid endpoint")
	}
	if _, err := adapter.Plan(domain.AppIdentity{Executable: `C:\Apps\ChatGPT.exe`}, domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP, Host: "127.0.0.1", Port: 7897}, emptyBypassPolicy(t)); err == nil {
		t.Fatal("Plan should reject an identity without Codex package metadata")
	}
}
