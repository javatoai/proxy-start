//go:build darwin

package darwin

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"proxy-start/internal/application"
	"proxy-start/internal/domain"
)

type ProcessController struct{}

var errStopWaitTimeout = errors.New("等待应用退出超时")

var _ application.ProcessController = (*ProcessController)(nil)

func NewProcessController() *ProcessController { return &ProcessController{} }

func (c *ProcessController) Start(ctx context.Context, spec domain.LaunchSpec) (domain.ProcessHandle, error) {
	if err := ctx.Err(); err != nil {
		return domain.ProcessHandle{}, err
	}
	executable := strings.TrimSpace(spec.Identity.Executable)
	if executable == "" {
		return domain.ProcessHandle{}, errors.New("应用缺少可执行路径")
	}
	info, err := os.Stat(executable)
	if err != nil || info.IsDir() {
		return domain.ProcessHandle{}, fmt.Errorf("应用可执行文件不可用: %s", executable)
	}
	args := append([]string{executable}, spec.Arguments...)
	cmd := exec.Command(executable, args[1:]...)
	cmd.Env = mergedEnvironment(spec.Environment, spec.UnsetEnvironment)
	workingDirectory := spec.Identity.WorkingDirectory
	if workingDirectory == "" {
		workingDirectory = filepath.Dir(filepath.Dir(executable))
	}
	cmd.Dir = workingDirectory
	// A new session keeps the launched application alive when Proxy Start
	// exits. It does not alter the application layer's process ownership.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return domain.ProcessHandle{}, fmt.Errorf("启动 %s 失败: %w", executable, err)
	}
	pid := cmd.Process.Pid
	// Keep the Cmd owned by this goroutine until Wait reaps the child. The
	// session leader is detached through Setsid, so abandoning this goroutine
	// when Proxy Start exits does not terminate the target application.
	go func() { _ = cmd.Wait() }()
	var started time.Time
	for attempt := 0; attempt < 20; attempt++ {
		if path, observed, err := processMetadataContext(ctx, pid); err == nil && sameExecutablePath(path, executable) {
			started = observed
			break
		}
		select {
		case <-ctx.Done():
			// The process has already been created and released. Preserve its
			// known PID/path handle even if optional metadata collection is
			// cancelled, so the application layer can still manage it.
			attempt = 20
		case <-time.After(25 * time.Millisecond):
		}
	}
	return domain.ProcessHandle{PID: pid, StartedAt: started, Executable: cleanExecutable(executable), PackageFamily: spec.Identity.PackageFamily}, nil
}

func (c *ProcessController) Find(ctx context.Context, identity domain.AppIdentity) ([]domain.ProcessHandle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	target := cleanExecutable(identity.Executable)
	if target == "" {
		return nil, errors.New("应用缺少可执行路径")
	}
	cmd := exec.CommandContext(ctx, "/bin/ps", "-axo", "pid=,lstart=")
	cmd.Env = localeEnvironment()
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("读取进程列表失败: %w", err)
	}
	result := make([]domain.ProcessHandle, 0)
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		pid, started, ok := parsePSLine(scanner.Text())
		if !ok {
			continue
		}
		executable, pathErr := processExecutablePath(pid)
		if pathErr != nil || !sameExecutablePath(executable, target) {
			continue
		}
		result = append(result, domain.ProcessHandle{PID: pid, StartedAt: started, Executable: target, PackageFamily: identity.PackageFamily})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *ProcessController) Stop(ctx context.Context, identity domain.AppIdentity, handles []domain.ProcessHandle) error {
	target := cleanExecutable(identity.Executable)
	if target == "" {
		return errors.New("应用缺少可执行路径")
	}
	for _, handle := range handles {
		if err := ctx.Err(); err != nil {
			return err
		}
		if handle.PID <= 0 {
			continue
		}
		// Re-check both path and start time before signalling. The caller can
		// hold stale handles after a PID is recycled; never kill an unrelated app.
		if !c.matchesHandle(ctx, handle, target) {
			continue
		}
		if err := syscall.Kill(handle.PID, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
			return fmt.Errorf("结束进程 %d 失败: %w", handle.PID, err)
		}
		if err := waitForExit(ctx, handle, target, 5*time.Second); err == nil {
			continue
		} else if !errors.Is(err, errStopWaitTimeout) {
			return err
		}
		// Escalate only after the same PID/path/start tuple was observed again.
		if c.matchesHandle(ctx, handle, target) {
			if killErr := syscall.Kill(handle.PID, syscall.SIGKILL); killErr != nil && !errors.Is(killErr, syscall.ESRCH) {
				return fmt.Errorf("强制结束进程 %d 失败: %w", handle.PID, killErr)
			}
		}
		if err := waitForExit(ctx, handle, target, 2*time.Second); err != nil {
			return fmt.Errorf("进程 %d 在超时后仍未退出: %w", handle.PID, err)
		}
	}
	return nil
}

func (c *ProcessController) Alive(ctx context.Context, handle domain.ProcessHandle) bool {
	if handle.PID <= 0 {
		return false
	}
	if err := syscall.Kill(handle.PID, 0); err != nil {
		return false
	}
	path, started, err := processMetadataContext(ctx, handle.PID)
	if err != nil || !sameExecutablePath(path, handle.Executable) {
		return false
	}
	return handle.StartedAt.IsZero() || started.Equal(handle.StartedAt)
}

func (c *ProcessController) matchesHandle(ctx context.Context, handle domain.ProcessHandle, target string) bool {
	if handle.PID <= 0 {
		return false
	}
	path, started, err := processMetadataContext(ctx, handle.PID)
	if err != nil || !sameExecutablePath(path, target) {
		return false
	}
	return handle.StartedAt.IsZero() || started.Equal(handle.StartedAt)
}

func parsePSLine(line string) (int, time.Time, bool) {
	line = strings.TrimSpace(line)
	fields := strings.Fields(line)
	if len(fields) < 6 {
		return 0, time.Time{}, false
	}
	pid, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0, time.Time{}, false
	}
	started, err := time.Parse("Mon Jan 2 15:04:05 2006", strings.Join(fields[1:6], " "))
	if err != nil {
		return 0, time.Time{}, false
	}
	return pid, started, true
}

func processMetadata(pid int) (string, time.Time, error) {
	return processMetadataContext(context.Background(), pid)
}

func processMetadataContext(parent context.Context, pid int) (string, time.Time, error) {
	path, err := processExecutablePath(pid)
	if err != nil {
		return "", time.Time{}, err
	}
	commandCtx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, "/bin/ps", "-p", strconv.Itoa(pid), "-o", "lstart=")
	cmd.Env = localeEnvironment()
	output, err := cmd.Output()
	if err != nil {
		if commandCtx.Err() != nil {
			return "", time.Time{}, commandCtx.Err()
		}
		return "", time.Time{}, err
	}
	_, started, ok := parsePSLine("0 " + strings.TrimSpace(string(output)))
	if !ok {
		return "", time.Time{}, errors.New("无法读取进程启动时间")
	}
	return path, started, nil
}

func waitForExit(ctx context.Context, handle domain.ProcessHandle, target string, timeout time.Duration) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := waitCtx.Err(); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return errStopWaitTimeout
		}
		if !(&ProcessController{}).matchesHandle(waitCtx, handle, target) {
			return nil
		}
		select {
		case <-waitCtx.Done():
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return errStopWaitTimeout
		case <-ticker.C:
		}
	}
}
