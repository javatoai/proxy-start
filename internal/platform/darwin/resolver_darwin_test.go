//go:build darwin

package darwin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Opt-in because it reads the host's installed applications and invokes
// plutil. It never writes settings or launches an application.
func TestResolverInstalledIntegration(t *testing.T) {
	if os.Getenv("PROXY_START_DARWIN_INTEGRATION") != "1" {
		t.Skip("set PROXY_START_DARWIN_INTEGRATION=1 to run macOS resolver integration")
	}
	items, err := NewResolver().Installed(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, identity := range items {
		if !strings.Contains(strings.ToLower(identity.Executable), ".app"+string(filepath.Separator)+"contents"+string(filepath.Separator)+"macos"+string(filepath.Separator)) {
			t.Fatalf("installed identity escaped app executable directory: %#v", identity)
		}
		if identity.PackageName == "" && identity.AUMID != "" {
			t.Fatalf("bundle without ID produced an identity key: %#v", identity)
		}
	}
}
