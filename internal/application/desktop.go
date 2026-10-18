package application

import (
	"context"
	"fmt"
	"proxy-start/internal/domain"
	"sync"
	"time"
)

type Dependencies struct {
	Store       SettingsStore
	Credentials CredentialStore
	Resolver    AppResolver
	Processes   ProcessController
	Probe       ProxyProbe
	Backend     ForcedBackend
	Adapters    []NativeAdapter
	Icons       AppIconProvider
	Platform    PlatformCapabilities
	Console     ConsoleLauncher
}

type Snapshot struct {
	Settings domain.Settings
	Runtimes map[string]domain.AppRuntime
	Backend  domain.BackendStatus
	Platform PlatformCapabilities
}

// Desktop is a thin UI facade; use cases and resource ownership live separately.
type Desktop struct {
	repository *settingsRepository
	profiles   *profileService
	catalog    *catalogService
	launcher   *launchService
	backend    ForcedBackend
	cancel     context.CancelFunc
	done       chan struct{}
	statusMu   sync.RWMutex
	status     domain.BackendStatus
	icons      AppIconProvider
	platform   PlatformCapabilities
	console    ConsoleLauncher
}

func NewDesktop(ctx context.Context, d Dependencies) (*Desktop, error) {
	if d.Store == nil || d.Credentials == nil || d.Resolver == nil || d.Processes == nil || d.Probe == nil || d.Backend == nil {
		return nil, fmt.Errorf("应用依赖未完整配置")
	}
	repository, err := newSettingsRepository(ctx, d.Store)
	if err != nil {
		return nil, err
	}
	profiles := &profileService{repository: repository, credentials: d.Credentials, probe: d.Probe}
	launcher := &launchService{repository: repository, profiles: profiles, resolver: d.Resolver, processes: d.Processes, selector: NewStrategySelector(d.Adapters...), forced: newForcedRuntime(d.Backend), runtimes: map[string]domain.AppRuntime{}, locks: map[string]*sync.Mutex{}}
	monitor, cancel := context.WithCancel(ctx)
	desktop := &Desktop{repository: repository, profiles: profiles, catalog: &catalogService{repository: repository, resolver: d.Resolver}, launcher: launcher, backend: d.Backend, cancel: cancel, done: make(chan struct{}), icons: d.Icons, platform: clonePlatform(d.Platform), console: d.Console}
	initialCtx, initialCancel := context.WithTimeout(ctx, 5*time.Second)
	launcher.discoverConfigured(initialCtx)
	initialCancel()
	go desktop.monitor(monitor)
	return desktop, nil
}

