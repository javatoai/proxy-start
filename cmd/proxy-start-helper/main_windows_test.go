//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseArgsRejectsArbitraryService(t *testing.T) {
	_, err := parseArgs([]string{"--pipe", `\\.\pipe\test`, "--token", "token", "--engine", `C:\Program Files\ProxiFyre\ProxiFyre.exe`, "--service", "OtherService"})
	if err == nil || !strings.Contains(err.Error(), "not permitted") {
		t.Fatalf("expected fixed-service rejection, got %v", err)
	}
}

func TestProtectedEnginePathRejectsUserWritableLocation(t *testing.T) {
	path := filepath.Join(t.TempDir(), engineName)
	if _, err := validateProtectedEnginePath(path); err == nil {
		t.Fatal("user-writable engine location was accepted")
	}
}

func TestRegisteredEnginePathParsesQuotedImage(t *testing.T) {
	path, err := registeredEnginePath(`SERVICE_NAME: ProxiFyreService
        BINARY_PATH_NAME   : "C:\Program Files\ProxiFyre\ProxiFyre.exe" --service`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(path, `C:\Program Files\ProxiFyre\ProxiFyre.exe`) {
		t.Fatalf("unexpected path %q", path)
	}
}

func TestMarkerOwnershipRequiresCurrentSessionAndDigest(t *testing.T) {
	h := &helper{engine: `C:\Program Files\ProxiFyre\ProxiFyre.exe`, service: expectedServiceName, sessionID: "session-a"}
	config := []byte(`{"proxies":[]}`)
	marker, err := h.markerFor(config)
	if err != nil {
		t.Fatal(err)
	}
	if !h.markerBelongsToSession(marker, config) {
		t.Fatal("current session marker was rejected")
	}
	h.sessionID = "session-b"
	if h.markerBelongsToSession(marker, config) {
		t.Fatal("marker from another helper session was accepted")
	}
}

func TestValidateRequestRejectsPathWithDifferentProxy(t *testing.T) {
	path := `C:\Program Files\Example\app.exe`
	request := helperApplyRequest{
		Sessions: []helperSession{
			{AppID: "one", ExecutablePaths: []string{path}, Upstream: proxyRule{AppNames: []string{path}, Socks5ProxyEndpoint: "127.0.0.1:1", Socks5Transport: "TCP"}},
			{AppID: "two", ExecutablePaths: []string{path}, Upstream: proxyRule{AppNames: []string{path}, Socks5ProxyEndpoint: "127.0.0.1:2", Socks5Transport: "TCP"}},
		},
		Config: proxyStartConfig{Proxies: []proxyRule{
			{AppNames: []string{path}, Socks5ProxyEndpoint: "127.0.0.1:1", Socks5Transport: "TCP", SupportedProtocols: []string{"TCP", "UDP"}},
			{AppNames: []string{path}, Socks5ProxyEndpoint: "127.0.0.1:2", Socks5Transport: "TCP", SupportedProtocols: []string{"TCP", "UDP"}},
		}},
	}
	if err := (&helper{}).validateRequest(request); err == nil || !strings.Contains(err.Error(), "different proxy") {
		t.Fatalf("expected duplicate path rejection, got %v", err)
	}
}

func TestVerifyInfoLogOnceRequiresFreshServiceAndAssociations(t *testing.T) {
	root := t.TempDir()
	logDir := filepath.Join(root, "logs")
	if err := os.MkdirAll(logDir, 0700); err != nil {
		t.Fatal(err)
	}
	app := `C:\Program Files\Example\alpha.exe`
	endpoint := "127.0.0.1:7897"
	old := "ProxiFyre Service is running.\nSuccessfully associated old.exe to 127.0.0.1:1 TCP proxy with protocols TCP, UDP\n"
	fresh := "ProxiFyre Service is running.\nSuccessfully associated " + app + " to " + endpoint + " TCP proxy with protocols TCP, UDP\n"
	logPath := filepath.Join(logDir, "logfile.txt")
	if err := os.WriteFile(logPath, []byte(old+fresh), 0600); err != nil {
		t.Fatal(err)
	}
	h := &helper{engine: filepath.Join(root, engineName), logOffset: int64(len(old))}
	rule := proxyRule{AppNames: []string{app}, Socks5ProxyEndpoint: endpoint}
	if err := h.verifyInfoLogOnce([]proxyRule{rule}); err != nil {
		t.Fatalf("fresh upstream association was rejected: %v", err)
	}
}

func TestVerifyInfoLogOnceRejectsOldAssociationsAndMissingRules(t *testing.T) {
	root := t.TempDir()
	logDir := filepath.Join(root, "logs")
	if err := os.MkdirAll(logDir, 0700); err != nil {
		t.Fatal(err)
	}
	appA := `C:\Program Files\Example\alpha.exe`
	appB := `C:\Program Files\Example\beta.exe`
	old := "ProxiFyre Service is running.\nSuccessfully associated " + appA + " to 127.0.0.1:7897 TCP proxy with protocols TCP, UDP\n"
	logPath := filepath.Join(logDir, "logfile.txt")
	if err := os.WriteFile(logPath, []byte(old), 0600); err != nil {
		t.Fatal(err)
	}
	h := &helper{engine: filepath.Join(root, engineName), logOffset: int64(len(old))}
	if err := h.verifyInfoLogOnce([]proxyRule{{AppNames: []string{appA}, Socks5ProxyEndpoint: "127.0.0.1:7897"}}); err == nil {
		t.Fatal("accepted an association that existed only before this start")
	}
	fresh := "ProxiFyre Service is running.\nSuccessfully associated " + appA + " to 127.0.0.1:7897 TCP proxy with protocols TCP, UDP\n"
	if err := os.WriteFile(logPath, []byte(old+fresh), 0600); err != nil {
		t.Fatal(err)
	}
	h.logOffset = int64(len(old))
	if err := h.verifyInfoLogOnce([]proxyRule{
		{AppNames: []string{appA}, Socks5ProxyEndpoint: "127.0.0.1:7897"},
		{AppNames: []string{appB}, Socks5ProxyEndpoint: "127.0.0.1:7898"},
	}); err == nil || !strings.Contains(err.Error(), "beta.exe") {
		t.Fatalf("missing second rule was not rejected: %v", err)
	}
}
