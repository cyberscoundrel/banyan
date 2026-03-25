// The proxy addon provides an HTTP and SOCKS5 proxy server with special routing for
// .fig and .peer domains. It enables transparent access to remote services and peers
// through the Banyan network.
//
// .fig domains (e.g., myservice.fig) are resolved via the management API to find
// the associated peer, then tunneled through the p2p connection.
//
// .peer domains (e.g., peerID.peer or alias.peer) route directly to specific peers,
// supporting both full peer IDs and 4-character aliases.
//
// Usage:
//
//	proxy [-port PORT] [-addr ADDR]
//
// The addon requires BANYAN_MGMT_URL to be set by the addon manager.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"banyan/addon/sdk"
)

var (
	managerURL string
	listenAddr = ":8080"
)

func main() {
	// Parse command-line arguments
	portFlag := flag.Int("port", 0, "Port to listen on (default: 8080, or from PROXY_LISTEN_ADDR env)")
	addrFlag := flag.String("addr", "", "Full address to listen on (e.g., :8080, 127.0.0.1:8080)")
	flag.Parse()

	// Get manager URL from environment (set by addon manager)
	managerURL = os.Getenv("BANYAN_MGMT_URL")
	if managerURL == "" {
		log.Fatal("BANYAN_MGMT_URL not set")
	}

	// Priority: command-line args > environment variable > default
	if *addrFlag != "" {
		listenAddr = *addrFlag
	} else if *portFlag != 0 {
		listenAddr = fmt.Sprintf(":%d", *portFlag)
	} else if addr := os.Getenv("PROXY_LISTEN_ADDR"); addr != "" {
		listenAddr = addr
	}

	client := sdk.New()

	_ = client.Disclose("http-proxy", "0.1.0", map[string]any{
		"description": "HTTP/SOCKS5 proxy with .fig and .peer routing",
		"listen":      listenAddr,
	})

	// Start proxy server in background
	go startProxyServer()

	// Register a status endpoint
	mux := sdk.NewMux().WithLogger(nil)
	mux.GET("/addons/proxy/status", http.HandlerFunc(handleStatus))

	_ = client.RunMux(mux)
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	sdk.WriteJSON(w, 200, map[string]any{
		"status":      "running",
		"listen":      listenAddr,
		"manager_url": managerURL,
	})
}

func startProxyServer() {
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", listenAddr, err)
	}
	log.Printf("Proxy server listening on %s", listenAddr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Accept error: %v", err)
			continue
		}
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	// Peek first bytes to detect protocol
	buf := make([]byte, 1)
	n, err := conn.Read(buf)
	if err != nil || n == 0 {
		return
	}

	// Create buffered connection with the peeked byte
	bc := &bufferedConn{Conn: conn, buf: buf[:n]}

	switch {
	case buf[0] == 0x05:
		// SOCKS5
		handleSOCKS5(bc)
	default:
		// Assume HTTP
		handleHTTP(bc)
	}
}

// bufferedConn wraps a net.Conn with a pre-read buffer for protocol detection.
type bufferedConn struct {
	net.Conn
	buf []byte
	mu  sync.Mutex
}

func (bc *bufferedConn) Read(p []byte) (int, error) {
	bc.mu.Lock()
	if len(bc.buf) > 0 {
		n := copy(p, bc.buf)
		bc.buf = bc.buf[n:]
		bc.mu.Unlock()
		return n, nil
	}
	bc.mu.Unlock()
	return bc.Conn.Read(p)
}

func handleHTTP(conn net.Conn) {
	reader := bufio.NewReader(conn)
	req, err := http.ReadRequest(reader)
	if err != nil {
		log.Printf("Failed to read HTTP request: %v", err)
		return
	}

	if req.Method == "CONNECT" {
		handleHTTPConnect(conn, req)
	} else {
		handleHTTPProxy(conn, req, reader)
	}
}

func handleHTTPConnect(conn net.Conn, req *http.Request) {
	host := req.Host
	if !strings.Contains(host, ":") {
		host = host + ":443"
	}

	switch {
	case isFigAddress(host):
		handleFigTunnel(conn, host)
	case isSvcAddress(host):
		handleSvcTunnel(conn, host)
	case isPeerAddress(host):
		handlePeerTunnel(conn, host)
	default:
		// Direct tunnel to internet
		target, err := net.Dial("tcp", host)
		if err != nil {
			conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
			return
		}
		defer target.Close()

		if _, err := conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
			return
		}
		defer target.Close()

		conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		relay(conn, target)
	}
}

