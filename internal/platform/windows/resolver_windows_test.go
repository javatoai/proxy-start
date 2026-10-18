//go:build windows

package windows

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"proxy-start/internal/domain"
)

func TestResolverDirectExecutableIdentity(t *testing.T) {
	tempDir := t.TempDir()
	executable := filepath.Join(tempDir, "fixture.exe")
	if err := os.WriteFile(executable, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	identity, err := NewResolver().Resolve(context.Background(), executable)
	if err != nil {
		t.Fatal(err)
	}
	if !samePath(identity.Executable, executable) {
		t.Fatalf("executable = %q, want %q", identity.Executable, executable)
	}
	if identity.Name != "fixture" {
		t.Fatalf("name = %q, want fixture", identity.Name)
	}
	if !samePath(identity.WorkingDirectory, tempDir) {
		t.Fatalf("working directory = %q, want %q", identity.WorkingDirectory, tempDir)
	}
}

func TestResolverRejectsScriptTarget(t *testing.T) {
	tempDir := t.TempDir()
	script := filepath.Join(tempDir, "fixture.cmd")
	if err := os.WriteFile(script, []byte("echo fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewResolver().Resolve(context.Background(), script); err == nil {
		t.Fatal("expected script target to be rejected")
	}
}

func TestPackageRowIdentityUsesFriendlyMetadata(t *testing.T) {
	identity := packageRowIdentity(packageRow{
		Name:          "OpenAI.Codex",
		DisplayName:   "Codex",
		Publisher:     "CN=OpenAI",
		Executable:    `C:\Program Files\WindowsApps\OpenAI.Codex\app\ChatGPT.exe`,
		PackageFamily: "OpenAI.Codex_abc",
		ApplicationID: "App",
		AUMID:         "OpenAI.Codex_abc!App",
		SystemApp:     false,
	})
	if identity.Name != "Codex" {
		t.Fatalf("friendly name = %q, want Codex", identity.Name)
	}
	if identity.Publisher != "CN=OpenAI" || identity.SystemApp {
		t.Fatalf("metadata was not retained: %#v", identity)
	}
	if identity.AUMID == "" || identity.ApplicationID == "" {
		t.Fatalf("packaged identity missing launch metadata: %#v", identity)
	}
}

func TestFriendlyPackageNameRejectsResourceReferences(t *testing.T) {
	if got := friendlyPackageName("ms-resource://Microsoft.WindowsCalculator/Resources/AppStoreName", "Microsoft.WindowsCalculator", "App"); got != "Microsoft.WindowsCalculator" {
		t.Fatalf("resource display name fallback = %q", got)
	}
	if got := friendlyPackageName("@{Microsoft.WindowsCalculator_8wekyb3d8bbwe?ms-resource://App/Name}", "Calculator"); got != "Calculator" {
		t.Fatalf("@ resource display name fallback = %q", got)
	}
}

func TestCatalogIdentityKeyPrefersAUMIDAndDedupesKnownBrowser(t *testing.T) {
	packaged := domain.AppIdentity{
		Name: "Microsoft Edge", Executable: `C:\Program Files\WindowsApps\Microsoft.MicrosoftEdge.Stable\msedge.exe`,
		PackageFamily: "Microsoft.MicrosoftEdge.Stable_abc", ApplicationID: "App", AUMID: "Microsoft.MicrosoftEdge.Stable_abc!App",
	}
	legacy := domain.AppIdentity{Name: "Microsoft Edge", Executable: `C:\Program Files\Microsoft\Edge\Application\msedge.exe`}
	if catalogIdentityKey(packaged) != "aumid:microsoft.microsoftedge.stable_abc!app" {
		t.Fatalf("catalog key did not prefer AUMID: %q", catalogIdentityKey(packaged))
	}
	if !sameKnownBrowser(packaged, legacy) {
		t.Fatal("packaged and legacy Edge entries should collapse to one visible browser")
	}
	otherPackaged := packaged
	otherPackaged.AUMID = "Microsoft.MicrosoftEdge.Stable_abc!Secondary"
	otherPackaged.ApplicationID = "Secondary"
	if sameKnownBrowser(packaged, otherPackaged) {
		t.Fatal("different packaged browser AUMIDs must remain separate catalog entries")
	}
}

// Run with PROXY_START_WINDOWS_DIAGNOSTIC=1 on a machine with Codex Desktop
// installed. This only reads AppX metadata and never launches the application.
func TestResolverInstalledPackageDiagnostics(t *testing.T) {
	if os.Getenv("PROXY_START_WINDOWS_DIAGNOSTIC") != "1" {
		t.Skip("set PROXY_START_WINDOWS_DIAGNOSTIC=1 to query local AppX metadata")
	}
	resolver := NewResolver()
	identity, err := resolver.Resolve(context.Background(), "package:OpenAI.Codex")
	if err != nil {
		t.Fatal(err)
	}
	if identity.Executable == "" || identity.PackageFamily == "" || identity.ApplicationID == "" || identity.AUMID == "" {
		t.Fatalf("incomplete Codex identity: %#v", identity)
	}
	byAUMID, err := resolver.Resolve(context.Background(), identity.AUMID)
	if err != nil {
		t.Fatal(err)
	}
	if !samePath(byAUMID.Executable, identity.Executable) || byAUMID.AUMID != identity.AUMID {
		t.Fatalf("AUMID resolution mismatch: %#v vs %#v", byAUMID, identity)
	}
	byPackageAUMID, err := resolver.Resolve(context.Background(), "package:"+identity.AUMID)
	if err != nil {
		t.Fatal(err)
	}
	if !samePath(byPackageAUMID.Executable, identity.Executable) || byPackageAUMID.AUMID != identity.AUMID {
		t.Fatalf("package:AUMID resolution mismatch: %#v vs %#v", byPackageAUMID, identity)
	}
	if _, err := resolver.Resolve(context.Background(), "package:"+identity.PackageFamily+"!__proxy_start_missing_application__"); err == nil {
		t.Fatal("expected an unknown explicit application ID to fail")
	}
	installed, err := resolver.Installed(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range installed {
		if item.AUMID == identity.AUMID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Installed did not include %s", identity.AUMID)
	}
	primaryCodex := 0
	var installedCodex domain.AppIdentity
	systemCount := 0
	chromeCount, edgeCount := 0, 0
	for _, item := range installed {
		if item.SystemApp {
			systemCount++
		}
		if strings.EqualFold(item.Name, "Google Chrome") {
			chromeCount++
		}
		if strings.EqualFold(item.Name, "Microsoft Edge") {
			edgeCount++
			if !item.SystemApp {
				t.Fatalf("Edge must be classified as a system app: %#v", item)
			}
		}
		if strings.EqualFold(item.PackageName, "OpenAI.Codex") {
			installedCodex = item
			if item.Name == "OpenAI.Codex" || item.Name == "OpenAI" {
				t.Fatalf("Codex display name fell back to package identity: %q", item.Name)
			}
			if strings.Contains(strings.ToLower(item.AUMID), "codexcorecommandrunner") {
				t.Fatalf("hidden Codex command runner leaked into catalog: %#v", item)
			}
			primaryCodex++
		}
	}
	if primaryCodex != 1 {
		t.Fatalf("Codex visible entry count = %d, want exactly one primary entry", primaryCodex)
	}
	if identity.Name != "Codex" || installedCodex.Name != identity.Name {
		t.Fatalf("Codex naming diverged between Resolve and Installed: resolve=%q installed=%q", identity.Name, installedCodex.Name)
	}
	t.Logf("installed catalog: total=%d, system=%d, default-visible=%d; Codex=%q (%d primary); Chrome=%d, Edge=%d", len(installed), systemCount, len(installed)-systemCount, identity.Name, primaryCodex, chromeCount, edgeCount)
}
