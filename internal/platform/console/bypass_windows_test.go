//go:build windows

package console

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"proxy-start/internal/application"
	"proxy-start/internal/domain"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestWindowsConsoleProxyBypassRoutingIntegration exercises the production
// Windows launcher with a bounded PowerShell child. The child invokes
// curl.exe without --proxy/--noproxy; routing is therefore determined only by
// the environment injected by BuildProxyEnvironment.
//
// This is opt-in because it starts short-lived children and local listeners.
// It never touches saved settings, the system proxy, 127.0.0.1:7897, or any
// process outside this test.
func TestWindowsConsoleProxyBypassRoutingIntegration(t *testing.T) {
	if os.Getenv("PROXY_START_CONSOLE_BYPASS_REAL") != "1" {
		t.Skip("set PROXY_START_CONSOLE_BYPASS_REAL=1 to run the Windows bypass routing check")
	}
	if _, err := exec.LookPath("curl.exe"); err != nil {
		t.Skip("curl.exe is required for the Windows bypass routing check")
	}

	direct, directPort := newBypassDirectServer(t)
	proxy, proxyHost, proxyPort := newBypassHTTPProxy(t)
	policy, err := domain.NewBypassPolicy([]string{" ROOT.TEST ", "root.test"})
	if err != nil {
		t.Fatal(err)
	}
	if got := policy.Domains(); len(got) != 1 || got[0] != "root.test" {
		t.Fatalf("policy normalization = %#v, want [root.test]", got)
	}

	endpoint := domain.ProxyEndpoint{Host: proxyHost, Port: proxyPort}
	environment, unset, err := application.BuildProxyEnvironment(endpoint, domain.ProtocolHTTP, policy)
	if err != nil {
		t.Fatal(err)
	}
	if environment["NO_PROXY"] != "root.test" || environment["no_proxy"] != "root.test" {
		t.Fatalf("unexpected HTTP bypass environment: %#v", environment)
	}
	resultPath := filepath.Join(t.TempDir(), "http-route.txt")
	command := bypassRoutingPowerShell(resultPath, directPort, false)
	if strings.Contains(strings.ToLower(command), "--proxy") || strings.Contains(strings.ToLower(command), "--noproxy") {
		t.Fatalf("test command must rely on environment, got %q", command)
	}
	launcher := &Launcher{commandArgs: []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", command}, waitForExit: true}
	if err := launcher.Open(context.Background(), application.ConsoleSpec{Environment: environment, UnsetEnvironment: unset}); err != nil {
		t.Fatal(err)
	}
	result, err := waitForBypassRouteResult(resultPath, 20*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"root", "sub"} {
		if result[name].Exit != 0 || result[name].Code != "200" {
			t.Fatalf("%s did not bypass to direct server: %#v", name, result[name])
		}
	}
	if result["through"].Exit != 0 || result["through"].Code != "200" {
		t.Fatalf("non-bypass request did not use recording proxy: %#v", result["through"])
	}
	if result["env"].Proxy != environment["HTTP_PROXY"] || result["env"].NoProxy != "root.test" {
		t.Fatalf("child environment mismatch: %#v", result["env"])
	}
	if got := direct.Count(); got != 2 {
		t.Fatalf("direct server saw %d requests, want root and subdomain only", got)
	}
	hosts := direct.Hosts()
	hostSet := map[string]bool{}
	for _, host := range hosts {
		hostSet[strings.ToLower(host)] = true
	}
	if !hostSet["root.test:"+strconv.Itoa(directPort)] || !hostSet["sub.root.test:"+strconv.Itoa(directPort)] {
		t.Fatalf("direct server hosts = %#v, want root.test and sub.root.test", hosts)
	}
	if target := proxy.WaitForRequest(5 * time.Second); target != "http://through.test:"+strconv.Itoa(directPort)+"/proxy" {
		t.Fatalf("recording proxy target = %q, want non-bypass request", target)
	}
	if extra := proxy.TryRequest(150 * time.Millisecond); extra != "" {
		t.Fatalf("bypass requests unexpectedly reached proxy: %q", extra)
	}

	// Close the proxy, then prove a matching host still goes direct while a
	// non-matching host fails instead of falling back to a direct connection.
	proxy.Close()
	closedProxy := endpoint
	badEnvironment, badUnset, err := application.BuildProxyEnvironment(closedProxy, domain.ProtocolHTTP, policy)
	if err != nil {
		t.Fatal(err)
	}
	badResultPath := filepath.Join(t.TempDir(), "http-unreachable.txt")
	badCommand := bypassRoutingPowerShell(badResultPath, directPort, true)
	badLauncher := &Launcher{commandArgs: []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", badCommand}, waitForExit: true}
	if err := badLauncher.Open(context.Background(), application.ConsoleSpec{Environment: badEnvironment, UnsetEnvironment: badUnset}); err != nil {
		t.Fatal(err)
	}
	badResult, err := waitForBypassRouteResult(badResultPath, 20*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if badResult["root"].Exit != 0 || badResult["root"].Code != "200" {
		t.Fatalf("matching host did not remain direct with unavailable proxy: %#v", badResult["root"])
	}
	if badResult["through"].Exit == 0 || badResult["through"].Code != "000" {
		t.Fatalf("non-matching host unexpectedly succeeded with unavailable proxy: %#v", badResult["through"])
	}
	if got := direct.Count(); got != 3 {
		t.Fatalf("unavailable-proxy control changed direct count to %d, want one additional root request", got)
	}

	// The same policy is carried into a SOCKS5 console. A tiny local listener
	// records the attempted SOCKS connection; the matching host still bypasses
	// it and succeeds directly.
	socksProxy, socksHost, socksPort := newBypassSOCKS5Proxy(t, directPort)
	socksEnvironment, socksUnset, err := application.BuildProxyEnvironment(domain.ProxyEndpoint{Host: socksHost, Port: socksPort}, domain.ProtocolSOCKS5, policy)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(socksEnvironment["HTTPS_PROXY"], "socks5h://") || socksEnvironment["NO_PROXY"] != "root.test" {
		t.Fatalf("unexpected SOCKS5 bypass environment: %#v", socksEnvironment)
	}
	socksResultPath := filepath.Join(t.TempDir(), "socks-route.txt")
	socksCommand := bypassSocksRoutingPowerShell(socksResultPath, directPort)
	socksLauncher := &Launcher{commandArgs: []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", socksCommand}, waitForExit: true}
	if err := socksLauncher.Open(context.Background(), application.ConsoleSpec{Environment: socksEnvironment, UnsetEnvironment: socksUnset}); err != nil {
		t.Fatal(err)
	}
	socksResult, err := waitForBypassRouteResult(socksResultPath, 20*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if socksResult["root"].Exit != 0 || socksResult["root"].Code != "200" {
		t.Fatalf("SOCKS5 matching host did not bypass: %#v", socksResult["root"])
	}
	if socksResult["through"].Exit != 0 || socksResult["through"].Code != "200" {
		t.Fatalf("SOCKS5 non-matching host did not use the local SOCKS5 proxy: %#v", socksResult["through"])
	}
	if socksResult["env"].Proxy != socksEnvironment["HTTPS_PROXY"] || socksResult["env"].NoProxy != "root.test" {
		t.Fatalf("SOCKS5 child environment mismatch: %#v", socksResult["env"])
	}
	if target := socksProxy.WaitForRequest(5 * time.Second); target != "through.test:"+strconv.Itoa(directPort) {
		t.Fatalf("SOCKS5 CONNECT target = %q, want through.test", target)
	}
	if got := direct.Count(); got != 5 {
		t.Fatalf("SOCKS5 routing changed direct count to %d, want matching root plus proxied through request", got)
	}
	t.Logf("HTTP bypass: direct=%d proxyTarget=%s unavailableProxyThroughExit=%d; SOCKS5 CONNECT=%s non-bypass exit=%d", direct.Count(), "http://through.test:"+strconv.Itoa(directPort)+"/proxy", badResult["through"].Exit, "through.test:"+strconv.Itoa(directPort), socksResult["through"].Exit)
}

type bypassRouteValue struct {
	Exit    int
	Code    string
	Proxy   string
	NoProxy string
}

func waitForBypassRouteResult(path string, timeout time.Duration) (map[string]bypassRouteValue, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf}))), "\n")
			result := make(map[string]bypassRouteValue)
			valid := true
			for _, line := range lines {
				fields := strings.SplitN(strings.TrimSpace(line), "|", 4)
				if len(fields) < 3 {
					valid = false
					break
				}
				if fields[0] == "env" {
					if len(fields) != 4 {
						valid = false
						break
					}
					result["env"] = bypassRouteValue{Proxy: fields[2], NoProxy: fields[3]}
					continue
				}
				exitCode, parseErr := strconv.Atoi(fields[1])
				if parseErr != nil {
					valid = false
					break
				}
				result[fields[0]] = bypassRouteValue{Exit: exitCode, Code: fields[2]}
			}
			if valid && result["root"].Code != "" && result["through"].Code != "" {
				return result, nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, fmt.Errorf("timed out waiting for route result %s", path)
}

func bypassRoutingPowerShell(path string, directPort int, includeSub bool) string {
	rootResolve := fmt.Sprintf("root.test:%d:127.0.0.1", directPort)
	throughResolve := fmt.Sprintf("through.test:%d:127.0.0.1", directPort)
	rootURL := fmt.Sprintf("http://root.test:%d/root", directPort)
	throughURL := fmt.Sprintf("http://through.test:%d/proxy", directPort)
	root := fmt.Sprintf("$root = & curl.exe -q -sS --connect-timeout 3 --max-time 8 --resolve '%s' -o NUL -w '%%{http_code}' '%s' 2>$null; $rootExit = $LASTEXITCODE", rootResolve, rootURL)
	through := fmt.Sprintf("$through = & curl.exe -q -sS --connect-timeout 3 --max-time 8 --resolve '%s' -o NUL -w '%%{http_code}' '%s' 2>$null; $throughExit = $LASTEXITCODE", throughResolve, throughURL)
	sub := ""
	if !includeSub {
		subResolve := fmt.Sprintf("sub.root.test:%d:127.0.0.1", directPort)
		subURL := fmt.Sprintf("http://sub.root.test:%d/sub", directPort)
		sub = fmt.Sprintf("$sub = & curl.exe -q -sS --connect-timeout 3 --max-time 8 --resolve '%s' -o NUL -w '%%{http_code}' '%s' 2>$null; $subExit = $LASTEXITCODE", subResolve, subURL)
	} else {
		sub = "$sub = 'skip'; $subExit = 0"
	}
	output := fmt.Sprintf("%s; %s; %s; [IO.File]::WriteAllText(%s, ('root|' + $rootExit + '|' + [string]$root + \"`n\" + 'through|' + $throughExit + '|' + [string]$through + \"`n\" + 'sub|' + $subExit + '|' + [string]$sub + \"`n\" + 'env|0|' + [string]$env:HTTP_PROXY + '|' + [string]$env:NO_PROXY)); exit 0", root, sub, through, bypassPowerShellLiteral(path))
	return output
}

func bypassSocksRoutingPowerShell(path string, directPort int) string {
	rootResolve := fmt.Sprintf("root.test:%d:127.0.0.1", directPort)
	rootURL := fmt.Sprintf("http://root.test:%d/root", directPort)
	throughURL := fmt.Sprintf("http://through.test:%d/proxy", directPort)
	root := fmt.Sprintf("$root = & curl.exe -q -sS --connect-timeout 3 --max-time 8 --resolve '%s' -o NUL -w '%%{http_code}' '%s' 2>$null; $rootExit = $LASTEXITCODE", rootResolve, rootURL)
	through := fmt.Sprintf("$through = & curl.exe -q -sS --connect-timeout 3 --max-time 8 -o NUL -w '%%{http_code}' '%s' 2>$null; $throughExit = $LASTEXITCODE", throughURL)
	return fmt.Sprintf("%s; %s; [IO.File]::WriteAllText(%s, ('root|' + $rootExit + '|' + [string]$root + \"`n\" + 'through|' + $throughExit + '|' + [string]$through + \"`n\" + 'env|0|' + [string]$env:HTTPS_PROXY + '|' + [string]$env:NO_PROXY)); exit 0", root, through, bypassPowerShellLiteral(path))
}

func bypassPowerShellLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

type bypassDirectServer struct {
	server *httptest.Server
	mu     sync.Mutex
	hosts  []string
}

func newBypassDirectServer(t *testing.T) (*bypassDirectServer, int) {
	t.Helper()
	direct := &bypassDirectServer{}
	direct.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		direct.mu.Lock()
		direct.hosts = append(direct.hosts, r.Host)
		direct.mu.Unlock()
		w.Header().Set("Content-Length", "2")
		_, _ = io.WriteString(w, "ok")
	}))
	_, portText, err := net.SplitHostPort(strings.TrimPrefix(direct.server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(direct.server.Close)
	return direct, port
}

func (s *bypassDirectServer) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.hosts)
}