func handleHTTPProxy(conn net.Conn, req *http.Request, reader *bufio.Reader) {
	// For regular HTTP proxy requests
	targetURL := req.URL.String()
	if req.URL.Host == "" {
		targetURL = "http://" + req.Host + req.URL.Path
		if req.URL.RawQuery != "" {
			targetURL += "?" + req.URL.RawQuery
		}
	}

	host := req.Host
	if host == "" {
		host = req.URL.Host
	}

	switch {
	case isFigAddress(host):
		handleFigHTTPRequest(conn, req, host)
	case isSvcAddress(host):
		handleSvcHTTPRequest(conn, req, host)
	case isPeerAddress(host):
		handlePeerHTTPRequest(conn, req, host)
	default:
		// Direct proxy to internet
		proxyReq, err := http.NewRequest(req.Method, targetURL, req.Body)
		if err != nil {
			conn.Write([]byte("HTTP/1.1 500 Internal Server Error\r\n\r\n"))
			return
		}
		proxyReq.Header = req.Header

		client := &http.Client{}
		resp, err := client.Do(proxyReq)
		if err != nil {
			conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
			return
		}
		defer resp.Body.Close()

		resp.Write(conn)
	}
}

func isFigAddress(host string) bool {
	// Remove port if present
	h := host
	if idx := strings.LastIndex(h, ":"); idx != -1 {
		h = h[:idx]
	}
	return strings.HasSuffix(strings.ToLower(h), ".fig")
}

func isPeerAddress(host string) bool {
	// Remove port if present
	h := host
	if idx := strings.LastIndex(h, ":"); idx != -1 {
		h = h[:idx]
	}
	return strings.HasSuffix(strings.ToLower(h), ".peer")
}

func isSvcAddress(host string) bool {
	h := host
	if idx := strings.LastIndex(h, ":"); idx != -1 {
		h = h[:idx]
	}
	return strings.HasSuffix(strings.ToLower(h), ".svc")
}

// peerAddressInfo contains parsed components from a .peer hostname.
type peerAddressInfo struct {
	PeerID    string
	Multiaddr string // Empty if no multiaddr provided (use DHT)
}

// parsePeerAddress parses a .peer hostname and extracts peer ID and optional multiaddr
// Formats supported:
//   - {peerID}.peer - simple peer ID, use DHT to find
//   - {multiaddr-encoded}.p2p.{peerID}.peer - with explicit multiaddr
//
// Multiaddr encoding:
//   - `-` replaces `/` (component separator)
//   - `_` replaces `.` (for IPs/domains)
//
// Examples:
//   - 12D3KooWExample.peer -> peerID=12D3KooWExample, multiaddr=""
//   - ip4-127_0_0_1-tcp-9000.p2p.12D3KooWExample.peer -> peerID=12D3KooWExample, multiaddr="/ip4/127.0.0.1/tcp/9000"
//
// decodePeerID decodes a DNS-safe encoded peer ID back to its original case-sensitive form.
// Since DNS is case-insensitive, uppercase letters in base58 peer IDs are encoded as
// '0' followed by the lowercase letter. For example, "12D3KooW" encodes as "120d30k0o0o0w".
func decodePeerID(encoded string) string {
	var result strings.Builder
	i := 0
	for i < len(encoded) {
		if encoded[i] == '0' && i+1 < len(encoded) {
			nextChar := encoded[i+1]
			// Check if next char is a letter (should be made uppercase)
			if (nextChar >= 'a' && nextChar <= 'z') || (nextChar >= 'A' && nextChar <= 'Z') {
				result.WriteByte(byte(strings.ToUpper(string(nextChar))[0]))
				i += 2
				continue
			}
		}
		result.WriteByte(encoded[i])
		i++
	}
	return result.String()
}

