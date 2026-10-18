package application

import (
	"context"
	"errors"
	"proxy-start/internal/domain"
	"strings"
	"sync"
	"testing"
)

type toggleSettingsStore struct {
	mu        sync.Mutex
	settings  domain.Settings
	failSaves int
}

func (s *toggleSettingsStore) Load(context.Context) (domain.Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneSettings(s.settings), nil
}

func (s *toggleSettingsStore) Save(_ context.Context, settings domain.Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failSaves > 0 {
		s.failSaves--
		return errors.New("fixture settings save failed")
	}
	s.settings = cloneSettings(settings)
	return nil
}

type consistencyCredentials struct {
	mu             sync.Mutex
	values         map[string]string
	readErr        error
	writeErr       error
	writeErrOnCall int
	writes         int
	deleteErr      error
}

func (s *consistencyCredentials) Read(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.readErr != nil {
		return "", s.readErr
	}
	value, ok := s.values[key]
	if !ok {
		return "", ErrCredentialNotFound
	}
	return value, nil
}

func (s *consistencyCredentials) Write(_ context.Context, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writes++
	if s.writeErr != nil && (s.writeErrOnCall == 0 || s.writes == s.writeErrOnCall) {
		return s.writeErr
	}
	s.values[key] = value
	return nil
}

func TestSaveProfileReportsCredentialRestoreFailureWithoutSecret(t *testing.T) {
	settings := domain.DefaultSettings()
	settings.Profiles[0].CredentialRef = "proxy-start/local-clash"
	store := &toggleSettingsStore{settings: settings, failSaves: 1}
	credentials := &consistencyCredentials{
		values:         map[string]string{"proxy-start/local-clash": "old-secret"},
		writeErr:       errors.New("fixture restore failed"),
		writeErrOnCall: 2,
	}
	desktop := newConsistencyDesktop(t, settings, store, credentials)
	_, err := desktop.SaveProfileResult(context.Background(), settings.Profiles[0], "new-secret")
	if err == nil || !strings.Contains(err.Error(), "保存配置失败") || !strings.Contains(err.Error(), "还原代理凭据失败") {
		t.Fatalf("combined failure was not reported: %v", err)
	}
	if strings.Contains(err.Error(), "old-secret") || strings.Contains(err.Error(), "new-secret") {
		t.Fatalf("credential material leaked through error: %v", err)
	}
}

func (s *consistencyCredentials) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deleteErr != nil {
		return s.deleteErr
	}
	if _, ok := s.values[key]; !ok {
		return ErrCredentialNotFound
	}
	delete(s.values, key)
	return nil
}

func (s *consistencyCredentials) value(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[key]
	return value, ok
}

