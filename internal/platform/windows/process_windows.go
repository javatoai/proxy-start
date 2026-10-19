//go:build windows

package windows

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"proxy-start/internal/application"
	"proxy-start/internal/domain"
)

const (
	processQueryLimitedInformation = 0x1000
	processQueryInformation        = 0x0400
	processTerminate               = 0x0001
	processSynchronize             = 0x00100000
	createNewProcessGroup          = 0x00000200
	createBreakawayFromJob         = 0x01000000
	snapshotProcess                = 0x00000002
	waitObject0                    = 0x00000000
	waitTimeout                    = 0x00000102
)

var (
	kernel32                  = syscall.NewLazyDLL("kernel32.dll")
	createToolhelp32Snapshot  = kernel32.NewProc("CreateToolhelp32Snapshot")
	process32First            = kernel32.NewProc("Process32FirstW")
	process32Next             = kernel32.NewProc("Process32NextW")
	openProcess               = kernel32.NewProc("OpenProcess")
	closeHandle               = kernel32.NewProc("CloseHandle")
	queryFullProcessImageName = kernel32.NewProc("QueryFullProcessImageNameW")
	getProcessTimes           = kernel32.NewProc("GetProcessTimes")
	terminateProcess          = kernel32.NewProc("TerminateProcess")
	waitForSingleObject       = kernel32.NewProc("WaitForSingleObject")
)

// ProcessController owns only process lifecycle operations. It has no global
// process registry, so the GUI can be closed while launched applications keep
// running normally.
type ProcessController struct{}

var _ application.ProcessController = (*ProcessController)(nil)

// NewProcessController constructs a Windows process controller.
func NewProcessController() *ProcessController { return &ProcessController{} }

func (c *ProcessController) Start(ctx context.Context, spec domain.LaunchSpec) (domain.ProcessHandle, error) {
	if err := ctx.Err(); err != nil {
		return domain.ProcessHandle{}, err
	}
	identity := spec.Identity
	identity.Executable = canonicalPath(identity.Executable)
	if identity.Executable == "" {
		return domain.ProcessHandle{}, errors.New("应用缺少可执行路径")
	}
	args := append([]string(nil), spec.Arguments...)
	if args == nil {
		args = append([]string(nil), identity.Arguments...)
	}
	if identity.PackageFamily != "" && identity.ApplicationID != "" {
		return c.startPackaged(ctx, domain.LaunchSpec{Identity: identity, Arguments: args, Environment: spec.Environment, UnsetEnvironment: spec.UnsetEnvironment})
	}

	cmd := exec.Command(identity.Executable, args...)
	cmd.Dir = canonicalPath(identity.WorkingDirectory)
	if cmd.Dir == "" {
		cmd.Dir = filepath.Dir(identity.Executable)
	}
	cmd.Env = mergeEnvironment(spec.Environment, spec.UnsetEnvironment)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags:    createNewProcessGroup | createBreakawayFromJob,
		NoInheritHandles: true,
	}
	if err := cmd.Start(); err != nil {
		return domain.ProcessHandle{}, fmt.Errorf("启动 %s 失败: %w", identity.Executable, err)
	}
	pid := cmd.Process.Pid
	// Release closes Go's process handle while leaving the child detached from
	// this controller. Identity checks below use a fresh kernel handle.
	_ = cmd.Process.Release()
	handle, err := processHandle(pid)
	if err != nil {
		return domain.ProcessHandle{}, fmt.Errorf("读取新进程身份失败: %w", err)
	}
	handle.PackageFamily = identity.PackageFamily
	return handle, nil
}

func (c *ProcessController) Find(ctx context.Context, identity domain.AppIdentity) ([]domain.ProcessHandle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	target := canonicalPath(identity.Executable)
	if target == "" {
		return nil, errors.New("查找进程需要可执行路径")
	}
	entries, err := enumerateProcesses(target)
	if err != nil {
		return nil, err
	}
	result := make([]domain.ProcessHandle, 0)
	for _, entry := range entries {
		if !samePath(entry.executable, target) {
			continue
		}
		result = append(result, domain.ProcessHandle{PID: int(entry.pid), StartedAt: entry.startedAt, Executable: entry.executable, PackageFamily: identity.PackageFamily})
	}
	return result, nil
}

