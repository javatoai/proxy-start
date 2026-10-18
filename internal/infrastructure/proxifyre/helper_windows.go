//go:build windows

package proxifyre

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	pipeAccessDuplex        = 0x00000003
	pipeTypeByte            = 0x00000000
	pipeReadModeByte        = 0x00000000
	pipeWait                = 0x00000000
	pipeRejectRemoteClients = 0x00000008
	seeMaskNoCloseProcess   = 0x00000040
	swShown                 = 5
	errorPipeConnected      = syscall.Errno(535)
)

var (
	kernel32                        = syscall.NewLazyDLL("kernel32.dll")
	createNamedPipeW                = kernel32.NewProc("CreateNamedPipeW")
	connectNamedPipe                = kernel32.NewProc("ConnectNamedPipe")
	disconnectNamedPipe             = kernel32.NewProc("DisconnectNamedPipe")
	closeHandle                     = kernel32.NewProc("CloseHandle")
	getNamedPipeClientProcessID     = kernel32.NewProc("GetNamedPipeClientProcessId")
	getProcessID                    = kernel32.NewProc("GetProcessId")
	queryFullProcessImageNameW      = kernel32.NewProc("QueryFullProcessImageNameW")
	getExitCodeProcess              = kernel32.NewProc("GetExitCodeProcess")
	localFree                       = kernel32.NewProc("LocalFree")
	shell32                         = syscall.NewLazyDLL("shell32.dll")
	shellExecuteExW                 = shell32.NewProc("ShellExecuteExW")
	advapi32                        = syscall.NewLazyDLL("advapi32.dll")
	convertStringSecurityDescriptor = advapi32.NewProc("ConvertStringSecurityDescriptorToSecurityDescriptorW")
	commandLineToArgvW              = shell32.NewProc("CommandLineToArgvW")
)

type securityAttributes struct {
	length             uint32
	securityDescriptor uintptr
	inheritHandle      int32
}

type shellExecuteInfo struct {
	cbSize       uint32
	fMask        uint32
	hwnd         uintptr
	lpVerb       *uint16
	lpFile       *uint16
	lpParameters *uint16
	lpDirectory  *uint16
	nShow        int32
	hInstApp     uintptr
	lpIDList     uintptr
	lpClass      *uint16
	hkeyClass    uintptr
	dwHotKey     uint32
	hIcon        uintptr
	hProcess     syscall.Handle
}

type pipeServer struct {
	handle syscall.Handle
	name   string
}

type pipeConnection struct {
	file *os.File
	pid  uint32
}

type helperClient struct {
	file   *os.File
	reader *bufio.Reader
	writer *bufio.Writer
	mu     sync.Mutex
}

func (b *Backend) ensureHelper(ctx context.Context) (helperTransport, error) {
	b.stateMu.RLock()
	current := b.helper
	b.stateMu.RUnlock()
	if current != nil {
		return current, nil
	}
	if strings.TrimSpace(b.opts.HelperPath) == "" {
		return nil, fmt.Errorf("%w: helper path is empty", ErrComponentMissing)
	}
	helper, err := filepath.Abs(b.opts.HelperPath)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(helper); err != nil {
		return nil, fmt.Errorf("%w: helper %q is unavailable: %v", ErrComponentMissing, helper, err)
	}
	if b.opts.HelperSHA256 != "" {
		if err := verifySHA256(ctx, helper, b.opts.HelperSHA256); err != nil {
			return nil, fmt.Errorf("%w: helper integrity check failed: %v", ErrComponentMissing, err)
		}
	}
	if strings.TrimSpace(b.opts.EnginePath) == "" {
		return nil, fmt.Errorf("%w: configure the installed ProxiFyre.exe path", ErrComponentMissing)
	}
	pipe, err := newPipeServer()
	if err != nil {
		return nil, fmt.Errorf("create authenticated helper pipe: %w", err)
	}
	token, err := randomToken()
	if err != nil {
		pipe.Close()
		return nil, err
	}
	processHandle, err := launchElevatedHelper(helper, pipe.name, token, b.opts.EnginePath, b.opts.ServiceName)
	if err != nil {
		pipe.Close()
		return nil, err
	}
	connection, err := pipe.accept(ctx, b.opts.PipeTimeout)
	if err != nil {
		closeProcessHandle(processHandle)
		pipe.Close()
		return nil, fmt.Errorf("%w: %v", ErrHelperUnavailable, err)
	}
	if err := validateHelperProcess(processHandle, connection.pid, helper); err != nil {
		_ = connection.file.Close()
		closeProcessHandle(processHandle)
		return nil, fmt.Errorf("%w: %v", ErrHelperUnavailable, err)
	}
	closeProcessHandle(processHandle)
	client := &helperClient{file: connection.file, reader: bufio.NewReader(connection.file), writer: bufio.NewWriter(connection.file)}
	if err := client.expectHello(ctx, token); err != nil {
		client.Close()
		return nil, fmt.Errorf("%w: helper authentication failed: %v", ErrHelperUnavailable, err)
	}
	b.stateMu.Lock()
	if b.helper == nil {
		b.helper = client
		current = client
	} else {
		current = b.helper
	}
	b.stateMu.Unlock()
	if current != client {
		_ = client.Close()
	}
	return current, nil
}

