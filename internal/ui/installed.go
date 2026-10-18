package ui

import (
	"context"
	"sort"
	"strings"

	nativeui "github.com/egoist/mygo/ui"
	"proxy-start/internal/application"
	"proxy-start/internal/domain"
)

// installedPickerState owns the dialog's transient query, list and icon work.
// None of these fields are persisted or shared with the launch-page search.
type installedPickerState struct {
	open, loading bool
	err, query    string
	items         []domain.AppIdentity
	list          nativeui.ListState
	generation    uint64
	cancel        context.CancelFunc
	ctx           context.Context
	icons         map[string]*nativeui.Bitmap
	attempts      map[string]bool
	iconSlots     chan struct{}
}

func (m *model) closeInstalled() {
	p := &m.installed
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.open, p.loading = false, false
	p.generation++
}

func (m *model) loadInstalled() {
	p := &m.installed
	if p.loading {
		return
	}
	m.closeInstalled()
	ctx, cancel := context.WithCancel(context.Background())
	p.ctx, p.cancel = ctx, cancel
	p.open, p.loading, p.err, p.query = true, true, "", ""
	p.items = nil
	p.list = nativeui.ListState{}
	p.attempts = make(map[string]bool)
	if m.desktop == nil || m.win == nil {
		p.loading, p.err = false, "当前环境无法读取已安装应用"
		return
	}
	generation := p.generation
	go func() {
		loadCtx, stop := context.WithTimeout(ctx, operationTimeout)
		items, err := m.desktop.Installed(loadCtx)
		stop()
		if ctx.Err() != nil || m.quitting.Load() {
			return
		}
		sort.SliceStable(items, func(i, j int) bool {
			return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
		})
		m.win.Update(func() {
			if !p.open || p.generation != generation || ctx.Err() != nil {
				return
			}
			p.loading = false
			if err != nil {
				p.err = err.Error()
				return
			}
			p.items = items
		})
	}()
}

func (m *model) setInstalledSearch(query string) {
	if query != m.installed.query {
		m.installed.query = query
		m.installed.list.ScrollTo(0, nativeui.Start)
	}
}

func filteredInstalledApps(items []domain.AppIdentity, query string) []domain.AppIdentity {
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return items
	}
	filtered := make([]domain.AppIdentity, 0)
	for _, item := range items {
		text := strings.ToLower(strings.Join([]string{item.Name, item.Executable, item.PackageName, item.PackageFamily, item.ApplicationID, item.AUMID}, "\n"))
		matches := true
		for _, term := range terms {
			if !strings.Contains(text, term) {
				matches = false
				break
			}
		}
		if matches {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func installedIdentityKey(identity domain.AppIdentity) string {
	return strings.ToLower(strings.Join([]string{identity.Executable, identity.AUMID, identity.PackageName, identity.PackageFamily, identity.ApplicationID}, "\x00"))
}

func (m *model) installedAlreadyAdded(identity domain.AppIdentity) bool {
	for _, entry := range m.snapshot.Settings.Apps {
		if application.SameApplication(entry.Identity, identity) {
			return true
		}
	}
	return false
}
