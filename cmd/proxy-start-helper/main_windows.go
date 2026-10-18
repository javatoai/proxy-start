//go:build windows

// proxy-start-helper is intentionally a narrow elevated boundary. It accepts
// only the session/configuration messages emitted by the client and never
// executes arbitrary commands received over IPC.
package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	managedMarkerVersion = 2
	expectedServiceName  = "ProxiFyreService"
	engineName           = "ProxiFyre.exe"
	driverDevicePath     = `\\.\NDISRD`
)

type args struct {
	pipe    string
	token   string
	engine  string
	service string
}

type proxyStartConfig struct {
	LogLevel  string      `json:"logLevel"`
	BypassLAN bool        `json:"bypassLan"`
	Proxies   []proxyRule `json:"proxies"`
	Excludes  []string    `json:"excludes"`
}

type proxyRule struct {
	AppNames                 []string `json:"appNames"`
	Socks5ProxyEndpoint      string   `json:"socks5ProxyEndpoint"`
	Username                 string   `json:"username,omitempty"`
	Password                 string   `json:"password,omitempty"`
	Socks5Transport          string   `json:"socks5Transport"`
	SupportedProtocols       []string `json:"supportedProtocols"`
	SupportedAddressFamilies []string `json:"supportedAddressFamilies"`
}

type helperSession struct {
	AppID           string    `json:"appId"`
	ExecutablePaths []string  `json:"executablePaths"`
	Upstream        proxyRule `json:"upstream"`
}

type helperApplyRequest struct {
	Sessions     []helperSession  `json:"sessions"`
	Config       proxyStartConfig `json:"config"`
	ConfigSHA256 string           `json:"configSha256"`
}

