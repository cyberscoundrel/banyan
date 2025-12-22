package main

import (
	"bufio"
	"net"
	"net/http"
	"strings"
	"testing"
)

func TestIsFigAddress(t *testing.T) {
	tests := []struct {
		host     string
		expected bool
	}{
		{"example.fig", true},
		{"subdomain.example.fig", true},
		{"EXAMPLE.FIG", true},
		{"example.fig:443", true},
		{"example.fig:8080", true},
		{"example.com", false},
		{"fig.example.com", false},
		{"localhost", false},
		{"localhost:8080", false},
		{"192.168.1.1", false},
		{"192.168.1.1:80", false},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			result := isFigAddress(tt.host)
			if result != tt.expected {
				t.Errorf("isFigAddress(%q) = %v, want %v", tt.host, result, tt.expected)
			}
		})
	}
}

func TestBufferedConn(t *testing.T) {
	// Create a pipe to simulate a connection
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// Send test data
	go func() {
		server.Write([]byte("Hello, World!"))
	}()

	// Read first byte
	buf := make([]byte, 1)
	n, err := client.Read(buf)
	if err != nil || n != 1 {
		t.Fatalf("Failed to read first byte: %v", err)
	}

	// Create buffered connection with the peeked byte
	bc := &bufferedConn{Conn: client, buf: buf[:n]}

	// Read all data (should include the buffered byte)
	result := make([]byte, 13)
	totalRead := 0
	for totalRead < 13 {
		n, err := bc.Read(result[totalRead:])
		if err != nil {
			t.Fatalf("Failed to read from buffered conn: %v", err)
		}
		totalRead += n
	}

	expected := "Hello, World!"
	if string(result) != expected {
		t.Errorf("Expected %q, got %q", expected, string(result))
	}
}

func TestProtocolDetection(t *testing.T) {
	tests := []struct {
		name      string
		firstByte byte
		isSOCKS5  bool
	}{
		{"SOCKS5 version byte", 0x05, true},
		{"HTTP GET first char", 'G', false},
		{"HTTP POST first char", 'P', false},
		{"HTTP CONNECT first char", 'C', false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isSOCKS5 := tt.firstByte == 0x05
			if isSOCKS5 != tt.isSOCKS5 {
				t.Errorf("Protocol detection for byte 0x%02x: got SOCKS5=%v, want %v",
					tt.firstByte, isSOCKS5, tt.isSOCKS5)
			}
		})
	}
}

func TestParseHTTPConnectRequest(t *testing.T) {
	// Test parsing HTTP CONNECT request
	reqStr := "CONNECT example.fig:443 HTTP/1.1\r\nHost: example.fig:443\r\n\r\n"
	reader := bufio.NewReader(strings.NewReader(reqStr))

	req, err := http.ReadRequest(reader)
	if err != nil {
		t.Fatalf("Failed to parse request: %v", err)
	}

	if req.Method != "CONNECT" {
		t.Errorf("Expected method CONNECT, got %s", req.Method)
	}

	if req.Host != "example.fig:443" {
		t.Errorf("Expected host example.fig:443, got %s", req.Host)
	}

	if !isFigAddress(req.Host) {
		t.Error("Expected host to be a .fig address")
	}
}

func TestParseHTTPGetRequest(t *testing.T) {
	// Test parsing HTTP GET request
	reqStr := "GET /path HTTP/1.1\r\nHost: example.com\r\n\r\n"
	reader := bufio.NewReader(strings.NewReader(reqStr))

	req, err := http.ReadRequest(reader)
	if err != nil {
		t.Fatalf("Failed to parse request: %v", err)
	}

	if req.Method != "GET" {
		t.Errorf("Expected method GET, got %s", req.Method)
	}

	if isFigAddress(req.Host) {
		t.Error("Expected host NOT to be a .fig address")
	}
}

// TestSOCKS5AddressParsing tests SOCKS5 address type parsing
func TestSOCKS5AddressParsing(t *testing.T) {
	tests := []struct {
		name     string
		addrType byte
		data     []byte
		expected string
	}{
		{"IPv4", 0x01, []byte{127, 0, 0, 1}, "127.0.0.1"},
		{"Domain", 0x03, append([]byte{11}, []byte("example.fig")...), "example.fig"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result string
			switch tt.addrType {
			case 0x01: // IPv4
				result = net.IP(tt.data).String()
			case 0x03: // Domain
				result = string(tt.data[1:]) // Skip length byte
			}
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestIsPeerAddress(t *testing.T) {
	tests := []struct {
		host     string
		expected bool
	}{
		{"12D3KooWExample.peer", true},
		{"QmExample.peer", true},
		{"12D3KooWExample.PEER", true},
		{"12D3KooWExample.peer:443", true},
		{"12D3KooWExample.peer:8080", true},
		{"example.com", false},
		{"peer.example.com", false},
		{"example.fig", false},
		{"localhost", false},
		{"localhost:8080", false},
		{"192.168.1.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			result := isPeerAddress(tt.host)
			if result != tt.expected {
				t.Errorf("isPeerAddress(%q) = %v, want %v", tt.host, result, tt.expected)
			}
		})
	}
}

func TestExtractPeerID(t *testing.T) {
	tests := []struct {
		host     string
		expected string
	}{
		{"12D3KooWExample.peer", "12D3KooWExample"},
		{"QmExamplePeerID.peer", "QmExamplePeerID"},
		{"12D3KooWExample.peer:443", "12D3KooWExample"},
		{"12D3KooWExample.peer:8080", "12D3KooWExample"},
		// With multiaddr - should still extract just peer ID
		{"ip4-127_0_0_1-tcp-9000.p2p.12D3KooWExample.peer", "12D3KooWExample"},
		{"dns4-example_com-tcp-443.p2p.QmPeerID.peer:8080", "QmPeerID"},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			result := extractPeerID(tt.host)
			if result != tt.expected {
				t.Errorf("extractPeerID(%q) = %q, want %q", tt.host, result, tt.expected)
			}
		})
	}
}

