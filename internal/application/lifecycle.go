package application

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"proxy-start/internal/adapters/native"
	"proxy-start/internal/domain"
)

// StopCodex does not depend on an available proxy. It uses current process
// handles and the platform controller rechecks path and creation time.
func (d *Desktop) StopCodex(ctx context.Context) error {
	d.operation.Lock()
	defer d.operation.Unlock()
	ctx, cancel := d.operationContext(ctx)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	identity, err := d.deps.Resolver.Discover(ctx)
	if err != nil {
		identity = d.Snapshot().Identity
	}
	if !native.IsCodex(identity) {
		if err != nil {
			return err
		}
		return ErrNotFound
	}
	return d.stopTarget(ctx, identity)
}

// Restart prepares the new proxy launch before stopping anything. A proxy,
// discovery or plan failure leaves the existing instance running.
func (d *Desktop) RestartCodex(ctx context.Context) error {
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
	spec, err := native.PlanCodex(identity, d.Snapshot().Settings.ProxyPort)
	if err != nil {
		return err
	}
	if err := d.deps.CheckPort(ctx, d.Snapshot().Settings.ProxyPort); err != nil {
		return err
	}
	d.mu.Lock()
	d.snapshot.Identity, d.snapshot.ScanError, d.snapshot.ScanNotFound = identity, "", false
	d.mu.Unlock()
	if err := d.stopTarget(ctx, identity); err != nil {
		return err
	}
	// A different launcher may have created a new instance while stopping.
	if err := d.refresh(ctx, identity); err != nil {
		return err
	}
	if d.Snapshot().State != "stopped" {
		return ErrAlreadyRunning
	}
	return d.launchPrepared(ctx, identity, spec)
}

func (d *Desktop) stopTarget(ctx context.Context, identity domain.AppIdentity) error {
	handles, err := d.deps.Processes.Find(ctx, identity)
	if err != nil {
		_ = d.refresh(ctx, identity)
		return err
	}
	if len(handles) == 0 {
		return d.refresh(ctx, identity)
	}
	d.mu.Lock()
	d.snapshot.State = "stopping"
	d.mu.Unlock()
	pids := make(map[int]bool)
	for _, handle := range handles {
		pids[handle.PID] = true
	}
	groups := make(map[string][]domain.ProcessHandle)
	for _, handle := range handles {
		if pids[handle.ParentPID] {
			continue
		}
		path := handle.Executable
		if path == "" {
			path = identity.Executable
		}
		groups[path] = append(groups[path], handle)
	}
	if len(groups) == 0 {
		_ = d.refresh(ctx, identity)
		return errors.New("未找到可确认身份的 Codex 主进程")
	}
	var stopErrors []error
	for path, group := range groups {
		effective := identity
		effective.Executable = path
		if err := d.deps.Processes.Stop(ctx, effective, group); err != nil {
			stopErrors = append(stopErrors, err)
		}
	}
	if err := errors.Join(stopErrors...); err != nil {
		remaining, queryErr := d.deps.Processes.Find(ctx, identity)
		if queryErr == nil && len(remaining) == 0 {
			log.Printf("Codex 已退出，子进程清理竞争：%v", err)
			return d.refresh(ctx, identity)
		}
		_ = d.refresh(ctx, identity)
		return fmt.Errorf("停止 Codex 失败：%w", err)
	}
	wait, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for {
		remaining, err := d.deps.Processes.Find(wait, identity)
		if err != nil {
			_ = d.refresh(ctx, identity)
			return err
		}
		if len(remaining) == 0 {
			return d.refresh(ctx, identity)
		}
		select {
		case <-wait.Done():
			_ = d.refresh(ctx, identity)
			return fmt.Errorf("等待 Codex 退出失败：%w", wait.Err())
		case <-time.After(80 * time.Millisecond):
		}
	}
}
