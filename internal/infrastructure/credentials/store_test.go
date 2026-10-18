package credentials

import (
	"context"
	"errors"
	"testing"
)

func TestStoreRejectsEmptyKeyAndCancellation(t *testing.T) {
	store := New()
	if _, err := store.Read(context.Background(), " "); err == nil {
		t.Fatal("expected empty key error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.Write(ctx, "proxy-start.test", "secret"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Write error = %v", err)
	}
}
