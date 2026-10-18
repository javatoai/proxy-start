package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"proxy-start/internal/domain"
)

// settingsRepository makes a validated, durable change visible as one operation.
type settingsRepository struct {
	mu       sync.RWMutex
	store    SettingsStore
	settings domain.Settings
}

func newSettingsRepository(ctx context.Context, store SettingsStore) (*settingsRepository, error) {
	settings, err := store.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("读取配置失败：%w", err)
	}
	if settings.Version == 0 {
		settings.Version = 1
	}
	return &settingsRepository{store: store, settings: cloneSettings(settings)}, nil
}

func (r *settingsRepository) snapshot() domain.Settings {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return cloneSettings(r.settings)
}

func (r *settingsRepository) change(ctx context.Context, change func(*domain.Settings) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	next := cloneSettings(r.settings)
	if err := change(&next); err != nil {
		return err
	}
	if err := r.store.Save(ctx, next); err != nil {
		return fmt.Errorf("保存配置失败：%w", err)
	}
	r.settings = next
	return nil
}

func (r *settingsRepository) app(id string) (domain.AppEntry, error) {
	for _, app := range r.snapshot().Apps {
		if app.ID == id {
			return app, nil
		}
	}
	return domain.AppEntry{}, ErrNotFound
}

func (r *settingsRepository) profile(id string) (domain.ProxyProfile, error) {
	s := r.snapshot()
	if id == "" {
		id = s.DefaultProxyID
	}
	for _, p := range s.Profiles {
		if p.ID == id {
			return p, nil
		}
	}
	return domain.ProxyProfile{}, ErrNotFound
}

func cloneSettings(s domain.Settings) domain.Settings {
	s.Profiles = append([]domain.ProxyProfile(nil), s.Profiles...)
	s.Apps = append([]domain.AppEntry(nil), s.Apps...)
	for i := range s.Apps {
		s.Apps[i].Identity.Arguments = append([]string(nil), s.Apps[i].Identity.Arguments...)
		s.Apps[i].RelatedExecutables = append([]string(nil), s.Apps[i].RelatedExecutables...)
	}
	return s
}

func newID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("operating system random source unavailable")
	}
	return hex.EncodeToString(b[:])
}

// SameApplication reports whether two platform-resolved identities refer to
// the same launch entry. Display names are deliberately ignored: Windows
// applications can share a name while pointing at different executables.
//
// AUMID/application IDs are authoritative for packaged apps. When they are
// unavailable (for example, a legacy Win32 entry), the normalized executable
// path is used. Package metadata is checked before the path fallback so two
// entries in one package with different application IDs are never collapsed.
func SameApplication(a, b domain.AppIdentity) bool {
	if aumidA, aumidB := normalizeAUMID(a.AUMID), normalizeAUMID(b.AUMID); aumidA != "" || aumidB != "" {
		if aumidA != "" && aumidB != "" {
			return aumidA == aumidB
		}
	}

	// Structured package metadata can explicitly disprove equivalence even
	// when a package exposes the same executable for multiple app entries. If
	// one source omits a package field, retain the path fallback for legacy
	// entries; a known app-id conflict still returns false immediately.
	packageA, hasPackageA := packagePartsOf(a)
	packageB, hasPackageB := packagePartsOf(b)
	if hasPackageA && hasPackageB {
		if packagePartsConflict(packageA, packageB) {
			return false
		}
		if packagePartsMatch(packageA, packageB) {
			return true
		}
	}
	pathA, pathB := normalizeExecutable(a.Executable), normalizeExecutable(b.Executable)
	return pathA != "" && pathA == pathB
}

// identityEqual remains the local name used by catalogService. Keep it as a
// small wrapper so all duplicate checks share the exported UI predicate.
func identityEqual(a, b domain.AppIdentity) bool { return SameApplication(a, b) }

func normalizeAUMID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= len("package:") && strings.EqualFold(value[:len("package:")], "package:") {
		value = strings.TrimSpace(value[len("package:"):])
	}
	return strings.ToLower(value)
}

func normalizePackagePart(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= len("package:") && strings.EqualFold(value[:len("package:")], "package:") {
		value = strings.TrimSpace(value[len("package:"):])
	}
	return strings.ToLower(value)
}

// packageParts keeps the stable package-family/name + app-id pieces separate
// so an incomplete legacy source can still fall back to its executable path.
type packageParts struct {
	family string
	name   string
	appID  string
}

func packagePartsOf(identity domain.AppIdentity) (packageParts, bool) {
	parts := packageParts{
		family: normalizePackagePart(identity.PackageFamily),
		name:   normalizePackagePart(identity.PackageName),
		appID:  normalizePackagePart(identity.ApplicationID),
	}
	if aumid := normalizeAUMID(identity.AUMID); aumid != "" {
		if separator := strings.IndexByte(aumid, '!'); separator > 0 && separator < len(aumid)-1 {
			if parts.family == "" {
				parts.family = normalizePackagePart(aumid[:separator])
			}
			if parts.appID == "" {
				parts.appID = normalizePackagePart(aumid[separator+1:])
			}
		} else if parts.family == "" && parts.name == "" && parts.appID == "" {
			// An opaque AUMID still identifies a package source, even though it
			// cannot be compared with split package fields.
			parts.family = aumid
		}
	}
	return parts, parts.family != "" || parts.name != "" || parts.appID != ""
}

func packagePartsConflict(a, b packageParts) bool {
	if a.family != "" && b.family != "" && a.family != b.family {
		return true
	}
	if a.name != "" && b.name != "" && a.name != b.name {
		return true
	}
	return a.appID != "" && b.appID != "" && a.appID != b.appID
}

func packagePartsMatch(a, b packageParts) bool {
	if packagePartsConflict(a, b) {
		return false
	}
	// A family name is stronger than a short package name. When both sources
	// expose the same family/name and both sources carry the same explicit app
	// id, they describe one packaged launch entry even if their executable
	// paths differ. With a missing app id we must fall back to the executable
	// path: package-level metadata alone cannot distinguish multiple entries.
	packageMatched := (a.family != "" && b.family != "" && a.family == b.family) ||
		(a.name != "" && b.name != "" && a.name == b.name) ||
		(a.family != "" && b.name != "" && a.family == b.name) ||
		(a.name != "" && b.family != "" && a.name == b.family)
	if !packageMatched {
		return false
	}
	return a.appID != "" && b.appID != "" && a.appID == b.appID
}

func normalizeExecutable(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	// Resolver inputs may use either slash style. filepath.Clean removes
	// redundant separators and dot segments while preserving relative paths
	// used by tests and legacy settings.
	value = strings.ReplaceAll(value, "/", string(filepath.Separator))
	value = filepath.Clean(value)
	if value == "." {
		return ""
	}
	return strings.ToLower(value)
}