func newPipeServer() (*pipeServer, error) {
	token, err := randomToken()
	if err != nil {
		return nil, err
	}
	name := `\\.\pipe\proxy-start-` + token
	namePtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	// Explicitly limit the pipe to the current owner, local administrators and
	// SYSTEM. The random handshake token provides a second authentication factor
	// for any process that can discover the pipe name.
	sddl, err := syscall.UTF16PtrFromString("D:P(A;;GA;;;OW)(A;;GA;;;BA)(A;;GA;;;SY)")
	if err != nil {
		return nil, err
	}
	var descriptor uintptr
	if result, _, callErr := convertStringSecurityDescriptor.Call(uintptr(unsafe.Pointer(sddl)), 1, uintptr(unsafe.Pointer(&descriptor)), 0); result == 0 {
		return nil, callErr
	}
	defer localFree.Call(descriptor)
	security := &securityAttributes{length: uint32(unsafe.Sizeof(securityAttributes{})), securityDescriptor: descriptor}
	handle, _, callErr := createNamedPipeW.Call(
		uintptr(unsafe.Pointer(namePtr)),
		pipeAccessDuplex,
		pipeTypeByte|pipeReadModeByte|pipeWait|pipeRejectRemoteClients,
		1, 64*1024, 64*1024, 0, uintptr(unsafe.Pointer(security)),
	)
	if handle == uintptr(syscall.InvalidHandle) {
		return nil, callErr
	}
	return &pipeServer{handle: syscall.Handle(handle), name: name}, nil
}

func (p *pipeServer) accept(ctx context.Context, timeout time.Duration) (pipeConnection, error) {
	if timeout <= 0 {
		timeout = defaultPipeTimeout
	}
	result := make(chan error, 1)
	go func() {
		r, _, err := connectNamedPipe.Call(uintptr(p.handle), 0)
		if r == 0 && err != nil && err != errorPipeConnected {
			result <- err
			return
		}
		result <- nil
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-result:
		if err != nil {
			return pipeConnection{}, err
		}
		var pid uint32
		if result, _, callErr := getNamedPipeClientProcessID.Call(uintptr(p.handle), uintptr(unsafe.Pointer(&pid))); result == 0 {
			return pipeConnection{}, callErr
		}
		file := os.NewFile(uintptr(p.handle), p.name)
		p.handle = syscall.InvalidHandle
		return pipeConnection{file: file, pid: pid}, nil
	case <-ctx.Done():
		p.Close()
		return pipeConnection{}, ctx.Err()
	case <-timer.C:
		p.Close()
		return pipeConnection{}, fmt.Errorf("timed out waiting for elevated helper")
	}
}

func (p *pipeServer) Close() error {
	if p == nil || p.handle == syscall.InvalidHandle {
		return nil
	}
	_, _, _ = disconnectNamedPipe.Call(uintptr(p.handle))
	_, _, _ = closeHandle.Call(uintptr(p.handle))
	p.handle = syscall.InvalidHandle
	return nil
}

func (c *helperClient) expectHello(ctx context.Context, token string) error {
	line, err := c.readLine(ctx)
	if err != nil {
		return err
	}
	var envelope helperEnvelope
	if err := json.Unmarshal(line, &envelope); err != nil {
		return err
	}
	if envelope.Op != "hello" || envelope.Token != token {
		return errors.New("unexpected helper handshake")
	}
	return nil
}

func (c *helperClient) Apply(ctx context.Context, request helperApplyRequest) (helperResponse, error) {
	return c.call(ctx, helperEnvelope{Op: "apply", Apply: &request})
}

func (c *helperClient) Shutdown(ctx context.Context) (helperResponse, error) {
	return c.call(ctx, helperEnvelope{Op: "shutdown"})
}

func (c *helperClient) call(ctx context.Context, request helperEnvelope) (helperResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	data, err := marshalEnvelope(request)
	if err != nil {
		return helperResponse{}, err
	}
	if _, err := c.writer.Write(data); err != nil {
		return helperResponse{}, err
	}
	if err := c.writer.Flush(); err != nil {
		return helperResponse{}, err
	}
	line, err := c.readLine(ctx)
	if err != nil {
		return helperResponse{}, err
	}
	var envelope helperEnvelope
	if err := json.Unmarshal(line, &envelope); err != nil {
		return helperResponse{}, err
	}
	if envelope.Response == nil {
		return helperResponse{}, errors.New("helper returned no response")
	}
	return *envelope.Response, nil
}

