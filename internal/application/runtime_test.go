package application

import (
	"context"
	"errors"
	"proxy-start/internal/domain"
	"testing"
)

type fakeBackend struct {
	applies   [][]domain.ForcedSession
	shutdowns int
	fail      bool
}

func (f *fakeBackend) Apply(_ context.Context, s []domain.ForcedSession) error {
	if f.fail {
		return errors.New("apply failed")
	}
	f.applies = append(f.applies, append([]domain.ForcedSession(nil), s...))
	return nil
}
func (f *fakeBackend) Shutdown(context.Context) error              { f.shutdowns++; return nil }
func (f *fakeBackend) Status(context.Context) domain.BackendStatus { return domain.BackendStatus{} }
func (f *fakeBackend) Install(context.Context) error               { return nil }

func TestSharedRuntimeKeepsOtherApplicationsAndCanRestart(t *testing.T) {
	backend := &fakeBackend{}
	runtime := newForcedRuntime(backend)
	ctx := context.Background()
	for _, id := range []string{"one", "two"} {
		if err := runtime.acquire(ctx, domain.ForcedSession{AppID: id}); err != nil {
			t.Fatal(err)
		}
	}
	if err := runtime.release(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	last := backend.applies[len(backend.applies)-1]
	if len(last) != 1 || last[0].AppID != "two" || backend.shutdowns != 0 {
		t.Fatal("stopping one app disturbed another")
	}
	if err := runtime.release(ctx, "two"); err != nil {
		t.Fatal(err)
	}
	if backend.shutdowns != 1 {
		t.Fatal("last session did not release engine")
	}
	if err := runtime.acquire(ctx, domain.ForcedSession{AppID: "three"}); err != nil {
		t.Fatal(err)
	}
}

func TestFailedAcquirePreservesCommittedSessionSet(t *testing.T) {
	backend := &fakeBackend{}
	runtime := newForcedRuntime(backend)
	ctx := context.Background()
	_ = runtime.acquire(ctx, domain.ForcedSession{AppID: "one"})
	backend.fail = true
	if err := runtime.acquire(ctx, domain.ForcedSession{AppID: "two"}); err == nil {
		t.Fatal("expected apply failure")
	}
	if len(runtime.sessions) != 1 || runtime.sessions["one"].AppID != "one" {
		t.Fatal("failed apply altered the committed sessions")
	}
}
