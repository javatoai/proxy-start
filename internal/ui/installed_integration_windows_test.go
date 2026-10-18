//go:build windows

package ui

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode"

	nativeui "github.com/egoist/mygo/ui"
	"proxy-start/internal/domain"
	platformwindows "proxy-start/internal/platform/windows"
)

// TestInstalledPickerRealWindowsRender is intentionally opt-in because it
// reads the current user's AppX catalog and writes review screenshots. It
// never creates a Desktop, starts an installed application, or writes user
// configuration.
func TestInstalledPickerRealWindowsRender(t *testing.T) {
	if os.Getenv("PROXY_START_INSTALLED_INTEGRATION") != "1" {
		t.Skip("set PROXY_START_INSTALLED_INTEGRATION=1 to inspect the real Windows catalog")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	items, err := platformwindows.NewResolver().Installed(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 {
		t.Fatal("Windows installed-app catalog is empty")
	}
	sort.SliceStable(items, func(i, j int) bool {
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})

	selected := selectRealIconIdentities(items, 24)
	visible := make([]domain.AppIdentity, 0, len(items))
	for _, item := range items {
		if !item.SystemApp {
			visible = append(visible, item)
		}
	}
	if len(visible) == 0 {
		t.Skip("Windows catalog has no applications visible with the default system filter")
	}
	model := newModel(nil)
	model.snapshot.Settings = domain.DefaultSettings()
	model.snapshot.Settings.Theme = "light"
	model.installed.open = true
	model.installed.items = visible
	for _, item := range visible {
		if strings.Contains(strings.ToLower(item.PackageName), "openai.codex") {
			model.snapshot.Settings.Apps = []domain.AppEntry{{ID: "review-codex", Name: "OpenAI.Codex", Identity: item}}
			break
		}
	}
	model.installed.icons = make(map[string]*nativeui.Bitmap)
	model.installed.attempts = make(map[string]bool)
	model.installed.iconSlots = make(chan struct{}, 4)

	provider := platformwindows.NewIconProvider()
	decoded := 0
	for _, identity := range selected {
		data, iconErr := provider.Icon(ctx, identity)
		if iconErr != nil {
			t.Logf("icon unavailable name=%q exe=%q: %v", identity.Name, identity.Executable, iconErr)
			continue
		}
		bitmap, decodeErr := nativeui.DecodeBitmap(data)
		if decodeErr != nil {
			t.Logf("icon PNG decode failed name=%q: %v", identity.Name, decodeErr)
			continue
		}
		width, height := bitmap.Size()
		if width != 32 || height != 32 {
			t.Errorf("decoded icon %q has size %dx%d, want 32x32", identity.Name, width, height)
			continue
		}
		model.installed.icons[installedIdentityKey(identity)] = bitmap
		decoded++
	}
	if decoded == 0 {
		t.Fatal("no real Windows application icon could be decoded")
	}
	t.Logf("real Windows catalog: total=%d, default-visible=%d, hidden-system=%d, decoded-icons=%d", len(items), len(visible), len(items)-len(visible), decoded)

	reviewDir := filepath.Join(projectRoot(t), ".impeccable", "review", "installed-picker")
	if err := os.MkdirAll(reviewDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeCheckedInstalledPNG(t, filepath.Join(reviewDir, "real-all-1180x760.png"), nativeui.NewTester(model.view, 1180, 760).Image())

	searches := preferredRealSearches(visible)
	if len(searches) == 0 {
		searches = append(searches, visible[0])
	}
	for _, identity := range searches {
		matched := filteredInstalledApps(visible, identity.Name)
		if !containsIdentityName(matched, identity.Name) {
			t.Fatalf("real search %q did not match the source item", identity.Name)
		}
		// MyGO ListState is bound to the tester/scene that first shows it.
		// Reset it between independent headless frames so the integration
		// screenshots do not share a virtual-list owner.
		model.installed.list = nativeui.ListState{}
		model.setInstalledSearch(identity.Name)
		tester := nativeui.NewTester(model.view, 1180, 760)
		if !tester.HasText("添加 " + identity.Name) {
			t.Fatalf("rendered picker search %q does not show its add action; texts=%v", identity.Name, tester.Texts())
		}
		label := realSearchFileLabel(identity)
		writeCheckedInstalledPNG(t, filepath.Join(reviewDir, "real-search-"+label+"-1180x760.png"), tester.Image())
	}
	if len(model.snapshot.Settings.Apps) > 0 {
		codex := model.snapshot.Settings.Apps[0].Identity
		model.installed.list = nativeui.ListState{}
		model.setInstalledSearch(codex.Name)
		tester := nativeui.NewTester(model.view, 1180, 760)
		if _, added := tester.Find("已添加 " + codex.Name); !added {
			t.Fatal("real Codex identity was not shown as already added")
		}
	}
	model.snapshot.Settings.ShowSystemApps = true
	model.installed.items = items
	model.installed.list = nativeui.ListState{}
	model.setInstalledSearch("Microsoft Edge")
	writeCheckedInstalledPNG(t, filepath.Join(reviewDir, "real-show-system-edge-1180x760.png"), nativeui.NewTester(model.view, 1180, 760).Image())
}

func selectRealIconIdentities(items []domain.AppIdentity, limit int) []domain.AppIdentity {
	if limit < 1 {
		return nil
	}
	selected := make([]domain.AppIdentity, 0, limit)
	seen := make(map[string]bool)
	add := func(identity domain.AppIdentity) {
		key := installedIdentityKey(identity)
		if seen[key] || len(selected) >= limit {
			return
		}
		seen[key] = true
		selected = append(selected, identity)
	}
	for _, needle := range []string{"codex", "chrome", "edge"} {
		for _, identity := range items {
			text := strings.ToLower(identity.Name + "\n" + identity.Executable + "\n" + identity.PackageName + "\n" + identity.AUMID)
			if strings.Contains(text, needle) {
				add(identity)
				break
			}
		}
	}
	for _, identity := range items {
		add(identity)
		if len(selected) == limit {
			break
		}
	}
	return selected
}

func preferredRealSearches(items []domain.AppIdentity) []domain.AppIdentity {
	result := make([]domain.AppIdentity, 0, 3)
	seen := make(map[string]bool)
	for _, needle := range []string{"codex", "chrome", "edge"} {
		for _, identity := range items {
			text := strings.ToLower(identity.Name + "\n" + identity.Executable + "\n" + identity.PackageName + "\n" + identity.AUMID)
			key := installedIdentityKey(identity)
			if strings.Contains(text, needle) && !seen[key] {
				seen[key] = true
				result = append(result, identity)
				break
			}
		}
	}
	return result
}

func containsIdentityName(items []domain.AppIdentity, name string) bool {
	for _, identity := range items {
		if identity.Name == name {
			return true
		}
	}
	return false
}

func realSearchFileLabel(identity domain.AppIdentity) string {
	var b strings.Builder
	for _, r := range strings.ToLower(identity.Name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else if b.Len() > 0 {
			b.WriteByte('-')
		}
		if b.Len() >= 36 {
			break
		}
	}
	label := strings.Trim(b.String(), "-")
	if label == "" {
		return "app"
	}
	return label
}

func writeCheckedInstalledPNG(t *testing.T, path string, frame *image.RGBA) {
	t.Helper()
	writeNativePNG(t, path, frame)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("rendered PNG %s is invalid: %v", path, err)
	}
	width, height := decoded.Bounds().Dx(), decoded.Bounds().Dy()
	if width != 1180 || height != 760 {
		t.Fatalf("rendered PNG %s has size %dx%d, want 1180x760", path, width, height)
	}
	t.Logf("wrote %s (%s)", path, fmt.Sprintf("%dx%d", width, height))
}
