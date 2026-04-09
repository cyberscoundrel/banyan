package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/libp2p/go-libp2p/core/peer"

	nodePkg "banyan/node"
	tunnelPkg "banyan/tunnel"
)

// TunnelHandlers provides HTTP endpoint handlers for TCP tunnel management.
// Tunnels allow forwarding local TCP connections through libp2p to remote peers.
type TunnelHandlers struct {
	node          *nodePkg.Node
	activeTunnels sync.Map // map[uint64]*activeTunnel
	nextTunnelID  atomic.Uint64
}

type activeTunnel struct {
	id         uint64
	listener   net.Listener
	peerID     peer.ID
	host       string
	port       uint16
	serviceKey string
}

// NewTunnelHandlers creates a new TunnelHandlers instance for managing TCP tunnels.
func NewTunnelHandlers(node *nodePkg.Node) *TunnelHandlers {
	return &TunnelHandlers{
		node: node,
	}
}

// checkAuth validates the auth token if configured
func (th *TunnelHandlers) checkAuth(w http.ResponseWriter, r *http.Request) bool {
	cfg := th.node.GetConfig()
	if cfg == nil || cfg.TunnelAuthToken == nil || *cfg.TunnelAuthToken == "" {
		return true // No auth required
	}

	// Check Authorization header (Bearer token)
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		if strings.HasPrefix(authHeader, "Bearer ") {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token == *cfg.TunnelAuthToken {
				return true
			}
		}
	}

	// Check X-Tunnel-Auth header (simple token)
	if r.Header.Get("X-Tunnel-Auth") == *cfg.TunnelAuthToken {
		return true
	}

	// Check query parameter (for WebSocket or simple clients)
	if r.URL.Query().Get("auth") == *cfg.TunnelAuthToken {
		return true
	}

	http.Error(w, "Unauthorized", http.StatusUnauthorized)
	return false
}

