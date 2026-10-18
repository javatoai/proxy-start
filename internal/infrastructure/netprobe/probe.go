// Package netprobe probes proxy endpoints without changing the user's global
// proxy settings. A probe reports protocol recognition separately from a
// verified TCP CONNECT, since a server can complete a greeting but reject the
// requested destination.
package netprobe

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"proxy-start/internal/application"
	"proxy-start/internal/domain"
)

const (
	probeTarget         = "example.com:443"
	defaultProbeTimeout = 5 * time.Second
)

// Probe is safe for concurrent use. A separate TCP connection is made for
// each protocol attempt, so Auto can try HTTP and SOCKS5 independently.
type Probe struct {
	Timeout time.Duration
}

type attempt struct {
	protocol   domain.Protocol
	recognized bool
	verified   bool
	detail     string
	err        error
}

// New returns the default proxy probe. It returns the application interface so
// callers only depend on the capability they consume.
func New() application.ProxyProbe {
	return &Probe{Timeout: defaultProbeTimeout}
}

// Test probes endpoint and reports both protocol recognition and actual TCP
// CONNECT verification. An error is returned for malformed endpoints,
// cancellation, or when no protocol could be recognized. If a proxy speaks a
// recognized protocol but rejects CONNECT, the result is still useful and the
// error explains why TCP verification failed.
func (p *Probe) Test(ctx context.Context, endpoint domain.ProxyEndpoint) (domain.ProbeResult, error) {
	result := domain.ProbeResult{CheckedAt: time.Now()}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if endpoint.Host == "" || endpoint.Port < 1 || endpoint.Port > 65535 {
		return result, errors.New("proxy host or port is invalid")
	}
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = defaultProbeTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var details []string
	var recognized bool
	var failures []error
	tryHTTP := endpoint.Protocol == domain.ProtocolHTTP || endpoint.Protocol == domain.ProtocolAuto
	trySOCKS := endpoint.Protocol == domain.ProtocolSOCKS5 || endpoint.Protocol == domain.ProtocolAuto
	attempts := make(chan attempt, 2)
	count := 0
	if tryHTTP {
		count++
		go func() {
			attemptCtx, cancelAttempt := context.WithTimeout(ctx, timeout)
			defer cancelAttempt()
			recognized, verified, detail, err := probeHTTP(attemptCtx, endpoint)
			attempts <- attempt{protocol: domain.ProtocolHTTP, recognized: recognized, verified: verified, detail: detail, err: err}
		}()
	}
	if trySOCKS {
		count++
		go func() {
			attemptCtx, cancelAttempt := context.WithTimeout(ctx, timeout)
			defer cancelAttempt()
			recognized, verified, detail, err := probeSOCKS5(attemptCtx, endpoint)
			attempts <- attempt{protocol: domain.ProtocolSOCKS5, recognized: recognized, verified: verified, detail: detail, err: err}
		}()
	}
	var httpAttempt, socksAttempt *attempt
	for index := 0; index < count; index++ {
		current := <-attempts
		copy := current
		if current.protocol == domain.ProtocolHTTP {
			httpAttempt = &copy
		} else {
			socksAttempt = &copy
		}
	}
	for _, current := range []*attempt{httpAttempt, socksAttempt} {
		if current == nil {
			continue
		}
		if current.recognized {
			recognized = true
		}
		if current.protocol == domain.ProtocolHTTP {
			result.HTTP = current.recognized
			result.HTTPVerified = current.verified
		} else {
			result.SOCKS5 = current.recognized
			result.SOCKS5Verified = current.verified
		}
		if current.verified {
			result.TCPVerified = true
		}
		if current.detail != "" {
			label := "HTTP"
			if current.protocol == domain.ProtocolSOCKS5 {
				label = "SOCKS5"
			}
			details = append(details, label+": "+current.detail)
		}
		if current.err != nil {
			failures = append(failures, current.err)
		}
	}
	// UDP ASSOCIATE is intentionally not treated as verified. A successful
	// association only proves that the control channel accepted the command;
	// it does not prove that a datagram reached its destination and returned.
	result.UDPVerified = false
	result.Detail = strings.Join(details, "; ")
	if !recognized {
		if result.Detail == "" {
			result.Detail = "代理未响应可识别的 HTTP 或 SOCKS5 握手"
		}
		if len(failures) > 0 {
			return result, errors.Join(failures...)
		}
		return result, errors.New(result.Detail)
	}
	if !result.TCPVerified && len(failures) > 0 {
		return result, errors.Join(failures...)
	}
	return result, nil
}

