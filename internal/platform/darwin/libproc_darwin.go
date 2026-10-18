//go:build darwin

package darwin

import (
	"errors"
	"fmt"
	"strings"
	"unsafe"

	"github.com/ebitengine/purego"
)

var procPIDPath func(int32, unsafe.Pointer, uint32) int32

func init() {
	if library, err := purego.Dlopen("/usr/lib/libproc.dylib", purego.RTLD_LAZY); err == nil {
		purego.RegisterLibFunc(&procPIDPath, library, "proc_pidpath")
	}
}

func processExecutablePath(pid int) (string, error) {
	if pid <= 0 {
		return "", errors.New("无效的进程 ID")
	}
	if procPIDPath == nil {
		return "", errors.New("libproc 不可用")
	}
	buffer := make([]byte, 4096)
	length := procPIDPath(int32(pid), unsafe.Pointer(&buffer[0]), uint32(len(buffer)))
	if length <= 0 || int(length) >= len(buffer) {
		return "", fmt.Errorf("无法读取 PID %d 的真实可执行路径", pid)
	}
	path := strings.TrimRight(string(buffer[:length]), "\x00")
	if path == "" {
		return "", fmt.Errorf("PID %d 的可执行路径为空", pid)
	}
	return cleanExecutable(path), nil
}