// resolvePeerAlias resolves a 4-character peer alias to a full peer ID via the management API.
func resolvePeerAlias(alias string) (string, error) {
	resp, err := http.Get(managerURL + "/network/connections")
	if err != nil {
		return "", fmt.Errorf("failed to get connections: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get connections: status %d", resp.StatusCode)
	}

	// Connections is an array, not a map
	var result struct {
		Connections []struct {
			PeerID string `json:"peer_id"`
			Alias  string `json:"alias"`
		} `json:"connections"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode connections: %w", err)
	}

	// Find peer by alias
	for _, conn := range result.Connections {
		if strings.EqualFold(conn.Alias, alias) {
			log.Printf("Resolved alias '%s' to peer ID %s", alias, conn.PeerID)
			return conn.PeerID, nil
		}
	}

	return "", fmt.Errorf("no peer found with alias '%s'", alias)
}

// isPeerAlias returns true if the string is a 4-character lowercase alphanumeric alias.
func isPeerAlias(s string) bool {
	if len(s) != 4 {
		return false
	}
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

// parsePeerAddress parses a .peer hostname and extracts the peer ID and optional multiaddr.
// Supported formats:
//   - {peerID}.peer - full peer ID, uses DHT for discovery
//   - {multiaddr-encoded}.p2p.{peerID}.peer - with explicit multiaddr
//   - {alias}.peer - 4-character alias resolved via management API
//
// Multiaddr encoding uses '-' for '/' and '_' for '.'.
func parsePeerAddress(host string) peerAddressInfo {
	// Remove port if present
	h := host
	if idx := strings.LastIndex(h, ":"); idx != -1 {
		h = h[:idx]
	}
	// Remove .peer suffix
	h = strings.TrimSuffix(h, ".peer")

	// Check if there's a multiaddr (contains .p2p.)
	if idx := strings.Index(h, ".p2p."); idx != -1 {
		multiaddrEncoded := h[:idx]
		peerID := h[idx+5:] // Skip ".p2p."

		// Decode multiaddr: replace - with /, then _ with .
		multiaddr := "/" + strings.ReplaceAll(strings.ReplaceAll(multiaddrEncoded, "-", "/"), "_", ".")

		// Check if peer ID is an alias (4 chars) or needs decoding
		if isPeerAlias(peerID) {
			if resolved, err := resolvePeerAlias(peerID); err == nil {
				peerID = resolved
			}
		} else {
			// Decode peer ID (handle DNS case-insensitivity)
			peerID = decodePeerID(peerID)
		}

		return peerAddressInfo{
			PeerID:    peerID,
			Multiaddr: multiaddr,
		}
	}

	// Check if it's a peer alias (4 lowercase alphanumeric characters)
	if isPeerAlias(h) {
		if resolved, err := resolvePeerAlias(h); err == nil {
			return peerAddressInfo{
				PeerID:    resolved,
				Multiaddr: "",
			}
		}
		// If alias resolution fails, return empty - caller will handle the error
		log.Printf("Failed to resolve peer alias '%s'", h)
		return peerAddressInfo{
			PeerID:    h, // Return as-is, will fail validation later
			Multiaddr: "",
		}
	}

	// Simple format: full peer ID - decode it
	return peerAddressInfo{
		PeerID:    decodePeerID(h),
		Multiaddr: "",
	}
}

// extractPeerID is a convenience that returns just the peer ID from a .peer hostname.
func extractPeerID(host string) string {
	return parsePeerAddress(host).PeerID
}

func handleFigTunnel(conn net.Conn, host string) {
	// Parse host:port
	parts := strings.Split(host, ":")
	figHost := parts[0]
	port := "443"
	if len(parts) > 1 {
		port = parts[1]
	}

	alias := strings.TrimSuffix(figHost, ".fig")

	// Resolve alias to peer via management API
	peerID, err := resolveAliasToPeer(alias)
	if err != nil {
		log.Printf("Failed to resolve alias %s: %v", alias, err)
		conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}

	// Open tunnel via management API
	var portNum uint16
	fmt.Sscanf(port, "%d", &portNum)

	localPort, err := openTunnel(peerID, "localhost", portNum)
	if err != nil {
		log.Printf("Failed to open tunnel: %v", err)
		conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}

	// Connect to local tunnel endpoint
	tunnelConn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", localPort))
	if err != nil {
		log.Printf("Failed to connect to tunnel: %v", err)
		conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}
	defer tunnelConn.Close()

	conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	relay(conn, tunnelConn)
}

func handleFigHTTPRequest(conn net.Conn, req *http.Request, host string) {
	// For HTTP (not HTTPS) requests to .fig addresses
	parts := strings.Split(host, ":")
	figHost := parts[0]
	alias := strings.TrimSuffix(figHost, ".fig")

	// Use the alias proxy endpoint
	proxyURL := fmt.Sprintf("%s/proxy/alias/%s%s", managerURL, alias, req.URL.Path)
	if req.URL.RawQuery != "" {
		proxyURL += "?" + req.URL.RawQuery
	}

	proxyReq, err := http.NewRequest(req.Method, proxyURL, req.Body)
	if err != nil {
		conn.Write([]byte("HTTP/1.1 500 Internal Server Error\r\n\r\n"))
		return
	}
	proxyReq.Header = req.Header

	client := &http.Client{}
	resp, err := client.Do(proxyReq)
	if err != nil {
		conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}
	defer resp.Body.Close()

	resp.Write(conn)
}

func handlePeerTunnel(conn net.Conn, host string) {
	// Parse host:port
	parts := strings.Split(host, ":")
	peerHost := parts[0]
	port := "443"
	if len(parts) > 1 {
		port = parts[1]
	}

	// Parse peer address to get peer ID and optional multiaddr
	peerInfo := parsePeerAddress(peerHost)

	// Ensure we're connected to the peer (connect via multiaddr or DHT if needed)
	if err := ensurePeerConnected(peerInfo); err != nil {
		log.Printf("Failed to connect to peer %s: %v", peerInfo.PeerID, err)
		conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}

	// Open tunnel via management API
	var portNum uint16
	fmt.Sscanf(port, "%d", &portNum)

	localPort, err := openTunnel(peerInfo.PeerID, "localhost", portNum)
	if err != nil {
		log.Printf("Failed to open tunnel to peer %s: %v", peerInfo.PeerID, err)
		conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}

	// Connect to local tunnel endpoint
	tunnelConn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", localPort))
	if err != nil {
		log.Printf("Failed to connect to tunnel: %v", err)
		conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}
	defer tunnelConn.Close()

	conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	relay(conn, tunnelConn)
}

func handlePeerHTTPRequest(conn net.Conn, req *http.Request, host string) {
	// For HTTP (not HTTPS) requests to .peer addresses
	parts := strings.Split(host, ":")
	peerHost := parts[0]

	// Parse peer address to get peer ID and optional multiaddr
	peerInfo := parsePeerAddress(peerHost)

	// Ensure we're connected to the peer (connect via multiaddr or DHT if needed)
	if err := ensurePeerConnected(peerInfo); err != nil {
		log.Printf("Failed to connect to peer %s: %v", peerInfo.PeerID, err)
		conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}

	// Use the peer proxy endpoint
	proxyURL := fmt.Sprintf("%s/proxy/peer/%s%s", managerURL, peerInfo.PeerID, req.URL.Path)
	if req.URL.RawQuery != "" {
		proxyURL += "?" + req.URL.RawQuery
	}

	proxyReq, err := http.NewRequest(req.Method, proxyURL, req.Body)
	if err != nil {
		conn.Write([]byte("HTTP/1.1 500 Internal Server Error\r\n\r\n"))
		return
	}
	proxyReq.Header = req.Header

	client := &http.Client{}
	resp, err := client.Do(proxyReq)
	if err != nil {
		log.Printf("Failed to proxy to peer %s: %v", peerInfo.PeerID, err)
		conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}
	defer resp.Body.Close()

	resp.Write(conn)
}

// resolveAliasToPeer resolves a .fig alias to a peer ID via the management API.
func resolveAliasToPeer(alias string) (string, error) {
	// Call management API to resolve alias
	resp, err := http.Get(fmt.Sprintf("%s/services/find?alias=%s", managerURL, url.QueryEscape(alias)))
	if err != nil {
		return "", fmt.Errorf("failed to call find service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("find service failed: %s", string(body))
	}

	var result struct {
		Peers []struct {
			PeerID string `json:"peer_id"`
		} `json:"peers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Peers) == 0 {
		return "", fmt.Errorf("no peers found for alias %s", alias)
	}

	return result.Peers[0].PeerID, nil
}

