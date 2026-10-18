// Package httpbridge implements a small, local SOCKS5 listener that forwards
// TCP CONNECT requests through an HTTP proxy. It deliberately has no direct
// connection fallback: if the upstream proxy cannot establish CONNECT, the
// SOCKS request fails.
package httpbridge

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"proxy-start/internal/domain"
)

const (
	defaultHandshakeTimeout = 10 * time.Second
	maxHTTPHeaderBytes      = 32 << 10
)

// Bridge is a one-shot bridge. Construct another Bridge to restart a bridge
// after Close. Start is safe to call more than once while it is running and
// returns the same listening address.
type Bridge struct {
	endpoint domain.ProxyEndpoint

	mu       sync.Mutex
	listener net.Listener
	address  string
	closed   bool
	started  bool
	done     chan struct{}
	closeErr error
	ctx      context.Context
	cancel   context.CancelFunc

	conns map[net.Conn]struct{}
	wg    sync.WaitGroup
}

// New creates a bridge for endpoint. The endpoint must be an HTTP (or Auto)
// proxy; SOCKS5 endpoints are rejected by Start because this bridge's purpose
// is specifically SOCKS5-to-HTTP conversion.
func New(endpoint domain.ProxyEndpoint) *Bridge {
	return &Bridge{endpoint: endpoint, conns: make(map[net.Conn]struct{})}
}

// Start binds a loopback TCP listener and returns its address. The address is
// suitable for clients that understand SOCKS5. Cancellation closes the
// listener and all active connections.
func (b *Bridge) Start(ctx context.Context) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := validateEndpoint(b.endpoint); err != nil {
		return "", err
	}
	bridgeContext, cancel := context.WithCancel(ctx)

	// Binding is serialized with Close. Keeping the mutex for the short
	// loopback Listen call means a concurrent Start cannot observe a started
	// bridge before its address is populated, and Close cannot miss the
	// listener between creation and publication.
	b.mu.Lock()
	if b.closed {
		cancel()
		b.mu.Unlock()
		return "", errors.New("http bridge is closed")
	}
	if b.started {
		if b.closed {
			cancel()
			b.mu.Unlock()
			return "", errors.New("http bridge is closed")
		}
		address := b.address
		cancel()
		b.mu.Unlock()
		return address, nil
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		cancel()
		b.mu.Unlock()
		return "", fmt.Errorf("listen for HTTP bridge: %w", err)
	}
	b.started = true
	b.done = make(chan struct{})
	b.ctx = bridgeContext
	b.cancel = cancel
	b.listener = listener
	b.address = listener.Addr().String()
	address := b.address
	// Add before releasing mu. Close waits only after acquiring mu, so no
	// Add can race with its Wait even when it arrives immediately after Start.
	b.wg.Add(1)
	go b.acceptLoop()
	done := b.done
	b.mu.Unlock()

	go func() {
		select {
		case <-ctx.Done():
			_ = b.Close()
		case <-done:
		}
	}()
	return address, nil
}

// Close stops accepting connections and closes active client/upstream
// sockets. It never terminates the target applications using the bridge.
func (b *Bridge) Close() error {
	b.mu.Lock()
	if !b.started {
		// Treat Close-before-Start as a terminal close. This prevents a
		// concurrent Start that has not yet acquired mu from binding a new
		// listener after the caller believes shutdown completed.
		b.closed = true
		b.mu.Unlock()
		return nil
	}
	if b.closed {
		err := b.closeErr
		b.mu.Unlock()
		return err
	}
	b.closed = true
	listener := b.listener
	cancel := b.cancel
	close(b.done)
	connections := make([]net.Conn, 0, len(b.conns))
	for conn := range b.conns {
		connections = append(connections, conn)
	}
	b.mu.Unlock()
	if cancel != nil {
		cancel()
	}

	if listener != nil {
		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			b.mu.Lock()
			b.closeErr = err
			b.mu.Unlock()
		}
	}
	for _, conn := range connections {
		_ = conn.Close()
	}
	b.wg.Wait()
	b.mu.Lock()
	err := b.closeErr
	b.mu.Unlock()
	return err
}

