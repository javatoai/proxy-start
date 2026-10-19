package darwin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBundleHelpersRejectTraversalAndPreserveSpaces(t *testing.T) {
	bundle := filepath.Join(t.TempDir(), "Codex Desktop.app")
	if got := enclosingBundle(filepath.Join(bundle, "Contents", "MacOS", "Codex Desktop")); got != bundle {
		t.Fatalf("enclosingBundle = %q, want %q", got, bundle)
	}
	if got, ok := bundleExecutablePath(bundle, "Codex Desktop"); !ok || !strings.HasSuffix(got, filepath.Join("Codex Desktop.app", "Contents", "MacOS", "Codex Desktop")) {
		t.Fatalf("bundleExecutablePath = %q, %v", got, ok)
	}
	for _, name := range []string{"../escape", "/tmp/escape", "Contents/MacOS/other"} {
		if got, ok := bundleExecutablePath(bundle, name); ok || got != "" {
			t.Fatalf("bundleExecutablePath accepted unsafe name %q: %q, %v", name, got, ok)
		}
	}
}

func TestMergedEnvironmentOverridesAndUnsets(t *testing.T) {
	previous, had := os.LookupEnv("PROXY_START_DARWIN_ENV_TEST")
	_ = os.Setenv("PROXY_START_DARWIN_ENV_TEST", "old")
	defer func() {
		if had {
			_ = os.Setenv("PROXY_START_DARWIN_ENV_TEST", previous)
		} else {
			_ = os.Unsetenv("PROXY_START_DARWIN_ENV_TEST")
		}
	}()
	values := mergedEnvironment(map[string]string{"PROXY_START_DARWIN_ENV_TEST": "new"}, nil)
	if !containsEnvironment(values, "PROXY_START_DARWIN_ENV_TEST", "new") {
		t.Fatal("environment override was not applied")
	}
	values = mergedEnvironment(nil, []string{"PROXY_START_DARWIN_ENV_TEST"})
	if containsEnvironment(values, "PROXY_START_DARWIN_ENV_TEST", "") {
		t.Fatal("environment unset was not applied")
	}
}

func TestMergedEnvironmentCarriesBothBypassVariableCases(t *testing.T) {
	values := mergedEnvironment(map[string]string{
		"NO_PROXY": "example.com,api.example",
		"no_proxy": "example.com,api.example",
	}, nil)
	if !containsEnvironment(values, "NO_PROXY", "example.com,api.example") || !containsEnvironment(values, "no_proxy", "example.com,api.example") {
		t.Fatalf("bypass environment cases were not preserved: %#v", values)
	}
	values = mergedEnvironment(map[string]string{
		"NO_PROXY": "example.com",
		"no_proxy": "example.com",
	}, []string{"NO_PROXY", "no_proxy"})
	if containsEnvironment(values, "NO_PROXY", "") || containsEnvironment(values, "no_proxy", "") {
		t.Fatalf("unset bypass variables remained: %#v", values)
	}
}

func TestLocaleEnvironmentForcesStablePSOutput(t *testing.T) {
	values := localeEnvironment()
	count := 0
	for _, value := range values {
		if strings.HasPrefix(value, "LC_ALL=") {
			count++
			if value != "LC_ALL=C" {
				t.Fatalf("LC_ALL = %q", value)
			}
		}
	}
	if count != 1 {
		t.Fatalf("LC_ALL entries = %d, want 1", count)
	}
}

func containsEnvironment(values []string, key, expected string) bool {
	prefix := key + "="
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			return value == prefix+expected
		}
	}
	return false
}
