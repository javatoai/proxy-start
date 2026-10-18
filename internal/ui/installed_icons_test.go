package ui

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"proxy-start/internal/domain"
)

func TestSubmitInstalledIconRequestCapsConcurrentWorkAndReleasesSlots(t *testing.T) {
	slots := make(chan struct{}, 4)
	release := make(chan struct{})
	startedSignal := make(chan struct{}, 4)
	done := make(chan struct{}, 5)
	var active atomic.Int32
	var maxActive atomic.Int32

	fetch := func(context.Context, domain.AppIdentity) ([]byte, error) {
		current := active.Add(1)
		for {
			previous := maxActive.Load()
			if current <= previous || maxActive.CompareAndSwap(previous, current) {
				break
			}
		}
		startedSignal <- struct{}{}
		<-release
		active.Add(-1)
		return []byte("png"), nil
	}
	dispatch := func(fn func()) { fn() }
	callback := func([]byte, error) { done <- struct{}{} }

	started := 0
	for i := 0; i < 8; i++ {
		if submitInstalledIconRequest(context.Background(), slots, domain.AppIdentity{Name: "app"}, fetch, dispatch, callback) {
			started++
		}
	}
	if started != 4 {
		t.Fatalf("started %d requests, want 4", started)
	}
	// Wait until all four accepted workers are inside fetch. Without this
	// barrier, releasing immediately can accidentally serialize the workers and
	// make a concurrency assertion depend on scheduler timing.
	for i := 0; i < 4; i++ {
		select {
		case <-startedSignal:
		case <-time.After(time.Second):
			t.Fatal("accepted icon workers did not reach provider")
		}
	}
	// Four releases unblock all accepted workers. The callback channel also
	// proves each worker reached dispatch before its slot was recycled.
	for i := 0; i < 4; i++ {
		release <- struct{}{}
	}
	for i := 0; i < 4; i++ {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("accepted icon request did not complete")
		}
	}
	if got := maxActive.Load(); got > 4 || got != 4 {
		t.Fatalf("maximum concurrent icon work = %d, want exactly 4", got)
	}

	// A released slot must be reusable after a failure-free request batch.
	if !submitInstalledIconRequest(context.Background(), slots, domain.AppIdentity{Name: "again"}, fetch, dispatch, callback) {
		t.Fatal("released icon slot was not reusable")
	}
	release <- struct{}{}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reused icon slot did not complete")
	}
}

func TestSubmitInstalledIconRequestPropagatesCancellation(t *testing.T) {
	slots := make(chan struct{}, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	fetch := func(ctx context.Context, _ domain.AppIdentity) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	dispatch := func(fn func()) { fn() }
	if !submitInstalledIconRequest(ctx, slots, domain.AppIdentity{Name: "cancelled"}, fetch, dispatch, func(_ []byte, err error) { result <- err }) {
		t.Fatal("cancellation request was not started")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("request error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled icon request did not dispatch")
	}
	select {
	case slots <- struct{}{}:
		// The worker released its slot.
	default:
		t.Fatal("cancelled icon request leaked its semaphore slot")
	}
}

func TestSubmitInstalledIconRequestPropagatesProviderFailure(t *testing.T) {
	slots := make(chan struct{}, 4)
	want := errors.New("icon provider failed")
	result := make(chan error, 1)
	fetch := func(context.Context, domain.AppIdentity) ([]byte, error) { return nil, want }
	if !submitInstalledIconRequest(context.Background(), slots, domain.AppIdentity{Name: "broken"}, fetch, func(fn func()) { fn() }, func(_ []byte, err error) { result <- err }) {
		t.Fatal("provider failure request was not started")
	}
	select {
	case err := <-result:
		if !errors.Is(err, want) {
			t.Fatalf("request error = %v, want provider failure", err)
		}
	case <-time.After(time.Second):
		t.Fatal("provider failure did not dispatch")
	}
	select {
	case slots <- struct{}{}:
	default:
		t.Fatal("failed icon request leaked its semaphore slot")
	}
}

func TestSubmitInstalledIconRequestRejectsMissingInputsWithoutStartingWork(t *testing.T) {
	slots := make(chan struct{}, 4)
	called := atomic.Bool{}
	fetch := func(context.Context, domain.AppIdentity) ([]byte, error) {
		called.Store(true)
		return nil, nil
	}
	if submitInstalledIconRequest(nil, slots, domain.AppIdentity{}, fetch, func(func()) {}, func([]byte, error) {}) {
		t.Fatal("nil context request unexpectedly started")
	}
	if submitInstalledIconRequest(context.Background(), nil, domain.AppIdentity{}, fetch, func(func()) {}, func([]byte, error) {}) {
		t.Fatal("nil semaphore request unexpectedly started")
	}
	if called.Load() {
		t.Fatal("invalid request invoked provider")
	}
}

func TestTryStartInstalledIconRequestDoesNotMarkRejectedSlot(t *testing.T) {
	attempts := make(map[string]bool)
	slots := make(chan struct{}, 1)
	slots <- struct{}{}
	started := tryStartInstalledIconRequest(attempts, "full", func() bool {
		return submitInstalledIconRequest(context.Background(), slots, domain.AppIdentity{Name: "full"},
			func(context.Context, domain.AppIdentity) ([]byte, error) { return []byte("png"), nil },
			func(fn func()) { fn() }, func([]byte, error) {})
	})
	if started {
		t.Fatal("request unexpectedly started while semaphore was full")
	}
	if attempts["full"] {
		t.Fatal("rejected request was marked as attempted")
	}
	<-slots

	done := make(chan struct{}, 1)
	started = tryStartInstalledIconRequest(attempts, "full", func() bool {
		return submitInstalledIconRequest(context.Background(), slots, domain.AppIdentity{Name: "full"},
			func(context.Context, domain.AppIdentity) ([]byte, error) { return []byte("png"), nil },
			func(fn func()) { fn() }, func([]byte, error) { done <- struct{}{} })
	})
	if !started || !attempts["full"] {
		t.Fatal("request was not marked after acquiring a slot")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("retry after a rejected request did not complete")
	}
}