func probeHTTP(ctx context.Context, endpoint domain.ProxyEndpoint) (recognized, verified bool, detail string, err error) {
	conn, err := dial(ctx, endpoint)
	if err != nil {
		return false, false, "连接失败: " + err.Error(), err
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	request := "CONNECT " + probeTarget + " HTTP/1.1\r\nHost: " + probeTarget + "\r\nConnection: close\r\n"
	if endpoint.Username != "" || endpoint.Password != "" {
		encoded := base64.StdEncoding.EncodeToString([]byte(endpoint.Username + ":" + endpoint.Password))
		request += "Proxy-Authorization: Basic " + encoded + "\r\n"
	}
	request += "\r\n"
	if _, err := io.WriteString(conn, request); err != nil {
		return false, false, "发送 CONNECT 失败: " + err.Error(), err
	}
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, &http.Request{Method: http.MethodConnect})
	if err != nil {
		return false, false, "HTTP 握手失败: " + err.Error(), err
	}
	if response.Body != nil {
		_ = response.Body.Close()
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return true, true, "CONNECT 已验证 (" + response.Status + ")", nil
	}
	return true, false, "已识别 HTTP，CONNECT 被拒绝 (" + response.Status + ")", fmt.Errorf("HTTP proxy CONNECT failed with status %s", response.Status)
}

func probeSOCKS5(ctx context.Context, endpoint domain.ProxyEndpoint) (recognized, verified bool, detail string, err error) {
	conn, err := dial(ctx, endpoint)
	if err != nil {
		return false, false, "连接失败: " + err.Error(), err
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return false, false, "发送 SOCKS5 握手失败: " + err.Error(), err
	}
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return false, false, "SOCKS5 握手失败: " + err.Error(), err
	}
	if header[0] != 0x05 {
		return false, false, fmt.Sprintf("服务端返回了 SOCKS 版本 %d", header[0]), nil
	}
	if header[1] == 0xff {
		return true, false, "已识别 SOCKS5，但服务端拒绝认证方式", errors.New("SOCKS5 server rejected authentication methods")
	}
	if header[1] == 0x02 {
		if endpoint.Username == "" && endpoint.Password == "" {
			return true, false, "已识别 SOCKS5，但需要用户名密码", errors.New("SOCKS5 server requires username and password")
		}
		if err := socksUserPassAuth(conn, endpoint.Username, endpoint.Password); err != nil {
			return true, false, "SOCKS5 用户认证失败: " + err.Error(), err
		}
	} else if header[1] != 0x00 {
		return true, false, fmt.Sprintf("已识别 SOCKS5，但认证方法 0x%02x 不支持", header[1]), fmt.Errorf("SOCKS5 authentication method 0x%02x is unsupported", header[1])
	}
	host, port, err := net.SplitHostPort(probeTarget)
	if err != nil {
		return true, false, err.Error(), nil
	}
	hostBytes := []byte(host)
	request := []byte{0x05, 0x01, 0x00, 0x03, byte(len(hostBytes))}
	request = append(request, hostBytes...)
	request = append(request, byte(portNumber(port)>>8), byte(portNumber(port)))
	if _, err := conn.Write(request); err != nil {
		return true, false, "发送 CONNECT 失败: " + err.Error(), err
	}
	reply, err := readSocksReply(conn)
	if err != nil {
		return true, false, "读取 CONNECT 响应失败: " + err.Error(), err
	}
	if reply == 0x00 {
		return true, true, "CONNECT 已验证", nil
	}
	return true, false, fmt.Sprintf("已识别 SOCKS5，CONNECT 被拒绝 (代码 0x%02x)", reply), fmt.Errorf("SOCKS5 CONNECT failed with reply code 0x%02x", reply)
}

func dial(ctx context.Context, endpoint domain.ProxyEndpoint) (net.Conn, error) {
	dialer := net.Dialer{}
	return dialer.DialContext(ctx, "tcp", endpoint.Address())
}

func socksUserPassAuth(conn net.Conn, username, password string) error {
	if len(username) > 255 || len(password) > 255 {
		return errors.New("用户名或密码过长")
	}
	packet := []byte{0x01, byte(len(username))}
	packet = append(packet, username...)
	packet = append(packet, byte(len(password)))
	packet = append(packet, password...)
	if _, err := conn.Write(packet); err != nil {
		return err
	}
	response := make([]byte, 2)
	if _, err := io.ReadFull(conn, response); err != nil {
		return err
	}
	if response[0] != 0x01 || response[1] != 0x00 {
		return errors.New("服务端拒绝用户名密码")
	}
	return nil
}

func readSocksReply(conn net.Conn) (byte, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return 0, err
	}
	if header[0] != 0x05 {
		return 0, errors.New("invalid SOCKS5 reply version")
	}
	var addressLength int
	switch header[3] {
	case 0x01:
		addressLength = net.IPv4len
	case 0x03:
		length := make([]byte, 1)
		if _, err := io.ReadFull(conn, length); err != nil {
			return 0, err
		}
		addressLength = int(length[0])
	case 0x04:
		addressLength = net.IPv6len
	default:
		return 0, fmt.Errorf("invalid SOCKS5 address type 0x%02x", header[3])
	}
	address := make([]byte, addressLength+2)
	if _, err := io.ReadFull(conn, address); err != nil {
		return 0, err
	}
	return header[1], nil
}

func portNumber(port string) int {
	n, _ := strconv.Atoi(port)
	return n
}
