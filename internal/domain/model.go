// Package domain contains the Codex launch and observation models.
package domain

import (
	"fmt"
	"strconv"
	"time"
)

type AppIdentity struct {
	Name             string   `json:"name"`
	Executable       string   `json:"executable"`
	Arguments        []string `json:"arguments,omitempty"`
	WorkingDirectory string   `json:"workingDirectory,omitempty"`
	// Publisher and SystemApp are catalog metadata. The platform resolver
	// supplies them; domain/application code uses them only for presentation
	// and filtering, never as an application identity key.
	Publisher     string `json:"publisher,omitempty"`
	SystemApp     bool   `json:"systemApp,omitempty"`
	PackageName   string `json:"packageName,omitempty"`
	PackageFamily string `json:"packageFamily,omitempty"`
	ApplicationID string `json:"applicationId,omitempty"`
	AUMID         string `json:"aumid,omitempty"`
}

type LaunchSpec struct {
	Identity         AppIdentity
	Arguments        []string
	Environment      map[string]string
	UnsetEnvironment []string
	// DirectLaunch starts a desktop executable without package activation,
	// while preserving its package identity for discovery and lifecycle checks.
	DirectLaunch bool
}

type ProcessHandle struct {
	PID           int
	ParentPID     int
	StartedAt     time.Time
	Executable    string
	PackageFamily string
}

type Settings struct {
	Version   int `json:"version"`
	ProxyPort int `json:"proxyPort"`
}

func DefaultSettings() Settings { return Settings{Version: 2, ProxyPort: 7897} }
func ValidatePort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("代理端口必须在 1 到 65535 之间")
	}
	return nil
}
func ProxyURL(port int) string { return "http://127.0.0.1:" + strconv.Itoa(port) }

type ProcessInfo struct {
	ParentPID int
	Arguments []string
	Readable  bool
}
type ProxyObservation struct {
	Kind    string
	Address string
	Detail  string
}
