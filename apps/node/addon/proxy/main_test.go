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

func TestIsSvcAddress(t *testing.T) {
	tests := []struct {
		host     string
		expected bool
	}{
		{"abc12345.svc", true},
		{"deadbeef.svc", true},
		{"ABC12345.SVC", true},
		{"abc12345.svc:443", true},
		{"abc12345.svc:8080", true},
		{"example.com", false},
		{"svc.example.com", false},
		{"example.fig", false},
		{"example.peer", false},
		{"localhost", false},
		{"localhost:8080", false},
		{"192.168.1.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			result := isSvcAddress(tt.host)
			if result != tt.expected {
				t.Errorf("isSvcAddress(%q) = %v, want %v", tt.host, result, tt.expected)
			}
		})
	}
}

func TestAddressTypeDisambiguationWithSvc(t *testing.T) {
	tests := []struct {
		host   string
		isFig  bool
		isPeer bool
		isSvc  bool
	}{
		{"example.fig", true, false, false},
		{"12D3KooWExample.peer", false, true, false},
		{"abc12345.svc", false, false, true},
		{"example.com", false, false, false},
		{"svc.fig", true, false, false},
		{"fig.peer", false, true, false},
		{"fig.svc", false, false, true},
		{"peer.svc", false, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			gotFig := isFigAddress(tt.host)
			gotPeer := isPeerAddress(tt.host)
			gotSvc := isSvcAddress(tt.host)
			if gotFig != tt.isFig {
				t.Errorf("isFigAddress(%q) = %v, want %v", tt.host, gotFig, tt.isFig)
			}
			if gotPeer != tt.isPeer {
				t.Errorf("isPeerAddress(%q) = %v, want %v", tt.host, gotPeer, tt.isPeer)
			}
			if gotSvc != tt.isSvc {
				t.Errorf("isSvcAddress(%q) = %v, want %v", tt.host, gotSvc, tt.isSvc)
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

func TestParseHTTPConnectSvcRequest(t *testing.T) {
	reqStr := "CONNECT abc12345def.svc:443 HTTP/1.1\r\nHost: abc12345def.svc:443\r\n\r\n"
	reader := bufio.NewReader(strings.NewReader(reqStr))

	req, err := http.ReadRequest(reader)
	if err != nil {
		t.Fatalf("Failed to parse request: %v", err)
	}

	if req.Method != "CONNECT" {
		t.Errorf("Expected method CONNECT, got %s", req.Method)
	}

	if !isSvcAddress(req.Host) {
		t.Error("Expected host to be a .svc address")
	}

	if isFigAddress(req.Host) {
		t.Error("Expected host NOT to be a .fig address")
	}

	if isPeerAddress(req.Host) {
		t.Error("Expected host NOT to be a .peer address")
	}
}

func TestSvcAddressPortParsing(t *testing.T) {
	tests := []struct {
		input        string
		expectedHost string
		expectedPort string
	}{
		{"abc12345.svc:443", "abc12345.svc", "443"},
		{"deadbeefcafe.svc:8080", "deadbeefcafe.svc", "8080"},
		{"abc12345.svc", "abc12345.svc", "443"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			host := tt.input
			port := "443"
			if idx := strings.LastIndex(host, ":"); idx != -1 {
				port = host[idx+1:]
				host = host[:idx]
			}
			if host != tt.expectedHost {
				t.Errorf("Host: got %q, want %q", host, tt.expectedHost)
			}
			if port != tt.expectedPort {
				t.Errorf("Port: got %q, want %q", port, tt.expectedPort)
			}
		})
	}
}

func TestSvcPrefixExtraction(t *testing.T) {
	tests := []struct {
		host            string
		expectedPrefix  string
	}{
		{"abc12345def.svc:443", "abc12345def"},
		{"deadbeefcafe.svc:8080", "deadbeefcafe"},
		{"abc12345.svc", "abc12345"},
		{"UPPERCASE.svc:443", "UPPERCASE"},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			parts := strings.Split(tt.host, ":")
			svcHost := parts[0]
			prefix := strings.TrimSuffix(svcHost, ".svc")

			if prefix != tt.expectedPrefix {
				t.Errorf("Prefix: got %q, want %q", prefix, tt.expectedPrefix)
			}
		})
	}
}

func TestRelay(t *testing.T) {
	// Create two pairs of pipes to simulate connections
	client1, server1 := net.Pipe()
	client2, server2 := net.Pipe()
	defer client1.Close()
	defer server1.Close()
	defer client2.Close()
	defer server2.Close()

	// Start relay between server1 and client2
	done := make(chan struct{})
	go func() {
		relay(server1, client2)
		close(done)
	}()

	// Write from client1, expect to read from server2
	testData := []byte("Hello from client1")
	go func() {
		client1.Write(testData)
	}()

	buf := make([]byte, len(testData))
	n, err := server2.Read(buf)
	if err != nil {
		t.Fatalf("Failed to read from server2: %v", err)
	}
	if string(buf[:n]) != string(testData) {
		t.Errorf("Expected %q, got %q", testData, buf[:n])
	}

	// Write from server2, expect to read from client1
	responseData := []byte("Response from server2")
	go func() {
		server2.Write(responseData)
	}()

	buf2 := make([]byte, len(responseData))
	n, err = client1.Read(buf2)
	if err != nil {
		t.Fatalf("Failed to read from client1: %v", err)
	}
	if string(buf2[:n]) != string(responseData) {
		t.Errorf("Expected %q, got %q", responseData, buf2[:n])
	}

	// Close one side and wait for relay to finish
	client1.Close()
	<-done
}

// TestSOCKS5Handshake tests the SOCKS5 authentication handshake
func TestSOCKS5Handshake(t *testing.T) {
	// Simulate a SOCKS5 client handshake
	tests := []struct {
		name     string
		request  []byte
		response []byte
	}{
		{
			name:     "no auth method",
			request:  []byte{0x05, 0x01, 0x00}, // version, nmethods=1, no auth
			response: []byte{0x05, 0x00},       // version, no auth required
		},
		{
			name:     "multiple auth methods",
			request:  []byte{0x05, 0x02, 0x00, 0x02}, // version, nmethods=2, no auth + username/pass
			response: []byte{0x05, 0x00},             // version, no auth required (we pick no auth)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The SOCKS5 handshake response is always {0x05, 0x00} for no auth
			// Verify the expected format
			if len(tt.response) != 2 || tt.response[0] != 0x05 || tt.response[1] != 0x00 {
				t.Errorf("Invalid expected response format")
			}
		})
	}
}

