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

// TestIsAllowedTarget tests the target access control logic for tunnel
// connections.
//
// WHAT IT IS DOING:
//   Creates a tunnel handler and tests various target addresses against
//   different allowed target configurations including nil (default),
//   wildcard, and explicit target lists.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Asserts that localhost addresses (localhost, 127.0.0.1, ::1) are allowed
//   by default, external addresses are denied by default, wildcard allows
//   all targets, and explicit target lists work correctly.
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

// TestTunnelProtocol tests the full tunnel protocol flow between two
// libp2p hosts.
//
// WHAT IT IS DOING:
//   Creates two connected libp2p hosts, sets up a TCP echo server on localhost,
//   configures the target handler to allow localhost connections, and opens
//   a tunnel from one host to the other to reach the echo server.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Asserts that a tunnel stream can be opened successfully, data can be
//   written through the tunnel, and the echo response matches the original
//   data sent.
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

// TestTunnelTargetDenied tests that tunnel connections to non-allowed
// targets are properly rejected.
//
// WHAT IT IS DOING:
//   Creates two connected libp2p hosts with the default (localhost-only)
//   allowed targets configuration and attempts to open a tunnel to an
//   external address (google.com).
//
// HOW IT DEMONSTRATES IT WORKS:
//   Asserts that opening a tunnel to a denied target returns an error,
//   confirming that access control is enforced.
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

// TestTunnelConcurrentStreams tests that multiple tunnel streams can be
// opened and used concurrently without issues.
//
// WHAT IT IS DOING:
//   Creates two connected libp2p hosts, sets up a TCP echo server, and
//   opens multiple concurrent tunnel streams (5 by default) from one host
//   to the other, each sending unique data.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Asserts that all concurrent streams open successfully, each stream's
//   data is echoed back correctly without cross-stream interference, and
//   no errors occur during concurrent operations.
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

// TestTunnelStreamClose tests that tunnel streams can be properly closed
// and that operations on closed streams fail appropriately.
//
// WHAT IT IS DOING:
//   Creates two connected libp2p hosts, sets up a TCP echo server, opens
//   a tunnel stream, sends and receives data, then closes the stream and
//   attempts to write to it again.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Asserts that data can be sent and received before closing, the stream
//   closes without error, and writing to a closed stream returns an error.
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

