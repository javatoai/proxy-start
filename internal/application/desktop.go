package application

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"proxy-start/internal/adapters/native"
	"proxy-start/internal/domain"
)

var ErrAlreadyRunning = errors.New("Codex 已启动，当前进程保持原样")
var ErrNotFound = errors.New("未找到 Codex 桌面程序，请安装后重新扫描")

type Dependencies struct {
	Store     SettingsStore
	Resolver  CodexResolver
	Processes ProcessController
	Console   ConsoleLauncher
	Platform  PlatformCapabilities
	CheckPort func(context.Context, int) error
}
type Snapshot struct {
	Settings     domain.Settings
	Identity     domain.AppIdentity
	Scanning     bool
	ScanError    string
	ScanNotFound bool
	State        string
	Proxy        domain.ProxyObservation
	RuntimeError string
	Platform     PlatformCapabilities
}

// Desktop observes Codex and changes its lifecycle only on explicit actions.
type Desktop struct {
	deps      Dependencies
	mu        sync.RWMutex
	operation sync.Mutex
	snapshot  Snapshot
	cancel    context.CancelFunc
	ctx       context.Context
	done      chan struct{}
	ready     chan struct{}
	cache     map[domain.ProcessHandle]domain.ProcessInfo
}

func NewDesktop(ctx context.Context, deps Dependencies) (*Desktop, error) {
	if deps.Store == nil || deps.Resolver == nil || deps.Processes == nil || deps.Console == nil {
		return nil, errors.New("应用依赖未完整配置")
	}
	settings, err := deps.Store.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("读取配置失败：%w", err)
	}
	if deps.CheckPort == nil {
		deps.CheckPort = CheckLocalPort
	}
	monitor, cancel := context.WithCancel(ctx)
	d := &Desktop{deps: deps, ctx: monitor, cancel: cancel, done: make(chan struct{}), ready: make(chan struct{}), cache: make(map[domain.ProcessHandle]domain.ProcessInfo),
		snapshot: Snapshot{Settings: settings, Scanning: true, State: "unknown", Platform: deps.Platform}}
	go d.monitor()
	return d, nil
}

func CheckLocalPort(ctx context.Context, port int) error {
	if err := domain.ValidatePort(port); err != nil {
		return err
	}
	connection, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return fmt.Errorf("无法连接本机代理端口 %d，请检查 Clash 是否运行及端口设置", port)
	}
	return connection.Close()
}

func (d *Desktop) Snapshot() Snapshot {
	d.mu.RLock()
	defer d.mu.RUnlock()
	s := d.snapshot
	s.Identity.Arguments = append([]string(nil), s.Identity.Arguments...)
	return s
}

func (d *Desktop) monitor() {
	defer close(d.done)
	ctx, cancel := context.WithTimeout(d.ctx, 15*time.Second)
	_ = d.ScanCodex(ctx)
	cancel()
	close(d.ready)
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(d.ctx, 5*time.Second)
			d.operation.Lock()
			if ctx.Err() == nil {
				_ = d.refresh(ctx, d.Snapshot().Identity)
			}
			d.operation.Unlock()
			cancel()
		}
	}
}

func (d *Desktop) operationContext(ctx context.Context) (context.Context, func()) {
	child, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(d.ctx, cancel)
	if d.ctx.Err() != nil {
		cancel()
	}
	return child, func() { stop(); cancel() }
}

func (d *Desktop) ScanCodex(ctx context.Context) error {
	d.operation.Lock()
	defer d.operation.Unlock()
	ctx, cancel := d.operationContext(ctx)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	d.mu.Lock()
	d.snapshot.Scanning = true
	d.mu.Unlock()
	identity, err := d.deps.Resolver.Discover(ctx)
	if err == nil && !native.IsCodex(identity) {
		err = ErrNotFound
	}
	d.mu.Lock()
	d.snapshot.Scanning, d.snapshot.ScanError = false, ""
	d.snapshot.ScanNotFound = errors.Is(err, ErrNotFound)
	if err != nil {
		d.snapshot.ScanError = err.Error()
		if d.snapshot.Identity.Executable == "" {
			d.snapshot.State = "unknown"
		}
	} else {
		d.snapshot.Identity = identity
	}
	d.mu.Unlock()
	if err != nil {
		return err
	}
	return d.refresh(ctx, identity)
}

