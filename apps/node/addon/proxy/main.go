package main

import (
	"bufio"
	"bytes"
	"encoding/json"
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
	// Get manager URL from environment
	managerURL = os.Getenv("BANYAN_MANAGER_URL")
	if managerURL == "" {
		log.Fatal("BANYAN_MANAGER_URL not set")
	}

	// Allow overriding listen address
	if addr := os.Getenv("PROXY_LISTEN_ADDR"); addr != "" {
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

// bufferedConn wraps a net.Conn with a buffer of already-read bytes
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

	if isFigAddress(host) {
		handleFigTunnel(conn, host)
	} else if isPeerAddress(host) {
		handlePeerTunnel(conn, host)
	} else {
		// Direct tunnel to internet
		target, err := net.Dial("tcp", host)
		if err != nil {
			conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
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

	if isFigAddress(host) {
		handleFigHTTPRequest(conn, req, host)
	} else if isPeerAddress(host) {
		handlePeerHTTPRequest(conn, req, host)
	} else {
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

// peerAddressInfo contains parsed peer address information
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

		return peerAddressInfo{
			PeerID:    peerID,
			Multiaddr: multiaddr,
		}
	}

	// Simple format: just peer ID
	return peerAddressInfo{
		PeerID:    h,
		Multiaddr: "",
	}
}

// extractPeerID is a convenience function that just extracts the peer ID
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

// checkPeerConnected checks if we're connected to a peer
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
		Connections map[string]interface{} `json:"connections"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, nil // Assume not connected if we can't parse
	}

	_, connected := result.Connections[peerID]
	return connected, nil
}

// connectToPeer attempts to connect to a peer via DHT
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

// connectToPeerWithMultiaddr attempts to connect to a peer using a specific multiaddr
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

// ensurePeerConnected makes sure we're connected to a peer, connecting if needed
// If multiaddr is provided, uses it directly; otherwise uses DHT
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

// SOCKS5 implementation
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

	// Handle .fig and .peer addresses
	if isFigAddress(targetHost) {
		handleSOCKS5Fig(conn, targetHost, targetPort)
	} else if isPeerAddress(targetHost) {
		handleSOCKS5Peer(conn, targetHost, targetPort)
	} else {
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
