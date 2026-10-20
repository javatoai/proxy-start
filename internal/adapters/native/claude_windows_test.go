//go:build windows

package native

import (
	"context"
	"os"
	"testing"
	"time"

	"proxy-start/internal/application"
	"proxy-start/internal/domain"
	"proxy-start/internal/infrastructure/credentials"
	"proxy-start/internal/infrastructure/netprobe"
	"proxy-start/internal/infrastructure/unsupportedproxy"
	"proxy-start/internal/platform/windows"
)

// Explicitly opt in because this reads the current user's installed AppX apps.
func TestClaudeWindowsInstalledIntegration(t *testing.T) {
	if os.Getenv("PROXY_START_CLAUDE_DIAGNOSTIC") != "1" {
		t.Skip("set PROXY_START_CLAUDE_DIAGNOSTIC=1 to inspect installed Claude")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	identity := installedClaude(t, ctx, windows.NewResolver())
	if !NewClaude().Match(identity) {
		t.Fatalf("installed Claude did not match the native adapter: %+v", identity)
	}
	if _, err := os.Stat(identity.Executable); err != nil {
		t.Fatal(err)
	}
	t.Logf("Claude catalog identity: %s; executable: %s", identity.AUMID, identity.Executable)
}

// This opt-in smoke uses the same Desktop use cases as the UI, an isolated
// in-memory configuration and the real resolver, process controller and probe.
// It preserves an existing Claude session and leaves a new launch open.
func TestClaudeWindowsLaunchIntegration(t *testing.T) {
	if os.Getenv("PROXY_START_CLAUDE_LAUNCH") != "1" {
		t.Skip("set PROXY_START_CLAUDE_LAUNCH=1 to launch Claude through local HTTP proxy 127.0.0.1:7897")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	resolver := windows.NewResolver()
	identity := installedClaude(t, ctx, resolver)
	processes := windows.NewProcessController()
	existing, err := processes.Find(ctx, identity)
	if err != nil {
		t.Fatal(err)
	}
	if len(existing) > 0 {
		t.Skip("Claude is already running; preserving the existing session")
	}
	settings := domain.DefaultSettings()
	settings.Profiles[0].Protocol = domain.ProtocolHTTP
	settings.Profiles[0].BypassDomains = []string{"localhost"}
	desktop, err := application.NewDesktop(ctx, application.Dependencies{
		Store:       &claudeSmokeSettings{settings: settings},
		Credentials: credentials.New(),
		Resolver:    resolver,
		Processes:   processes,
		Probe:       netprobe.New(),
		Backend:     unsupportedproxy.New("Claude native smoke must not acquire forced-proxy resources"),
		Adapters:    Production(),
		Platform:    application.WindowsCapabilities(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		shutdownCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if err := desktop.Shutdown(shutdownCtx); err != nil {
			t.Error(err)
		}
	})
	entry, err := desktop.AddApp(ctx, "package:"+identity.AUMID)
	if err != nil {
		t.Fatal(err)
	}
	if err := desktop.Start(ctx, entry.ID); err != nil {
		t.Fatal(err)
	}
	runtime := desktop.Snapshot().Runtimes[entry.ID]
	if runtime.Mode != domain.ModeNative || runtime.AdapterID != "claude" || runtime.State != domain.StateRunning || !runtime.ProxyApplied {
		t.Fatalf("unexpected Claude runtime: %+v", runtime)
	}
	if runtime.ConnectionVerified {
		t.Fatal("launch success must not claim verified target traffic")
	}
	if len(runtime.Processes) == 0 || !processes.Alive(ctx, runtime.Processes[0]) {
		t.Fatal("Claude launch did not leave a live process")
	}
	t.Logf("Claude started through Desktop: PID=%d mode=%s adapter=%s source=%s", runtime.Processes[0].PID, runtime.Mode, runtime.AdapterID, entry.Source)
}

func installedClaude(t *testing.T, ctx context.Context, resolver *windows.Resolver) domain.AppIdentity {
	t.Helper()
	items, err := resolver.Installed(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, identity := range items {
		if !NewClaude().Match(identity) {
			continue
		}
		resolved, err := resolver.Resolve(ctx, "package:"+identity.AUMID)
		if err != nil {
			t.Fatal(err)
		}
		if !application.SameApplication(identity, resolved) || !NewClaude().Match(resolved) {
			t.Fatal("Claude catalog and package resolution disagree")
		}
		return resolved
	}
	t.Fatal("Windows Store Claude is not present in the installed application catalog")
	return domain.AppIdentity{}
}

type claudeSmokeSettings struct{ settings domain.Settings }

func (s *claudeSmokeSettings) Load(context.Context) (domain.Settings, error) {
	return s.settings, nil
}

func (s *claudeSmokeSettings) Save(_ context.Context, settings domain.Settings) error {
	s.settings = settings
	return nil
}
