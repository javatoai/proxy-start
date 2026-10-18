// Package domain contains platform-independent application and proxy concepts.
package domain

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

type Protocol string

const (
	ProtocolAuto   Protocol = "auto"
	ProtocolHTTP   Protocol = "http"
	ProtocolSOCKS5 Protocol = "socks5"
)

type ProxyProfile struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Protocol      Protocol `json:"protocol"`
	Host          string   `json:"host"`
	Port          int      `json:"port"`
	Username      string   `json:"username,omitempty"`
	CredentialRef string   `json:"credentialRef,omitempty"`
}

func (p ProxyProfile) Validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("请填写代理名称")
	}
	if strings.TrimSpace(p.Host) == "" || strings.ContainsAny(p.Host, "/\\\r\n\t ") {
		return fmt.Errorf("请填写主机名或 IP，地址中不要包含协议或端口")
	}
	if p.Port < 1 || p.Port > 65535 {
		return fmt.Errorf("代理端口必须在 1 到 65535 之间")
	}
	if p.Protocol != ProtocolAuto && p.Protocol != ProtocolHTTP && p.Protocol != ProtocolSOCKS5 {
		return fmt.Errorf("不支持的代理协议")
	}
	return nil
}

func (p ProxyProfile) Address() string {
	return net.JoinHostPort(strings.Trim(p.Host, "[]"), strconv.Itoa(p.Port))
}

// ProxyEndpoint is runtime-only. Password must never be serialized or logged.
type ProxyEndpoint struct {
	Protocol Protocol
	Host     string
	Port     int
	Username string
	Password string
}

func (p ProxyEndpoint) Address() string {
	return net.JoinHostPort(strings.Trim(p.Host, "[]"), strconv.Itoa(p.Port))
}

type ProbeResult struct {
	HTTP           bool
	SOCKS5         bool
	HTTPVerified   bool
	SOCKS5Verified bool
	TCPVerified    bool
	UDPVerified    bool
	Detail         string
	CheckedAt      time.Time
}

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

type AppEntry struct {
	ID                 string      `json:"id"`
	Name               string      `json:"name"`
	Source             string      `json:"source,omitempty"`
	Identity           AppIdentity `json:"identity"`
	ProxyID            string      `json:"proxyId,omitempty"`
	RelatedExecutables []string    `json:"relatedExecutables,omitempty"`
}

type LaunchMode string

const (
	ModeNative LaunchMode = "native"
	ModeForced LaunchMode = "forced"
)

type LaunchSpec struct {
	Identity         AppIdentity
	Arguments        []string
	Environment      map[string]string
	UnsetEnvironment []string
}

type LaunchPlan struct {
	Mode      LaunchMode
	AdapterID string
	Reason    string
	Spec      LaunchSpec
	Proxy     ProxyEndpoint
}

type ProcessHandle struct {
	PID           int
	StartedAt     time.Time
	Executable    string
	PackageFamily string
}

type RuntimeState string

const (
	StateStopped  RuntimeState = "stopped"
	StateStarting RuntimeState = "starting"
	StateRunning  RuntimeState = "running"
	StateStopping RuntimeState = "stopping"
	StateFailed   RuntimeState = "failed"
)

type AppRuntime struct {
	AppID     string
	State     RuntimeState
	Mode      LaunchMode
	AdapterID string
	Detail    string
	Error     string
	Processes []ProcessHandle
	// EffectiveIdentity is the identity resolved at launch time. It is kept
	// separate from the persisted AppEntry identity because shortcuts and
	// packaged applications can change between launches.
	EffectiveIdentity  AppIdentity
	RelatedExecutables []string
	ProxyApplied       bool
	ConnectionVerified bool
	// CleanupPending means the target process is no longer managed, but the
	// forced-proxy lease still needs a successful release.
	CleanupPending bool
	UpdatedAt      time.Time
}

type ForcedSession struct {
	AppID           string
	ExecutablePaths []string
	Proxy           ProxyEndpoint
}

type BackendStatus struct {
	Installed bool
	Ready     bool
	Running   bool
	Detail    string
}

type Settings struct {
	Version        int            `json:"version"`
	DefaultProxyID string         `json:"defaultProxyId"`
	Profiles       []ProxyProfile `json:"profiles"`
	Apps           []AppEntry     `json:"apps"`
	Theme          string         `json:"theme,omitempty"`
	// ShowSystemApps defaults to false when absent in older settings files.
	// The UI exposes the inverse preference as “隐藏系统软件”.
	ShowSystemApps bool `json:"showSystemApps,omitempty"`
}

func DefaultSettings() Settings {
	return Settings{Version: 1, DefaultProxyID: "local-clash", Theme: "system", Profiles: []ProxyProfile{{ID: "local-clash", Name: "本机 Clash", Protocol: ProtocolAuto, Host: "127.0.0.1", Port: 7897}}, Apps: []AppEntry{}}
}