func (c *helperClient) readLine(ctx context.Context) ([]byte, error) {
	result := make(chan struct {
		line []byte
		err  error
	}, 1)
	go func() {
		line, err := c.reader.ReadBytes('\n')
		result <- struct {
			line []byte
			err  error
		}{line: line, err: err}
	}()
	select {
	case value := <-result:
		if value.err != nil {
			return nil, value.err
		}
		return value.line, nil
	case <-ctx.Done():
		// Closing the pipe is deliberate: it unblocks the one reader goroutine
		// and prevents a later request from competing with it.
		_ = c.Close()
		return nil, ctx.Err()
	}
}

func (c *helperClient) Close() error {
	if c == nil || c.file == nil {
		return nil
	}
	return c.file.Close()
}

func randomToken() (string, error) {
	var data [24]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(data[:]), nil
}

func launchElevatedHelper(path, pipeName, token, enginePath, serviceName string) (syscall.Handle, error) {
	return shellExecuteRunAs(path, helperLaunchArguments(pipeName, token, enginePath, serviceName))
}

func helperLaunchArguments(pipeName, token, enginePath, serviceName string) string {
	return strings.Join([]string{
		syscall.EscapeArg("--pipe"), syscall.EscapeArg(pipeName),
		syscall.EscapeArg("--token"), syscall.EscapeArg(token),
		syscall.EscapeArg("--engine"), syscall.EscapeArg(enginePath),
		syscall.EscapeArg("--service"), syscall.EscapeArg(serviceName),
	}, " ")
}

func launchInstaller(ctx context.Context, path string) error {
	handle, err := shellExecuteRunAs(path, "")
	if err != nil {
		return err
	}
	defer closeProcessHandle(handle)
	return waitProcess(ctx, handle)
}

func shellExecuteRunAs(path, args string) (syscall.Handle, error) {
	verb, err := syscall.UTF16PtrFromString("runas")
	if err != nil {
		return 0, err
	}
	file, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var params *uint16
	if args != "" {
		params, err = syscall.UTF16PtrFromString(args)
		if err != nil {
			return 0, err
		}
	}
	info := shellExecuteInfo{cbSize: uint32(unsafe.Sizeof(shellExecuteInfo{})), fMask: seeMaskNoCloseProcess, lpVerb: verb, lpFile: file, lpParameters: params, nShow: swShown}
	ret, _, callErr := shellExecuteExW.Call(uintptr(unsafe.Pointer(&info)))
	if ret == 0 {
		if callErr == nil {
			callErr = syscall.EINVAL
		}
		if errors.Is(callErr, syscall.ERROR_ACCESS_DENIED) {
			return 0, fmt.Errorf("%w: UAC elevation was cancelled or denied", ErrPrivilegeRequired)
		}
		return 0, fmt.Errorf("launch elevated process: %w", callErr)
	}
	return info.hProcess, nil
}

func closeProcessHandle(handle syscall.Handle) {
	if handle != 0 {
		_, _, _ = closeHandle.Call(uintptr(handle))
	}
}

func waitProcess(ctx context.Context, handle syscall.Handle) error {
	if handle == 0 {
		return errors.New("elevated installer returned no process handle")
	}
	waitForSingleObject := kernel32.NewProc("WaitForSingleObject")
	for {
		result, _, _ := waitForSingleObject.Call(uintptr(handle), 100)
		switch result {
		case 0:
			var exitCode uint32
			if ok, _, callErr := getExitCodeProcess.Call(uintptr(handle), uintptr(unsafe.Pointer(&exitCode))); ok == 0 {
				return fmt.Errorf("read elevated installer exit code: %w", callErr)
			}
			if exitCode != 0 {
				return fmt.Errorf("elevated installer exited with code %d", exitCode)
			}
			return nil
		case 0x102: // WAIT_TIMEOUT
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		default:
			return fmt.Errorf("wait for elevated installer: Win32 status 0x%x", result)
		}
	}
}

func validateHelperProcess(launched syscall.Handle, connectedPID uint32, expectedPath string) error {
	if launched == 0 || connectedPID == 0 {
		return errors.New("helper process identity is unavailable")
	}
	launchedPID, _, _ := getProcessID.Call(uintptr(launched))
	if uint32(launchedPID) != connectedPID {
		return fmt.Errorf("named pipe connected to unexpected process %d", connectedPID)
	}
	actual, err := processImagePath(launched)
	if err != nil {
		return err
	}
	expected, err := filepath.Abs(expectedPath)
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Clean(actual), filepath.Clean(expected)) {
		return fmt.Errorf("named pipe process image %q does not match helper %q", actual, expected)
	}
	return nil
}

func processImagePath(handle syscall.Handle) (string, error) {
	buffer := make([]uint16, 32*1024)
	length := uint32(len(buffer))
	result, _, callErr := queryFullProcessImageNameW.Call(uintptr(handle), 0, uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&length)))
	if result == 0 {
		return "", callErr
	}
	return syscall.UTF16ToString(buffer[:length]), nil
}
