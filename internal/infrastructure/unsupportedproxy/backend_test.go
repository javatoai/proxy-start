package unsupportedproxy

import (
	"context"
	"strings"
	"testing"

	"proxy-start/internal/domain"
)

func TestUnsupportedBackendRejectsInterceptionAndInstallation(t *testing.T) {
	backend := New("macOS 当前仅支持 Codex 原生代理")
	ctx := context.Background()
	if err := backend.Apply(ctx, []domain.ForcedSession{{AppID: "unsupported-app"}}); err == nil || !strings.Contains(err.Error(), "仅支持 Codex") {
		t.Fatalf("unsupported interception was accepted: %v", err)
	}
	if err := backend.Install(ctx); err == nil {
		t.Fatal("unsupported component installation was accepted")
	}
	if err := backend.Apply(ctx, nil); err != nil {
		t.Fatalf("empty cleanup failed: %v", err)
	}
	if err := backend.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	status := backend.Status(ctx)
	if status.Ready || status.Running || status.Installed || status.Detail == "" {
		t.Fatalf("unsupported backend reported success: %+v", status)
	}
}
