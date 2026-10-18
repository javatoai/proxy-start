package proxifyre

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"proxy-start/internal/domain"
)

type fakeBridge struct {
	mu           sync.Mutex
	started      int
	closed       int
	startContext context.Context
}

func (b *fakeBridge) Start(ctx context.Context) (string, error) {
	b.mu.Lock()
	b.started++
	b.startContext = ctx
	b.mu.Unlock()
	return "127.0.0.1:40001", nil
}

func (b *fakeBridge) Close() error {
	b.mu.Lock()
	b.closed++
	b.mu.Unlock()
	return nil
}

func (b *fakeBridge) waitStarted() {
	for i := 0; i < 100; i++ {
		b.mu.Lock()
		started := b.started > 0
		b.mu.Unlock()
		if started {
			return
		}
		time.Sleep(time.Millisecond)
	}
}

type fakeHelper struct {
	mu          sync.Mutex
	applyResult helperResponse
	applyErr    error
	shutdowns   int
}

func (h *fakeHelper) Apply(ctx context.Context, _ helperApplyRequest) (helperResponse, error) {
	if h.applyErr != nil {
		<-ctx.Done()
		return helperResponse{}, h.applyErr
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.applyResult, nil
}

func (h *fakeHelper) Shutdown(context.Context) (helperResponse, error) {
	h.mu.Lock()
	h.shutdowns++
	h.mu.Unlock()
	return helperResponse{OK: true}, nil
}

func (h *fakeHelper) Close() error { return nil }

func testSession(id string) domain.ForcedSession {
	return domain.ForcedSession{
		AppID:           id,
		ExecutablePaths: []string{`C:\Apps\` + id + `\` + id + `.exe`},
		Proxy:           domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP, Host: "proxy.example", Port: 8080},
	}
}

func testBackend(fake *fakeHelper, bridge *fakeBridge) *Backend {
	backend := NewBackend(Options{HelperPath: "fake-helper.exe", BridgeFactory: func(domain.ProxyEndpoint) Bridge { return bridge }})
	backend.stateMu.Lock()
	backend.helper = fake
	backend.stateMu.Unlock()
	return backend
}

func TestApplyFailureDoesNotCloseActiveBridge(t *testing.T) {
	fake := &fakeHelper{applyResult: helperResponse{OK: true}}
	bridge := &fakeBridge{}
	backend := testBackend(fake, bridge)
	if err := backend.Apply(context.Background(), []domain.ForcedSession{testSession("editor")}); err != nil {
		t.Fatal(err)
	}
	fake.applyResult = helperResponse{Code: "invalid_session", Error: "rejected"}
	if err := backend.Apply(context.Background(), []domain.ForcedSession{testSession("editor")}); err == nil {
		t.Fatal("expected helper rejection")
	}
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	if bridge.closed != 0 {
		t.Fatalf("active bridge closed after failed apply: %d", bridge.closed)
	}
}

func TestShutdownThenApplyReusesBackend(t *testing.T) {
	fake := &fakeHelper{applyResult: helperResponse{OK: true}}
	bridge := &fakeBridge{}
	backend := testBackend(fake, bridge)
	if err := backend.Apply(context.Background(), []domain.ForcedSession{testSession("editor")}); err != nil {
		t.Fatal(err)
	}
	if err := backend.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	newHelper := &fakeHelper{applyResult: helperResponse{OK: true}}
	backend.stateMu.Lock()
	backend.helper = newHelper
	backend.stateMu.Unlock()
	if err := backend.Apply(context.Background(), []domain.ForcedSession{testSession("editor")}); err != nil {
		t.Fatal(err)
	}
	if newHelper.applyResult.OK != true {
		t.Fatal("replacement helper was not used")
	}
}

func TestApplyCancellationDoesNotCancelBridgeContext(t *testing.T) {
	fake := &fakeHelper{applyErr: errors.New("cancelled")}
	bridge := &fakeBridge{}
	backend := testBackend(fake, bridge)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = backend.Apply(ctx, []domain.ForcedSession{testSession("editor")})
		close(done)
	}()
	bridge.waitStarted()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancelled apply did not return")
	}
	bridge.mu.Lock()
	startContext := bridge.startContext
	bridge.mu.Unlock()
	if startContext == nil {
		t.Fatal("bridge was not started")
	}
	select {
	case <-startContext.Done():
		t.Fatal("apply context cancellation leaked into bridge context")
	default:
	}
}

func TestInstallFailureDoesNotClaimInstalled(t *testing.T) {
	// This scenario describes an uninstalled component. The developer's real
	// machine may already have ProxiFyre installed, so isolate both paths.
	directory := t.TempDir()
	backend := NewBackend(Options{
		InstallerPath: filepath.Join(directory, "ProxiFyre-2.6.4-win-x64-setup.exe"),
		EnginePath:    filepath.Join(directory, "missing-engine.exe"),
	})
	if err := backend.Install(context.Background()); err == nil {
		t.Fatal("expected missing installer error")
	}
	status := backend.Status(context.Background())
	if status.Installed {
		t.Fatal("missing installer must not claim installed")
	}
}

func TestOperationSemaphoreCanBeCancelled(t *testing.T) {
	backend := NewBackend(Options{})
	if err := backend.acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	if err := backend.acquire(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected cancelable operation acquire, got %v", err)
	}
	backend.release()
}
