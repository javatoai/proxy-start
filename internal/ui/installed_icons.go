package ui

import (
	"context"
	"time"

	nativeui "github.com/egoist/mygo/ui"
	"proxy-start/internal/domain"
)

const installedIconRequestTimeout = 10 * time.Second

// submitInstalledIconRequest starts one icon request when a slot is available.
// It deliberately does not queue work: rows outside the visible virtual list
// must not create a backlog that runs after the user has scrolled away. The
// callback is always handed to dispatch so callers can keep UI mutations on
// the MyGO window thread.
func submitInstalledIconRequest(
	ctx context.Context,
	slots chan struct{},
	identity domain.AppIdentity,
	fetch func(context.Context, domain.AppIdentity) ([]byte, error),
	dispatch func(func()),
	done func([]byte, error),
) bool {
	if ctx == nil || ctx.Err() != nil || slots == nil || fetch == nil || dispatch == nil || done == nil {
		return false
	}
	select {
	case slots <- struct{}{}:
	default:
		return false
	}
	go func() {
		data, err := fetch(ctx, identity)
		// Release before dispatching the UI callback. A newly invalidated
		// virtual-list frame can therefore claim this slot immediately.
		<-slots
		dispatch(func() { done(data, err) })
	}()
	return true
}

// tryStartInstalledIconRequest records an attempt only after the request has
// actually acquired a slot. A full semaphore is a temporary scheduling
// condition, so leaving the key marked in that case would permanently skip
// that row's icon.
func tryStartInstalledIconRequest(attempts map[string]bool, key string, start func() bool) bool {
	if attempts == nil || attempts[key] || start == nil || !start() {
		return false
	}
	attempts[key] = true
	return true
}

// installedAppIcon paints the icon for one visible installed-app row. The
// caller invokes it while rendering the virtual list, so requests are only
// started for rows that are actually on screen.
func (m *model) installedAppIcon(c *nativeui.Context, identity domain.AppIdentity) {
	t := c.Theme()
	key := installedIdentityKey(identity)
	if bitmap := m.installed.icons[key]; bitmap != nil {
		nativeui.Image(c, bitmap).Size(32, 32).Shrink(0).Fit(nativeui.Contain).Label(identity.Name + " 图标")
		return
	}

	nativeui.Row(c).Size(32, 32).Shrink(0).Radius(8).Background(t.Surface).Center().Children(func() {
		nativeui.Icon(c, appGlyph).Size(18, 18).TextColor(t.TextMuted)
	})

	// Rendering can happen during teardown or before the window has been
	// attached. In both cases the neutral placeholder above is the complete
	// result and no background work should be started.
	if m.desktop == nil || m.win == nil || m.quitting.Load() {
		return
	}
	if m.installed.ctx == nil || m.installed.cancel == nil {
		return
	}
	if m.installed.attempts == nil {
		m.installed.attempts = make(map[string]bool)
	}
	if m.installed.attempts[key] {
		return
	}
	if m.installed.icons == nil {
		m.installed.icons = make(map[string]*nativeui.Bitmap)
	}
	if m.installed.iconSlots == nil {
		m.installed.iconSlots = make(chan struct{}, 4)
	}
	generation := m.installed.generation
	parentCtx := m.installed.ctx
	requestCtx, cancel := context.WithTimeout(parentCtx, installedIconRequestTimeout)
	fetch := func(ctx context.Context, item domain.AppIdentity) ([]byte, error) {
		return m.desktop.IconForIdentity(ctx, item)
	}
	dispatch := func(fn func()) {
		if m.win != nil {
			m.win.Update(fn)
		}
	}
	done := func(data []byte, err error) {
		defer cancel()
		m.acceptInstalledIcon(requestCtx, generation, key, data, err)
	}
	started := tryStartInstalledIconRequest(m.installed.attempts, key, func() bool {
		return submitInstalledIconRequest(requestCtx, m.installed.iconSlots, identity, fetch, dispatch, done)
	})
	if !started {
		cancel()
	}
}

// acceptInstalledIcon runs on the window thread, including after asynchronous
// decoding inputs arrive. Stale or cancelled sessions cannot update the cache.
func (m *model) acceptInstalledIcon(ctx context.Context, generation uint64, key string, data []byte, err error) {
	if ctx == nil || ctx.Err() != nil || m.quitting.Load() || !m.installed.open || generation != m.installed.generation || err != nil || len(data) == 0 {
		return
	}
	bitmap, decodeErr := nativeui.DecodeBitmap(data)
	if decodeErr != nil {
		return
	}
	if m.installed.icons == nil {
		m.installed.icons = make(map[string]*nativeui.Bitmap)
	}
	m.installed.icons[key] = bitmap
}
