package application

import (
	"context"
	"testing"

	"proxy-start/internal/domain"
)

func TestPlatformCapabilitiesSnapshotsDoNotShareMutableSlices(t *testing.T) {
	platform := MacOSCapabilities()
	desktop, err := NewDesktop(context.Background(), Dependencies{
		Store:       &memorySettings{settings: domain.DefaultSettings()},
		Credentials: memoryCredentials{},
		Resolver:    fakeResolver{},
		Processes:   &fakeProcesses{},
		Probe:       verifiedProbe{},
		Backend:     &fakeBackend{},
		Platform:    platform,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = desktop.Shutdown(context.Background()) })
	platform.NativeAdapterIDs[0] = "changed"
	platform.AppExtensions[0] = "exe"
	first := desktop.Snapshot().Platform
	if first.Name != "macOS" || first.ForceProxySupported || first.ForceProxyInstallable || first.NativeAdapterIDs[0] != "codex" || first.AppExtensions[0] != "app" {
		t.Fatalf("platform wiring was mutated or claims unsupported interception: %+v", first)
	}
	first.NativeAdapterIDs[0] = "changed again"
	first.AppExtensions[0] = "lnk"
	second := desktop.Snapshot().Platform
	if second.NativeAdapterIDs[0] != "codex" || second.AppExtensions[0] != "app" {
		t.Fatal("snapshot exposed the stored platform capability slices")
	}
}
