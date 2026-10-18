package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"proxy-start/internal/domain"
)

func TestStoreRoundTripKeepsSecretsOutOfJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store := New(path)
	settings := domain.Settings{
		Version:        1,
		DefaultProxyID: "proxy-1",
		Profiles: []domain.ProxyProfile{{
			ID: "proxy-1", Name: "Local", Protocol: domain.ProtocolHTTP,
			Host: "127.0.0.1", Port: 7897, Username: "alice", CredentialRef: "cred-1",
		}},
		Apps: []domain.AppEntry{{ID: "app-1", Name: "Demo", Identity: domain.AppIdentity{Executable: "C:\\Demo.exe"}, ProxyID: "proxy-1"}},
	}
	if err := store.Save(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.DefaultProxyID != settings.DefaultProxyID || loaded.Profiles[0].CredentialRef != "cred-1" {
		t.Fatalf("round-trip mismatch: %+v", loaded)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(strings.ToLower(text), "password") || strings.Contains(text, "secret") {
		t.Fatalf("settings file contains credential material: %q", text)
	}
}

func TestStoreMissingFileReturnsDefaults(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "missing.json"))
	settings, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if settings.Version != 1 || settings.DefaultProxyID != "local-clash" {
		t.Fatalf("unexpected defaults: %+v", settings)
	}
}

func TestStoreCorruptionIsReturnedAndNotReplaced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	original := []byte("{not-json")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	store := New(path)
	if _, err := store.Load(context.Background()); err == nil {
		t.Fatal("expected corruption error")
	}
	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(actual) != string(original) {
		t.Fatalf("corrupt file was changed to %q", actual)
	}
}

func TestStoreRejectsCancelledSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store := New(path)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := store.Save(ctx, domain.DefaultSettings())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Save error = %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancelled save created file: %v", err)
	}
}

func TestStoreRejectsInvalidSchema(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "settings.json"))
	settings := domain.DefaultSettings()
	settings.DefaultProxyID = "missing"
	if err := store.Save(context.Background(), settings); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestStoreSaveReplacesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store := New(path)
	first := domain.DefaultSettings()
	if err := store.Save(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.Theme = "dark"
	if err := store.Save(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Theme != "dark" {
		t.Fatalf("replacement did not persist: %+v", loaded)
	}
}