// refresh is serialized with launches. Failed observations disable launching;
// they must never turn an inaccessible running process into a stopped one.
func (d *Desktop) refresh(ctx context.Context, identity domain.AppIdentity) error {
	if identity.Executable == "" {
		return nil
	}
	handles, err := d.deps.Processes.Find(ctx, identity)
	if err != nil {
		d.mu.Lock()
		d.snapshot.State, d.snapshot.RuntimeError = "unknown", "无法读取 Codex 运行状态，请重新扫描"
		d.snapshot.Proxy = domain.ProxyObservation{Kind: "unknown"}
		d.mu.Unlock()
		return err
	}
	if len(handles) == 0 {
		d.cache = make(map[domain.ProcessHandle]domain.ProcessInfo)
		d.mu.Lock()
		d.snapshot.State, d.snapshot.RuntimeError, d.snapshot.Proxy = "stopped", "", domain.ProxyObservation{}
		d.mu.Unlock()
		return nil
	}
	current := make(map[domain.ProcessHandle]domain.ProcessInfo)
	pids := make(map[int]bool)
	for _, handle := range handles {
		pids[handle.PID] = true
	}
	var roots []domain.ProxyObservation
	for _, handle := range handles {
		if pids[handle.ParentPID] {
			continue
		}
		info, cached := d.cache[handle]
		if !cached {
			var inspectErr error
			info, inspectErr = d.deps.Processes.Inspect(ctx, handle)
			if inspectErr != nil {
				info.Readable = false
			}
		}
		if info.Readable {
			current[handle] = info
		}
		if pids[info.ParentPID] || IsChromiumChild(info.Arguments) {
			continue
		}
		roots = append(roots, ObserveProxy(info))
	}
	d.cache = current
	d.mu.Lock()
	d.snapshot.State, d.snapshot.RuntimeError, d.snapshot.Proxy = "running", "", CombineProxyObservations(roots)
	d.mu.Unlock()
	return nil
}

func (d *Desktop) SaveProxyPort(ctx context.Context, port int) error {
	if err := domain.ValidatePort(port); err != nil {
		return err
	}
	return d.save(ctx, func(settings *domain.Settings) { settings.ProxyPort = port })
}
func (d *Desktop) save(ctx context.Context, change func(*domain.Settings)) error {
	d.operation.Lock()
	defer d.operation.Unlock()
	ctx, cancel := d.operationContext(ctx)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	settings := d.Snapshot().Settings
	change(&settings)
	if err := d.deps.Store.Save(ctx, settings); err != nil {
		return fmt.Errorf("保存配置失败：%w", err)
	}
	d.mu.Lock()
	d.snapshot.Settings = settings
	d.mu.Unlock()
	return nil
}

func (d *Desktop) StartCodex(ctx context.Context) error {
	d.operation.Lock()
	defer d.operation.Unlock()
	ctx, cancel := d.operationContext(ctx)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	identity, err := d.deps.Resolver.Discover(ctx)
	if err != nil {
		return err
	}
	if !native.IsCodex(identity) {
		return ErrNotFound
	}
	d.mu.Lock()
	d.snapshot.Identity = identity
	d.mu.Unlock()
	if err := d.refresh(ctx, identity); err != nil {
		return err
	}
	if d.Snapshot().State == "running" {
		return ErrAlreadyRunning
	}
	port := d.Snapshot().Settings.ProxyPort
	if err := d.deps.CheckPort(ctx, port); err != nil {
		return err
	}
	spec, err := native.PlanCodex(identity, port)
	if err != nil {
		return err
	}
	if err := d.refresh(ctx, identity); err != nil {
		return err
	}
	if d.Snapshot().State == "running" {
		return ErrAlreadyRunning
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return d.launchPrepared(ctx, identity, spec)
}

func (d *Desktop) launchPrepared(ctx context.Context, identity domain.AppIdentity, spec domain.LaunchSpec) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d.mu.Lock()
	d.snapshot.State = "starting"
	d.mu.Unlock()
	handle, err := d.deps.Processes.Start(ctx, spec)
	if err != nil {
		_ = d.refresh(ctx, identity)
		return err
	}
	d.cache[handle] = domain.ProcessInfo{Arguments: spec.Arguments, Readable: true}
	d.mu.Lock()
	d.snapshot.State, d.snapshot.RuntimeError = "running", ""
	d.snapshot.Proxy = ObserveProxy(domain.ProcessInfo{Arguments: spec.Arguments, Readable: true})
	d.mu.Unlock()
	return nil
}

func (d *Desktop) OpenProxyConsole(ctx context.Context) error {
	d.operation.Lock()
	defer d.operation.Unlock()
	ctx, cancel := d.operationContext(ctx)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	port := d.Snapshot().Settings.ProxyPort
	if err := d.deps.CheckPort(ctx, port); err != nil {
		return err
	}
	environment, unset := BuildProxyEnvironment(port)
	return d.deps.Console.Open(ctx, ConsoleSpec{Environment: environment, UnsetEnvironment: unset})
}
func (d *Desktop) Shutdown(ctx context.Context) error {
	d.cancel()
	select {
	case <-d.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (d *Desktop) WaitReady(ctx context.Context) error {
	select {
	case <-d.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
