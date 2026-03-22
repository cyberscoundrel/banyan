package tunnel

import (
	"context"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/peer"
)

func TestIsAllowedTarget(t *testing.T) {
	ctx := context.Background()
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("Failed to create host: %v", err)
	}
	defer h.Close()

	handler := NewHandler(h, ctx, nil)

	tests := []struct {
		name           string
		target         string
		allowedTargets []string
		expected       bool
	}{
		{"localhost allowed by default", "localhost", nil, true},
		{"127.0.0.1 allowed by default", "127.0.0.1", nil, true},
		{"::1 allowed by default", "::1", nil, true},
		{"external denied by default", "google.com", nil, false},
		{"external denied by default 2", "192.168.1.1", nil, false},
		{"wildcard allows all", "anything.com", []string{"*"}, true},
		{"explicit target allowed", "example.com", []string{"example.com"}, true},
		{"non-matching denied", "other.com", []string{"example.com"}, false},
		{"multiple targets", "second.com", []string{"first.com", "second.com"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler.SetAllowedTargets(tt.allowedTargets)
			result := handler.isAllowedTarget(tt.target)
			if result != tt.expected {
				t.Errorf("isAllowedTarget(%q) = %v, want %v", tt.target, result, tt.expected)
			}
		})
	}
}

func TestTunnelProtocol(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create two libp2p hosts
	h1, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("Failed to create host1: %v", err)
	}
	defer h1.Close()

	h2, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("Failed to create host2: %v", err)
	}
	defer h2.Close()

	// Connect hosts
	h1.Peerstore().AddAddrs(h2.ID(), h2.Addrs(), time.Hour)
	if err := h1.Connect(ctx, peer.AddrInfo{ID: h2.ID(), Addrs: h2.Addrs()}); err != nil {
		t.Fatalf("Failed to connect hosts: %v", err)
	}

	// Create tunnel handlers
	logs := make([]string, 0)
	logf := func(format string, v ...interface{}) {
		logs = append(logs, fmt.Sprintf(format, v...))
	}

	handler1 := NewHandler(h1, ctx, logf)
	handler2 := NewHandler(h2, ctx, logf)
	_ = handler1 // handler1 is for outbound, handler2 accepts inbound

	// Start a simple TCP echo server on localhost
	echoListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start echo server: %v", err)
	}
	defer echoListener.Close()
	echoPort := echoListener.Addr().(*net.TCPAddr).Port

	go func() {
		for {
			conn, err := echoListener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c) // Echo back
			}(conn)
		}
	}()

	// Test: Open tunnel from h1 to h2, requesting connection to echo server
	handler2.SetAllowedTargets([]string{"localhost", "127.0.0.1"})

	stream, err := handler1.OpenTunnel(h2.ID(), "127.0.0.1", uint16(echoPort))
	if err != nil {
		t.Fatalf("Failed to open tunnel: %v", err)
	}
	defer stream.Close()

	// Send data through tunnel
	testData := []byte("Hello through tunnel!")
	_, err = stream.Write(testData)
	if err != nil {
		t.Fatalf("Failed to write to tunnel: %v", err)
	}

	// Read echo response
	response := make([]byte, len(testData))
	_, err = io.ReadFull(stream, response)
	if err != nil {
		t.Fatalf("Failed to read from tunnel: %v", err)
	}

	if string(response) != string(testData) {
		t.Errorf("Expected %q, got %q", testData, response)
	}
}

func TestTunnelTargetDenied(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	h1, _ := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	defer h1.Close()
	h2, _ := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	defer h2.Close()

	h1.Peerstore().AddAddrs(h2.ID(), h2.Addrs(), time.Hour)
	h1.Connect(ctx, peer.AddrInfo{ID: h2.ID(), Addrs: h2.Addrs()})

	handler1 := NewHandler(h1, ctx, nil)
	NewHandler(h2, ctx, nil) // handler2 with default (localhost only)

	// Try to tunnel to an external address (should be denied)
	_, err := handler1.OpenTunnel(h2.ID(), "google.com", 80)
	if err == nil {
		t.Error("Expected error when tunneling to denied target")
	}
}