func (c *ProcessController) Alive(_ context.Context, handle domain.ProcessHandle) bool {
	if handle.PID <= 0 || handle.StartedAt.IsZero() {
		return false
	}
	current, err := processHandle(handle.PID)
	if err != nil {
		return false
	}
	if !current.StartedAt.Equal(handle.StartedAt) {
		return false
	}
	return handle.Executable == "" || samePath(current.Executable, handle.Executable)
}

func (c *ProcessController) Stop(ctx context.Context, identity domain.AppIdentity, handles []domain.ProcessHandle) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(handles) == 0 {
		return nil
	}
	rawEntries, err := snapshotProcesses()
	if err != nil {
		return err
	}
	byParent := make(map[uint32][]rawProcessEntry)
	for _, entry := range rawEntries {
		byParent[entry.parentPID] = append(byParent[entry.parentPID], entry)
	}
	selected := make(map[uint32]processEntry)
	for _, handle := range handles {
		if handle.PID <= 0 || handle.PID == os.Getpid() || handle.StartedAt.IsZero() {
			continue
		}
		current, currentErr := processHandle(handle.PID)
		if currentErr != nil || !current.StartedAt.Equal(handle.StartedAt) || (handle.Executable != "" && !samePath(current.Executable, handle.Executable)) {
			continue
		}
		if identity.Executable != "" && !samePath(current.Executable, identity.Executable) {
			continue
		}
		selected[uint32(handle.PID)] = processEntry{pid: uint32(handle.PID), executable: current.Executable, startedAt: current.StartedAt}
	}
	// Only descend from identity-verified target processes. This protects
	// unrelated processes with the same executable name and avoids bulk kills.
	queue := make([]uint32, 0, len(selected))
	for pid := range selected {
		queue = append(queue, pid)
	}
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		for _, rawChild := range byParent[parent] {
			if rawChild.pid <= 4 || rawChild.pid == uint32(os.Getpid()) {
				continue
			}
			if _, exists := selected[rawChild.pid]; exists {
				continue
			}
			started := queryProcessStart(rawChild.pid)
			if started.IsZero() || started.Before(selected[parent].startedAt) {
				continue
			}
			selected[rawChild.pid] = processEntry{pid: rawChild.pid, parentPID: rawChild.parentPID, executable: canonicalPath(rawChild.name), startedAt: started}
			queue = append(queue, rawChild.pid)
		}
	}

	var failures []string
	for _, entry := range selected {
		if err := terminateAndWait(ctx, entry); err != nil {
			failures = append(failures, fmt.Sprintf("PID %d: %v", entry.pid, err))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("结束进程失败: %s", strings.Join(failures, "; "))
	}
	return nil
}

type processEntry struct {
	pid        uint32
	parentPID  uint32
	executable string
	startedAt  time.Time
}

type rawProcessEntry struct {
	pid       uint32
	parentPID uint32
	name      string
}

type processEntry32 struct {
	Size            uint32
	Usage           uint32
	ProcessID       uint32
	DefaultHeapID   uintptr
	ModuleID        uint32
	Threads         uint32
	ParentProcessID uint32
	PriorityClass   int32
	Flags           uint32
	ExeFile         [260]uint16
}