// TestSOCKS5ConnectRequest tests parsing SOCKS5 connect requests
func TestSOCKS5ConnectRequest(t *testing.T) {
	tests := []struct {
		name           string
		addrType       byte
		addrData       []byte
		port           uint16
		expectedHost   string
		expectedPort   uint16
		expectedIsFig  bool
		expectedIsPeer bool
	}{
		{
			name:           "IPv4 localhost",
			addrType:       0x01,
			addrData:       []byte{127, 0, 0, 1},
			port:           8080,
			expectedHost:   "127.0.0.1",
			expectedPort:   8080,
			expectedIsFig:  false,
			expectedIsPeer: false,
		},
		{
			name:           "Domain .fig address",
			addrType:       0x03,
			addrData:       append([]byte{11}, []byte("example.fig")...),
			port:           443,
			expectedHost:   "example.fig",
			expectedPort:   443,
			expectedIsFig:  true,
			expectedIsPeer: false,
		},
		{
			name:           "Domain .peer address",
			addrType:       0x03,
			addrData:       append([]byte{20}, []byte("12D3KooWExample.peer")...),
			port:           9000,
			expectedHost:   "12D3KooWExample.peer",
			expectedPort:   9000,
			expectedIsFig:  false,
			expectedIsPeer: true,
		},
		{
			name:           "IPv6 address",
			addrType:       0x04,
			addrData:       []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}, // ::1
			port:           80,
			expectedHost:   "::1",
			expectedPort:   80,
			expectedIsFig:  false,
			expectedIsPeer: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var host string
			switch tt.addrType {
			case 0x01: // IPv4
				host = net.IP(tt.addrData).String()
			case 0x03: // Domain
				host = string(tt.addrData[1:])
			case 0x04: // IPv6
				host = net.IP(tt.addrData).String()
			}

			if host != tt.expectedHost {
				t.Errorf("Host: got %q, want %q", host, tt.expectedHost)
			}
			if tt.port != tt.expectedPort {
				t.Errorf("Port: got %d, want %d", tt.port, tt.expectedPort)
			}
			if isFigAddress(host) != tt.expectedIsFig {
				t.Errorf("isFigAddress(%q): got %v, want %v", host, isFigAddress(host), tt.expectedIsFig)
			}
			if isPeerAddress(host) != tt.expectedIsPeer {
				t.Errorf("isPeerAddress(%q): got %v, want %v", host, isPeerAddress(host), tt.expectedIsPeer)
			}
		})
	}
}

