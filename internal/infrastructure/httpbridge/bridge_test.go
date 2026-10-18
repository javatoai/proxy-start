package httpbridge

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"proxy-start/internal/domain"
)

func TestBridgeConnectsThroughHTTPProxyAndPreservesBufferedBytes(t *testing.T) {
	proxyAddress, stopProxy := startFakeHTTPProxy(t, true, true)
	defer stopProxy()
	host, port, err := net.SplitHostPort(proxyAddress)
	if err != nil {
		t.Fatal(err)
	}
	var proxyPort int
	if _, err := fmt.Sscanf(port, "%d", &proxyPort); err != nil {
		t.Fatal(err)
	}
	bridge := New(domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP, Host: host, Port: proxyPort, Username: "alice", Password: "secret"})
	address, err := bridge.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.Close()

	client, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := client.Write([]byte{5, 1, 0}); err != nil {
		t.Fatal(err)
	}
	response := make([]byte, 2)
	if _, err := io.ReadFull(client, response); err != nil {
		t.Fatal(err)
	}
	if string(response) != string([]byte{5, 0}) {
		t.Fatalf("greeting response = %v", response)
	}
	request := []byte{5, 1, 0, 3, 11}
	request = append(request, []byte("example.com")...)
	request = append(request, 1, 187) // 443, big endian
	request = append(request, []byte("hello")...)
	if _, err := client.Write(request); err != nil {
		t.Fatal(err)
	}
	connectResponse := make([]byte, 10)
	if _, err := io.ReadFull(client, connectResponse); err != nil {
		t.Fatal(err)
	}
	if connectResponse[1] != 0 {
		t.Fatalf("CONNECT failed with SOCKS reply %v", connectResponse)
	}
	buffer := make([]byte, len("upstream-buffer"))
	if _, err := io.ReadFull(client, buffer); err != nil {
		t.Fatal(err)
	}
	if string(buffer) != "upstream-buffer" {
		t.Fatalf("buffered upstream bytes = %q", buffer)
	}
	initial := make([]byte, 5)
	if _, err := io.ReadFull(client, initial); err != nil {
		t.Fatal(err)
	}
	if string(initial) != "hello" {
		t.Fatalf("buffered client bytes = %q", initial)
	}
	if _, err := client.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	echo := make([]byte, 4)
	if _, err := io.ReadFull(client, echo); err != nil {
		t.Fatal(err)
	}
	if string(echo) != "ping" {
		t.Fatalf("echo = %q", echo)
	}
}

func TestBridgeRejectsUDPAssociate(t *testing.T) {
	proxyAddress, stopProxy := startFakeHTTPProxy(t, false, false)
	defer stopProxy()
	host, portText, _ := net.SplitHostPort(proxyAddress)
	var port int
	_, _ = fmt.Sscanf(portText, "%d", &port)
	bridge := New(domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP, Host: host, Port: port})
	address, err := bridge.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.Close()
	client, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(time.Second))
	_, _ = client.Write([]byte{5, 1, 0})
	if _, err := io.ReadFull(client, make([]byte, 2)); err != nil {
		t.Fatal(err)
	}
	// UDP ASSOCIATE with an IPv4 address and port zero.
	_, _ = client.Write([]byte{5, 3, 0, 1, 0, 0, 0, 0, 0, 0})
	reply := make([]byte, 10)
	if _, err := io.ReadFull(client, reply); err != nil {
		t.Fatal(err)
	}
	if reply[1] != 7 {
		t.Fatalf("UDP reply code = %d, want 7", reply[1])
	}
}

func TestBridgeDoesNotFallbackOnHTTPFailure(t *testing.T) {
	proxyAddress, stopProxy := startFakeHTTPProxy(t, false, false)
	defer stopProxy()
	host, portText, _ := net.SplitHostPort(proxyAddress)
	var port int
	_, _ = fmt.Sscanf(portText, "%d", &port)
	bridge := New(domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP, Host: host, Port: port})
	address, err := bridge.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.Close()
	client, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(time.Second))
	_, _ = client.Write([]byte{5, 1, 0})
	if _, err := io.ReadFull(client, make([]byte, 2)); err != nil {
		t.Fatal(err)
	}
	request := []byte{5, 1, 0, 1, 127, 0, 0, 1, 0, 80}
	_, _ = client.Write(request)
	reply := make([]byte, 10)
	if _, err := io.ReadFull(client, reply); err != nil {
		t.Fatal(err)
	}
	if reply[1] == 0 {
		t.Fatal("bridge reported success after HTTP proxy rejection")
	}
}