func snapshotProcesses() ([]rawProcessEntry, error) {
	snapshot, _, callErr := createToolhelp32Snapshot.Call(snapshotProcess, 0)
	if snapshot == ^uintptr(0) || snapshot == 0 {
		return nil, fmt.Errorf("创建进程快照失败: %w", callErr)
	}
	defer closeHandle.Call(snapshot)
	entry := processEntry32{Size: uint32(unsafe.Sizeof(processEntry32{}))}
	first, _, err := process32First.Call(snapshot, uintptr(unsafe.Pointer(&entry)))
	if first == 0 {
		return nil, fmt.Errorf("读取进程快照失败: %w", err)
	}
	result := make([]rawProcessEntry, 0)
	for {
		pid := entry.ProcessID
		if pid == 0 {
			next, _, _ := process32Next.Call(snapshot, uintptr(unsafe.Pointer(&entry)))
			if next == 0 {
				break
			}
			continue
		}
		result = append(result, rawProcessEntry{pid: pid, parentPID: entry.ParentProcessID, name: syscall.UTF16ToString(entry.ExeFile[:])})
		next, _, _ := process32Next.Call(snapshot, uintptr(unsafe.Pointer(&entry)))
		if next == 0 {
			break
		}
	}
	return result, nil
}

func enumerateProcesses(target string) ([]processEntry, error) {
	rawEntries, err := snapshotProcesses()
	if err != nil {
		return nil, err
	}
	targetName := strings.ToLower(filepath.Base(target))
	result := make([]processEntry, 0)
	for _, raw := range rawEntries {
		if targetName != "" && !strings.EqualFold(raw.name, targetName) {
			continue
		}
		path := queryProcessPath(raw.pid)
		if path == "" {
			// QueryFullProcessImageName can be denied for a protected process;
			// its basename is still useful only when the caller asked for that
			// exact basename, so retain it for path comparison to reject below.
			path = raw.name
		}
		started := queryProcessStart(raw.pid)
		if started.IsZero() {
			continue
		}
		result = append(result, processEntry{pid: raw.pid, parentPID: raw.parentPID, executable: canonicalPath(path), startedAt: started})
	}
	return result, nil
}

func processHandle(pid int) (domain.ProcessHandle, error) {
	if pid <= 0 {
		return domain.ProcessHandle{}, errors.New("无效的进程 ID")
	}
	path := queryProcessPath(uint32(pid))
	started := queryProcessStart(uint32(pid))
	if path == "" || started.IsZero() {
		return domain.ProcessHandle{}, fmt.Errorf("无法读取 PID %d 的身份", pid)
	}
	return domain.ProcessHandle{PID: pid, StartedAt: started, Executable: canonicalPath(path)}, nil
}

func matchingEntry(entries []processEntry, handle domain.ProcessHandle) (processEntry, bool) {
	for _, entry := range entries {
		if entry.pid != uint32(handle.PID) || entry.startedAt.IsZero() || !entry.startedAt.Equal(handle.StartedAt) {
			continue
		}
		if handle.Executable != "" && !samePath(entry.executable, handle.Executable) {
			continue
		}
		return entry, true
	}
	return processEntry{}, false
}

func queryProcessPath(pid uint32) string {
	handle, _, _ := openProcess.Call(processQueryLimitedInformation, 0, uintptr(pid))
	if handle == 0 {
		handle, _, _ = openProcess.Call(processQueryInformation, 0, uintptr(pid))
	}
	if handle == 0 {
		return ""
	}
	defer closeHandle.Call(handle)
	buffer := make([]uint16, 32768)
	length := uint32(len(buffer))
	result, _, _ := queryFullProcessImageName.Call(handle, 0, uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&length)))
	if result == 0 || length == 0 {
		return ""
	}
	return syscall.UTF16ToString(buffer[:length])
}

func queryProcessStart(pid uint32) time.Time {
	handle, _, _ := openProcess.Call(processQueryLimitedInformation, 0, uintptr(pid))
	if handle == 0 {
		handle, _, _ = openProcess.Call(processQueryInformation, 0, uintptr(pid))
	}
	if handle == 0 {
		return time.Time{}
	}
	defer closeHandle.Call(handle)
	var creation, exit, kernel, user syscall.Filetime
	result, _, _ := getProcessTimes.Call(handle, uintptr(unsafe.Pointer(&creation)), uintptr(unsafe.Pointer(&exit)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user)))
	if result == 0 {
		return time.Time{}
	}
	return filetimeToTime(creation)
}

