package application

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"proxy-start/internal/domain"
	"strings"
	"sync"
	"time"
)

type launchService struct {
	repository *settingsRepository
	profiles   *profileService
	resolver   AppResolver
	processes  ProcessController
	selector   *StrategySelector
	forced     *forcedRuntime
	// lifecycle is held for the entire duration of every public operation.
	// Shutdown takes the write lock, so no operation can start after the
	// backend teardown barrier has been acquired.
	lifecycle   sync.RWMutex
	mu          sync.RWMutex
	runtimes    map[string]domain.AppRuntime
	locks       map[string]*sync.Mutex
	closed      bool
	shutdownErr error
}

func (s *launchService) appLock(id string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.locks[id] == nil {
		s.locks[id] = &sync.Mutex{}
	}
	return s.locks[id]
}

func (s *launchService) set(state domain.AppRuntime) {
	state.UpdatedAt = time.Now()
	state.Processes = append([]domain.ProcessHandle(nil), state.Processes...)
	state.EffectiveIdentity = cloneIdentity(state.EffectiveIdentity)
	state.RelatedExecutables = append([]string(nil), state.RelatedExecutables...)
	s.mu.Lock()
	s.runtimes[state.AppID] = state
	s.mu.Unlock()
}

func (s *launchService) snapshot() map[string]domain.AppRuntime {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]domain.AppRuntime, len(s.runtimes))
	for k, v := range s.runtimes {
		v.Processes = append([]domain.ProcessHandle(nil), v.Processes...)
		v.EffectiveIdentity.Arguments = append([]string(nil), v.EffectiveIdentity.Arguments...)
		v.RelatedExecutables = append([]string(nil), v.RelatedExecutables...)
		out[k] = v
	}
	return out
}

func (s *launchService) withOperation(fn func() error) error {
	s.lifecycle.RLock()
	defer s.lifecycle.RUnlock()
	s.mu.RLock()
	closed := s.closed
	s.mu.RUnlock()
	if closed {
		return ErrClosed
	}
	return fn()
}

func (s *launchService) start(ctx context.Context, id string) error {
	return s.withOperation(func() error {
		lock := s.appLock(id)
		lock.Lock()
		defer lock.Unlock()
		return s.startLocked(ctx, id)
	})
}

