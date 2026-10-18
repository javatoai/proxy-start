//go:build windows

package main

import (
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
	"unsafe"
)

const (
	processQueryLimitedInformation = 0x1000
	pipeServerPIDBufferSize        = 32768
	fileShareRead                  = 0x00000001
	fileShareWrite                 = 0x00000002
	openExisting                   = 3
	fileAttributeNormal            = 0x00000080
)

var helperKernel32 = syscall.NewLazyDLL("kernel32.dll")

var (
	getNamedPipeServerProcessID = helperKernel32.NewProc("GetNamedPipeServerProcessId")
	openProcess                 = helperKernel32.NewProc("OpenProcess")
	queryFullProcessImageNameW  = helperKernel32.NewProc("QueryFullProcessImageNameW")
	closeHelperHandle           = helperKernel32.NewProc("CloseHandle")
	createFileW                 = helperKernel32.NewProc("CreateFileW")
)

func systemRoot() string {
	if value := strings.TrimSpace(os.Getenv("SystemRoot")); value != "" {
		return value
	}
	return `C:\Windows`
}

func systemTool(name string) string {
	return filepath.Join(systemRoot(), "System32", name)
}

func systemCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	command := exec.CommandContext(ctx, systemTool(name), args...)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return command
}

func icaclsCommand(ctx context.Context, path string) *exec.Cmd {
	// S-1-5-18 is LocalSystem and S-1-5-32-544 is the built-in
	// Administrators group. The configuration contains plaintext credentials,
	// so inheriting a standard-user read ACL is not acceptable.
	return systemCommand(ctx, "icacls.exe", path, "/inheritance:r", "/grant:r", "*S-1-5-18:F", "*S-1-5-32-544:F")
}

func restrictACL(path string) error {
	command := icaclsCommand(context.Background(), path)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("protect %s ACL: %w (%s)", path, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func writeAtomicProtected(path string, data []byte) error {
	temp := path + ".tmp-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	file, err := os.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		_ = file.Close()
		if !committed {
			_ = os.Remove(temp)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := restrictACL(temp); err != nil {
		return err
	}
	if err := os.Rename(temp, path); err != nil {
		return err
	}
	committed = true
	return nil
}

func validateProtectedEnginePath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	absolute = filepath.Clean(absolute)
	if !strings.EqualFold(filepath.Base(absolute), engineName) {
		return "", fmt.Errorf("engine must be %s", engineName)
	}
	programFiles := strings.TrimSpace(os.Getenv("ProgramFiles"))
	if programFiles == "" {
		programFiles = `C:\Program Files`
	}
	protected := filepath.Clean(filepath.Join(programFiles, "ProxiFyre", engineName))
	if !strings.EqualFold(absolute, protected) {
		return "", fmt.Errorf("engine must be the protected Program Files installation %q", protected)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(filepath.Clean(resolved), absolute) {
		return "", errors.New("engine path must not resolve through a reparse point")
	}
	return absolute, nil
}

func (h *helper) queryService() (bool, error) {
	output, err := systemCommand(context.Background(), "sc.exe", "query", expectedServiceName).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("query %s: %w (%s)", expectedServiceName, err, strings.TrimSpace(string(output)))
	}
	return strings.Contains(strings.ToUpper(string(output)), "RUNNING"), nil
}

func registeredEnginePath(output string) (string, error) {
	for _, line := range strings.Split(output, "\n") {
		index := strings.Index(line, ":")
		if index < 0 || !strings.Contains(strings.ToUpper(line[:index]), "BINARY_PATH_NAME") {
			continue
		}
		value := strings.TrimSpace(line[index+1:])
		if strings.HasPrefix(value, "\"") {
			if end := strings.Index(value[1:], "\""); end >= 0 {
				value = value[1 : end+1]
			}
		} else if fields := strings.Fields(value); len(fields) > 0 {
			value = fields[0]
		}
		if value == "" {
			break
		}
		absolute, err := filepath.Abs(value)
		if err != nil {
			return "", err
		}
		return filepath.Clean(absolute), nil
	}
	return "", errors.New("service registration has no BINARY_PATH_NAME")
}

func (h *helper) verifyConfiguration(ctx context.Context, expected []byte, logLevel string, rules []proxyRule) error {
	running, err := h.queryServiceContext(ctx)
	if err != nil {
		return err
	}
	if !running {
		return errors.New("service stopped before configuration verification")
	}
	actual, err := os.ReadFile(h.configPath)
	if err != nil {
		return fmt.Errorf("read applied engine configuration: %w", err)
	}
	if !strings.EqualFold(digestBytes(actual), digestBytes(expected)) {
		return errors.New("engine configuration changed before service verification")
	}
	if err := checkDriverDevice(); err != nil {
		return err
	}
	if !infoLoggingEnabled(logLevel) {
		return errors.New("engine logLevel must be Info, Debug, or All to verify the loaded-rule summary")
	}
	if err := h.verifyInfoLog(ctx, rules); err != nil {
		return err
	}
	return nil
}

func infoLoggingEnabled(level string) bool {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "info", "debug", "all":
		return true
	default:
		return false
	}
}

func (h *helper) verifyInfoLog(ctx context.Context, rules []proxyRule) error {
	if ctx == nil {
		ctx = context.Background()
	}
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	var lastErr error
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		lastErr = h.verifyInfoLogOnce(rules)
		if lastErr == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return lastErr
		case <-ticker.C:
		}
	}
}

