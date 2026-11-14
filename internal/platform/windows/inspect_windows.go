//go:build windows

package windows

import (
	"context"
	"encoding/json"
	"errors"
	"proxy-start/internal/domain"
	"strconv"
)

func (c *ProcessController) Inspect(ctx context.Context, handle domain.ProcessHandle) (domain.ProcessInfo, error) {
	if !c.Alive(ctx, handle) {
		return domain.ProcessInfo{}, errors.New("进程身份已改变")
	}
	output, err := runPowerShellJSON(ctx, `$ErrorActionPreference = 'Stop'
$p = Get-CimInstance Win32_Process -Filter ("ProcessId=" + $env:PROXY_START_INSPECT_PID)
if (-not $p) { throw 'Process exited' }
[pscustomobject]@{parentPID=[int]$p.ParentProcessId; commandLine=[string]$p.CommandLine} | ConvertTo-Json -Compress`,
		map[string]string{"PROXY_START_INSPECT_PID": strconv.Itoa(handle.PID)})
	if err != nil {
		return domain.ProcessInfo{}, err
	}
	var row struct {
		ParentPID   int
		CommandLine string
	}
	if err := json.Unmarshal([]byte(output), &row); err != nil {
		return domain.ProcessInfo{}, err
	}
	info := domain.ProcessInfo{ParentPID: row.ParentPID}
	if !c.Alive(ctx, handle) {
		return info, errors.New("进程身份已改变")
	}
	if row.CommandLine != "" {
		info.Arguments, info.Readable = splitWindowsCommandLine(row.CommandLine), true
	}
	return info, nil
}