func TestTunnelConcurrentStreams(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	h1, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("Failed to create host1: %v", err)
	}
	defer h1.Close()

	h2, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("Failed to create host2: %v", err)
	}
	defer h2.Close()

	h1.Peerstore().AddAddrs(h2.ID(), h2.Addrs(), time.Hour)
	if err := h1.Connect(ctx, peer.AddrInfo{ID: h2.ID(), Addrs: h2.Addrs()}); err != nil {
		t.Fatalf("Failed to connect hosts: %v", err)
	}

	handler1 := NewHandler(h1, ctx, nil)
	NewHandler(h2, ctx, nil)

	echoListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start echo server: %v", err)
	}
	defer echoListener.Close()
	echoPort := echoListener.Addr().(*net.TCPAddr).Port

	go func() {
		for {
			conn, err := echoListener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c)
			}(conn)
		}
	}()

	numStreams := 5
	errChan := make(chan error, numStreams)

	for i := 0; i < numStreams; i++ {
		go func(idx int) {
			stream, err := handler1.OpenTunnel(h2.ID(), "127.0.0.1", uint16(echoPort))
			if err != nil {
				errChan <- fmt.Errorf("stream %d: failed to open: %w", idx, err)
				return
			}
			defer stream.Close()

			testData := fmt.Appendf(nil, "Stream %d data", idx)
			_, err = stream.Write(testData)
			if err != nil {
				errChan <- fmt.Errorf("stream %d: failed to write: %w", idx, err)
				return
			}

			response := make([]byte, len(testData))
			_, err = io.ReadFull(stream, response)
			if err != nil {
				errChan <- fmt.Errorf("stream %d: failed to read: %w", idx, err)
				return
			}

			if string(response) != string(testData) {
				errChan <- fmt.Errorf("stream %d: data mismatch", idx)
				return
			}

			errChan <- nil
		}(i)
	}

	for i := 0; i < numStreams; i++ {
		if err := <-errChan; err != nil {
			t.Errorf("Concurrent stream error: %v", err)
		}
	}
}

func TestTunnelStreamClose(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	h1, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("Failed to create host1: %v", err)
	}
	defer h1.Close()

	h2, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("Failed to create host2: %v", err)
	}
	defer h2.Close()

	h1.Peerstore().AddAddrs(h2.ID(), h2.Addrs(), time.Hour)
	if err := h1.Connect(ctx, peer.AddrInfo{ID: h2.ID(), Addrs: h2.Addrs()}); err != nil {
		t.Fatalf("Failed to connect hosts: %v", err)
	}

	handler1 := NewHandler(h1, ctx, nil)
	NewHandler(h2, ctx, nil)

	echoListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start echo server: %v", err)
	}
	defer echoListener.Close()
	echoPort := echoListener.Addr().(*net.TCPAddr).Port

	go func() {
		for {
			conn, err := echoListener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c)
			}(conn)
		}
	}()

	stream, err := handler1.OpenTunnel(h2.ID(), "127.0.0.1", uint16(echoPort))
	if err != nil {
		t.Fatalf("Failed to open tunnel: %v", err)
	}

	testData := []byte("Before close")
	_, err = stream.Write(testData)
	if err != nil {
		t.Fatalf("Failed to write before close: %v", err)
	}

	response := make([]byte, len(testData))
	_, err = io.ReadFull(stream, response)
	if err != nil {
		t.Fatalf("Failed to read before close: %v", err)
	}

	err = stream.Close()
	if err != nil {
		t.Errorf("Failed to close stream: %v", err)
	}

	_, err = stream.Write([]byte("After close"))
	if err == nil {
		t.Error("Expected error when writing to closed stream")
	}
}

