//go:build darwin

package console

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"proxy-start/internal/application"
	"strings"
	"time"
)

// Launcher opens Terminal.app through a short-lived private .command script.
// Secrets are written only to the mode-0700 temporary file and are removed by
// the script before handing control to a clean zsh session.
type terminalOpener func(context.Context, string) error

type Launcher struct {
	open    terminalOpener
	timeout time.Duration
}

var _ application.ConsoleLauncher = (*Launcher)(nil)

func NewLauncher() *Launcher { return &Launcher{open: openTerminal, timeout: 10 * time.Second} }

func openTerminal(ctx context.Context, path string) error {
	return exec.CommandContext(ctx, "/usr/bin/open", "-a", "Terminal", "--", path).Run()
}

func (l *Launcher) Open(ctx context.Context, spec application.ConsoleSpec) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "proxy-start-console-")
	if err != nil {
		return errors.New("创建代理控制台临时目录失败")
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		_ = os.RemoveAll(dir)
		return err
	}
	path := filepath.Join(dir, "open.command")
	content := commandScript(spec)
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		_ = os.RemoveAll(dir)
		return errors.New("写入代理控制台脚本失败")
	}
	if err := os.Chmod(path, 0o700); err != nil {
		_ = os.RemoveAll(dir)
		return err
	}
	opener := l.open
	if opener == nil {
		opener = openTerminal
	}
	if err := opener(ctx, path); err != nil {
		_ = os.RemoveAll(dir)
		return errors.New("打开 Terminal 失败")
	}
	// Terminal consumes .command files asynchronously. Do not report success
	// until the script has removed itself; otherwise a client exit could leave
	// credentials behind in the temporary directory.
	timeout := l.timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
			_ = os.Remove(dir)
			return nil
		}
		select {
		case <-ctx.Done():
			_ = os.RemoveAll(dir)
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	_ = os.RemoveAll(dir)
	return errors.New("Terminal 未在限定时间内读取代理控制台脚本")
}

func commandScript(spec application.ConsoleSpec) string {
	var b strings.Builder
	b.WriteString("#!/bin/sh\nset -e\nset +x\n")
	b.WriteString("_proxy_start_script=$0\n")
	b.WriteString("rm -f -- \"$_proxy_start_script\"\n")
	for _, key := range spec.UnsetEnvironment {
		if key != "" {
			b.WriteString("unset ")
			b.WriteString(shellQuote(key))
			b.WriteByte('\n')
		}
	}
	for key, value := range spec.Environment {
		b.WriteString("export ")
		b.WriteString(shellQuote(key))
		b.WriteByte('=')
		b.WriteString(shellQuote(value))
		b.WriteByte('\n')
	}
	b.WriteString("exec /bin/zsh -f\n")
	return b.String()
}