func TestParsePeerAddress(t *testing.T) {
	tests := []struct {
		host              string
		expectedPeerID    string
		expectedMultiaddr string
	}{
		// Simple peer ID only (DHT lookup)
		{"12D3KooWExample.peer", "12D3KooWExample", ""},
		{"QmPeerID.peer", "QmPeerID", ""},
		{"12D3KooWExample.peer:443", "12D3KooWExample", ""},
		{"12D3KooWExample.peer:8080", "12D3KooWExample", ""},

		// With multiaddr - IPv4
		{"ip4-127_0_0_1-tcp-9000.p2p.12D3KooWExample.peer", "12D3KooWExample", "/ip4/127.0.0.1/tcp/9000"},
		{"ip4-192_168_1_100-tcp-4001.p2p.QmPeerID.peer", "QmPeerID", "/ip4/192.168.1.100/tcp/4001"},
		{"ip4-10_0_0_1-tcp-443.p2p.12D3KooWABC.peer:8080", "12D3KooWABC", "/ip4/10.0.0.1/tcp/443"},

		// With multiaddr - DNS
		{"dns4-example_com-tcp-443.p2p.12D3KooWExample.peer", "12D3KooWExample", "/dns4/example.com/tcp/443"},
		{"dns4-node_example_org-tcp-9000.p2p.QmPeerID.peer", "QmPeerID", "/dns4/node.example.org/tcp/9000"},

		// With multiaddr - UDP/QUIC
		{"ip4-127_0_0_1-udp-9000-quic.p2p.12D3KooWExample.peer", "12D3KooWExample", "/ip4/127.0.0.1/udp/9000/quic"},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			result := parsePeerAddress(tt.host)
			if result.PeerID != tt.expectedPeerID {
				t.Errorf("parsePeerAddress(%q).PeerID = %q, want %q", tt.host, result.PeerID, tt.expectedPeerID)
			}
			if result.Multiaddr != tt.expectedMultiaddr {
				t.Errorf("parsePeerAddress(%q).Multiaddr = %q, want %q", tt.host, result.Multiaddr, tt.expectedMultiaddr)
			}
		})
	}
}

func TestParseHTTPConnectPeerRequest(t *testing.T) {
	// Test parsing HTTP CONNECT request for .peer address
	reqStr := "CONNECT 12D3KooWExample.peer:443 HTTP/1.1\r\nHost: 12D3KooWExample.peer:443\r\n\r\n"
	reader := bufio.NewReader(strings.NewReader(reqStr))

	req, err := http.ReadRequest(reader)
	if err != nil {
		t.Fatalf("Failed to parse request: %v", err)
	}

	if req.Method != "CONNECT" {
		t.Errorf("Expected method CONNECT, got %s", req.Method)
	}

	if !isPeerAddress(req.Host) {
		t.Error("Expected host to be a .peer address")
	}

	if isFigAddress(req.Host) {
		t.Error("Expected host NOT to be a .fig address")
	}

	peerID := extractPeerID(req.Host)
	if peerID != "12D3KooWExample" {
		t.Errorf("Expected peer ID 12D3KooWExample, got %s", peerID)
	}
}

func TestAddressTypeDisambiguation(t *testing.T) {
	// Test that .fig and .peer addresses are mutually exclusive
	tests := []struct {
		host   string
		isFig  bool
		isPeer bool
	}{
		{"example.fig", true, false},
		{"12D3KooWExample.peer", false, true},
		{"example.com", false, false},
		{"peer.fig", true, false}, // .fig takes precedence in suffix
		{"fig.peer", false, true}, // .peer takes precedence in suffix
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			gotFig := isFigAddress(tt.host)
			gotPeer := isPeerAddress(tt.host)
			if gotFig != tt.isFig {
				t.Errorf("isFigAddress(%q) = %v, want %v", tt.host, gotFig, tt.isFig)
			}
			if gotPeer != tt.isPeer {
				t.Errorf("isPeerAddress(%q) = %v, want %v", tt.host, gotPeer, tt.isPeer)
			}
		})
	}
}
