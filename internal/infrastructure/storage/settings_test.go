package storage

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"proxy-start/internal/domain"
	"strings"
	"testing"
)

func TestLegacyMigrationIsReadOnlyUntilFirstSaveAndBacksUpExactOriginal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	original := []byte(`{"version":1,"defaultProxyId":"old","theme":"dark","profiles":[{"id":"old","protocol":"auto","host":"127.0.0.1","port":8800,"bypassDomains":["example.com"]}],"apps":[{"name":"Claude"}]}`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	store := New(path)
	settings, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if settings.Version != 2 || settings.ProxyPort != 8800 {
		t.Fatalf("%+v", settings)
	}
	unchanged, _ := os.ReadFile(path)
	if !bytes.Equal(original, unchanged) {
		t.Fatal("loading changed legacy file")
	}
	files, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "*backup*"))
	if len(files) != 0 {
		t.Fatal("loading wrote backup")
	}
	if err := store.Save(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	files, _ = filepath.Glob(filepath.Join(filepath.Dir(path), "settings.v1-backup-*.json"))
	if len(files) != 1 {
		t.Fatalf("backups: %v", files)
	}
	backup, _ := os.ReadFile(files[0])
	if !bytes.Equal(original, backup) {
		t.Fatal("backup changed original bytes")
	}
	current, _ := os.ReadFile(path)
	if strings.Contains(string(current), "profiles") || strings.Contains(string(current), "Claude") {
		t.Fatal("obsolete settings survived migration")
	}
	if err := store.Save(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	files, _ = filepath.Glob(filepath.Join(filepath.Dir(path), "*backup*"))
	if len(files) != 1 {
		t.Fatal("repeated backup")
	}
	loaded, err := New(path).Load(context.Background())
	if err != nil || loaded != settings {
		t.Fatalf("roundtrip: %+v, %v", loaded, err)
	}
}

func TestUnsupportedLegacyProxyUsesDefault(t *testing.T) {
	for _, extra := range []string{`"host":"remote.test","protocol":"http"`, `"host":"127.0.0.1","protocol":"socks5"`, `"host":"127.0.0.1","protocol":"http","credentialRef":"secret"`} {
		settings, err := migrate([]byte(`{"defaultProxyId":"old","profiles":[{"id":"old","port":8800,` + extra + `}]}`))
		if err != nil || settings.ProxyPort != 7897 {
			t.Fatalf("%+v %v", settings, err)
		}
	}
}

func TestBackupFailureAndExternalChangesPreserveOriginal(t *testing.T) {
	for _, external := range []bool{false, true} {
		t.Run(map[bool]string{false: "backup failure", true: "external change"}[external], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			original := []byte(`{"version":1,"profiles":[],"apps":[]}`)
			_ = os.WriteFile(path, original, 0600)
			store := New(path)
			settings, err := store.Load(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if external {
				original = append(original, '\n')
				_ = os.WriteFile(path, original, 0600)
			} else {
				store.createTemp = func(string, string) (*os.File, error) { return nil, errors.New("backup denied") }
			}
			if err := store.Save(context.Background(), settings); err == nil {
				t.Fatal("failure hidden")
			}
			current, _ := os.ReadFile(path)
			if !bytes.Equal(current, original) {
				t.Fatal("failed migration changed old config")
			}
		})
	}
}

func TestMissingCorruptAndCancelledSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store := New(path)
	settings, err := store.Load(context.Background())
	if err != nil || settings != domain.DefaultSettings() {
		t.Fatal(settings, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("loading created configuration")
	}
	for _, bad := range []string{`{"version":2,"proxyPort":0,"theme":"system"}`, `{"version":3}`, `{"version":2,"proxyPort":7897,"theme":"system","unexpected":true}`, `{"version":1}{}`} {
		_ = os.WriteFile(path, []byte(bad), 0600)
		if _, err := New(path).Load(context.Background()); err == nil {
			t.Fatal("invalid configuration accepted", bad)
		}
	}
	_ = os.WriteFile(path, []byte("original"), 0600)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.Save(ctx, domain.DefaultSettings()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	current, _ := os.ReadFile(path)
	if string(current) != "original" {
		t.Fatal("cancelled save replaced file")
	}
}
