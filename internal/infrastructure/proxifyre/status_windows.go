//go:build windows

package proxifyre

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"proxy-start/internal/domain"
)

func initialStatus(options Options) domain.BackendStatus {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	return statusFor(ctx, options)
}

func statusFor(ctx context.Context, options Options) domain.BackendStatus {
	status := domain.BackendStatus{Detail: "未检测到 ProxiFyre 安装"}
	engine := strings.TrimSpace(options.EnginePath)
	if engine == "" {
		return status
	}
	engine, _ = filepath.Abs(engine)
	if _, err := os.Stat(engine); err != nil {
		status.Detail = "ProxiFyre.exe 不存在，请先安装已校验的 ProxiFyre 2.6.4 x64"
		return status
	}
	service := options.ServiceName
	if service == "" {
		service = defaultServiceName
	}
	output, err := runSCQuery(ctx, "query", service)
	if err != nil {
		status.Detail = "ProxiFyre.exe 已存在，但 ProxiFyreService 未安装"
		return status
	}
	status.Installed = true
	status.Running = strings.Contains(strings.ToUpper(string(output)), "RUNNING")
	driver, driverErr := runSCQuery(ctx, "query", "NDISRD")
	_ = driver
	if driverErr == nil {
		status.Ready = true
		status.Detail = "ProxiFyreService 与 Windows Packet Filter 已就绪"
	} else {
		status.Detail = "ProxiFyreService 已安装，但 Windows Packet Filter / NDISRD 未就绪"
	}
	return status
}

func runSCQuery(ctx context.Context, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, systemSCPath(), args...)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return command.Output()
}

func systemSCPath() string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(root, "System32", "sc.exe")
}
