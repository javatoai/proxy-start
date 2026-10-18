package application

import (
	"context"
	"proxy-start/internal/domain"
	"sort"
	"sync"
)

// forcedRuntime owns the shared engine. A program lease changes the complete
// configuration atomically, never starts another interception engine.
type forcedRuntime struct {
	mu       sync.Mutex
	backend  ForcedBackend
	sessions map[string]domain.ForcedSession
}

func newForcedRuntime(backend ForcedBackend) *forcedRuntime {
	return &forcedRuntime{backend: backend, sessions: map[string]domain.ForcedSession{}}
}

func (r *forcedRuntime) acquire(ctx context.Context, session domain.ForcedSession) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	next := r.copySessions()
	next[session.AppID] = cloneSession(session)
	if err := r.backend.Apply(ctx, sessionList(next)); err != nil {
		return err
	}
	r.sessions = next
	return nil
}

func (r *forcedRuntime) release(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sessions[id]; !ok {
		return nil
	}
	next := r.copySessions()
	delete(next, id)
	var err error
	if len(next) == 0 {
		err = r.backend.Shutdown(ctx)
	} else {
		err = r.backend.Apply(ctx, sessionList(next))
	}
	if err == nil {
		r.sessions = next
	}
	return err
}

func (r *forcedRuntime) shutdown(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	err := r.backend.Shutdown(ctx)
	if err == nil {
		r.sessions = map[string]domain.ForcedSession{}
	}
	return err
}

func (r *forcedRuntime) copySessions() map[string]domain.ForcedSession {
	next := make(map[string]domain.ForcedSession, len(r.sessions))
	for k, v := range r.sessions {
		next[k] = cloneSession(v)
	}
	return next
}

func cloneSession(session domain.ForcedSession) domain.ForcedSession {
	session.ExecutablePaths = append([]string(nil), session.ExecutablePaths...)
	return session
}

func sessionList(sessions map[string]domain.ForcedSession) []domain.ForcedSession {
	keys := make([]string, 0, len(sessions))
	for k := range sessions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]domain.ForcedSession, 0, len(keys))
	for _, k := range keys {
		out = append(out, cloneSession(sessions[k]))
	}
	return out
}