func (s *launchService) startLocked(ctx context.Context, id string) (err error) {
	s.mu.RLock()
	closed := s.closed
	s.mu.RUnlock()
	if closed {
		return ErrClosed
	}
	entry, err := s.repository.app(id)
	if err != nil {
		return err
	}
	identity := entry.Identity
	if entry.Source != "" {
		identity, err = s.resolver.Resolve(ctx, entry.Source)
		if err != nil {
			return err
		}
	}
	related := cleanRelatedExecutables(entry.RelatedExecutables)
	existing, err := s.findProcesses(ctx, identity, related)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		state := s.snapshot()[id]
		state.AppID, state.State, state.Processes = id, domain.StateRunning, existing
		state.EffectiveIdentity = cloneIdentity(identity)
		state.RelatedExecutables = append([]string(nil), related...)
		state.Detail = ErrAlreadyRunning.Error()
		s.set(state)
		return ErrAlreadyRunning
	}
	failureState := domain.AppRuntime{
		AppID:              id,
		State:              domain.StateStarting,
		Detail:             "正在检测代理并准备启动",
		EffectiveIdentity:  cloneIdentity(identity),
		RelatedExecutables: append([]string(nil), related...),
	}
	s.set(failureState)
	defer func() {
		if err != nil {
			failureState.State = domain.StateFailed
			failureState.Error = err.Error()
			if failureState.Detail == "正在检测代理并准备启动" {
				failureState.Detail = "启动未完成"
			}
			s.set(failureState)
		}
	}()
	profile, err := s.repository.profile(entry.ProxyID)
	if err != nil {
		return err
	}
	endpoint, err := s.profiles.endpoint(ctx, profile)
	if err != nil {
		return err
	}
	cap, err := s.profiles.probe.Test(ctx, endpoint)
	if err != nil {
		return fmt.Errorf("代理检测失败：%w", err)
	}
	plan, err := s.selector.Select(identity, endpoint, cap)
	if err != nil {
		return err
	}
	failureState.Mode = plan.Mode
	failureState.AdapterID = plan.AdapterID
	if plan.Mode == domain.ModeForced {
		paths := append([]string{identity.Executable}, related...)
		if err = s.forced.acquire(ctx, domain.ForcedSession{AppID: id, ExecutablePaths: paths, Proxy: plan.Proxy}); err != nil {
			return err
		}
		failureState.ProxyApplied = true
	}
	handle, err := s.processes.Start(ctx, plan.Spec)
	if err != nil {
		if plan.Mode == domain.ModeForced {
			cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			cleanupErr := s.forced.release(cleanup, id)
			cancel()
			if cleanupErr != nil {
				failureState.CleanupPending = true
				failureState.Detail = "启动失败，代理规则清理待重试"
				// Keep ProxyApplied true while the lease remains committed. The
				// monitor will retry release even though the process never started.
				err = errors.Join(err, fmt.Errorf("代理规则清理失败：%w", cleanupErr))
			} else {
				failureState.ProxyApplied = false
			}
		}
		return err
	}
	s.set(domain.AppRuntime{
		AppID:              id,
		State:              domain.StateRunning,
		Mode:               plan.Mode,
		AdapterID:          plan.AdapterID,
		Detail:             plan.Reason,
		ProxyApplied:       plan.Mode == domain.ModeForced || plan.Mode == domain.ModeNative,
		ConnectionVerified: false,
		Processes:          []domain.ProcessHandle{handle},
		EffectiveIdentity:  cloneIdentity(identity),
		RelatedExecutables: append([]string(nil), related...),
	})
	return nil
}

func (s *launchService) stop(ctx context.Context, id string) error {
	return s.withOperation(func() error {
		lock := s.appLock(id)
		lock.Lock()
		defer lock.Unlock()
		return s.stopLocked(ctx, id)
	})
}

func (s *launchService) stopLocked(ctx context.Context, id string) error {
	entry, err := s.repository.app(id)
	if err != nil {
		return err
	}
	state := s.snapshot()[id]
	identity := state.EffectiveIdentity
	if identity.Executable == "" {
		identity = entry.Identity
		if entry.Source != "" {
			identity, err = s.resolver.Resolve(ctx, entry.Source)
			if err != nil {
				return err
			}
		}
	}
	related := append([]string(nil), state.RelatedExecutables...)
	if len(related) == 0 {
		related = cleanRelatedExecutables(entry.RelatedExecutables)
	}
	if len(state.Processes) == 0 {
		state.Processes, err = s.findProcesses(ctx, identity, related)
		if err != nil {
			return err
		}
	}
	state.AppID = id
	state.EffectiveIdentity = cloneIdentity(identity)
	state.RelatedExecutables = append([]string(nil), related...)
	state.State = domain.StateStopping
	s.set(state)
	if err = s.stopProcesses(ctx, identity, related, state.Processes); err != nil {
		state.State = domain.StateRunning
		state.Error = err.Error()
		s.set(state)
		return err
	}
	if err = s.forced.release(ctx, id); err != nil {
		state.State = domain.StateStopped
		state.CleanupPending = true
		state.ProxyApplied = true
		state.Error = err.Error()
		state.Detail = "程序已结束，代理规则清理失败，请重试或退出客户端"
		s.set(state)
		return err
	}
	state.State = domain.StateStopped
	state.CleanupPending = false
	state.ProxyApplied = false
	state.Processes = nil
	state.Error = ""
	state.Detail = "程序已结束"
	s.set(state)
	return nil
}

func (s *launchService) restart(ctx context.Context, id string) error {
	return s.withOperation(func() error {
		lock := s.appLock(id)
		lock.Lock()
		defer lock.Unlock()
		if err := s.stopLocked(ctx, id); err != nil {
			return err
		}
		return s.startLocked(ctx, id)
	})
}