func (b *Bridge) acceptLoop() {
	defer b.wg.Done()
	for {
		conn, err := b.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) || b.isClosed() {
				return
			}
			// A temporary accept failure should not tear down the bridge.
			var temporary net.Error
			if errors.As(err, &temporary) && temporary.Temporary() {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			return
		}
		if !b.registerAccepted(conn) {
			continue
		}
		go func() {
			defer b.wg.Done()
			defer b.untrack(conn)
			_ = b.handle(conn)
		}()
	}
}

// registerAccepted publishes a client connection and its handler's waitgroup
// slot under the same mutex Close uses. This prevents Wait from racing with an
// Add when a connection is accepted at the same time as shutdown.
func (b *Bridge) registerAccepted(conn net.Conn) bool {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		_ = conn.Close()
		return false
	}
	b.conns[conn] = struct{}{}
	b.wg.Add(1)
	b.mu.Unlock()
	return true
}

func (b *Bridge) handle(client net.Conn) error {
	defer client.Close()
	if err := client.SetDeadline(time.Now().Add(defaultHandshakeTimeout)); err != nil {
		return err
	}
	clientReader := bufio.NewReader(client)
	if err := socksGreeting(client, clientReader); err != nil {
		return err
	}
	destination, command, err := readSocksRequest(clientReader)
	if err != nil {
		_ = writeSocksReply(client, 0x01)
		return err
	}
	if command != 0x01 { // BIND and UDP ASSOCIATE are intentionally unsupported.
		_ = writeSocksReply(client, 0x07)
		return fmt.Errorf("SOCKS5 command 0x%02x is unsupported", command)
	}

	b.mu.Lock()
	bridgeContext := b.ctx
	b.mu.Unlock()
	upstream, err := dialHTTPProxy(bridgeContext, b.endpoint)
	if err != nil {
		_ = writeSocksReply(client, 0x01)
		return err
	}
	b.track(upstream)
	defer b.untrack(upstream)
	defer upstream.Close()
	if err := upstream.SetDeadline(time.Now().Add(defaultHandshakeTimeout)); err != nil {
		_ = writeSocksReply(client, 0x01)
		return err
	}

	responseReader, err := connectHTTPProxy(upstream, destination, b.endpoint)
	if err != nil {
		_ = writeSocksReply(client, 0x01)
		return err
	}
	if err := writeSocksReply(client, 0x00); err != nil {
		return err
	}
	_ = client.SetDeadline(time.Time{})
	_ = upstream.SetDeadline(time.Time{})

	// responseReader retains bytes buffered past the HTTP headers. Relaying
	// from it (instead of the raw upstream connection) preserves those bytes.
	return relay(client, clientReader, upstream, responseReader)
}

func socksGreeting(conn net.Conn, reader *bufio.Reader) error {
	header := make([]byte, 2)
	if _, err := io.ReadFull(reader, header); err != nil {
		return err
	}
	if header[0] != 0x05 {
		return fmt.Errorf("unsupported SOCKS version %d", header[0])
	}
	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(reader, methods); err != nil {
		return err
	}
	for _, method := range methods {
		if method == 0x00 {
			_, err := conn.Write([]byte{0x05, 0x00})
			return err
		}
	}
	_, _ = conn.Write([]byte{0x05, 0xff})
	return errors.New("SOCKS5 client does not offer no-authentication")
}

func readSocksRequest(reader *bufio.Reader) (destination string, command byte, err error) {
	header := make([]byte, 4)
	if _, err = io.ReadFull(reader, header); err != nil {
		return "", 0, err
	}
	if header[0] != 0x05 {
		return "", 0, fmt.Errorf("unsupported SOCKS request version %d", header[0])
	}
	command = header[1]
	var host string
	switch header[3] {
	case 0x01:
		buf := make([]byte, net.IPv4len)
		if _, err = io.ReadFull(reader, buf); err != nil {
			return "", command, err
		}
		host = net.IP(buf).String()
	case 0x03:
		length, readErr := reader.ReadByte()
		if readErr != nil {
			return "", command, readErr
		}
		if length == 0 {
			return "", command, errors.New("empty SOCKS domain")
		}
		buf := make([]byte, int(length))
		if _, err = io.ReadFull(reader, buf); err != nil {
			return "", command, err
		}
		host = string(buf)
	case 0x04:
		buf := make([]byte, net.IPv6len)
		if _, err = io.ReadFull(reader, buf); err != nil {
			return "", command, err
		}
		host = net.IP(buf).String()
	default:
		return "", command, fmt.Errorf("unsupported SOCKS address type 0x%02x", header[3])
	}
	portBytes := make([]byte, 2)
	if _, err = io.ReadFull(reader, portBytes); err != nil {
		return "", command, err
	}
	port := int(portBytes[0])<<8 | int(portBytes[1])
	if strings.TrimSpace(host) == "" || strings.ContainsAny(host, "\r\n\t ") {
		return "", command, errors.New("invalid SOCKS destination host")
	}
	return net.JoinHostPort(host, fmt.Sprintf("%d", port)), command, nil
}