type helperResponse struct {
	OK     bool   `json:"ok"`
	Code   string `json:"code,omitempty"`
	Error  string `json:"error,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type helperEnvelope struct {
	Op       string              `json:"op"`
	Token    string              `json:"token,omitempty"`
	Apply    *helperApplyRequest `json:"apply,omitempty"`
	Response *helperResponse     `json:"response,omitempty"`
}

type helper struct {
	engine             string
	service            string
	configPath         string
	markerPath         string
	sessionID          string
	serviceOwned       bool
	serviceWasRunning  bool
	transactionActive  bool
	originalConfig     []byte
	originalConfigSeen bool
	originalMarker     []byte
	originalMarkerSeen bool
	logOffset          int64
	mu                 sync.Mutex
}

func main() {
	options, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	file, err := os.OpenFile(options.pipe, os.O_RDWR, 0)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open client pipe:", err)
		os.Exit(3)
	}
	defer file.Close()
	if err := validatePipePeer(file); err != nil {
		fmt.Fprintln(os.Stderr, "validate client pipe:", err)
		os.Exit(3)
	}
	reader := bufio.NewReader(file)
	writer := bufio.NewWriter(file)
	writeEnvelope(writer, helperEnvelope{Op: "hello", Token: options.token})
	sessionID, err := newSessionID()
	if err != nil {
		fmt.Fprintln(os.Stderr, "create helper session:", err)
		os.Exit(3)
	}
	enginePath, err := filepath.Abs(options.engine)
	if err != nil {
		fmt.Fprintln(os.Stderr, "resolve engine path:", err)
		os.Exit(3)
	}
	worker := &helper{engine: enginePath, service: expectedServiceName, sessionID: sessionID, configPath: filepath.Join(filepath.Dir(enginePath), "app-config.json"), markerPath: filepath.Join(filepath.Dir(enginePath), ".proxy-start-managed.json")}
	for {
		line, readErr := reader.ReadBytes('\n')
		if readErr != nil {
			worker.shutdown(context.Background())
			if errors.Is(readErr, io.EOF) {
				return
			}
			fmt.Fprintln(os.Stderr, "client pipe disconnected:", readErr)
			return
		}
		var envelope helperEnvelope
		if err := json.Unmarshal(line, &envelope); err != nil {
			writeEnvelope(writer, helperEnvelope{Response: &helperResponse{Code: "invalid_request", Error: err.Error()}})
			continue
		}
		var response helperResponse
		switch envelope.Op {
		case "apply":
			if envelope.Apply == nil {
				response = helperResponse{Code: "invalid_request", Error: "missing apply request"}
			} else {
				response = worker.apply(context.Background(), *envelope.Apply)
			}
		case "shutdown":
			response = worker.shutdown(context.Background())
			writeEnvelope(writer, helperEnvelope{Response: &response})
			return
		default:
			response = helperResponse{Code: "invalid_request", Error: "unsupported helper operation"}
		}
		writeEnvelope(writer, helperEnvelope{Response: &response})
	}
}

func parseArgs(values []string) (args, error) {
	var result args
	seen := make(map[string]bool, 4)
	for i := 0; i < len(values); i++ {
		if i+1 >= len(values) {
			return result, fmt.Errorf("missing value for %s", values[i])
		}
		value := values[i+1]
		switch values[i] {
		case "--pipe":
			if seen["pipe"] {
				return result, errors.New("duplicate --pipe")
			}
			result.pipe = value
			seen["pipe"] = true
		case "--token":
			if seen["token"] {
				return result, errors.New("duplicate --token")
			}
			result.token = value
			seen["token"] = true
		case "--engine":
			if seen["engine"] {
				return result, errors.New("duplicate --engine")
			}
			result.engine = value
			seen["engine"] = true
		case "--service":
			if seen["service"] {
				return result, errors.New("duplicate --service")
			}
			result.service = value
			seen["service"] = true
		default:
			return result, fmt.Errorf("unknown argument %s", values[i])
		}
		i++
	}
	if result.pipe == "" || result.token == "" || result.engine == "" || result.service == "" {
		return result, errors.New("--pipe, --token, --engine and --service are required")
	}
	if !strings.EqualFold(strings.TrimSpace(result.service), expectedServiceName) {
		return result, fmt.Errorf("service %q is not permitted", result.service)
	}
	return result, nil
}

func writeEnvelope(writer *bufio.Writer, envelope helperEnvelope) error {
	data, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	if _, err := writer.Write(append(data, '\n')); err != nil {
		return err
	}
	return writer.Flush()
}

func (h *helper) apply(ctx context.Context, request helperApplyRequest) helperResponse {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.validateRequest(request); err != nil {
		return classifyError(err)
	}
	if len(request.Sessions) == 0 {
		return h.shutdownLocked(ctx)
	}
	if err := h.ensureEngine(); err != nil {
		return classifyError(err)
	}
	data, err := json.MarshalIndent(request.Config, "", "  ")
	if err != nil {
		return helperResponse{Code: "invalid_config", Error: err.Error()}
	}
	data = append(data, '\n')
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != strings.ToLower(request.ConfigSHA256) {
		return helperResponse{Code: "invalid_config", Error: "configuration checksum does not match request"}
	}
	snapshot, err := h.readSnapshot()
	if err != nil {
		return classifyError(err)
	}
	desiredChanged := !snapshot.configExists || !bytesEqual(snapshot.config, data)
	transactionStarted := false
	if desiredChanged {
		transactionStarted = !h.transactionActive
		if err := h.beginTransaction(snapshot, data); err != nil {
			return classifyError(err)
		}
		if err := writeAtomicProtected(h.configPath, data); err != nil {
			if transactionStarted {
				h.forgetTransaction()
			}
			return classifyError(err)
		}
	} else if snapshot.markerExists && !h.markerBelongsToSession(snapshot.marker, snapshot.config) {
		return classifyError(fmt.Errorf("%s: existing managed marker belongs to another helper session", errConfigurationConflict))
	}
	if desiredChanged || !snapshot.serviceRun {
		if err := h.startService(ctx, snapshot.serviceRun); err != nil {
			if desiredChanged {
				_ = h.rollbackApply(ctx, snapshot)
				if transactionStarted {
					h.forgetTransaction()
				}
			} else {
				_ = h.ensureServiceStopped(ctx)
				h.serviceOwned = false
			}
			return classifyError(err)
		}
	}
	if err := h.verifyConfiguration(ctx, data, request.Config.LogLevel, request.Config.Proxies); err != nil {
		_ = h.rollbackApply(ctx, snapshot)
		if transactionStarted {
			h.forgetTransaction()
		}
		return classifyError(err)
	}
	if desiredChanged {
		marker, markerErr := h.markerFor(data)
		if markerErr != nil {
			_ = h.rollbackApply(ctx, snapshot)
			if transactionStarted {
				h.forgetTransaction()
			}
			return classifyError(markerErr)
		}
		if markerErr = writeAtomicProtected(h.markerPath, marker); markerErr != nil {
			_ = h.rollbackApply(ctx, snapshot)
			if transactionStarted {
				h.forgetTransaction()
			}
			return helperResponse{Code: "privilege_required", Error: "configuration applied but managed marker could not be written: " + markerErr.Error()}
		}
	}
	return helperResponse{OK: true, Detail: "ProxiFyreService 已启动，已应用所选程序规则；已验证服务、配置摘要与 NDISRD 设备，未验证实际流量"}
}

func (h *helper) validateRequest(request helperApplyRequest) error {
	if len(request.Sessions) != len(request.Config.Proxies) {
		return fmt.Errorf("invalid session/rule count")
	}
	seenIDs := make(map[string]struct{}, len(request.Sessions))
	assignedPaths := make(map[string]string)
	for index, session := range request.Sessions {
		if strings.TrimSpace(session.AppID) == "" || len(session.ExecutablePaths) == 0 {
			return fmt.Errorf("session %d has no explicit identity", index)
		}
		if _, duplicate := seenIDs[session.AppID]; duplicate {
			return fmt.Errorf("duplicate app id %q", session.AppID)
		}
		seenIDs[session.AppID] = struct{}{}
		rule := request.Config.Proxies[index]
		if rule.Socks5Transport != "TCP" {
			return fmt.Errorf("session %q has unsupported SOCKS5 transport", session.AppID)
		}
		if host, port, splitErr := net.SplitHostPort(rule.Socks5ProxyEndpoint); splitErr != nil || strings.TrimSpace(host) == "" || port == "" {
			return fmt.Errorf("session %q has invalid SOCKS5 endpoint", session.AppID)
		}
		if len(rule.AppNames) == 0 {
			return fmt.Errorf("session %q has no explicit app names", session.AppID)
		}
		for _, name := range rule.AppNames {
			if strings.TrimSpace(name) == "" {
				return fmt.Errorf("session %q contains a catch-all app name", session.AppID)
			}
		}
		if !sameStrings(rule.AppNames, session.Upstream.AppNames) || rule.Socks5ProxyEndpoint != session.Upstream.Socks5ProxyEndpoint {
			return fmt.Errorf("session %q rule association changed in transit", session.AppID)
		}
		if !hasTCPUDP(rule.SupportedProtocols) {
			return fmt.Errorf("session %q must declare TCP and UDP", session.AppID)
		}
		if len(session.ExecutablePaths) != len(uniquePaths(session.ExecutablePaths)) {
			return fmt.Errorf("session %q has duplicate executable paths", session.AppID)
		}
		endpoint := strings.ToLower(strings.TrimSpace(rule.Socks5ProxyEndpoint))
		for _, path := range session.ExecutablePaths {
			if strings.ContainsRune(path, '\x00') || !filepath.IsAbs(path) {
				return fmt.Errorf("session %q contains a non-absolute executable path", session.AppID)
			}
			key := strings.ToLower(filepath.Clean(strings.TrimSpace(path)))
			if prior, exists := assignedPaths[key]; exists && prior != endpoint {
				return fmt.Errorf("session %q reuses executable path %q with a different proxy", session.AppID, path)
			}
			assignedPaths[key] = endpoint
		}
	}
	return nil
}

func hasTCPUDP(protocols []string) bool {
	hasTCP, hasUDP := false, false
	for _, protocol := range protocols {
		switch strings.ToUpper(protocol) {
		case "TCP":
			hasTCP = true
		case "UDP":
			hasUDP = true
		}
	}
	return hasTCP && hasUDP
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if strings.ToLower(strings.TrimSpace(left[i])) != strings.ToLower(strings.TrimSpace(right[i])) {
			return false
		}
	}
	return true
}

func uniquePaths(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		key := strings.ToLower(filepath.Clean(value))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func (h *helper) ensureEngine() error {
	if !strings.EqualFold(h.service, expectedServiceName) {
		return fmt.Errorf("service %q is not permitted", h.service)
	}
	path, err := validateProtectedEnginePath(h.engine)
	if err != nil {
		return err
	}
	h.engine = path
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("%s is unavailable: %w", engineName, err)
	}
	query := systemCommand(context.Background(), "sc.exe", "qc", expectedServiceName)
	output, err := query.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s is not installed: %w (%s)", expectedServiceName, err, strings.TrimSpace(string(output)))
	}
	registered, err := registeredEnginePath(string(output))
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Clean(registered), filepath.Clean(path)) {
		return fmt.Errorf("%s is registered to another engine path", expectedServiceName)
	}
	return nil
}

func (h *helper) startService(ctx context.Context, wasRunning bool) error {
	if wasRunning {
		if err := runSC(ctx, "stop", expectedServiceName); err != nil {
			return err
		}
		if err := waitService(ctx, expectedServiceName, false); err != nil {
			return err
		}
	}
	if err := runSC(ctx, "start", expectedServiceName); err != nil {
		return err
	}
	if err := waitService(ctx, expectedServiceName, true); err != nil {
		return err
	}
	h.serviceOwned = true
	return nil
}

func (h *helper) shutdown(ctx context.Context) helperResponse {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.shutdownLocked(ctx)
}

func (h *helper) shutdownLocked(ctx context.Context) helperResponse {
	var joined error
	if h.serviceOwned {
		if err := h.ensureServiceStopped(ctx); err != nil {
			joined = errors.Join(joined, err)
		}
	}
	if h.transactionActive {
		baseline := configSnapshot{config: h.originalConfig, configExists: h.originalConfigSeen, marker: h.originalMarker, markerExists: h.originalMarkerSeen, serviceRun: h.serviceWasRunning}
		if err := h.restoreSnapshot(baseline); err != nil {
			joined = errors.Join(joined, err)
		}
		if baseline.serviceRun {
			if err := h.ensureServiceRunning(ctx); err != nil {
				joined = errors.Join(joined, err)
			}
		} else if err := h.ensureServiceStopped(ctx); err != nil {
			joined = errors.Join(joined, err)
		}
	}
	h.serviceOwned = false
	h.transactionActive = false
	if joined != nil {
		return classifyError(joined)
	}
	return helperResponse{OK: true, Detail: "Proxy Start 已释放其配置与代理组件；未停止未由本会话启动的服务"}
}

func runSC(ctx context.Context, action, service string) error {
	if !strings.EqualFold(service, expectedServiceName) {
		return fmt.Errorf("service %q is not permitted", service)
	}
	command := systemCommand(ctx, "sc.exe", action, expectedServiceName)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sc %s %s: %w (%s)", action, service, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func waitService(ctx context.Context, service string, running bool) error {
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if !strings.EqualFold(service, expectedServiceName) {
			return fmt.Errorf("service %q is not permitted", service)
		}
		query := systemCommand(ctx, "sc.exe", "query", expectedServiceName)
		output, err := query.CombinedOutput()
		if err == nil {
			isRunning := strings.Contains(strings.ToUpper(string(output)), "RUNNING")
			if isRunning == running {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("timed out waiting for ProxiFyreService state")
		case <-ticker.C:
		}
	}
}

const errConfigurationConflict = "configuration_conflict"

func classifyError(err error) helperResponse {
	if err == nil {
		return helperResponse{OK: true}
	}
	message := err.Error()
	code := "helper_error"
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "not installed"), strings.Contains(lower, "unavailable"):
		code = "component_missing"
	case strings.Contains(lower, "permission"), strings.Contains(lower, "access denied"), strings.Contains(lower, "privilege"):
		code = "privilege_required"
	case strings.Contains(lower, errConfigurationConflict):
		code = errConfigurationConflict
	case strings.Contains(lower, "session"), strings.Contains(lower, "app name"):
		code = "invalid_session"
	}
	return helperResponse{Code: code, Error: message}
}
