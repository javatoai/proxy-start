//go:build windows

package credentials

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

// This test is opt-in because it writes a temporary credential to the
// current user's Windows Credential Manager. It never reads or modifies an
// existing target.
func TestWindowsCredentialManagerRoundTrip(t *testing.T) {
	if os.Getenv("PROXY_START_WINDOWS_INTEGRATION") != "1" {
		t.Skip("set PROXY_START_WINDOWS_INTEGRATION=1 to test Windows Credential Manager")
	}
	key := fmt.Sprintf("proxy-start.integration.%d.%d", os.Getpid(), time.Now().UnixNano())
	store := New()
	defer func() { _ = store.Delete(context.Background(), key) }()
	value := fmt.Sprintf("integration-secret-%d", time.Now().UnixNano())
	if err := store.Write(context.Background(), key, value); err != nil {
		t.Fatal(err)
	}
	got, err := store.Read(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if got != value {
		t.Fatal("credential round-trip did not preserve the value")
	}
}
