package netprobe

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"proxy-start/internal/domain"
)

func TestProbeHTTPReportsVerifiedCONNECT(t *testing.T) {
	address, stop := startProbeServer(t, func(conn net.Conn) {
		defer conn.Close()
		reader := bufio.NewReader(conn)
		if _, err := readHeaders(reader); err != nil {
			return
		}
		_, _ = io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
	})
	defer stop()
	host, portText, _ := net.SplitHostPort(address)
	var port int
	_, _ = fmt.Sscanf(portText, "%d", &port)
	probe := New()
	result, err := probe.Test(context.Background(), domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP, Host: host, Port: port})
	if err != nil {
		t.Fatal(err)
	}
	if !result.HTTP || !result.TCPVerified || result.UDPVerified {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestProbeHTTPAuthFailureIsReturned(t *testing.T) {
	address, stop := startProbeServer(t, func(conn net.Conn) {
		defer conn.Close()
		reader := bufio.NewReader(conn)
		if _, err := readHeaders(reader); err != nil {
			return
		}
		_, _ = io.WriteString(conn, "HTTP/1.1 407 Proxy Authentication Required\r\nContent-Length: 0\r\n\r\n")
	})
	defer stop()
	host, portText, _ := net.SplitHostPort(address)
	var port int
	_, _ = fmt.Sscanf(portText, "%d", &port)
	result, err := New().Test(context.Background(), domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP, Host: host, Port: port, Username: "alice", Password: "wrong"})
	if err == nil {
		t.Fatal("expected authentication failure")
	}
	if !result.HTTP || result.HTTPVerified || result.TCPVerified {
		t.Fatalf("unexpected auth result: %+v", result)
	}
}

func TestProbeSOCKS5DistinguishesGreetingFromCONNECT(t *testing.T) {
	address, stop := startProbeServer(t, func(conn net.Conn) {
		defer conn.Close()
		reader := bufio.NewReader(conn)
		greeting := make([]byte, 3)
		if _, err := io.ReadFull(reader, greeting); err != nil {
			return
		}
		_, _ = conn.Write([]byte{5, 0})
		request := make([]byte, 4)
		if _, err := io.ReadFull(reader, request); err != nil {
			return
		}
		if request[1] != 1 {
			return
		}
		length := make([]byte, 1)
		if _, err := io.ReadFull(reader, length); err != nil {
			return
		}
		rest := make([]byte, int(length[0])+2)
		_, _ = io.ReadFull(reader, rest)
		// A valid SOCKS5 reply with a nonzero REP means the protocol was
		// recognized but the requested TCP connection was not verified.
		_, _ = conn.Write([]byte{5, 5, 0, 1, 0, 0, 0, 0, 0, 0})
	})
	defer stop()
	host, portText, _ := net.SplitHostPort(address)
	var port int
	_, _ = fmt.Sscanf(portText, "%d", &port)
	result, err := New().Test(context.Background(), domain.ProxyEndpoint{Protocol: domain.ProtocolSOCKS5, Host: host, Port: port})
	if err == nil {
		t.Fatal("expected CONNECT verification error")
	}
	if !result.SOCKS5 || result.TCPVerified || result.UDPVerified {
		t.Fatalf("unexpected result: %+v", result)
	}
	if !strings.Contains(result.Detail, "CONNECT") {
		t.Fatalf("detail does not mention CONNECT: %q", result.Detail)
	}
}

func TestProbeCancellation(t *testing.T) {
	address, stop := startProbeServer(t, func(conn net.Conn) {
		defer conn.Close()
		time.Sleep(time.Second)
	})
	defer stop()
	host, portText, _ := net.SplitHostPort(address)
	var port int
	_, _ = fmt.Sscanf(portText, "%d", &port)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := New().Test(ctx, domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP, Host: host, Port: port})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
}

func startProbeServer(t *testing.T, handler func(net.Conn)) (string, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-stop:
					return
				default:
				}
				continue
			}
			go handler(conn)
		}
	}()
	return listener.Addr().String(), func() {
		close(stop)
		_ = listener.Close()
	}
}

func readHeaders(reader *bufio.Reader) (string, error) {
	var text strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		text.WriteString(line)
		if line == "\r\n" {
			return text.String(), nil
		}
	}
}
