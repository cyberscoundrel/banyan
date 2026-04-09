// Package tunnel provides TCP tunneling over libp2p streams, enabling peers to
// forward TCP connections through the libp2p network. This allows accessing services
// on a remote peer's local network as if they were directly accessible.
//
// The package implements a simple request-response protocol:
//  1. The initiator opens a libp2p stream and sends a TunnelRequest specifying the target host and port
//  2. The responder connects to the local TCP target and sends a TunnelResponse
//  3. If successful, the stream becomes a bidirectional pipe between initiator and target
//
// Security is enforced through target whitelisting - by default only localhost targets
// are allowed, but this can be configured via SetAllowedTargets.
package tunnel

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

// ProtocolID is the libp2p protocol identifier for TCP tunneling.
const ProtocolID = protocol.ID("/banyan/tcp-tunnel/1.0.0")

// ServiceProtocolID is the libp2p protocol identifier for service-key-routed tunneling.
// Instead of specifying host:port, the initiator sends a service key and the
// receiver resolves it to a backend URL via its local route table.
const ServiceProtocolID = protocol.ID("/banyan/service-tunnel/1.0.0")

// TunnelRequest represents a request to establish a tunnel to a target host and port.
type TunnelRequest struct {
	TargetHost string
	TargetPort uint16
}

// TunnelResponse indicates the result of a tunnel establishment request.
type TunnelResponse struct {
	Success bool
	Error   string
}

// Handler manages TCP tunnel streams, handling both incoming tunnel requests
// and outgoing tunnel establishment. It enforces target access controls and
// provides bidirectional data relay between libp2p streams and TCP connections.
type Handler struct {
	host           host.Host
	ctx            context.Context
	allowedTargets []string
	routeLookup    func(string) (string, bool)
	logf           func(format string, v ...interface{})
	mu             sync.RWMutex
}

// NewHandler creates a new tunnel handler and registers the stream handler with
// the provided libp2p host. The logf function is used for logging; if nil, a
// no-op logger is used.
func NewHandler(h host.Host, ctx context.Context, logf func(format string, v ...interface{})) *Handler {
	if logf == nil {
		logf = func(format string, v ...interface{}) {}
	}
	handler := &Handler{
		host: h,
		ctx:  ctx,
		logf: logf,
	}
	h.SetStreamHandler(ProtocolID, handler.handleIncomingStream)
	h.SetStreamHandler(ServiceProtocolID, handler.handleIncomingServiceStream)
	return handler
}

// SetRouteLookup sets a callback that resolves service key identifiers to
// backend URLs. When set, the service tunnel protocol uses this to route
// incoming service-key-based tunnel requests to the correct backend.
func (h *Handler) SetRouteLookup(fn func(string) (string, bool)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.routeLookup = fn
}

// SetAllowedTargets configures the list of allowed target hosts for incoming
// tunnel requests. Use "*" to allow all targets. If the list is empty, only
// localhost targets (localhost, 127.0.0.1, ::1) are permitted.
func (h *Handler) SetAllowedTargets(targets []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.allowedTargets = targets
}

// handleIncomingStream handles an incoming tunnel request from a peer
func (h *Handler) handleIncomingStream(s network.Stream) {
	defer s.Close()

	h.logf("Incoming tunnel request from %s", s.Conn().RemotePeer())

	// Read tunnel request (length-prefixed)
	var reqLen uint32
	if err := binary.Read(s, binary.BigEndian, &reqLen); err != nil {
		h.logf("Failed to read request length: %v", err)
		h.writeResponse(s, false, "failed to read request")
		return
	}
	if reqLen > 1024 { // Sanity check
		h.writeResponse(s, false, "request too large")
		return
	}

	reqBuf := make([]byte, reqLen)
	if _, err := io.ReadFull(s, reqBuf); err != nil {
		h.logf("Failed to read request: %v", err)
		h.writeResponse(s, false, "failed to read request")
		return
	}

	// Parse request: host\0port (port as 2 bytes)
	var targetHost string
	var targetPort uint16
	for i, b := range reqBuf {
		if b == 0 {
			targetHost = string(reqBuf[:i])
			if len(reqBuf) >= i+3 {
				targetPort = binary.BigEndian.Uint16(reqBuf[i+1 : i+3])
			}
			break
		}
	}

	if targetHost == "" || targetPort == 0 {
		h.writeResponse(s, false, "invalid request format")
		return
	}

	h.logf("Tunnel request: %s:%d", targetHost, targetPort)

	// Validate target
	if !h.isAllowedTarget(targetHost) {
		h.logf("Target not allowed: %s", targetHost)
		h.writeResponse(s, false, "target not allowed")
		return
	}

	// Connect to target
	targetAddr := fmt.Sprintf("%s:%d", targetHost, targetPort)
	conn, err := net.DialTimeout("tcp", targetAddr, 10*time.Second)
	if err != nil {
		h.logf("Failed to connect to target %s: %v", targetAddr, err)
		h.writeResponse(s, false, fmt.Sprintf("failed to connect: %v", err))
		return
	}
	defer conn.Close()

	// Send success response
	if err := h.writeResponse(s, true, ""); err != nil {
		h.logf("Failed to write response: %v", err)
		return
	}

	h.logf("Tunnel established to %s", targetAddr)

	// Bidirectional relay
	done := make(chan struct{}, 2)
	go func() {
		io.Copy(conn, s)
		done <- struct{}{}
	}()
	go func() {
		io.Copy(s, conn)
		done <- struct{}{}
	}()

	// Wait for either direction to finish
	<-done
	h.logf("Tunnel closed to %s", targetAddr)
}