// openTunnel requests a tunnel to a peer's service via the management API and returns the local port.
func openTunnel(peerID string, targetHost string, targetPort uint16) (int, error) {
	reqBody, _ := json.Marshal(map[string]any{
		"peer_id":     peerID,
		"target_host": targetHost,
		"target_port": targetPort,
	})

	resp, err := http.Post(
		managerURL+"/tunnel/open",
		"application/json",
		bytes.NewReader(reqBody),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to call tunnel open: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("tunnel open failed: %s", string(body))
	}

	var result struct {
		LocalPort int `json:"local_port"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.LocalPort, nil
}

// checkPeerConnected queries the management API to check if already connected to a peer.
func checkPeerConnected(peerID string) (bool, error) {
	resp, err := http.Get(managerURL + "/network/connections")
	if err != nil {
		return false, fmt.Errorf("failed to check connections: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, nil // Assume not connected if we can't check
	}

	var result struct {
		Connections []struct {
			PeerID string `json:"peer_id"`
		} `json:"connections"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("failed to decode response: %w", err)
	}

	for _, conn := range result.Connections {
		if conn.PeerID == peerID {
			return true, nil
		}
	}

	return false, nil
}

// encodePeerIDForDNS encodes a case-sensitive peer ID for use in DNS hostnames.
// Uppercase letters are prefixed with '0' and converted to lowercase.
func encodePeerIDForDNS(peerID string) string {
	var result strings.Builder
	for _, c := range peerID {
		if c >= 'A' && c <= 'Z' {
			result.WriteByte('0')
			result.WriteRune(c + 32) // Convert to lowercase
		} else {
			result.WriteRune(c)
		}
	}
	return result.String()
}