func writeSocksReply(conn net.Conn, code byte) error {
	// 0.0.0.0:0 is a valid unspecified bound address for a CONNECT reply.
	_, err := conn.Write([]byte{0x05, code, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
	return err
}

func dialHTTPProxy(ctx context.Context, endpoint domain.ProxyEndpoint) (net.Conn, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	dialer := net.Dialer{Timeout: defaultHandshakeTimeout}
	return dialer.DialContext(ctx, "tcp", endpoint.Address())
}

func connectHTTPProxy(conn net.Conn, destination string, endpoint domain.ProxyEndpoint) (*bufio.Reader, error) {
	var request strings.Builder
	request.WriteString("CONNECT ")
	request.WriteString(destination)
	request.WriteString(" HTTP/1.1\r\nHost: ")
	request.WriteString(destination)
	request.WriteString("\r\nConnection: Keep-Alive\r\nProxy-Connection: Keep-Alive\r\n")
	if endpoint.Username != "" || endpoint.Password != "" {
		encoded := base64.StdEncoding.EncodeToString([]byte(endpoint.Username + ":" + endpoint.Password))
		request.WriteString("Proxy-Authorization: Basic ")
		request.WriteString(encoded)
		request.WriteString("\r\n")
	}
	request.WriteString("\r\n")
	if _, err := io.WriteString(conn, request.String()); err != nil {
		return nil, err
	}
	reader := bufio.NewReaderSize(conn, maxHTTPHeaderBytes)
	response, err := http.ReadResponse(reader, &http.Request{Method: http.MethodConnect})
	if err != nil {
		return nil, fmt.Errorf("read HTTP proxy CONNECT response: %w", err)
	}
	if response.Body != nil {
		_ = response.Body.Close()
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP proxy CONNECT failed with status %s", response.Status)
	}
	return reader, nil
}

func relay(client net.Conn, clientReader *bufio.Reader, upstream net.Conn, upstreamReader *bufio.Reader) error {
	result := make(chan error, 2)
	go func() {
		_, err := io.Copy(upstream, clientReader)
		if tcp, ok := upstream.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
		result <- err
	}()
	go func() {
		_, err := io.Copy(client, upstreamReader)
		if tcp, ok := client.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
		result <- err
	}()
	first := <-result
	_ = client.SetDeadline(time.Now().Add(2 * time.Second))
	_ = upstream.SetDeadline(time.Now().Add(2 * time.Second))
	second := <-result
	if first != nil && !errors.Is(first, net.ErrClosed) {
		return first
	}
	if second != nil && !errors.Is(second, net.ErrClosed) {
		return second
	}
	return nil
}

func (b *Bridge) track(conn net.Conn) {
	b.mu.Lock()
	closed := b.closed
	if !closed {
		b.conns[conn] = struct{}{}
	}
	b.mu.Unlock()
	if closed {
		_ = conn.Close()
	}
}

func (b *Bridge) untrack(conn net.Conn) {
	b.mu.Lock()
	delete(b.conns, conn)
	b.mu.Unlock()
}

func (b *Bridge) isClosed() bool {
	b.mu.Lock()
	closed := b.closed
	b.mu.Unlock()
	return closed
}

func validateEndpoint(endpoint domain.ProxyEndpoint) error {
	if endpoint.Protocol != domain.ProtocolHTTP && endpoint.Protocol != domain.ProtocolAuto {
		return fmt.Errorf("HTTP bridge requires an HTTP proxy endpoint, got %q", endpoint.Protocol)
	}
	if strings.TrimSpace(endpoint.Host) == "" {
		return errors.New("HTTP proxy host is empty")
	}
	if endpoint.Port < 1 || endpoint.Port > 65535 {
		return fmt.Errorf("HTTP proxy port %d is out of range", endpoint.Port)
	}
	return nil
}
