package proxifyre

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"proxy-start/internal/domain"
)

const (
	defaultServiceName = "ProxiFyreService"
	defaultPipeTimeout = 15 * time.Second
)

// Options contains platform seams for the forced backend. The composition
// root supplies the elevated helper path and the HTTP bridge factory; neither
// the UI nor application layer needs to know how either is implemented.
type Options struct {
	HelperPath           string
	HelperSHA256         string
	EnginePath           string
	ServiceName          string
	OwnerExecutable      string
	AdditionalExclusions []string
	BridgeFactory        BridgeFactory
	PipeTimeout          time.Duration
	InstallerPath        string
	InstallerSHA256      string
	InstallerURL         string
	Logger               func(string, ...any)
}

// Backend coordinates one elevated ProxiFyre helper and the bridges shared by
// all active HTTP profiles. Apply is transactional from the caller's point of
// view: bridge resources are created first, then the helper receives a complete
// desired session set. A failed helper call closes resources created for the
// failed generation.
type Backend struct {
	operation       chan struct{}
	stateMu         sync.RWMutex
	opts            Options
	helper          helperTransport
	bridges         map[string]Bridge
	bridgeEndpoints map[string]string
	status          domain.BackendStatus
	active          []domain.ForcedSession
	bridgeCtx       context.Context
}

var _ interface {
	Apply(context.Context, []domain.ForcedSession) error
	Shutdown(context.Context) error
	Status(context.Context) domain.BackendStatus
	Install(context.Context) error
} = (*Backend)(nil)

func NewBackend(options Options) *Backend {
	if options.ServiceName == "" {
		options.ServiceName = defaultServiceName
	}
	if options.PipeTimeout <= 0 {
		options.PipeTimeout = defaultPipeTimeout
	}
	if strings.TrimSpace(options.EnginePath) == "" {
		options.EnginePath = defaultEnginePath()
	}
	if options.Logger == nil {
		options.Logger = func(string, ...any) {}
	}
	// Bridge lifetimes are owned by Backend.Shutdown/Close, never by a
	// cancellable Apply context. A background context also ensures a UAC wait
	// cannot unexpectedly tear down an active HTTP bridge.
	bridgeCtx := context.Background()
	return &Backend{
		operation:       make(chan struct{}, 1),
		opts:            options,
		bridges:         make(map[string]Bridge),
		bridgeEndpoints: make(map[string]string),
		status:          initialStatus(options),
		bridgeCtx:       bridgeCtx,
	}
}

func (b *Backend) Apply(ctx context.Context, sessions []domain.ForcedSession) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := b.acquire(ctx); err != nil {
		return err
	}
	defer b.release()
	if err := validateSessions(sessions); err != nil {
		b.setStatus(domain.BackendStatus{Detail: err.Error()})
		return err
	}
	if len(sessions) > 0 && strings.TrimSpace(b.opts.HelperPath) == "" {
		err := fmt.Errorf("%w: configure the proxy-start-helper executable first", ErrComponentMissing)
		b.setStatus(domain.BackendStatus{Detail: err.Error()})
		return err
	}

	workingBridges, endpoints, err := b.prepareBridges(ctx, sessions)
	if err != nil {
		b.setStatus(domain.BackendStatus{Detail: err.Error()})
		return err
	}
	config, err := makeConfig(sessions, b.exclusions(), endpoints)
	if err != nil {
		closeNewBridges(workingBridges, b.bridges)
		b.setStatus(domain.BackendStatus{Detail: err.Error()})
		return err
	}
	helperSessions, err := makeHelperSessions(sessions, endpoints)
	if err != nil {
		closeNewBridges(workingBridges, b.bridges)
		b.setStatus(domain.BackendStatus{Detail: err.Error()})
		return err
	}
	_, digest, err := marshalConfig(config)
	if err != nil {
		closeNewBridges(workingBridges, b.bridges)
		return err
	}

	transport, err := b.ensureHelper(ctx)
	if err != nil {
		closeNewBridges(workingBridges, b.bridges)
		b.setStatus(domain.BackendStatus{Detail: err.Error()})
		return err
	}
	request := helperApplyRequest{Sessions: helperSessions, Config: config, ConfigSHA256: digest}
	result, err := transport.Apply(ctx, request)
	if err != nil {
		b.discardHelper(transport)
		closeNewBridges(workingBridges, b.bridges)
		b.setStatus(domain.BackendStatus{Detail: err.Error()})
		return err
	}
	if !result.OK {
		closeNewBridges(workingBridges, b.bridges)
		err = helperError(result)
		b.setStatus(domain.BackendStatus{Detail: err.Error()})
		return err
	}

	closeReplacedBridges(b.bridges, workingBridges)
	b.bridges = workingBridges
	b.bridgeEndpoints = endpoints
	b.active = cloneSessions(sessions)
	b.setStatus(domain.BackendStatus{Installed: true, Ready: true, Running: len(sessions) > 0, Detail: result.Detail})
	return nil
}