// connectToPeer initiates a connection to a peer via DHT lookup through the management API.
func connectToPeer(peerID string) error {
	resp, err := http.Post(
		managerURL+"/network/connect/"+peerID,
		"application/json",
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to initiate connection: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("connection failed: %s", string(body))
	}

	return nil
}

// connectToPeerWithMultiaddr initiates a connection to a peer using a specific multiaddr.
func connectToPeerWithMultiaddr(peerID string, multiaddr string) error {
	reqBody, _ := json.Marshal(map[string]string{
		"peer_id":   peerID,
		"multiaddr": multiaddr,
	})

	resp, err := http.Post(
		managerURL+"/network/connect-multiaddr",
		"application/json",
		bytes.NewReader(reqBody),
	)
	if err != nil {
		return fmt.Errorf("failed to initiate connection: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("connection failed: %s", string(body))
	}

	return nil
}

// ensurePeerConnected verifies connection to a peer, connecting via multiaddr or DHT if needed.
func ensurePeerConnected(peerInfo peerAddressInfo) error {
	// First check if already connected
	connected, _ := checkPeerConnected(peerInfo.PeerID)
	if connected {
		log.Printf("Already connected to peer %s", peerInfo.PeerID)
		return nil
	}

	log.Printf("Not connected to peer %s, attempting connection...", peerInfo.PeerID)

	// Try to connect
	var err error
	if peerInfo.Multiaddr != "" {
		log.Printf("Connecting to peer %s via multiaddr: %s", peerInfo.PeerID, peerInfo.Multiaddr)
		err = connectToPeerWithMultiaddr(peerInfo.PeerID, peerInfo.Multiaddr)
	} else {
		log.Printf("Connecting to peer %s via DHT lookup", peerInfo.PeerID)
		err = connectToPeer(peerInfo.PeerID)
	}

	if err != nil {
		return fmt.Errorf("failed to connect to peer %s: %w", peerInfo.PeerID, err)
	}

	log.Printf("Successfully initiated connection to peer %s", peerInfo.PeerID)
	return nil
}

// relay bidirectionally copies data between two connections until one side closes.
func relay(c1, c2 net.Conn) {
	done := make(chan struct{}, 2)
	go func() {
		io.Copy(c1, c2)
		done <- struct{}{}
	}()
	go func() {
		io.Copy(c2, c1)
		done <- struct{}{}
	}()
	<-done
}

// handleSOCKS5 processes a SOCKS5 connection request after protocol detection.
func handleSOCKS5(conn net.Conn) {
	// We already read the first byte (0x05) in handleConnection
	// Read number of authentication methods
	buf := make([]byte, 1)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return
	}
	nmethods := int(buf[0])

	// Read authentication methods
	methods := make([]byte, nmethods)
	if _, err := io.ReadFull(conn, methods); err != nil {
		return
	}

	// Reply with no authentication required (0x00)
	conn.Write([]byte{0x05, 0x00})

	// Read connection request
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return
	}

	if header[0] != 0x05 {
		return
	}

	cmd := header[1]
	if cmd != 0x01 { // Only support CONNECT
		conn.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0}) // Command not supported
		return
	}

	// Parse address
	var targetHost string
	var targetPort uint16

	addrType := header[3]
	switch addrType {
	case 0x01: // IPv4
		addr := make([]byte, 4)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return
		}
		targetHost = net.IP(addr).String()
	case 0x03: // Domain name
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return
		}
		domain := make([]byte, lenBuf[0])
		if _, err := io.ReadFull(conn, domain); err != nil {
			return
		}
		targetHost = string(domain)
	case 0x04: // IPv6
		addr := make([]byte, 16)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return
		}
		targetHost = net.IP(addr).String()
	default:
		conn.Write([]byte{0x05, 0x08, 0x00, 0x01, 0, 0, 0, 0, 0, 0}) // Address type not supported
		return
	}

	// Read port
	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return
	}
	targetPort = uint16(portBuf[0])<<8 | uint16(portBuf[1])

	// Handle .fig, .svc, and .peer addresses
	switch {
	case isFigAddress(targetHost):
		handleSOCKS5Fig(conn, targetHost, targetPort)
	case isSvcAddress(targetHost):
		handleSOCKS5Svc(conn, targetHost, targetPort)
	case isPeerAddress(targetHost):
		handleSOCKS5Peer(conn, targetHost, targetPort)
	default:
		handleSOCKS5Direct(conn, targetHost, targetPort)
	}
}

