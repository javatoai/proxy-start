//go:build darwin

package darwin

import (
	"context"
	"errors"
	"github.com/ebitengine/purego"
	"os/exec"
	"proxy-start/internal/domain"
	"strconv"
	"strings"
	"unsafe"
)

var querySysctl func(*int32, uint32, unsafe.Pointer, *uintptr, unsafe.Pointer, uintptr) int32

func init() {
	if library, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_LAZY); err == nil {
		purego.RegisterLibFunc(&querySysctl, library, "sysctl")
	}
}

func (c *ProcessController) Inspect(ctx context.Context, handle domain.ProcessHandle) (domain.ProcessInfo, error) {
	if err := ctx.Err(); err != nil {
		return domain.ProcessInfo{}, err
	}
	if !c.Alive(ctx, handle) || querySysctl == nil {
		return domain.ProcessInfo{}, errors.New("无法读取进程身份")
	}
	parent, err := exec.CommandContext(ctx, "/bin/ps", "-p", strconv.Itoa(handle.PID), "-o", "ppid=").Output()
	if err != nil {
		return domain.ProcessInfo{}, err
	}
	parentPID, _ := strconv.Atoi(strings.TrimSpace(string(parent)))
	info := domain.ProcessInfo{ParentPID: parentPID}
	// CTL_KERN/KERN_PROCARGS2 returns argc, executable, argv, then environment.
	// Parse exactly argc entries; environment values are never inspected.
	mib := []int32{1, 49, int32(handle.PID)}
	data := make([]byte, 1024*1024)
	size := uintptr(len(data))
	if querySysctl(&mib[0], uint32(len(mib)), unsafe.Pointer(&data[0]), &size, nil, 0) != 0 || size > uintptr(len(data)) {
		return info, nil
	}
	args, err := parseProcArgs(data[:size])
	if err != nil {
		return info, err
	}
	if !c.Alive(ctx, handle) {
		return info, errors.New("进程身份已改变")
	}
	info.Arguments, info.Readable = args, true
	return info, nil
}