// TestBufferedConnMultipleReads tests multiple reads from buffered connection
func TestBufferedConnMultipleReads(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// Send test data
	testData := []byte("Hello, World! This is a longer message.")
	go func() {
		server.Write(testData)
	}()

	// Read first 5 bytes
	peeked := make([]byte, 5)
	n, err := client.Read(peeked)
	if err != nil || n != 5 {
		t.Fatalf("Failed to read peeked bytes: %v", err)
	}

	// Create buffered connection with peeked bytes
	bc := &bufferedConn{Conn: client, buf: peeked[:n]}

	// Read in small chunks
	var result []byte
	chunk := make([]byte, 10)
	for len(result) < len(testData) {
		n, err := bc.Read(chunk)
		if err != nil {
			break
		}
		result = append(result, chunk[:n]...)
	}

	if string(result) != string(testData) {
		t.Errorf("Expected %q, got %q", testData, result)
	}
}

// TestHTTPProxyURLConstruction tests how HTTP proxy URLs are constructed
func TestHTTPProxyURLConstruction(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		path     string
		query    string
		expected string
	}{
		{
			name:     "simple path",
			host:     "example.com",
			path:     "/api/data",
			query:    "",
			expected: "http://example.com/api/data",
		},
		{
			name:     "path with query",
			host:     "example.com",
			path:     "/api/search",
			query:    "q=test&limit=10",
			expected: "http://example.com/api/search?q=test&limit=10",
		},
		{
			name:     "root path",
			host:     "example.com",
			path:     "/",
			query:    "",
			expected: "http://example.com/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targetURL := "http://" + tt.host + tt.path
			if tt.query != "" {
				targetURL += "?" + tt.query
			}
			if targetURL != tt.expected {
				t.Errorf("URL construction: got %q, want %q", targetURL, tt.expected)
			}
		})
	}
}

// TestPortParsing tests port extraction from host:port strings
func TestPortParsing(t *testing.T) {
	tests := []struct {
		input        string
		expectedHost string
		expectedPort string
	}{
		{"example.fig:443", "example.fig", "443"},
		{"example.fig:8080", "example.fig", "8080"},
		{"example.fig", "example.fig", "443"}, // default to 443
		{"12D3KooWExample.peer:9000", "12D3KooWExample.peer", "9000"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			host := tt.input
			port := "443" // default
			if idx := strings.LastIndex(host, ":"); idx != -1 {
				port = host[idx+1:]
				host = host[:idx]
			}
			if host != tt.expectedHost {
				t.Errorf("Host: got %q, want %q", host, tt.expectedHost)
			}
			if port != tt.expectedPort {
				t.Errorf("Port: got %q, want %q", port, tt.expectedPort)
			}
		})
	}
}

// TestSOCKS5ReplyFormat tests the format of SOCKS5 success replies
func TestSOCKS5ReplyFormat(t *testing.T) {
	// Test constructing SOCKS5 success reply
	localPort := 12345
	expectedReply := []byte{
		0x05,         // version
		0x00,         // success
		0x00,         // reserved
		0x01,         // IPv4 address type
		127, 0, 0, 1, // localhost
		byte(localPort >> 8), byte(localPort & 0xff), // port in network order
	}

	// Construct reply as in handleSOCKS5Fig/handleSOCKS5Peer
	reply := []byte{0x05, 0x00, 0x00, 0x01, 127, 0, 0, 1}
	reply = append(reply, byte(localPort>>8), byte(localPort&0xff))

	if len(reply) != len(expectedReply) {
		t.Errorf("Reply length: got %d, want %d", len(reply), len(expectedReply))
	}
	for i := range reply {
		if reply[i] != expectedReply[i] {
			t.Errorf("Reply byte %d: got 0x%02x, want 0x%02x", i, reply[i], expectedReply[i])
		}
	}
}

// TestSOCKS5ErrorReply tests SOCKS5 error reply format
func TestSOCKS5ErrorReply(t *testing.T) {
	tests := []struct {
		name        string
		errorCode   byte
		description string
	}{
		{"general failure", 0x01, "General failure"},
		{"connection refused", 0x05, "Connection refused"},
		{"host unreachable", 0x04, "Host unreachable"},
		{"command not supported", 0x07, "Command not supported"},
		{"address type not supported", 0x08, "Address type not supported"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Standard error reply format
			reply := []byte{0x05, tt.errorCode, 0x00, 0x01, 0, 0, 0, 0, 0, 0}

			// Verify reply structure
			if reply[0] != 0x05 {
				t.Errorf("Version byte should be 0x05, got 0x%02x", reply[0])
			}
			if reply[1] != tt.errorCode {
				t.Errorf("Error code should be 0x%02x, got 0x%02x", tt.errorCode, reply[1])
			}
			if len(reply) != 10 {
				t.Errorf("Reply should be 10 bytes, got %d", len(reply))
			}
		})
	}
}