func (b *Backend) Shutdown(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := b.acquire(ctx); err != nil {
		return err
	}
	defer b.release()
	var err error
	b.stateMu.RLock()
	helper := b.helper
	bridges := b.bridges
	b.stateMu.RUnlock()
	if helper != nil {
		if result, callErr := helper.Shutdown(ctx); callErr != nil {
			err = callErr
		} else if !result.OK {
			err = helperError(result)
		}
		_ = helper.Close()
		b.stateMu.Lock()
		b.helper = nil
		b.stateMu.Unlock()
	}
	closeBridges(bridges)
	b.stateMu.Lock()
	installed := b.status.Installed
	b.bridges = make(map[string]Bridge)
	b.bridgeEndpoints = make(map[string]string)
	b.active = nil
	b.status = domain.BackendStatus{Installed: installed, Ready: false, Running: false, Detail: "ProxiFyre 已停止"}
	b.stateMu.Unlock()
	return err
}

func (b *Backend) Status(ctx context.Context) domain.BackendStatus {
	if ctx == nil {
		ctx = context.Background()
	}
	probeCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	status := statusFor(probeCtx, b.opts)
	b.stateMu.Lock()
	// Keep a locally applied session flag when the service query is not
	// available during a short status refresh.
	if len(b.active) > 0 && b.status.Running && !status.Running {
		status.Running = true
	}
	b.status = status
	b.stateMu.Unlock()
	return status
}

func (b *Backend) Install(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := b.acquire(ctx); err != nil {
		return err
	}
	defer b.release()
	spec := OfficialDependency()
	if b.opts.InstallerURL != "" {
		spec.URL = b.opts.InstallerURL
	}
	if b.opts.InstallerSHA256 != "" {
		spec.SHA256 = b.opts.InstallerSHA256
	}
	path := strings.TrimSpace(b.opts.InstallerPath)
	var err error
	if path == "" {
		path, err = downloadPinned(ctx, spec)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrComponentMissing, err)
		}
	} else if b.opts.InstallerSHA256 == "" && !strings.EqualFold(filepath.Base(path), spec.Name) {
		return fmt.Errorf("%w: provide the SHA-256 pin for a custom ProxiFyre installer", ErrComponentMissing)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("%w: installer %q is unavailable: %v", ErrComponentMissing, path, err)
	}
	if err := verifySHA256(ctx, path, spec.SHA256); err != nil {
		return err
	}
	if err := launchInstaller(ctx, path); err != nil {
		return err
	}
	b.setStatus(statusFor(ctx, b.opts))
	return nil
}

