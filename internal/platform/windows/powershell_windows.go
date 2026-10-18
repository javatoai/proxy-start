//go:build windows

package windows

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unicode/utf16"
)

type shortcutRow struct {
	TargetPath       string `json:"TargetPath"`
	Arguments        string `json:"Arguments"`
	WorkingDirectory string `json:"WorkingDirectory"`
	Description      string `json:"Description"`
}

func runPowerShellJSON(ctx context.Context, script string, values map[string]string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	path := os.Getenv("SystemRoot")
	if path == "" {
		path = os.Getenv("WINDIR")
	}
	if path == "" {
		path = `C:\Windows`
	}
	powershell := path + `\System32\WindowsPowerShell\v1.0\powershell.exe`
	if _, err := os.Stat(powershell); err != nil {
		powershell = "powershell.exe"
	}
	// Windows PowerShell 5.1 inherits the active console code page (often
	// GBK/CP936) when stdout is redirected. Explicitly selecting UTF-8 keeps
	// localized Start menu names intact across the JSON boundary.
	script = powershellUTF8Preamble + "\r\n" + script
	encoded := base64.StdEncoding.EncodeToString(utf16LE(script))
	cmd := exec.CommandContext(ctx, powershell, "-NoLogo", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-ExecutionPolicy", "Bypass", "-EncodedCommand", encoded)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	cmd.Env = os.Environ()
	for key, value := range values {
		cmd.Env = setEnvironmentValue(cmd.Env, key, value)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		message := strings.TrimSpace(stdout.String())
		if stderrMessage := strings.TrimSpace(stderr.String()); stderrMessage != "" {
			if message == "" {
				message = stderrMessage
			} else {
				message += ": " + stderrMessage
			}
		}
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("PowerShell helper failed: %s", message)
	}
	return strings.TrimSpace(stdout.String()), nil
}

const powershellUTF8Preamble = `$utf8 = New-Object System.Text.UTF8Encoding
[Console]::OutputEncoding = $utf8
$OutputEncoding = $utf8
$ProgressPreference = 'SilentlyContinue'`

func utf16LE(value string) []byte {
	encoded := utf16.Encode([]rune(value))
	bytes := make([]byte, len(encoded)*2)
	for i, codeUnit := range encoded {
		bytes[i*2] = byte(codeUnit)
		bytes[i*2+1] = byte(codeUnit >> 8)
	}
	return bytes
}

func setEnvironmentValue(environment []string, key, value string) []string {
	prefix := strings.ToUpper(key) + "="
	for i, item := range environment {
		if index := strings.IndexByte(item, '='); index >= 0 && strings.EqualFold(item[:index], key) {
			environment[i] = prefix + value
			return environment
		}
	}
	return append(environment, prefix+value)
}

const createNoWindow = 0x08000000
