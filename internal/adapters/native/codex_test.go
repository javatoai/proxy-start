package native

import (
	"proxy-start/internal/domain"
	"reflect"
	"strings"
	"testing"
)

func TestCodexPlanReplacesConflictingProxyFlagsAndClearsBypass(t *testing.T) {
	identity := domain.AppIdentity{PackageName: "OpenAI.Codex", Executable: "C:/Apps/Codex.exe",
		Arguments: []string{"--keep", "literal", "--proxy-server", "http://old", "--proxy-bypass-list=example.com", "--no-proxy-server", "--proxy-auto-detect"}}
	before := append([]string(nil), identity.Arguments...)
	spec, err := PlanCodex(identity, 7897)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--keep", "literal", "--proxy-server=http://127.0.0.1:7897"}
	if !reflect.DeepEqual(spec.Arguments, want) || !reflect.DeepEqual(identity.Arguments, before) {
		t.Fatalf("arguments: %#v", spec.Arguments)
	}
	if len(spec.Environment) != 6 || !reflect.DeepEqual(spec.UnsetEnvironment, []string{"NO_PROXY", "no_proxy"}) {
		t.Fatal("proxy environment incomplete")
	}
	for _, value := range spec.Environment {
		if !strings.Contains(value, "127.0.0.1:7897") {
			t.Fatal(value)
		}
	}
}

func TestCodexIdentityRequiresPackageAndExecutable(t *testing.T) {
	for _, identity := range []domain.AppIdentity{
		{Name: "Codex", Executable: "C:/Codex.exe"},
		{PackageName: "Claude", Executable: "C:/Codex.exe"},
		{PackageName: "OpenAI.Codex", Executable: "C:/other.exe"},
		{PackageName: "com.openai.codex", Executable: "/Applications/Codex.app/Contents/MacOS/other"},
	} {
		if _, err := PlanCodex(identity, 7897); err == nil {
			t.Fatalf("accepted %+v", identity)
		}
	}
	identity := domain.AppIdentity{PackageName: "com.openai.codex", Executable: "/Applications/Codex.app/Contents/MacOS/Codex"}
	if _, err := PlanCodex(identity, 7897); err != nil {
		t.Fatal(err)
	}
	if _, err := PlanCodex(identity, 65536); err == nil {
		t.Fatal("invalid port accepted")
	}
}