func filetimeToTime(filetime syscall.Filetime) time.Time {
	const epochOffset = 116444736000000000
	value := (uint64(filetime.HighDateTime) << 32) | uint64(filetime.LowDateTime)
	if value < epochOffset {
		return time.Time{}
	}
	return time.Unix(0, int64(value-epochOffset)*100).UTC()
}

func terminateAndWait(ctx context.Context, entry processEntry) error {
	if entry.pid == uint32(os.Getpid()) || entry.startedAt.IsZero() {
		return errors.New("拒绝结束当前进程或未知身份进程")
	}
	handle, _, callErr := openProcess.Call(processTerminate|processSynchronize, 0, uintptr(entry.pid))
	if handle == 0 {
		return callErr
	}
	defer closeHandle.Call(handle)
	if result, _, err := terminateProcess.Call(handle, 1); result == 0 {
		return err
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		result, _, _ := waitForSingleObject.Call(handle, 100)
		if result == waitObject0 {
			return nil
		}
		if result != waitTimeout {
			return fmt.Errorf("等待进程退出失败")
		}
	}
}

func mergeEnvironment(overrides map[string]string, unset []string) []string {
	base := os.Environ()
	for key, value := range overrides {
		base = setEnvironmentValue(base, key, value)
	}
	for _, key := range unset {
		prefix := strings.ToUpper(key) + "="
		filtered := base[:0]
		for _, item := range base {
			index := strings.IndexByte(item, '=')
			if index < 0 || !strings.EqualFold(item[:index], key) {
				filtered = append(filtered, item)
			}
		}
		base = filtered
		_ = prefix
	}
	return base
}

type packagedLaunchConfig struct {
	Executable       string            `json:"executable"`
	Arguments        string            `json:"arguments"`
	WorkingDirectory string            `json:"workingDirectory"`
	Environment      map[string]string `json:"environment"`
	UnsetEnvironment []string          `json:"unsetEnvironment"`
}

type packagedLaunchResult struct {
	ProcessID int    `json:"processId"`
	Error     string `json:"error"`
}

