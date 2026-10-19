package storage

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"proxy-start/internal/domain"
)

func TestLegacyProfileWithoutBypassDomainsLoadsAsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	legacy := `{"version":1,"defaultProxyId":"local-clash","profiles":[{"id":"local-clash","name":"本机 Clash","protocol":"auto","host":"127.0.0.1","port":7897}],"apps":[]}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	settings, err := New(path).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(settings.Profiles) != 1 || len(settings.Profiles[0].BypassDomains) != 0 {
		t.Fatalf("legacy bypass field was not treated as empty: %#v", settings.Profiles)
	}
}

func TestProfileBypassDomainsRoundTripThroughJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store := New(path)
	settings := domain.DefaultSettings()
	settings.Profiles[0].BypassDomains = []string{"example.com", "api.example.com"}
	if err := store.Save(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.Profiles[0].BypassDomains; !reflect.DeepEqual(got, settings.Profiles[0].BypassDomains) {
		t.Fatalf("bypass domains did not round-trip: got %#v, want %#v", got, settings.Profiles[0].BypassDomains)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"bypassDomains"`) {
		t.Fatalf("serialized settings omitted bypass domains: %s", data)
	}
}