func (s *launchService) poll(ctx context.Context) {
	if err := s.withOperation(func() error {
		s.pollLocked(ctx)
		return nil
	}); err != nil {
		// A closed service is expected when the monitor is being stopped.
		return
	}
}

func (s *launchService) pollLocked(ctx context.Context) {
	for id, state := range s.snapshot() {
		lock := s.appLock(id)
		if !lock.TryLock() {
			continue
		}
		if state.CleanupPending {
			if err := s.forced.release(ctx, id); err == nil {
				state.CleanupPending = false
				state.ProxyApplied = false
				state.Error = ""
				state.Detail = "代理规则清理完成"
				s.set(state)
			} else {
				state.Error = err.Error()
				state.Detail = "代理规则清理待重试"
				s.set(state)
			}
			lock.Unlock()
			continue
		}
		if state.State != domain.StateRunning {
			lock.Unlock()
			continue
		}
		identity := state.EffectiveIdentity
		related := append([]string(nil), state.RelatedExecutables...)
		if identity.Executable == "" {
			if entry, err := s.repository.app(id); err == nil {
				identity = entry.Identity
				related = cleanRelatedExecutables(entry.RelatedExecutables)
				if entry.Source != "" {
					if resolved, resolveErr := s.resolver.Resolve(ctx, entry.Source); resolveErr == nil {
						identity = resolved
					}
				}
			}
		}
		alive := false
		for _, handle := range state.Processes {
			if s.processes.Alive(ctx, handle) {
				alive = true
				break
			}
		}
		if !alive {
			found, findErr := s.findProcesses(ctx, identity, related)
			if findErr != nil {
				lock.Unlock()
				continue
			}
			if len(found) > 0 {
				state.Processes = found
				state.EffectiveIdentity = cloneIdentity(identity)
				state.RelatedExecutables = append([]string(nil), related...)
				s.set(state)
				alive = true
			}
		}
		if !alive {
			if err := s.forced.release(ctx, id); err == nil {
				state.State = domain.StateStopped
				state.Processes = nil
				state.ProxyApplied = false
				state.CleanupPending = false
				state.Error = ""
				state.Detail = "程序已退出"
				s.set(state)
			} else {
				state.State = domain.StateStopped
				state.CleanupPending = true
				state.ProxyApplied = true
				state.Error = err.Error()
				state.Detail = "程序已退出，代理规则清理待重试"
				s.set(state)
			}
		}
		lock.Unlock()
	}
	// A process can have been started outside this client. Reconcile those
	// processes from the cached catalog identity without resolving the source
	// again. Source resolution is reserved for an explicit Start, while this
	// cheap pass keeps the launch list honest between monitor ticks.
	s.discoverConfiguredLocked(ctx)
}

// discoverConfigured performs the initial catalog reconciliation. It is also
// used after an item is added or edited so an already-running process appears
// immediately without applying proxy rules or terminating it.
func (s *launchService) discoverConfigured(ctx context.Context) {
	_ = s.withOperation(func() error {
		s.discoverConfiguredLocked(ctx)
		return nil
	})
}

func (s *launchService) discoverApp(ctx context.Context, id string) {
	_ = s.withOperation(func() error {
		entry, err := s.repository.app(id)
		if err != nil {
			return err
		}
		lock := s.appLock(id)
		lock.Lock()
		defer lock.Unlock()
		state, exists := s.snapshot()[id]
		if exists && (state.State != domain.StateStopped || state.CleanupPending) {
			return nil
		}
		s.discoverEntryLocked(ctx, entry)
		return nil
	})
}