// HandleOpenTunnel handles the /tunnel/open endpoint to create a TCP tunnel
// to a remote peer. The tunnel forwards local connections to a target host/port
// on the peer's machine.
// POST /tunnel/open
// Body: {"peer_id": "12D3...", "target_host": "localhost", "target_port": 8080}
// Returns: {"tunnel_id": 1, "local_port": 54321}
func (th *TunnelHandlers) HandleOpenTunnel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !th.checkAuth(w, r) {
		return
	}

	var req struct {
		PeerID     string `json:"peer_id"`
		TargetHost string `json:"target_host"`
		TargetPort uint16 `json:"target_port"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if req.PeerID == "" {
		http.Error(w, "peer_id is required", http.StatusBadRequest)
		return
	}
	if req.TargetHost == "" {
		req.TargetHost = "localhost"
	}
	if req.TargetPort == 0 {
		http.Error(w, "target_port is required", http.StatusBadRequest)
		return
	}

	peerID, err := peer.Decode(req.PeerID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid peer_id: %v", err), http.StatusBadRequest)
		return
	}

	tunnelHandler := th.node.GetTunnelHandler()
	if tunnelHandler == nil {
		http.Error(w, "Tunnel handler not available", http.StatusServiceUnavailable)
		return
	}

	// Create a local listener for the tunnel
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create local listener: %v", err), http.StatusInternalServerError)
		return
	}

	tunnelID := th.nextTunnelID.Add(1)
	localAddr := listener.Addr().(*net.TCPAddr)

	tunnel := &activeTunnel{
		id:       tunnelID,
		listener: listener,
		peerID:   peerID,
		host:     req.TargetHost,
		port:     req.TargetPort,
	}
	th.activeTunnels.Store(tunnelID, tunnel)

	// Start accepting connections in background
	go th.acceptTunnelConnections(tunnel, tunnelHandler)

	log.Printf("Opened tunnel %d: local port %d -> peer %s -> %s:%d",
		tunnelID, localAddr.Port, peerID.String(), req.TargetHost, req.TargetPort)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tunnel_id":  tunnelID,
		"local_port": localAddr.Port,
		"peer_id":    peerID.String(),
		"target":     fmt.Sprintf("%s:%d", req.TargetHost, req.TargetPort),
	})
}

// acceptTunnelConnections accepts connections on the local listener and tunnels them
func (th *TunnelHandlers) acceptTunnelConnections(tunnel *activeTunnel, tunnelHandler *tunnelPkg.Handler) {
	defer tunnel.listener.Close()
	defer th.activeTunnels.Delete(tunnel.id)

	for {
		conn, err := tunnel.listener.Accept()
		if err != nil {
			// Listener closed
			return
		}

		go func(localConn net.Conn) {
			defer localConn.Close()

			// Open tunnel stream to peer
			stream, err := tunnelHandler.OpenTunnel(tunnel.peerID, tunnel.host, tunnel.port)
			if err != nil {
				log.Printf("Failed to open tunnel stream: %v", err)
				return
			}
			defer stream.Close()

			// Bidirectional relay
			done := make(chan struct{}, 2)
			go func() {
				io.Copy(stream, localConn)
				done <- struct{}{}
			}()
			go func() {
				io.Copy(localConn, stream)
				done <- struct{}{}
			}()
			<-done
			localConn.Close()
			stream.Close()
		}(conn)
	}
}

// HandleCloseTunnel handles the /tunnel/close/{id} endpoint to close an active
// TCP tunnel by its ID.
// DELETE /tunnel/close/{id}
func (th *TunnelHandlers) HandleCloseTunnel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !th.checkAuth(w, r) {
		return
	}

	// Extract tunnel ID from path
	path := strings.TrimPrefix(r.URL.Path, "/tunnel/close/")
	var tunnelID uint64
	if _, err := fmt.Sscanf(path, "%d", &tunnelID); err != nil {
		http.Error(w, "Invalid tunnel ID", http.StatusBadRequest)
		return
	}

	val, ok := th.activeTunnels.Load(tunnelID)
	if !ok {
		http.Error(w, "Tunnel not found", http.StatusNotFound)
		return
	}

	tunnel := val.(*activeTunnel)
	tunnel.listener.Close()
	th.activeTunnels.Delete(tunnelID)

	log.Printf("Closed tunnel %d", tunnelID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"closed":    true,
		"tunnel_id": tunnelID,
	})
}

// HandleListTunnels handles the /tunnel/list endpoint to list all active
// TCP tunnels with their local ports and target information.
// GET /tunnel/list
func (th *TunnelHandlers) HandleListTunnels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !th.checkAuth(w, r) {
		return
	}

	tunnels := make([]map[string]interface{}, 0)
	th.activeTunnels.Range(func(key, value interface{}) bool {
		tunnel := value.(*activeTunnel)
		localAddr := tunnel.listener.Addr().(*net.TCPAddr)
		tunnels = append(tunnels, map[string]interface{}{
			"tunnel_id":  tunnel.id,
			"local_port": localAddr.Port,
			"peer_id":    tunnel.peerID.String(),
			"target":     fmt.Sprintf("%s:%d", tunnel.host, tunnel.port),
		})
		return true
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tunnels": tunnels,
		"count":   len(tunnels),
	})
}

// HandleOpenServiceTunnel handles the /tunnel/open-service endpoint to create a
// service-key-routed TCP tunnel. Unlike HandleOpenTunnel which specifies host:port,
// this endpoint sends a service key to the remote peer, which resolves it to a
// backend URL via its local route table. The backend address remains opaque to the initiator.
// POST /tunnel/open-service
// Body: {"peer_id": "12D3...", "service_key": "abcd1234..."}
// Returns: {"tunnel_id": 1, "local_port": 54321}
func (th *TunnelHandlers) HandleOpenServiceTunnel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !th.checkAuth(w, r) {
		return
	}

	var req struct {
		PeerID     string `json:"peer_id"`
		ServiceKey string `json:"service_key"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if req.PeerID == "" {
		http.Error(w, "peer_id is required", http.StatusBadRequest)
		return
	}
	if req.ServiceKey == "" {
		http.Error(w, "service_key is required", http.StatusBadRequest)
		return
	}

	peerID, err := peer.Decode(req.PeerID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid peer_id: %v", err), http.StatusBadRequest)
		return
	}

	tunnelHandler := th.node.GetTunnelHandler()
	if tunnelHandler == nil {
		http.Error(w, "Tunnel handler not available", http.StatusServiceUnavailable)
		return
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create local listener: %v", err), http.StatusInternalServerError)
		return
	}

	tunnelID := th.nextTunnelID.Add(1)
	localAddr := listener.Addr().(*net.TCPAddr)

	tunnel := &activeTunnel{
		id:         tunnelID,
		listener:   listener,
		peerID:     peerID,
		serviceKey: req.ServiceKey,
	}
	th.activeTunnels.Store(tunnelID, tunnel)

	go th.acceptServiceTunnelConnections(tunnel, tunnelHandler)

	log.Printf("Opened service tunnel %d: local port %d -> peer %s (key: %s)",
		tunnelID, localAddr.Port, peerID.String(), req.ServiceKey[:16]+"...")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tunnel_id":   tunnelID,
		"local_port":  localAddr.Port,
		"peer_id":     peerID.String(),
		"service_key": req.ServiceKey,
	})
}

// acceptServiceTunnelConnections accepts connections on the local listener and
// tunnels them using service-key routing to the remote peer.
func (th *TunnelHandlers) acceptServiceTunnelConnections(tunnel *activeTunnel, tunnelHandler *tunnelPkg.Handler) {
	defer tunnel.listener.Close()
	defer th.activeTunnels.Delete(tunnel.id)

	for {
		conn, err := tunnel.listener.Accept()
		if err != nil {
			return
		}

		go func(localConn net.Conn) {
			defer localConn.Close()

			stream, err := tunnelHandler.OpenServiceTunnel(tunnel.peerID, tunnel.serviceKey)
			if err != nil {
				log.Printf("Failed to open service tunnel stream: %v", err)
				return
			}
			defer stream.Close()

			done := make(chan struct{}, 2)
			go func() {
				io.Copy(stream, localConn)
				done <- struct{}{}
			}()
			go func() {
				io.Copy(localConn, stream)
				done <- struct{}{}
			}()
			<-done
			localConn.Close()
			stream.Close()
		}(conn)
	}
}