func (s *bypassDirectServer) Hosts() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.hosts...)
}

type bypassHTTPProxy struct {
	listener  net.Listener
	requests  chan string
	closeOnce sync.Once
	wg        sync.WaitGroup
}

type bypassSOCKS5Proxy struct {
	listener  net.Listener
	direct    string
	requests  chan string
	closeOnce sync.Once
	wg        sync.WaitGroup
}

func newBypassSOCKS5Proxy(t *testing.T, directPort int) (*bypassSOCKS5Proxy, string, int) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	host, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		_ = listener.Close()
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		_ = listener.Close()
		t.Fatal(err)
	}
	proxy := &bypassSOCKS5Proxy{
		listener: listener,
		direct:   net.JoinHostPort("127.0.0.1", strconv.Itoa(directPort)),
		requests: make(chan string, 4),
	}
	proxy.wg.Add(1)
	go proxy.acceptLoop()
	t.Cleanup(proxy.Close)
	return proxy, host, port
}

func (p *bypassSOCKS5Proxy) acceptLoop() {
	defer p.wg.Done()
	for {
		conn, err := p.listener.Accept()
		if err != nil {
			return
		}
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			p.handle(conn)
		}()
	}
}

func (p *bypassSOCKS5Proxy) handle(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	reader := bufio.NewReader(conn)
	var greeting [2]byte
	if _, err := io.ReadFull(reader, greeting[:]); err != nil || greeting[0] != 5 {
		return
	}
	methods := make([]byte, int(greeting[1]))
	if _, err := io.ReadFull(reader, methods); err != nil {
		return
	}
	method := byte(0xff)
	for _, candidate := range methods {
		if candidate == 0 {
			method = 0
			break
		}
	}
	if _, err := conn.Write([]byte{5, method}); err != nil || method == 0xff {
		return
	}
	var requestHeader [4]byte
	if _, err := io.ReadFull(reader, requestHeader[:]); err != nil || requestHeader[0] != 5 || requestHeader[1] != 1 {
		return
	}
	target, err := readBypassSOCKS5Address(reader, requestHeader[3])
	if err != nil {
		return
	}
	var portBytes [2]byte
	if _, err := io.ReadFull(reader, portBytes[:]); err != nil {
		return
	}
	target = net.JoinHostPort(target, strconv.Itoa(int(portBytes[0])<<8|int(portBytes[1])))
	select {
	case p.requests <- target:
	default:
	}
	upstream, err := net.DialTimeout("tcp", p.direct, 5*time.Second)
	if err != nil {
		_, _ = conn.Write([]byte{5, 5, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	defer upstream.Close()
	if _, err := conn.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(upstream, reader)
		closeBypassWrite(upstream)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(conn, upstream)
		closeBypassWrite(conn)
		done <- struct{}{}
	}()
	<-done
}

func readBypassSOCKS5Address(reader *bufio.Reader, addressType byte) (string, error) {
	switch addressType {
	case 1:
		address := make([]byte, 4)
		if _, err := io.ReadFull(reader, address); err != nil {
			return "", err
		}
		return net.IP(address).String(), nil
	case 3:
		length, err := reader.ReadByte()
		if err != nil {
			return "", err
		}
		address := make([]byte, int(length))
		if _, err := io.ReadFull(reader, address); err != nil {
			return "", err
		}
		return string(address), nil
	case 4:
		address := make([]byte, 16)
		if _, err := io.ReadFull(reader, address); err != nil {
			return "", err
		}
		return net.IP(address).String(), nil
	default:
		return "", fmt.Errorf("unsupported SOCKS5 address type %d", addressType)
	}
}

func closeBypassWrite(conn net.Conn) {
	if halfCloser, ok := conn.(interface{ CloseWrite() error }); ok {
		_ = halfCloser.CloseWrite()
	}
}

func (p *bypassSOCKS5Proxy) WaitForRequest(timeout time.Duration) string {
	select {
	case request := <-p.requests:
		return request
	case <-time.After(timeout):
		return ""
	}
}

func (p *bypassSOCKS5Proxy) Close() {
	p.closeOnce.Do(func() { _ = p.listener.Close() })
	p.wg.Wait()
}

func newBypassHTTPProxy(t *testing.T) (*bypassHTTPProxy, string, int) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	host, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		_ = listener.Close()
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		_ = listener.Close()
		t.Fatal(err)
	}
	proxy := &bypassHTTPProxy{listener: listener, requests: make(chan string, 8)}
	proxy.wg.Add(1)
	go proxy.acceptLoop()
	t.Cleanup(proxy.Close)
	return proxy, host, port
}