func (s *launchService) discoverConfiguredLocked(ctx context.Context) {
	settings := s.repository.snapshot()
	for _, entry := range settings.Apps {
		state, exists := s.snapshot()[entry.ID]
		if exists && (state.State != domain.StateStopped || state.CleanupPending) {
			// Keep managed, transitional, and failed states intact. In
			// particular, discovery must not hide a cleanup error.
			continue
		}
		lock := s.appLock(entry.ID)
		if !lock.TryLock() {
			continue
		}
		current, currentExists := s.snapshot()[entry.ID]
		if currentExists && (current.State != domain.StateStopped || current.CleanupPending) {
			lock.Unlock()
			continue
		}
		s.discoverEntryLocked(ctx, entry)
		lock.Unlock()
	}
}

func (s *launchService) discoverEntryLocked(ctx context.Context, entry domain.AppEntry) {
	identity := cloneIdentity(entry.Identity)
	related := cleanRelatedExecutables(entry.RelatedExecutables)
	found, err := s.findProcesses(ctx, identity, related)
	if err != nil || len(found) == 0 {
		return
	}
	s.set(domain.AppRuntime{
		AppID:              entry.ID,
		State:              domain.StateRunning,
		Detail:             ErrAlreadyRunning.Error(),
		Processes:          found,
		EffectiveIdentity:  identity,
		RelatedExecutables: append([]string(nil), related...),
		ProxyApplied:       false,
		ConnectionVerified: false,
		CleanupPending:     false,
	})
}

func (s *launchService) findProcesses(ctx context.Context, identity domain.AppIdentity, related []string) ([]domain.ProcessHandle, error) {
	paths := append([]string{identity.Executable}, related...)
	result := make([]domain.ProcessHandle, 0)
	seen := make(map[string]struct{})
	for _, path := range cleanRelatedExecutables(paths) {
		candidate := identity
		candidate.Executable = path
		found, err := s.processes.Find(ctx, candidate)
		if err != nil {
			return nil, err
		}
		for _, handle := range found {
			key := fmt.Sprintf("%d|%s|%s", handle.PID, handle.StartedAt.UTC().Format(time.RFC3339Nano), strings.ToLower(filepath.Clean(handle.Executable)))
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			result = append(result, handle)
		}
	}
	return result, nil
}

func (s *launchService) stopProcesses(ctx context.Context, identity domain.AppIdentity, related []string, handles []domain.ProcessHandle) error {
	if len(handles) == 0 {
		return nil
	}
	root := make([]domain.ProcessHandle, 0, len(handles))
	groups := make(map[string][]domain.ProcessHandle)
	for _, handle := range handles {
		if sameExecutable(handle.Executable, identity.Executable) || strings.TrimSpace(handle.Executable) == "" {
			root = append(root, handle)
			continue
		}
		for _, path := range related {
			if sameExecutable(handle.Executable, path) {
				groups[filepath.Clean(strings.ToLower(path))] = append(groups[filepath.Clean(strings.ToLower(path))], handle)
				break
			}
		}
	}
	if len(root) > 0 {
		if err := s.processes.Stop(ctx, identity, root); err != nil {
			return err
		}
	}
	for path, grouped := range groups {
		candidate := identity
		candidate.Executable = path
		if err := s.processes.Stop(ctx, candidate, grouped); err != nil {
			return err
		}
	}
	return nil
}

func sameExecutable(a, b string) bool {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func cleanRelatedExecutables(paths []string) []string {
	result := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		clean := filepath.Clean(path)
		key := strings.ToLower(clean)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, clean)
	}
	return result
}

func cloneIdentity(identity domain.AppIdentity) domain.AppIdentity {
	identity.Arguments = append([]string(nil), identity.Arguments...)
	return identity
}

func (s *launchService) shutdown(ctx context.Context) error {
	// The write lock waits for every operation holding the read lock. Once it
	// is acquired, no new operation can create an app lock or touch the backend.
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	s.mu.Lock()
	if s.closed {
		err := s.shutdownErr
		s.mu.Unlock()
		return err
	}
	s.closed = true
	s.mu.Unlock()
	err := s.forced.shutdown(ctx)
	s.mu.Lock()
	s.shutdownErr = err
	s.mu.Unlock()
	return err
}