func newConsistencyDesktop(t *testing.T, settings domain.Settings, store *toggleSettingsStore, credentials *consistencyCredentials) *Desktop {
	t.Helper()
	desktop, err := NewDesktop(context.Background(), Dependencies{Store: store, Credentials: credentials, Resolver: fakeResolver{}, Processes: &fakeProcesses{}, Probe: verifiedProbe{}, Backend: &fakeBackend{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = desktop.Shutdown(context.Background()) })
	return desktop
}

func TestSaveProfileRestoresPreviousCredentialWhenSettingsSaveFails(t *testing.T) {
	settings := domain.DefaultSettings()
	settings.Profiles[0].CredentialRef = "proxy-start/local-clash"
	store := &toggleSettingsStore{settings: settings, failSaves: 1}
	credentials := &consistencyCredentials{values: map[string]string{"proxy-start/local-clash": "old-secret"}}
	desktop := newConsistencyDesktop(t, settings, store, credentials)
	profile := settings.Profiles[0]
	if _, err := desktop.SaveProfileResult(context.Background(), profile, "new-secret"); err == nil {
		t.Fatal("settings failure was swallowed")
	}
	if got, ok := credentials.value(profile.CredentialRef); !ok || got != "old-secret" {
		t.Fatalf("old credential was not restored: %q %v", got, ok)
	}
}

func TestClearCredentialKeepsReferenceWhenCredentialDeleteFails(t *testing.T) {
	settings := domain.DefaultSettings()
	settings.Profiles[0].CredentialRef = "proxy-start/local-clash"
	store := &toggleSettingsStore{settings: settings}
	credentials := &consistencyCredentials{values: map[string]string{"proxy-start/local-clash": "secret"}, deleteErr: errors.New("fixture delete failed")}
	desktop := newConsistencyDesktop(t, settings, store, credentials)
	if err := desktop.ClearCredential(context.Background(), settings.Profiles[0].ID); err == nil {
		t.Fatal("credential deletion failure was swallowed")
	}
	if got := desktop.Snapshot().Settings.Profiles[0].CredentialRef; got != settings.Profiles[0].CredentialRef {
		t.Fatalf("credential reference was cleared despite delete failure: %q", got)
	}
}

func TestClearCredentialRestoresSecretWhenSettingsSaveFails(t *testing.T) {
	settings := domain.DefaultSettings()
	settings.Profiles[0].CredentialRef = "proxy-start/local-clash"
	store := &toggleSettingsStore{settings: settings, failSaves: 1}
	credentials := &consistencyCredentials{values: map[string]string{"proxy-start/local-clash": "secret"}}
	desktop := newConsistencyDesktop(t, settings, store, credentials)
	if err := desktop.ClearCredential(context.Background(), settings.Profiles[0].ID); err == nil {
		t.Fatal("settings failure was swallowed")
	}
	if got, ok := credentials.value(settings.Profiles[0].CredentialRef); !ok || got != "secret" {
		t.Fatalf("credential was not restored after settings failure: %q %v", got, ok)
	}
	if got := desktop.Snapshot().Settings.Profiles[0].CredentialRef; got == "" {
		t.Fatal("credential reference was cleared after failed settings save")
	}
}

func TestDeleteProfileKeepsReferenceWhenCredentialDeleteFails(t *testing.T) {
	settings := domain.DefaultSettings()
	settings.Profiles = append(settings.Profiles, domain.ProxyProfile{ID: "backup", Name: "备用", Protocol: domain.ProtocolHTTP, Host: "127.0.0.1", Port: 8080, CredentialRef: "proxy-start/backup"})
	store := &toggleSettingsStore{settings: settings}
	credentials := &consistencyCredentials{values: map[string]string{"proxy-start/backup": "secret"}, deleteErr: errors.New("fixture delete failed")}
	desktop := newConsistencyDesktop(t, settings, store, credentials)
	if err := desktop.DeleteProfile(context.Background(), "backup"); err == nil {
		t.Fatal("credential deletion failure was swallowed")
	}
	snapshot := desktop.Snapshot()
	if len(snapshot.Settings.Profiles) != 2 {
		t.Fatalf("profile was removed despite credential delete failure: %#v", snapshot.Settings.Profiles)
	}
	if got := snapshot.Settings.Profiles[1].CredentialRef; got != "proxy-start/backup" {
		t.Fatalf("profile was removed despite credential delete failure: %q", got)
	}
}

func TestDeleteProfileRestoresSecretWhenSettingsSaveFails(t *testing.T) {
	settings := domain.DefaultSettings()
	settings.Profiles = append(settings.Profiles, domain.ProxyProfile{ID: "backup", Name: "备用", Protocol: domain.ProtocolHTTP, Host: "127.0.0.1", Port: 8080, CredentialRef: "proxy-start/backup"})
	store := &toggleSettingsStore{settings: settings, failSaves: 1}
	credentials := &consistencyCredentials{values: map[string]string{"proxy-start/backup": "secret"}}
	desktop := newConsistencyDesktop(t, settings, store, credentials)
	if err := desktop.DeleteProfile(context.Background(), "backup"); err == nil {
		t.Fatal("settings failure was swallowed")
	}
	if got, ok := credentials.value("proxy-start/backup"); !ok || got != "secret" {
		t.Fatalf("credential was not restored after settings failure: %q %v", got, ok)
	}
	if len(desktop.Snapshot().Settings.Profiles) != 2 {
		t.Fatal("profile was removed despite settings save failure")
	}
}

func TestMissingCredentialIsIdempotentForClear(t *testing.T) {
	settings := domain.DefaultSettings()
	settings.Profiles[0].CredentialRef = "proxy-start/local-clash"
	store := &toggleSettingsStore{settings: settings}
	credentials := &consistencyCredentials{values: map[string]string{}}
	desktop := newConsistencyDesktop(t, settings, store, credentials)
	if err := desktop.ClearCredential(context.Background(), settings.Profiles[0].ID); err != nil {
		t.Fatal(err)
	}
	if got := desktop.Snapshot().Settings.Profiles[0].CredentialRef; got != "" {
		t.Fatalf("missing credential was not cleared idempotently: %q", got)
	}
}

func TestEndpointRejectsProfileDeletedAfterCallerSnapshot(t *testing.T) {
	settings := domain.DefaultSettings()
	store := &toggleSettingsStore{settings: settings}
	credentials := &consistencyCredentials{values: map[string]string{}}
	desktop := newConsistencyDesktop(t, settings, store, credentials)
	stale := settings.Profiles[0]
	if err := desktop.repository.change(context.Background(), func(current *domain.Settings) error {
		current.Profiles = nil
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := desktop.profiles.endpoint(context.Background(), stale); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted profile snapshot was accepted: %v", err)
	}
}
