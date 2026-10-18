package application

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"proxy-start/internal/domain"
)

type iconProviderSpy struct {
	mu      sync.Mutex
	calls   []domain.AppIdentity
	data    []byte
	err     error
	started chan struct{}
}

func (p *iconProviderSpy) Icon(ctx context.Context, identity domain.AppIdentity) ([]byte, error) {
	p.mu.Lock()
	p.calls = append(p.calls, cloneIconIdentity(identity))
	started := p.started
	data := append([]byte(nil), p.data...)
	err := p.err
	p.mu.Unlock()
	if started != nil {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}

func cloneIconIdentity(identity domain.AppIdentity) domain.AppIdentity {
	identity.Arguments = append([]string(nil), identity.Arguments...)
	return identity
}

type iconCountingStore struct {
	mu       sync.Mutex
	settings domain.Settings
	saves    int
}

func (s *iconCountingStore) Load(context.Context) (domain.Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneSettings(s.settings), nil
}

func (s *iconCountingStore) Save(_ context.Context, settings domain.Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saves++
	s.settings = cloneSettings(settings)
	return nil
}

func TestIconForIdentityPassesCompleteIdentityToProvider(t *testing.T) {
	identity := domain.AppIdentity{
		Name:             "Packaged Codex",
		Executable:       `C:\\Program Files\\Codex\\codex.exe`,
		Arguments:        []string{"--profile", "work"},
		WorkingDirectory: `C:\\Program Files\\Codex`,
		PackageName:      "OpenAI.Codex",
		PackageFamily:    "OpenAI.Codex_12345",
		ApplicationID:    "CodexApp",
		AUMID:            "OpenAI.Codex_12345!CodexApp",
	}
	provider := &iconProviderSpy{data: []byte("png")}
	desktop := &Desktop{icons: provider}

	got, err := desktop.IconForIdentity(context.Background(), identity)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []byte("png")) {
		t.Fatalf("unexpected icon bytes: %q", got)
	}
	provider.mu.Lock()
	calls := append([]domain.AppIdentity(nil), provider.calls...)
	provider.mu.Unlock()
	if len(calls) != 1 || !reflect.DeepEqual(calls[0], identity) {
		t.Fatalf("provider received incomplete identity: %#v", calls)
	}
}

func TestIconForIdentityPropagatesCancellation(t *testing.T) {
	provider := &iconProviderSpy{started: make(chan struct{})}
	desktop := &Desktop{icons: provider}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	result := make(chan error, 1)
	go func() {
		_, err := desktop.IconForIdentity(ctx, domain.AppIdentity{Executable: `C:\\app.exe`})
		result <- err
	}()
	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("icon provider was not called")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation was not propagated: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("icon request did not stop after cancellation")
	}
}

func TestIconForIdentityPropagatesProviderFailureWithoutPersistence(t *testing.T) {
	settings := domain.DefaultSettings()
	store := &iconCountingStore{settings: settings}
	providerErr := errors.New("icon unavailable")
	provider := &iconProviderSpy{err: providerErr}
	desktop, err := NewDesktop(context.Background(), Dependencies{
		Store:       store,
		Credentials: memoryCredentials{},
		Resolver:    fakeResolver{},
		Processes:   &fakeProcesses{},
		Probe:       verifiedProbe{},
		Backend:     &fakeBackend{},
		Icons:       provider,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = desktop.Shutdown(ctx)
	})

	before := desktop.Snapshot().Settings
	_, err = desktop.IconForIdentity(context.Background(), domain.AppIdentity{
		Name:          "Not Yet Added",
		Executable:    `C:\\Apps\\not-yet-added.exe`,
		PackageFamily: "Example_123",
	})
	if !errors.Is(err, providerErr) {
		t.Fatalf("provider failure was not propagated: %v", err)
	}
	after := desktop.Snapshot().Settings
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("icon lookup changed persisted settings:\nbefore=%#v\nafter=%#v", before, after)
	}
	store.mu.Lock()
	saves := store.saves
	store.mu.Unlock()
	if saves != 0 {
		t.Fatalf("icon lookup persisted settings %d times", saves)
	}
}

func TestIconForIdentityIsSafeWithoutProvider(t *testing.T) {
	desktop := &Desktop{}
	data, err := desktop.IconForIdentity(context.Background(), domain.AppIdentity{Executable: `C:\\app.exe`})
	if err != nil || data != nil {
		t.Fatalf("nil icon provider should be a safe no-op: data=%v err=%v", data, err)
	}
}

func TestIconDelegatesPersistedEntryIdentityToIconForIdentity(t *testing.T) {
	identity := domain.AppIdentity{Name: "Configured", Executable: `C:\\configured.exe`, PackageFamily: "Configured_123"}
	settings := domain.DefaultSettings()
	settings.Apps = []domain.AppEntry{{ID: "app", Name: identity.Name, Identity: identity}}
	store := &iconCountingStore{settings: settings}
	provider := &iconProviderSpy{data: []byte("png")}
	desktop, err := NewDesktop(context.Background(), Dependencies{
		Store:       store,
		Credentials: memoryCredentials{},
		Resolver:    fakeResolver{},
		Processes:   &fakeProcesses{},
		Probe:       verifiedProbe{},
		Backend:     &fakeBackend{},
		Icons:       provider,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = desktop.Shutdown(ctx)
	})
	if _, err := desktop.Icon(context.Background(), "app"); err != nil {
		t.Fatal(err)
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if len(provider.calls) != 1 || !reflect.DeepEqual(provider.calls[0], identity) {
		t.Fatalf("persisted icon lookup did not delegate complete identity: %#v", provider.calls)
	}
}