func handleSOCKS5Direct(conn net.Conn, host string, port uint16) {
	target, err := net.Dial("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		conn.Write([]byte{0x05, 0x04, 0x00, 0x01, 0, 0, 0, 0, 0, 0}) // Host unreachable
		return
	}
	defer target.Close()

	// Success reply
	localAddr := target.LocalAddr().(*net.TCPAddr)
	if localAddr == nil {
		conn.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0}) // General failure
		return
	}
	reply := []byte{0x05, 0x00, 0x00, 0x01}
	reply = append(reply, localAddr.IP.To4()...)
	reply = append(reply, byte(localAddr.Port>>8), byte(localAddr.Port&0xff))
	conn.Write(reply)

	relay(conn, target)
}

func handleSOCKS5Fig(conn net.Conn, host string, port uint16) {
	alias := strings.TrimSuffix(host, ".fig")

	peerID, err := resolveAliasToPeer(alias)
	if err != nil {
		log.Printf("Failed to resolve alias %s: %v", alias, err)
		conn.Write([]byte{0x05, 0x04, 0x00, 0x01, 0, 0, 0, 0, 0, 0}) // Host unreachable
		return
	}

	localPort, err := openTunnel(peerID, "localhost", port)
	if err != nil {
		log.Printf("Failed to open tunnel: %v", err)
		conn.Write([]byte{0x05, 0x04, 0x00, 0x01, 0, 0, 0, 0, 0, 0}) // Host unreachable
		return
	}

	tunnelConn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", localPort))
	if err != nil {
		log.Printf("Failed to connect to tunnel: %v", err)
		conn.Write([]byte{0x05, 0x04, 0x00, 0x01, 0, 0, 0, 0, 0, 0}) // Host unreachable
		return
	}
	defer tunnelConn.Close()

	// Success reply with localhost bind address
	reply := []byte{0x05, 0x00, 0x00, 0x01, 127, 0, 0, 1}
	reply = append(reply, byte(localPort>>8), byte(localPort&0xff))
	conn.Write(reply)

	relay(conn, tunnelConn)
}

func handleSOCKS5Peer(conn net.Conn, host string, port uint16) {
	// Parse peer address to get peer ID and optional multiaddr
	peerInfo := parsePeerAddress(host)

	// Ensure we're connected to the peer (connect via multiaddr or DHT if needed)
	if err := ensurePeerConnected(peerInfo); err != nil {
		log.Printf("Failed to connect to peer %s: %v", peerInfo.PeerID, err)
		conn.Write([]byte{0x05, 0x04, 0x00, 0x01, 0, 0, 0, 0, 0, 0}) // Host unreachable
		return
	}

	localPort, err := openTunnel(peerInfo.PeerID, "localhost", port)
	if err != nil {
		log.Printf("Failed to open tunnel to peer %s: %v", peerInfo.PeerID, err)
		conn.Write([]byte{0x05, 0x04, 0x00, 0x01, 0, 0, 0, 0, 0, 0}) // Host unreachable
		return
	}

	tunnelConn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", localPort))
	if err != nil {
		log.Printf("Failed to connect to tunnel: %v", err)
		conn.Write([]byte{0x05, 0x04, 0x00, 0x01, 0, 0, 0, 0, 0, 0}) // Host unreachable
		return
	}
	defer tunnelConn.Close()

	// Success reply with localhost bind address
	reply := []byte{0x05, 0x00, 0x00, 0x01, 127, 0, 0, 1}
	reply = append(reply, byte(localPort>>8), byte(localPort&0xff))
	conn.Write(reply)

	relay(conn, tunnelConn)
}