func (p *bypassHTTPProxy) acceptLoop() {
	defer p.wg.Done()
	for {
		conn, err := p.listener.Accept()
		if err != nil {
			return
		}
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			p.handle(conn)
		}()
	}
}

func (p *bypassHTTPProxy) handle(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	reader := bufio.NewReader(conn)
	header, err := readBypassHTTPHeader(reader)
	if err != nil {
		return
	}
	line := strings.SplitN(string(header), "\r\n", 2)[0]
	fields := strings.Fields(line)
	if len(fields) < 2 || fields[0] != "GET" {
		return
	}
	select {
	case p.requests <- fields[1]:
	default:
	}
	_, _ = io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nok")
}

func readBypassHTTPHeader(reader *bufio.Reader) ([]byte, error) {
	var header []byte
	for {
		line, err := reader.ReadBytes('\n')
		header = append(header, line...)
		if err != nil {
			return nil, err
		}
		if bytes.HasSuffix(header, []byte("\r\n\r\n")) {
			return header, nil
		}
		if len(header) > 64*1024 {
			return nil, fmt.Errorf("HTTP header exceeds 64 KiB")
		}
	}
}

func (p *bypassHTTPProxy) WaitForRequest(timeout time.Duration) string {
	select {
	case request := <-p.requests:
		return request
	case <-time.After(timeout):
		return ""
	}
}

func (p *bypassHTTPProxy) TryRequest(timeout time.Duration) string {
	select {
	case request := <-p.requests:
		return request
	case <-time.After(timeout):
		return ""
	}
}

func (p *bypassHTTPProxy) Close() {
	p.closeOnce.Do(func() { _ = p.listener.Close() })
	p.wg.Wait()
}
