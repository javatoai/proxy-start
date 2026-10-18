// Package storage persists non-secret application settings as versioned JSON.
// Credentials are referenced by ID and are intentionally not part of this
// file; callers should keep their values in the CredentialStore.
package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"proxy-start/internal/application"
	"proxy-start/internal/domain"
)

// Store is an atomic JSON-backed settings store.
type Store struct {
	path string
}

// New creates a settings store for path. The file is not created until Save.
func New(path string) application.SettingsStore {
	return &Store{path: path}
}

// Load reads and validates settings. A missing file returns the domain's safe
// defaults and does not create or mutate the file system. Corrupt data is
// returned as an error and is never silently replaced.
func (s *Store) Load(ctx context.Context) (domain.Settings, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return domain.Settings{}, err
	}
	if strings.TrimSpace(s.path) == "" {
		return domain.Settings{}, errors.New("settings path is empty")
	}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return domain.DefaultSettings(), nil
	}
	if err != nil {
		return domain.Settings{}, fmt.Errorf("read settings: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return domain.Settings{}, err
	}
	var settings domain.Settings
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&settings); err != nil {
		return domain.Settings{}, fmt.Errorf("decode settings: %w", err)
	}
	// Reject trailing JSON values so a partially corrupted file cannot appear
	// valid just because its first object decoded successfully.
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return domain.Settings{}, errors.New("decode settings: trailing data")
		}
		return domain.Settings{}, fmt.Errorf("decode settings trailing data: %w", err)
	}
	if err := validate(settings); err != nil {
		return domain.Settings{}, err
	}
	return settings, nil
}

// Save validates settings, writes them to a sibling temporary file, flushes
// the bytes, and replaces the destination with a single rename. A cancelled
// operation leaves the previous file untouched and removes its temporary file.
func (s *Store) Save(ctx context.Context, settings domain.Settings) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(s.path) == "" {
		return errors.New("settings path is empty")
	}
	if err := validate(settings); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	data = append(data, '\n')
	if err := ctx.Err(); err != nil {
		return err
	}
	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create settings directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".proxy-start-settings-*.tmp")
	if err != nil {
		return fmt.Errorf("create settings temporary file: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write settings temporary file: %w", err)
	}
	if err := ctx.Err(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("flush settings temporary file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close settings temporary file: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, s.path); err != nil {
		return fmt.Errorf("replace settings file: %w", err)
	}
	return nil
}

func validate(settings domain.Settings) error {
	if settings.Version != 1 {
		return fmt.Errorf("unsupported settings version %d", settings.Version)
	}
	profileIDs := make(map[string]struct{}, len(settings.Profiles))
	for _, profile := range settings.Profiles {
		if strings.TrimSpace(profile.ID) == "" {
			return errors.New("proxy profile ID is empty")
		}
		if _, exists := profileIDs[profile.ID]; exists {
			return fmt.Errorf("duplicate proxy profile ID %q", profile.ID)
		}
		profileIDs[profile.ID] = struct{}{}
		if err := profile.Validate(); err != nil {
			return fmt.Errorf("proxy profile %q is invalid: %w", profile.ID, err)
		}
	}
	if settings.DefaultProxyID != "" {
		if _, exists := profileIDs[settings.DefaultProxyID]; !exists {
			return fmt.Errorf("default proxy profile %q does not exist", settings.DefaultProxyID)
		}
	}
	appIDs := make(map[string]struct{}, len(settings.Apps))
	for _, app := range settings.Apps {
		if strings.TrimSpace(app.ID) == "" {
			return errors.New("application ID is empty")
		}
		if _, exists := appIDs[app.ID]; exists {
			return fmt.Errorf("duplicate application ID %q", app.ID)
		}
		appIDs[app.ID] = struct{}{}
		if strings.TrimSpace(app.Identity.Executable) == "" {
			return fmt.Errorf("application %q executable is empty", app.ID)
		}
	}
	return nil
}