func resolveServiceKeyPrefix(prefix string) (string, string, error) {
	resp, err := http.Get(managerURL + "/network/connections")
	if err != nil {
		return "", "", fmt.Errorf("failed to get connections: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Connections []struct {
			PeerID      string   `json:"peer_id"`
			ServiceKeys []string `json:"service_keys"`
		} `json:"connections"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", fmt.Errorf("failed to decode response: %w", err)
	}

	var matchingKeys []string
	var matchingPeerID string
	prefixLower := strings.ToLower(prefix)

	for _, conn := range result.Connections {
		for _, key := range conn.ServiceKeys {
			if strings.HasPrefix(strings.ToLower(key), prefixLower) {
				matchingKeys = append(matchingKeys, key)
				matchingPeerID = conn.PeerID
			}
		}
	}

	if len(matchingKeys) == 0 {
		return "", "", fmt.Errorf("no service key found matching prefix '%s'", prefix)
	}
	if len(matchingKeys) > 1 {
		return "", "", fmt.Errorf("ambiguous prefix matches %d keys: %v", len(matchingKeys), matchingKeys)
	}

	return matchingPeerID, matchingKeys[0], nil
}

func handleSvcTunnel(conn net.Conn, host string) {
	parts := strings.Split(host, ":")
	svcHost := parts[0]
	port := "443"
	if len(parts) > 1 {
		port = parts[1]
	}

	prefix := strings.TrimSuffix(svcHost, ".svc")

	peerID, _, err := resolveServiceKeyPrefix(prefix)
	if err != nil {
		log.Printf("Failed to resolve service key prefix %s: %v", prefix, err)
		conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}

	var portNum uint16
	fmt.Sscanf(port, "%d", &portNum)

	localPort, err := openTunnel(peerID, "localhost", portNum)
	if err != nil {
		log.Printf("Failed to open tunnel: %v", err)
		conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}

	tunnelConn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", localPort))
	if err != nil {
		log.Printf("Failed to connect to tunnel: %v", err)
		conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}
	defer tunnelConn.Close()

	conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	relay(conn, tunnelConn)
}

func handleSvcHTTPRequest(conn net.Conn, req *http.Request, host string) {
	parts := strings.Split(host, ":")
	svcHost := parts[0]
	prefix := strings.TrimSuffix(svcHost, ".svc")

	proxyURL := fmt.Sprintf("%s/proxy/service-key/%s%s", managerURL, prefix, req.URL.Path)
	if req.URL.RawQuery != "" {
		proxyURL += "?" + req.URL.RawQuery
	}

	proxyReq, err := http.NewRequest(req.Method, proxyURL, req.Body)
	if err != nil {
		conn.Write([]byte("HTTP/1.1 500 Internal Server Error\r\n\r\n"))
		return
	}
	proxyReq.Header = req.Header

	client := &http.Client{}
	resp, err := client.Do(proxyReq)
	if err != nil {
		conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}
	defer resp.Body.Close()

	resp.Write(conn)
}

func handleSOCKS5Svc(conn net.Conn, host string, port uint16) {
	prefix := strings.TrimSuffix(host, ".svc")

	peerID, _, err := resolveServiceKeyPrefix(prefix)
	if err != nil {
		log.Printf("Failed to resolve service key prefix %s: %v", prefix, err)
		conn.Write([]byte{0x05, 0x04, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	localPort, err := openTunnel(peerID, "localhost", port)
	if err != nil {
		log.Printf("Failed to open tunnel: %v", err)
		conn.Write([]byte{0x05, 0x04, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	tunnelConn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", localPort))
	if err != nil {
		log.Printf("Failed to connect to tunnel: %v", err)
		conn.Write([]byte{0x05, 0x04, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	defer tunnelConn.Close()

	reply := []byte{0x05, 0x00, 0x00, 0x01, 127, 0, 0, 1}
	reply = append(reply, byte(localPort>>8), byte(localPort&0xff))
	conn.Write(reply)

	relay(conn, tunnelConn)
}