func (c *ProcessController) startPackaged(ctx context.Context, spec domain.LaunchSpec) (domain.ProcessHandle, error) {
	identity := spec.Identity
	applicationID := identity.ApplicationID
	if applicationID == "" && identity.AUMID != "" {
		parts := strings.SplitN(identity.AUMID, "!", 2)
		if len(parts) == 2 {
			applicationID = parts[1]
		}
	}
	if identity.PackageFamily == "" || applicationID == "" {
		return domain.ProcessHandle{}, errors.New("打包应用缺少包族或应用 ID")
	}
	args := append([]string(nil), spec.Arguments...)
	config := packagedLaunchConfig{
		Executable:       identity.Executable,
		Arguments:        joinWindowsArguments(args),
		WorkingDirectory: firstNonEmpty(identity.WorkingDirectory, filepath.Dir(identity.Executable)),
		Environment:      spec.Environment,
		UnsetEnvironment: spec.UnsetEnvironment,
	}
	configBytes, err := json.Marshal(config)
	if err != nil {
		return domain.ProcessHandle{}, err
	}
	resultFile, err := os.CreateTemp("", "proxy-start-*.result.json")
	if err != nil {
		return domain.ProcessHandle{}, err
	}
	resultPath := resultFile.Name()
	_ = resultFile.Close()
	_ = os.Remove(resultPath)
	defer os.Remove(resultPath)

	env := map[string]string{
		"PROXY_START_PACKAGE_FAMILY": identity.PackageFamily,
		"PROXY_START_APPLICATION_ID": applicationID,
		"PROXY_START_LAUNCH_B64":     base64.StdEncoding.EncodeToString(configBytes),
		"PROXY_START_RESULT_PATH":    resultPath,
	}
	if _, err := runPowerShellJSON(ctx, packagedOuterScript, env); err != nil {
		return domain.ProcessHandle{}, err
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return domain.ProcessHandle{}, err
		}
		if data, readErr := os.ReadFile(resultPath); readErr == nil {
			var result packagedLaunchResult
			if jsonErr := json.Unmarshal(data, &result); jsonErr != nil {
				return domain.ProcessHandle{}, fmt.Errorf("解析打包启动结果失败: %w", jsonErr)
			}
			if result.Error != "" {
				return domain.ProcessHandle{}, errors.New(result.Error)
			}
			if result.ProcessID <= 0 {
				return domain.ProcessHandle{}, errors.New("打包启动未返回进程 ID")
			}
			handle, handleErr := processHandle(result.ProcessID)
			if handleErr != nil {
				return domain.ProcessHandle{}, handleErr
			}
			handle.PackageFamily = identity.PackageFamily
			return handle, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return domain.ProcessHandle{}, errors.New("打包应用启动超时")
}

func joinWindowsArguments(args []string) string {
	var builder strings.Builder
	for i, arg := range args {
		if i > 0 {
			builder.WriteByte(' ')
		}
		builder.WriteString(quoteWindowsArgument(arg))
	}
	return builder.String()
}

func quoteWindowsArgument(value string) string {
	if value != "" && !strings.ContainsAny(value, " \t\"") {
		return value
	}
	var b strings.Builder
	b.WriteByte('"')
	backslashes := 0
	for _, char := range value {
		if char == '\\' {
			backslashes++
			continue
		}
		if char == '"' {
			for i := 0; i < backslashes*2+1; i++ {
				b.WriteByte('\\')
			}
			b.WriteRune(char)
			backslashes = 0
			continue
		}
		for i := 0; i < backslashes; i++ {
			b.WriteByte('\\')
		}
		backslashes = 0
		b.WriteRune(char)
	}
	for i := 0; i < backslashes*2; i++ {
		b.WriteByte('\\')
	}
	b.WriteByte('"')
	return b.String()
}

const packagedOuterScript = `$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$bootstrap = @'
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
try {
  $json = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($env:PROXY_START_LAUNCH_B64))
  $config = $json | ConvertFrom-Json
  $startInfo = New-Object System.Diagnostics.ProcessStartInfo
  $startInfo.FileName = [string]$config.executable
  $startInfo.Arguments = [string]$config.arguments
  $startInfo.UseShellExecute = $false
  $startInfo.WorkingDirectory = [string]$config.workingDirectory
  foreach ($name in @($config.unsetEnvironment)) { if ($name) { $startInfo.EnvironmentVariables.Remove([string]$name) } }
  foreach ($property in $config.environment.PSObject.Properties) { $startInfo.EnvironmentVariables[$property.Name] = [string]$property.Value }
  $process = [Diagnostics.Process]::Start($startInfo)
  $result = @{ processId = $process.Id }
} catch { $result = @{ error = $_.Exception.Message } }
$tmp = $env:PROXY_START_RESULT_PATH + '.tmp'
[IO.File]::WriteAllText($tmp, ($result | ConvertTo-Json -Compress), (New-Object Text.UTF8Encoding($false)))
[IO.File]::Move($tmp, $env:PROXY_START_RESULT_PATH)
'@
$bootstrapB64 = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($bootstrap))
$arguments = '-NoLogo -NoProfile -NonInteractive -WindowStyle Hidden -ExecutionPolicy Bypass -EncodedCommand ' + $bootstrapB64
Invoke-CommandInDesktopPackage -PackageFamilyName $env:PROXY_START_PACKAGE_FAMILY -AppId $env:PROXY_START_APPLICATION_ID -Command "$env:SystemRoot\System32\WindowsPowerShell\v1.0\powershell.exe" -Args $arguments -PreventBreakaway`