func (h *helper) verifyInfoLogOnce(rules []proxyRule) error {
	path := filepath.Join(filepath.Dir(h.engine), "logs", "logfile.txt")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read ProxiFyre Info log: %w", err)
	}
	start := h.logOffset
	if start < 0 || start > int64(len(data)) {
		start = 0
	}
	lines := strings.Split(strings.ToLower(string(data[start:])), "\n")
	serviceRunning := false
	associations := make([]string, 0, len(rules))
	for _, line := range lines {
		if strings.Contains(line, "service is running") {
			serviceRunning = true
		}
		if strings.Contains(line, "successfully associated") {
			associations = append(associations, line)
		}
	}
	if !serviceRunning {
		return errors.New("ProxiFyre Info log contains no post-start service-running record")
	}
	for ruleIndex, rule := range rules {
		endpoint := strings.ToLower(strings.TrimSpace(rule.Socks5ProxyEndpoint))
		for _, appName := range rule.AppNames {
			appName = strings.ToLower(strings.TrimSpace(appName))
			if appName == "" || endpoint == "" {
				return fmt.Errorf("rule %d contains an empty log identity", ruleIndex)
			}
			matched := false
			for _, line := range associations {
				if strings.Contains(line, appName) && strings.Contains(line, endpoint) {
					matched = true
					break
				}
			}
			if !matched {
				return fmt.Errorf("ProxiFyre Info log has no association for %q to %q", appName, endpoint)
			}
		}
	}
	return nil
}

func (h *helper) ensureServiceRunning(ctx context.Context) error {
	running, err := h.queryServiceContext(ctx)
	if err != nil {
		return err
	}
	if running {
		return nil
	}
	if err := runSC(ctx, "start", expectedServiceName); err != nil {
		return err
	}
	return waitService(ctx, expectedServiceName, true)
}

func (h *helper) ensureServiceStopped(ctx context.Context) error {
	running, err := h.queryServiceContext(ctx)
	if err != nil {
		return err
	}
	if !running {
		return nil
	}
	if err := runSC(ctx, "stop", expectedServiceName); err != nil {
		return err
	}
	return waitService(ctx, expectedServiceName, false)
}

func (h *helper) queryServiceContext(ctx context.Context) (bool, error) {
	output, err := systemCommand(ctx, "sc.exe", "query", expectedServiceName).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("query %s: %w (%s)", expectedServiceName, err, strings.TrimSpace(string(output)))
	}
	return strings.Contains(strings.ToUpper(string(output)), "RUNNING"), nil
}

func checkDriverDevice() error {
	path, err := syscall.UTF16PtrFromString(driverDevicePath)
	if err != nil {
		return err
	}
	// The upstream GUI deliberately requests no read/write access. This tests
	// that the active NDISRD device exists without requiring packet handles.
	handle, _, callErr := createFileW.Call(
		uintptr(unsafe.Pointer(path)),
		0,
		fileShareRead|fileShareWrite,
		0,
		openExisting,
		fileAttributeNormal,
		0,
	)
	if handle == uintptr(syscall.InvalidHandle) {
		if callErr == nil {
			callErr = syscall.EINVAL
		}
		return fmt.Errorf("Windows Packet Filter device is unavailable: %w", callErr)
	}
	closeHelperHandle.Call(handle)
	return nil
}

func validatePipePeer(file *os.File) error {
	if file == nil {
		return errors.New("nil pipe")
	}
	var pid uint32
	ret, _, callErr := getNamedPipeServerProcessID.Call(file.Fd(), uintptr(unsafe.Pointer(&pid)))
	if ret == 0 || pid == 0 {
		if callErr == nil {
			callErr = syscall.EINVAL
		}
		return fmt.Errorf("get named-pipe server identity: %w", callErr)
	}
	process, _, callErr := openProcess.Call(processQueryLimitedInformation, 0, uintptr(pid))
	if process == 0 {
		return fmt.Errorf("open named-pipe server process: %w", callErr)
	}
	defer closeHelperHandle.Call(process)
	buffer := make([]uint16, pipeServerPIDBufferSize)
	size := uint32(len(buffer))
	ret, _, callErr = queryFullProcessImageNameW.Call(process, 0, uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&size)))
	if ret == 0 {
		return fmt.Errorf("resolve named-pipe server image: %w", callErr)
	}
	actual := filepath.Clean(syscall.UTF16ToString(buffer[:size]))
	self, err := os.Executable()
	if err != nil {
		return err
	}
	expected := filepath.Join(filepath.Dir(self), "proxy-start.exe")
	if resolved, resolveErr := filepath.EvalSymlinks(expected); resolveErr == nil {
		expected = resolved
	}
	if !strings.EqualFold(actual, filepath.Clean(expected)) {
		return fmt.Errorf("named-pipe server %q is not the sibling proxy-start.exe", actual)
	}
	return nil
}
