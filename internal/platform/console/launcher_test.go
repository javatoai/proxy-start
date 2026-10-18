package console

import (
	"os"
	"strings"
	"testing"
)

func TestChildEnvironmentScopesOverridesAndUnsets(t *testing.T) {
	old, had := os.LookupEnv("PROXY_START_CONSOLE_TEST")
	defer func() {
		if had {
			_ = os.Setenv("PROXY_START_CONSOLE_TEST", old)
		} else {
			_ = os.Unsetenv("PROXY_START_CONSOLE_TEST")
		}
	}()
	_ = os.Setenv("PROXY_START_CONSOLE_TEST", "parent")
	env := childEnvironment(map[string]string{"PROXY_START_CONSOLE_TEST": "child", "HTTP_PROXY": "http://127.0.0.1:1"}, []string{"NO_PROXY", "no_proxy"})
	found := ""
	for _, item := range env {
		if strings.HasPrefix(item, "PROXY_START_CONSOLE_TEST=") {
			found = item
		}
		if strings.HasPrefix(item, "NO_PROXY=") || strings.HasPrefix(item, "no_proxy=") {
			t.Fatal("bypass variable leaked into child")
		}
	}
	if found != "PROXY_START_CONSOLE_TEST=child" {
		t.Fatalf("unexpected override: %q", found)
	}
}

// This test is intentionally platform independent; it checks the quoting
// primitive used by the Darwin .command adapter without launching a terminal.
func TestShellQuotePreservesSecretsLiterally(t *testing.T) {
	if got := shellQuote("p'a$$\nword"); got != "'p'\\''a$$\nword'" {
		t.Fatalf("unexpected shell quote: %q", got)
	}
}
