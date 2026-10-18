package application

import (
	"context"
	"testing"
	"time"

	"proxy-start/internal/domain"
)

type installedResolver struct {
	items []domain.AppIdentity
}

func (r installedResolver) Resolve(context.Context, string) (domain.AppIdentity, error) {
	return domain.AppIdentity{}, nil
}

func (r installedResolver) Installed(context.Context) ([]domain.AppIdentity, error) {
	return append([]domain.AppIdentity(nil), r.items...), nil
}

func newInstalledDesktop(t *testing.T, store *memorySettings, items []domain.AppIdentity) *Desktop {
	t.Helper()
	desktop, err := NewDesktop(context.Background(), Dependencies{
		Store:       store,
		Credentials: memoryCredentials{},
		Resolver:    installedResolver{items: items},
		Processes:   &fakeProcesses{},
		Probe:       verifiedProbe{},
		Backend:     &fakeBackend{},
		Adapters:    []NativeAdapter{testAdapter{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = desktop.Shutdown(ctx)
	})
	return desktop
}

func TestInstalledHidesSystemAppsByDefaultAndTogglePersists(t *testing.T) {
	items := []domain.AppIdentity{
		{Name: "Windows Settings", Executable: `C:\Windows\System32\SystemSettings.exe`, Publisher: "Microsoft Corporation", SystemApp: true},
		{Name: "OpenAI Codex", Executable: `C:\Program Files\WindowsApps\OpenAI.Codex\Codex.exe`, Publisher: "OpenAI", SystemApp: false},
		{Name: "Chrome", Executable: `C:\Program Files\Google\Chrome\Application\chrome.exe`, SystemApp: false},
	}
	store := &memorySettings{settings: domain.DefaultSettings()}
	desktop := newInstalledDesktop(t, store, items)
	visible, err := desktop.Installed(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 2 || visible[0].Name != "OpenAI Codex" || visible[1].Name != "Chrome" {
		t.Fatalf("default system filter removed the wrong entries: %#v", visible)
	}
	if desktop.Snapshot().Settings.ShowSystemApps {
		t.Fatal("legacy/default settings unexpectedly show system apps")
	}

	if err := desktop.SetHideSystemApps(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	visible, err = desktop.Installed(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != len(items) {
		t.Fatalf("show-system toggle did not expose all entries: got %d", len(visible))
	}
	if !store.settings.ShowSystemApps {
		t.Fatal("show-system preference was not persisted")
	}

	if err := desktop.SetHideSystemApps(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if store.settings.ShowSystemApps {
		t.Fatal("hide-system toggle did not persist false")
	}
}

func TestSameApplicationUsesStableIdentityInsteadOfDisplayName(t *testing.T) {
	tests := []struct {
		name string
		a    domain.AppIdentity
		b    domain.AppIdentity
		want bool
	}{
		{
			name: "win32 path is case and slash insensitive",
			a:    domain.AppIdentity{Name: "Same title", Executable: `C:\Apps\Demo\demo.exe`},
			b:    domain.AppIdentity{Name: "Different title", Executable: `c:/apps/demo/DEMO.EXE`},
			want: true,
		},
		{
			name: "cross platform path normalization removes dot segments",
			a:    domain.AppIdentity{Name: "Codex", Executable: `/Applications/Codex.app/Contents/MacOS/./Codex`},
			b:    domain.AppIdentity{Name: "Codex", Executable: `/Applications/Codex.app/Contents/MacOS/../MacOS/Codex`},
			want: true,
		},
		{
			name: "windows dot segments normalize on macOS style host",
			a:    domain.AppIdentity{Name: "Tool", Executable: `C:\Apps\.\Demo\..\Demo\tool.exe`},
			b:    domain.AppIdentity{Name: "Tool", Executable: `c:/apps/demo/tool.exe`},
			want: true,
		},
		{
			name: "same AUMID wins over changed package path",
			a:    domain.AppIdentity{Name: "Old", Executable: `C:\Apps\v1.exe`, AUMID: "Contoso.App_abc!Main"},
			b:    domain.AppIdentity{Name: "New", Executable: `D:\Apps\v2.exe`, AUMID: "contoso.app_ABC!main"},
			want: true,
		},
		{
			name: "different packaged application IDs do not collide",
			a:    domain.AppIdentity{Name: "Main", Executable: `C:\Apps\shared.exe`, AUMID: "Contoso.App_abc!Main"},
			b:    domain.AppIdentity{Name: "Settings", Executable: `C:\Apps\shared.exe`, AUMID: "Contoso.App_abc!Settings"},
			want: false,
		},
		{
			name: "package source and AUMID normalize to one identity",
			a:    domain.AppIdentity{PackageFamily: "Contoso.App_ABC", ApplicationID: "Main"},
			b:    domain.AppIdentity{AUMID: "package:contoso.app_abc!main", Executable: `D:\versioned\entry.exe`},
			want: true,
		},
		{
			name: "same package different IDs remain distinct",
			a:    domain.AppIdentity{PackageFamily: "Contoso.App_ABC", ApplicationID: "Main", Executable: `C:\Apps\shared.exe`},
			b:    domain.AppIdentity{PackageFamily: "contoso.app_abc", ApplicationID: "Settings", Executable: `C:\Apps\shared.exe`},
			want: false,
		},
		{
			name: "same package without app IDs requires the same executable",
			a:    domain.AppIdentity{PackageName: "Contoso.App", Executable: `C:\Apps\main.exe`},
			b:    domain.AppIdentity{PackageName: "contoso.app", Executable: `C:\Apps\settings.exe`},
			want: false,
		},
		{
			name: "same package and app ID can survive a versioned path",
			a:    domain.AppIdentity{PackageFamily: "Contoso.App_ABC", ApplicationID: "Main", Executable: `C:\Apps\v1\main.exe`},
			b:    domain.AppIdentity{PackageFamily: "contoso.app_abc", ApplicationID: "main", Executable: `C:\Apps\v2\main.exe`},
			want: true,
		},
		{
			name: "one missing app ID still requires the same executable",
			a:    domain.AppIdentity{PackageFamily: "Contoso.App_ABC", ApplicationID: "Main", Executable: `C:\Apps\main.exe`},
			b:    domain.AppIdentity{PackageFamily: "contoso.app_abc", Executable: `C:\Apps\other.exe`},
			want: false,
		},
		{
			name: "different executables are not equal",
			a:    domain.AppIdentity{Name: "Tool", Executable: `C:\Apps\one.exe`},
			b:    domain.AppIdentity{Name: "Tool", Executable: `C:\Apps\two.exe`},
			want: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := SameApplication(test.a, test.b); got != test.want {
				t.Fatalf("SameApplication() = %v, want %v for %#v / %#v", got, test.want, test.a, test.b)
			}
		})
	}
}
