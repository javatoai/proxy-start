//go:build windows

package windows

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

const packagedLaunchFlag = "--internal-package-launch"

type packagedLaunchRequest struct {
	Launch     packagedLaunchConfig `json:"launch"`
	ResultPath string               `json:"resultPath"`
}

func packagedEnvironment(config packagedLaunchConfig) []string {
	environment := mergeEnvironment(nil, config.UnsetEnvironment)
	for _, variable := range config.Environment {
		environment = setEnvironmentValue(environment, variable.Name, variable.Value)
	}
	return environment
}

// RunPackagedLaunch handles the GUI binary's background-only entry before
// configuration, discovery, single-instance handling or UI initialization.
// The package activator launches this GUI binary without allocating a console;
// CREATE_NO_WINDOW also prevents console allocation for its target child.
func RunPackagedLaunch(args []string) (bool, error) {
	if len(args) == 0 || args[0] != packagedLaunchFlag {
		return false, nil
	}
	if len(args) != 2 {
		return true, errors.New("包内启动请求参数不完整")
	}
	data, err := base64.StdEncoding.DecodeString(args[1])
	if err != nil {
		return true, fmt.Errorf("解码包内启动请求失败: %w", err)
	}
	var request packagedLaunchRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return true, fmt.Errorf("解析包内启动请求失败: %w", err)
	}
	if !filepath.IsAbs(request.ResultPath) || !filepath.IsAbs(request.Launch.Executable) {
		return true, errors.New("包内启动请求需要绝对路径")
	}
	config := request.Launch
	cmd := exec.Command(config.Executable)
	cmd.Dir = config.WorkingDirectory
	cmd.Env = packagedEnvironment(config)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine:          quoteWindowsArgument(config.Executable) + " " + config.Arguments,
		CreationFlags:    createNoWindow | createNewProcessGroup,
		NoInheritHandles: true,
	}
	result := packagedLaunchResult{}
	if err := cmd.Start(); err != nil {
		result.Error = fmt.Sprintf("启动 Codex 失败: %v", err)
	} else {
		result.ProcessID = cmd.Process.Pid
		_ = cmd.Process.Release()
	}
	data, err = json.Marshal(result)
	if err != nil {
		return true, err
	}
	temporary := request.ResultPath + ".tmp"
	defer os.Remove(temporary)
	if err := os.WriteFile(temporary, data, 0600); err != nil {
		return true, fmt.Errorf("写入包内启动回执失败: %w", err)
	}
	return true, os.Rename(temporary, request.ResultPath)
}
