//go:build windows

package console

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"proxy-start/internal/application"
	"syscall"
)

// Launcher opens a standalone Windows PowerShell console. It deliberately
// does not pass proxy values as command-line arguments.
//
// The fields other than the zero-value production path are only used by
// same-package integration tests. NewLauncher leaves them at zero, which
// keeps the production shell interactive with a new console and detached
// lifetime.
type Launcher struct {
	commandArgs []string
	waitForExit bool
}

var _ application.ConsoleLauncher = (*Launcher)(nil)

func NewLauncher() *Launcher { return &Launcher{} }

func powershellPath() (string, error) {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = os.Getenv("windir")
	}
	if root == "" {
		root = `C:\Windows`
	}
	candidate := filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		return candidate, nil
	}
	if path, err := exec.LookPath("powershell.exe"); err == nil {
		return path, nil
	} else {
		return "", fmt.Errorf("找不到 PowerShell：%w", err)
	}
}

func userHomeDirectory() (string, error) {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return home, nil
	}
	if home := os.Getenv("USERPROFILE"); home != "" {
		return home, nil
	}
	if drive, path := os.Getenv("HOMEDRIVE"), os.Getenv("HOMEPATH"); drive != "" && path != "" {
		return drive + path, nil
	}
	return "", errors.New("无法确定用户主目录")
}

func (l *Launcher) Open(ctx context.Context, spec application.ConsoleSpec) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	powershell, err := powershellPath()
	if err != nil {
		return err
	}
	home, err := userHomeDirectory()
	if err != nil {
		return err
	}
	args := l.commandArgs
	if len(args) == 0 {
		args = []string{"-NoLogo", "-NoProfile", "-NoExit"}
	}
	cmd := exec.Command(powershell, args...)
	cmd.Dir = home
	cmd.Env = childEnvironment(spec.Environment, spec.UnsetEnvironment)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags:    0x00000010, // CREATE_NEW_CONSOLE
		NoInheritHandles: true,
	}
	if err := cmd.Start(); err != nil {
		return errors.New("打开代理控制台失败")
	}
	if l.waitForExit {
		return cmd.Wait()
	}
	if err := cmd.Process.Release(); err != nil {
		return errors.New("释放代理控制台句柄失败")
	}
	return nil
}