// writeResponse writes a tunnel response to the stream
func (h *Handler) writeResponse(s network.Stream, success bool, errMsg string) error {
	var resp byte
	if success {
		resp = 1
	} else {
		resp = 0
	}
	if _, err := s.Write([]byte{resp}); err != nil {
		return err
	}
	if !success && errMsg != "" {
		// Write error message length + message
		msgBytes := []byte(errMsg)
		if err := binary.Write(s, binary.BigEndian, uint16(len(msgBytes))); err != nil {
			return err
		}
		if _, err := s.Write(msgBytes); err != nil {
			return err
		}
	}
	return nil
}

// isAllowedTarget checks if a target host is allowed
func (h *Handler) isAllowedTarget(targetHost string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// If no explicit targets configured, only allow localhost
	if len(h.allowedTargets) == 0 {
		return targetHost == "localhost" || targetHost == "127.0.0.1" || targetHost == "::1"
	}

	// Check against allowed patterns
	for _, pattern := range h.allowedTargets {
		if pattern == "*" || pattern == targetHost {
			return true
		}
		// Could add glob matching here if needed
	}
	return false
}

// OpenTunnel establishes a tunnel to the specified target via a remote peer.
// The returned stream can be used for bidirectional communication with the target.
// Returns an error if the peer rejects the tunnel request or connection fails.
func (h *Handler) OpenTunnel(peerID peer.ID, targetHost string, targetPort uint16) (network.Stream, error) {
	s, err := h.host.NewStream(h.ctx, peerID, ProtocolID)
	if err != nil {
		return nil, fmt.Errorf("failed to open stream: %w", err)
	}

	// Send tunnel request
	reqBuf := append([]byte(targetHost), 0)
	portBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(portBuf, targetPort)
	reqBuf = append(reqBuf, portBuf...)

	// Write length-prefixed request
	if err := binary.Write(s, binary.BigEndian, uint32(len(reqBuf))); err != nil {
		s.Close()
		return nil, fmt.Errorf("failed to write request length: %w", err)
	}
	if _, err := s.Write(reqBuf); err != nil {
		s.Close()
		return nil, fmt.Errorf("failed to write request: %w", err)
	}

	// Read response
	respBuf := make([]byte, 1)
	if _, err := io.ReadFull(s, respBuf); err != nil {
		s.Close()
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if respBuf[0] == 0 {
		// Read error message
		var errLen uint16
		if err := binary.Read(s, binary.BigEndian, &errLen); err != nil {
			s.Close()
			return nil, fmt.Errorf("tunnel request failed (couldn't read error)")
		}
		errBuf := make([]byte, errLen)
		if _, err := io.ReadFull(s, errBuf); err != nil {
			s.Close()
			return nil, fmt.Errorf("tunnel request failed (couldn't read error message)")
		}
		s.Close()
		return nil, fmt.Errorf("tunnel request failed: %s", string(errBuf))
	}

	h.logf("Tunnel opened to %s via peer %s", fmt.Sprintf("%s:%d", targetHost, targetPort), peerID)
	return s, nil
}

// handleIncomingServiceStream handles an incoming service-key-routed tunnel request.
// The request contains a service key identifier; the handler looks up the corresponding
// backend URL from its route table, extracts the host:port, and establishes a TCP connection.
func (h *Handler) handleIncomingServiceStream(s network.Stream) {
	defer s.Close()

	h.logf("Incoming service tunnel request from %s", s.Conn().RemotePeer())

	var reqLen uint32
	if err := binary.Read(s, binary.BigEndian, &reqLen); err != nil {
		h.logf("Failed to read service tunnel request length: %v", err)
		h.writeResponse(s, false, "failed to read request")
		return
	}
	if reqLen > 4096 {
		h.writeResponse(s, false, "request too large")
		return
	}

	reqBuf := make([]byte, reqLen)
	if _, err := io.ReadFull(s, reqBuf); err != nil {
		h.logf("Failed to read service tunnel request: %v", err)
		h.writeResponse(s, false, "failed to read request")
		return
	}

	serviceKey := string(reqBuf)
	if serviceKey == "" {
		h.writeResponse(s, false, "empty service key")
		return
	}

	h.logf("Service tunnel request: key=%s", serviceKey)

	h.mu.RLock()
	lookup := h.routeLookup
	h.mu.RUnlock()

	if lookup == nil {
		h.writeResponse(s, false, "no route lookup configured")
		return
	}

	routeURL, found := lookup(serviceKey)
	if !found {
		h.writeResponse(s, false, fmt.Sprintf("no route for service key: %s", serviceKey))
		return
	}

	parsed, err := url.Parse(routeURL)
	if err != nil {
		h.writeResponse(s, false, fmt.Sprintf("invalid route URL: %v", err))
		return
	}

	targetHost := parsed.Hostname()
	targetPortStr := parsed.Port()
	if targetPortStr == "" {
		if parsed.Scheme == "https" {
			targetPortStr = "443"
		} else {
			targetPortStr = "80"
		}
	}
	targetPort, err := strconv.Atoi(targetPortStr)
	if err != nil {
		h.writeResponse(s, false, fmt.Sprintf("invalid port in route URL: %v", err))
		return
	}

	targetAddr := fmt.Sprintf("%s:%d", targetHost, targetPort)
	h.logf("Service tunnel routing key=%s to %s", serviceKey, targetAddr)

	if !h.isAllowedTarget(targetHost) {
		h.writeResponse(s, false, "target not allowed")
		return
	}

	conn, err := net.DialTimeout("tcp", targetAddr, 10*time.Second)
	if err != nil {
		h.writeResponse(s, false, fmt.Sprintf("failed to connect: %v", err))
		return
	}
	defer conn.Close()

	if err := h.writeResponse(s, true, ""); err != nil {
		h.logf("Failed to write service tunnel response: %v", err)
		return
	}

	h.logf("Service tunnel established: key=%s -> %s", serviceKey, targetAddr)

	remotePeerID := s.Conn().RemotePeer().String()
	h.relayWithPeerID(s, conn, remotePeerID)
	h.logf("Service tunnel closed: key=%s", serviceKey)
}

// OpenServiceTunnel establishes a service-key-routed tunnel to a remote peer.
// The remote peer resolves the service key to a backend URL via its route table
// and connects to it, keeping the backend address opaque to the initiator.
func (h *Handler) OpenServiceTunnel(peerID peer.ID, serviceKey string) (network.Stream, error) {
	s, err := h.host.NewStream(h.ctx, peerID, ServiceProtocolID)
	if err != nil {
		return nil, fmt.Errorf("failed to open service tunnel stream: %w", err)
	}

	reqBuf := []byte(serviceKey)
	if err := binary.Write(s, binary.BigEndian, uint32(len(reqBuf))); err != nil {
		s.Close()
		return nil, fmt.Errorf("failed to write service key length: %w", err)
	}
	if _, err := s.Write(reqBuf); err != nil {
		s.Close()
		return nil, fmt.Errorf("failed to write service key: %w", err)
	}

	respBuf := make([]byte, 1)
	if _, err := io.ReadFull(s, respBuf); err != nil {
		s.Close()
		return nil, fmt.Errorf("failed to read service tunnel response: %w", err)
	}

	if respBuf[0] == 0 {
		var errLen uint16
		if err := binary.Read(s, binary.BigEndian, &errLen); err != nil {
			s.Close()
			return nil, fmt.Errorf("service tunnel request failed (couldn't read error)")
		}
		errBuf := make([]byte, errLen)
		if _, err := io.ReadFull(s, errBuf); err != nil {
			s.Close()
			return nil, fmt.Errorf("service tunnel request failed (couldn't read error message)")
		}
		s.Close()
		return nil, fmt.Errorf("service tunnel request failed: %s", string(errBuf))
	}

	h.logf("Service tunnel opened for key=%s via peer %s", serviceKey, peerID)
	return s, nil
}

// relayWithPeerID reads the first HTTP request from the libp2p stream, injects an
// X-Peer-Id header identifying the remote P2P peer, writes the modified request to
// the backend connection, then falls back to a raw bidirectional relay for the
// remainder of the stream (e.g. WebSocket frames, keep-alive requests).
func (h *Handler) relayWithPeerID(s network.Stream, conn net.Conn, peerID string) {
	br := bufio.NewReader(s)

	req, err := http.ReadRequest(br)
	if err != nil {
		h.logf("Failed to read initial HTTP request from tunnel: %v", err)
		io.Copy(conn, s)
		return
	}

	req.Header.Set("X-Peer-Id", peerID)

	if err := req.Write(conn); err != nil {
		h.logf("Failed to write modified request to backend: %v", err)
		return
	}

	remaining := br.Buffered()
	if remaining > 0 {
		leftover := make([]byte, remaining)
		n, _ := br.Read(leftover)
		if n > 0 {
			conn.Write(leftover[:n])
		}
	}

	done := make(chan struct{}, 2)
	go func() { io.Copy(conn, s); done <- struct{}{} }()
	go func() { io.Copy(s, conn); done <- struct{}{} }()
	<-done
	conn.Close()
	s.Close()
}