func (d *Desktop) monitor(ctx context.Context) {
	defer close(d.done)
	tick := time.NewTicker(3 * time.Second)
	defer tick.Stop()
	for {
		status := d.backend.Status(ctx)
		d.statusMu.Lock()
		d.status = status
		d.statusMu.Unlock()
		pollCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		d.launcher.poll(pollCtx)
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (d *Desktop) Snapshot() Snapshot {
	d.statusMu.RLock()
	status := d.status
	d.statusMu.RUnlock()
	return Snapshot{Settings: d.repository.snapshot(), Runtimes: d.launcher.snapshot(), Backend: status, Platform: clonePlatform(d.platform)}
}
func (d *Desktop) AddApp(ctx context.Context, input string) (domain.AppEntry, error) {
	entry, err := d.catalog.add(ctx, input)
	if err != nil {
		return domain.AppEntry{}, err
	}
	d.launcher.discoverApp(ctx, entry.ID)
	return entry, nil
}
func (d *Desktop) SaveApp(ctx context.Context, entry domain.AppEntry) error {
	if err := d.catalog.save(ctx, entry); err != nil {
		return err
	}
	d.launcher.discoverApp(ctx, entry.ID)
	return nil
}
func (d *Desktop) RemoveApp(ctx context.Context, id string) error {
	return d.launcher.withOperation(func() error {
		lock := d.launcher.appLock(id)
		lock.Lock()
		defer lock.Unlock()
		if state := d.launcher.snapshot()[id]; state.State == domain.StateRunning || state.State == domain.StateStarting {
			return fmt.Errorf("请先结束程序，再移除启动项")
		}
		return d.catalog.remove(ctx, id)
	})
}
func (d *Desktop) SaveProfile(ctx context.Context, p domain.ProxyProfile, password string) error {
	return d.profiles.save(ctx, p, password)
}

// SaveProfileResult is the ID-preserving variant used by modal editors. It
// returns the exact persisted profile, including a generated ID for new rows.
func (d *Desktop) SaveProfileResult(ctx context.Context, p domain.ProxyProfile, password string) (domain.ProxyProfile, error) {
	return d.profiles.saveResult(ctx, p, password)
}
func (d *Desktop) DeleteProfile(ctx context.Context, id string) error {
	return d.profiles.delete(ctx, id)
}
func (d *Desktop) ClearCredential(ctx context.Context, id string) error {
	return d.profiles.clearCredential(ctx, id)
}
func (d *Desktop) SetDefaultProxy(ctx context.Context, id string) error {
	return d.repository.change(ctx, func(s *domain.Settings) error {
		for _, profile := range s.Profiles {
			if profile.ID == id {
				s.DefaultProxyID = id
				return nil
			}
		}
		return ErrNotFound
	})
}
func (d *Desktop) SetTheme(ctx context.Context, theme string) error {
	if theme != "system" && theme != "light" && theme != "dark" {
		return fmt.Errorf("不支持的主题")
	}
	return d.repository.change(ctx, func(s *domain.Settings) error { s.Theme = theme; return nil })
}

// SetHideSystemApps persists the installed-app catalog filter. The persisted
// setting is phrased positively (ShowSystemApps) so older JSON files can
// safely omit it and retain the default of hiding system entries.
func (d *Desktop) SetHideSystemApps(ctx context.Context, hide bool) error {
	return d.repository.change(ctx, func(s *domain.Settings) error {
		s.ShowSystemApps = !hide
		return nil
	})
}
func (d *Desktop) TestProfile(ctx context.Context, id string) (domain.ProbeResult, error) {
	return d.profiles.test(ctx, id)
}

// OpenProxyConsole opens an independent shell whose proxy variables are
// limited to that shell and its descendants. Auto protocol detection only
// accepts verified probe results; an unverified guess is never presented as a
// successful console launch.
func (d *Desktop) OpenProxyConsole(ctx context.Context, id string) error {
	if d.console == nil {
		return fmt.Errorf("当前平台不支持代理控制台")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	profile, err := d.repository.profile(id)
	if err != nil {
		return err
	}
	endpoint, err := d.profiles.endpoint(ctx, profile)
	if err != nil {
		return err
	}
	protocol := endpoint.Protocol
	if protocol == domain.ProtocolAuto {
		probe, probeErr := d.profiles.probe.Test(ctx, endpoint)
		if probeErr != nil {
			return fmt.Errorf("自动检测代理协议失败：%w", probeErr)
		}
		switch {
		case probe.HTTPVerified:
			protocol = domain.ProtocolHTTP
		case probe.SOCKS5Verified:
			protocol = domain.ProtocolSOCKS5
		default:
			return fmt.Errorf("未验证出可用的 HTTP 或 SOCKS5 代理")
		}
	}
	environment, unset, err := BuildProxyEnvironment(endpoint, protocol)
	if err != nil {
		return err
	}
	return d.console.Open(ctx, ConsoleSpec{ProfileName: profile.Name, Environment: environment, UnsetEnvironment: unset})
}
func (d *Desktop) Start(ctx context.Context, id string) error   { return d.launcher.start(ctx, id) }
func (d *Desktop) Stop(ctx context.Context, id string) error    { return d.launcher.stop(ctx, id) }
func (d *Desktop) Restart(ctx context.Context, id string) error { return d.launcher.restart(ctx, id) }
func (d *Desktop) Installed(ctx context.Context) ([]domain.AppIdentity, error) {
	installed, err := d.catalog.resolver.Installed(ctx)
	if err != nil {
		return nil, err
	}
	if d.repository.snapshot().ShowSystemApps {
		return installed, nil
	}
	filtered := make([]domain.AppIdentity, 0, len(installed))
	for _, identity := range installed {
		if identity.SystemApp {
			continue
		}
		filtered = append(filtered, identity)
	}
	return filtered, nil
}

func (d *Desktop) Icon(ctx context.Context, id string) ([]byte, error) {
	entry, err := d.repository.app(id)
	if err != nil {
		return nil, err
	}
	return d.IconForIdentity(ctx, entry.Identity)
}

// IconForIdentity returns an application icon without changing the launch
// catalog. This is used for identities discovered by the installed-app picker
// before the user chooses to add one. Keeping the provider behind this facade
// prevents the UI from depending on platform-specific icon APIs.
func (d *Desktop) IconForIdentity(ctx context.Context, identity domain.AppIdentity) ([]byte, error) {
	if d.icons == nil {
		return nil, nil
	}
	return d.icons.Icon(ctx, identity)
}
func (d *Desktop) InstallBackend(ctx context.Context) error { return d.backend.Install(ctx) }
func (d *Desktop) Shutdown(ctx context.Context) error {
	d.cancel()
	select {
	case <-d.done:
	case <-ctx.Done():
		return ctx.Err()
	}
	return d.launcher.shutdown(ctx)
}