func TestBridgeConcurrentStartPublishesOneAddress(t *testing.T) {
	proxyAddress, stopProxy := startFakeHTTPProxy(t, false, false)
	defer stopProxy()
	host, portText, _ := net.SplitHostPort(proxyAddress)
	var port int
	_, _ = fmt.Sscanf(portText, "%d", &port)
	bridge := New(domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP, Host: host, Port: port})
	const callers = 32
	addresses := make(chan string, callers)
	errorsCh := make(chan error, callers)
	var group sync.WaitGroup
	for i := 0; i < callers; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			address, err := bridge.Start(context.Background())
			if err != nil {
				errorsCh <- err
				return
			}
			addresses <- address
		}()
	}
	group.Wait()
	close(addresses)
	close(errorsCh)
	var first string
	for address := range addresses {
		if address == "" {
			t.Fatal("concurrent Start returned an empty address")
		}
		if first == "" {
			first = address
		} else if address != first {
			t.Fatalf("concurrent Start returned different addresses %q and %q", first, address)
		}
	}
	for err := range errorsCh {
		t.Fatalf("concurrent Start failed: %v", err)
	}
	if first == "" {
		t.Fatal("no successful Start call")
	}
	if err := bridge.Close(); err != nil {
		t.Fatal(err)
	}
	conn, err := net.DialTimeout("tcp", first, 200*time.Millisecond)
	if err == nil {
		_ = conn.Close()
		t.Fatal("bridge listener remained open after Close")
	}
}

func TestBridgeCloseRacingStartCannotLeaveListener(t *testing.T) {
	proxyAddress, stopProxy := startFakeHTTPProxy(t, false, false)
	defer stopProxy()
	host, portText, _ := net.SplitHostPort(proxyAddress)
	var port int
	_, _ = fmt.Sscanf(portText, "%d", &port)
	for attempt := 0; attempt < 100; attempt++ {
		bridge := New(domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP, Host: host, Port: port})
		startResult := make(chan struct {
			address string
			err     error
		}, 1)
		go func() {
			address, err := bridge.Start(context.Background())
			startResult <- struct {
				address string
				err     error
			}{address: address, err: err}
		}()
		go func() { _ = bridge.Close() }()
		result := <-startResult
		if result.err == nil && result.address != "" {
			// A Close that overlapped binding must close the published listener
			// before returning. If Start won the race, this explicit Close makes
			// the same invariant observable for the next iteration.
			_ = bridge.Close()
			conn, err := net.DialTimeout("tcp", result.address, 50*time.Millisecond)
			if err == nil {
				_ = conn.Close()
				t.Fatalf("listener leaked after racing Close on attempt %d", attempt)
			}
		}
	}
}

func TestBridgeClearsHandshakeDeadlinesForLongLivedStreams(t *testing.T) {
	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer proxyListener.Close()
	proxyDone := make(chan struct{})
	go func() {
		defer close(proxyDone)
		conn, err := proxyListener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		if _, err := httpReadRequest(reader); err != nil {
			return
		}
		_, _ = io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
		time.Sleep(defaultHandshakeTimeout + 250*time.Millisecond)
		_, _ = io.WriteString(conn, "late")
	}()
	host, portText, _ := net.SplitHostPort(proxyListener.Addr().String())
	var port int
	_, _ = fmt.Sscanf(portText, "%d", &port)
	bridge := New(domain.ProxyEndpoint{Protocol: domain.ProtocolHTTP, Host: host, Port: port})
	address, err := bridge.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.Close()
	client, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(defaultHandshakeTimeout + 3*time.Second))
	_, _ = client.Write([]byte{5, 1, 0})
	if _, err := io.ReadFull(client, make([]byte, 2)); err != nil {
		t.Fatal(err)
	}
	request := []byte{5, 1, 0, 1, 127, 0, 0, 1, 0, 80}
	_, _ = client.Write(request)
	reply := make([]byte, 10)
	if _, err := io.ReadFull(client, reply); err != nil {
		t.Fatal(err)
	}
	if reply[1] != 0 {
		t.Fatalf("CONNECT failed with SOCKS reply %v", reply)
	}
	late := make([]byte, 4)
	if _, err := io.ReadFull(client, late); err != nil {
		t.Fatalf("long-lived stream was interrupted by handshake deadline: %v", err)
	}
	if string(late) != "late" {
		t.Fatalf("delayed stream bytes = %q", late)
	}
	<-proxyDone
}

func startFakeHTTPProxy(t *testing.T, echo bool, accept bool) (string, func()) {
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
			go func() {
				defer conn.Close()
				reader := bufio.NewReader(conn)
				request, err := httpReadRequest(reader)
				if err != nil {
					return
				}
				if !strings.Contains(request, "Proxy-Authorization: Basic "+base64.StdEncoding.EncodeToString([]byte("alice:secret"))) && echo {
					_, _ = io.WriteString(conn, "HTTP/1.1 407 Proxy Authentication Required\r\n\r\n")
					return
				}
				if !accept {
					_, _ = io.WriteString(conn, "HTTP/1.1 403 Forbidden\r\nContent-Length: 0\r\n\r\n")
					return
				}
				_, _ = io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\nupstream-buffer")
				if echo {
					_, _ = io.Copy(conn, reader)
				}
			}()
		}
	}()
	return listener.Addr().String(), func() {
		close(stop)
		_ = listener.Close()
	}
}

func httpReadRequest(reader *bufio.Reader) (string, error) {
	var builder strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		builder.WriteString(line)
		if line == "\r\n" {
			return builder.String(), nil
		}
	}
}
