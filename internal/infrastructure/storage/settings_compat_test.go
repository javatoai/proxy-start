package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"proxy-start/internal/domain"
)

func TestLegacySettingsWithoutSystemAppPreferenceDefaultsToHidden(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	legacy := `{"version":1,"defaultProxyId":"local-clash","profiles":[{"id":"local-clash","name":"本机 Clash","protocol":"auto","host":"127.0.0.1","port":7897}],"apps":[]}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	settings, err := New(path).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if settings.ShowSystemApps {
		t.Fatal("legacy settings unexpectedly enabled system apps")
	}
}

func TestSystemAppPreferenceIsOptionalInSerializedSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store := New(path)
	settings := domain.DefaultSettings()
	if err := store.Save(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"showSystemApps"`) {
		t.Fatal("false system-app preference should remain omitted for compact legacy-compatible JSON")
	}
	settings.ShowSystemApps = true
	if err := store.Save(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"showSystemApps": true`) {
		t.Fatalf("true system-app preference was not serialized: %s", data)
	}
}