func (b *Backend) acquire(ctx context.Context) error {
	select {
	case b.operation <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *Backend) release() { <-b.operation }

func (b *Backend) setStatus(status domain.BackendStatus) {
	b.stateMu.Lock()
	b.status = status
	b.stateMu.Unlock()
}

func (b *Backend) discardHelper(transport helperTransport) {
	b.stateMu.Lock()
	b.helper = nil
	b.stateMu.Unlock()
	_ = transport.Close()
}

func (b *Backend) exclusions() []string {
	values := append([]string{}, b.opts.AdditionalExclusions...)
	for _, value := range []string{b.opts.OwnerExecutable, b.opts.HelperPath, b.opts.EnginePath} {
		if strings.TrimSpace(value) != "" {
			values = append(values, value)
		}
	}
	return values
}

func validateSessions(sessions []domain.ForcedSession) error {
	seen := make(map[string]struct{}, len(sessions))
	paths := make(map[string]string, len(sessions))
	for _, session := range sessions {
		if _, ok := seen[session.AppID]; ok {
			return fmt.Errorf("%w: duplicate app id %q", ErrInvalidSession, session.AppID)
		}
		seen[session.AppID] = struct{}{}
		if err := validateSession(session); err != nil {
			return err
		}
		endpoint := endpointKey(session.Proxy)
		for _, path := range explicitPaths(session.ExecutablePaths) {
			key := strings.ToLower(path)
			if prior, ok := paths[key]; ok && prior != endpoint {
				return fmt.Errorf("%w: executable path %q is assigned to multiple proxy endpoints", ErrInvalidSession, path)
			}
			paths[key] = endpoint
		}
	}
	return nil
}

func (b *Backend) prepareBridges(ctx context.Context, sessions []domain.ForcedSession) (map[string]Bridge, map[string]string, error) {
	bridges := make(map[string]Bridge)
	endpoints := make(map[string]string)
	created := make(map[string]Bridge)
	for _, session := range sessions {
		if session.Proxy.Protocol != domain.ProtocolHTTP {
			continue
		}
		if b.opts.BridgeFactory == nil {
			return nil, nil, fmt.Errorf("%w: HTTP profile requires the local SOCKS5 bridge component", ErrComponentMissing)
		}
		key := endpointKey(session.Proxy)
		if _, ok := endpoints[key]; ok {
			continue
		}
		b.stateMu.RLock()
		bridge := b.bridges[key]
		knownAddress := b.bridgeEndpoints[key]
		b.stateMu.RUnlock()
		if bridge != nil {
			if address := knownAddress; address != "" {
				bridges[key] = bridge
				endpoints[key] = address
				continue
			}
		}
		if bridge == nil {
			bridge = b.opts.BridgeFactory(session.Proxy)
		}
		if bridge == nil {
			return nil, nil, fmt.Errorf("%w: bridge factory returned nil for %s", ErrComponentMissing, session.Proxy.Address())
		}
		address, err := bridge.Start(b.bridgeCtx)
		if err != nil {
			closeBridges(created)
			return nil, nil, fmt.Errorf("start HTTP bridge for %s: %w", session.Proxy.Address(), err)
		}
		if !validProxyEndpoint(address) {
			_ = bridge.Close()
			closeBridges(created)
			return nil, nil, fmt.Errorf("HTTP bridge returned invalid SOCKS5 address %q", address)
		}
		bridges[key] = bridge
		endpoints[key] = address
		created[key] = bridge
	}
	return bridges, endpoints, nil
}

func closeReplacedBridges(old, current map[string]Bridge) {
	for key, bridge := range old {
		if _, ok := current[key]; !ok && bridge != nil {
			_ = bridge.Close()
		}
	}
}

func closeBridges(bridges map[string]Bridge) {
	for _, bridge := range bridges {
		if bridge != nil {
			_ = bridge.Close()
		}
	}
}

func closeNewBridges(current, previous map[string]Bridge) {
	for key, bridge := range current {
		if _, existed := previous[key]; !existed && bridge != nil {
			_ = bridge.Close()
		}
	}
}

func cloneSessions(sessions []domain.ForcedSession) []domain.ForcedSession {
	copyOf := make([]domain.ForcedSession, len(sessions))
	copy(copyOf, sessions)
	for i := range copyOf {
		copyOf[i].ExecutablePaths = append([]string(nil), sessions[i].ExecutablePaths...)
	}
	return copyOf
}
