package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	nodePkg "banyan/node"
	"banyan/types"
)

// LibP2PHandlers provides HTTP endpoint handlers for libp2p-specific operations
// including node status, connection management, and peer connectivity.
type LibP2PHandlers struct {
	node *nodePkg.Node
}

// NewLibP2PHandlers creates a new LibP2PHandlers instance
func NewLibP2PHandlers(node *nodePkg.Node) *LibP2PHandlers {
	return &LibP2PHandlers{
		node: node,
	}
}

// HandleStatus handles the /node/status endpoint to return the current status
// of the libp2p node including peer counts, connection types, NAT status, and DHT status.
func (lh *LibP2PHandlers) HandleStatus(w http.ResponseWriter, r *http.Request) {
	if lh.node == nil {
		http.Error(w, "LibP2P node not available", http.StatusServiceUnavailable)
		return
	}

	// Get status directly from the node
	connectedPeers := lh.node.GetHost().Network().Peers()

	connections := lh.node.GetConnectionManager().GetConnectionsCopy()
	trackedCount := len(connections)
	httpCapableCount := 0
	bidirectionalCount := 0
	connectionTypes := make(map[string]int)

	for _, connItem := range connections {
		if connItem.HTTPCapable {
			httpCapableCount++
		}
		if connItem.BidirectionalHTTP {
			bidirectionalCount++
		}
		connectionTypes[connItem.ConnectionType]++
	}

	status := map[string]interface{}{
		"node_id":                  lh.node.GetHost().ID().String(),
		"total_connected_peers":    len(connectedPeers),
		"tracked_peers":            trackedCount,
		"http_capable_peers":       httpCapableCount,
		"bidirectional_http_peers": bidirectionalCount,
		"connection_types":         connectionTypes,
		"listening_addrs":          lh.node.GetHost().Addrs(),
		"dht_enabled":              lh.node.GetDiscoveryManager().IsDHTEnabled(),
		"discovery_methods":        lh.node.GetDiscoveryManager().GetDiscoveryMethods(),
		"timestamp":                time.Now(),
		"nat_status": map[string]interface{}{
			"enabled":      lh.node.IsNATTraversalEnabled(),
			"reachability": lh.node.GetNATReachability(),
			"relay_addr":   lh.node.HasRelayAddr(),
			"relay_addrs":  lh.node.GetRelayAddrs(),
		},
		"dht_status": map[string]interface{}{
			"enabled":      lh.node.GetDiscoveryManager().IsDHTEnabled(),
			"ready":        lh.node.IsDHTReady(),
			"routing_size": lh.node.GetDHTRoutingTableSize(),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(status)

	log.Printf("Served libp2p status: %s %s", r.Method, r.URL.Path)
}

// HandleConnections handles the /network/connections endpoint to return
// information about all tracked peer connections including status, HTTP capability,
// and connection type.
func (lh *LibP2PHandlers) HandleConnections(w http.ResponseWriter, r *http.Request) {
	if lh.node == nil {
		http.Error(w, "LibP2P node not available", http.StatusServiceUnavailable)
		return
	}

	connectionsMap := lh.node.GetConnectionManager().GetConnectionsCopy()
	connections := make([]map[string]interface{}, 0, len(connectionsMap))

	for peerID, connItem := range connectionsMap {
		connInfo := map[string]interface{}{
			"peer_id":            peerID.String(),
			"alias":              connItem.Alias,
			"status":             connItem.Status,
			"connected":          connItem.Connected,
			"last_activity":      connItem.LastActivity,
			"connect_attempts":   connItem.ConnectAttempts,
			"connection_type":    connItem.ConnectionType,
			"http_capable":       connItem.HTTPCapable,
			"bidirectional_http": connItem.BidirectionalHTTP,
			"http_test_result":   connItem.HTTPTestResult,
		}

		if connItem.LastDisconnect != nil {
			connInfo["last_disconnect"] = *connItem.LastDisconnect
		}

		if connItem.LastHTTPTest != nil {
			connInfo["last_http_test"] = *connItem.LastHTTPTest
		}

		// Add current libp2p connection status
		connInfo["libp2p_connected"] = lh.node.GetHost().Network().Connectedness(peerID) == network.Connected

		connections = append(connections, connInfo)
	}

	response := map[string]interface{}{
		"total_tracked": len(connections),
		"connections":   connections,
		"timestamp":     time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)

	log.Printf("Served libp2p connections: %s %s", r.Method, r.URL.Path)
}

// HandleConnectToPeer handles the /network/connect/{peerID} endpoint to connect
// to a peer by ID. It uses the connection manager which includes DHT lookup and tracking.
func (lh *LibP2PHandlers) HandleConnectToPeer(w http.ResponseWriter, r *http.Request) {
	if lh.node == nil {
		http.Error(w, "LibP2P node not available", http.StatusServiceUnavailable)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract peer ID from path
	peerIDStr := strings.TrimPrefix(r.URL.Path, "/network/connect/")
	if peerIDStr == "" {
		http.Error(w, "Peer ID not specified", http.StatusBadRequest)
		return
	}

	// Check if it's an alias (4 characters)
	if len(peerIDStr) == 4 {
		// Try to resolve alias
		if resolvedPeerID, exists := lh.node.GetConnectionManager().GetPeerByAlias(peerIDStr); exists {
			peerIDStr = resolvedPeerID.String()
		}
	}

	// Parse peer ID
	peerID, err := peer.Decode(peerIDStr)
	if err != nil {
		http.Error(w, "Invalid peer ID format: "+peerIDStr, http.StatusBadRequest)
		return
	}

	// Use the connection manager's ConnectToPeer method which includes DHT lookup and tracking
	err = lh.node.GetConnectionManager().ConnectToPeer(peerID)
	if err != nil {
		http.Error(w, "Failed to connect to peer "+peerIDStr+": "+err.Error(), http.StatusBadGateway)
		return
	}

	// Return success response
	response := map[string]interface{}{
		"status":    "connecting",
		"message":   "Connection attempt initiated",
		"peer_id":   peerIDStr,
		"timestamp": time.Now(),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)

	log.Printf("Handled connect-to-peer request: %s %s for peer %s", r.Method, r.URL.Path, peerIDStr)
}

// HandleConnectWithMultiaddr handles the /network/connect-multiaddr endpoint
// to connect to a peer using an explicit multiaddress.
// POST /network/connect-multiaddr
// Body: {"peer_id": "12D3...", "multiaddr": "/ip4/127.0.0.1/tcp/9000"}
func (lh *LibP2PHandlers) HandleConnectWithMultiaddr(w http.ResponseWriter, r *http.Request) {
	if lh.node == nil {
		http.Error(w, "LibP2P node not available", http.StatusServiceUnavailable)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		PeerID    string `json:"peer_id"`
		Multiaddr string `json:"multiaddr"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.PeerID == "" {
		http.Error(w, "peer_id is required", http.StatusBadRequest)
		return
	}

	if req.Multiaddr == "" {
		http.Error(w, "multiaddr is required", http.StatusBadRequest)
		return
	}

	// Parse peer ID
	peerID, err := peer.Decode(req.PeerID)
	if err != nil {
		http.Error(w, "Invalid peer ID: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Check if already connected
	if lh.node.GetHost().Network().Connectedness(peerID) == network.Connected {
		response := map[string]interface{}{
			"status":    "connected",
			"message":   "Already connected to peer",
			"peer_id":   req.PeerID,
			"timestamp": time.Now(),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	// Build full multiaddr with peer ID
	fullMultiaddr := req.Multiaddr + "/p2p/" + req.PeerID

	// Parse multiaddr
	maddr, err := multiaddr.NewMultiaddr(fullMultiaddr)
	if err != nil {
		http.Error(w, "Invalid multiaddr: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Extract peer info
	peerInfo, err := peer.AddrInfoFromP2pAddr(maddr)
	if err != nil {
		http.Error(w, "Failed to parse peer info: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Verify peer ID matches
	if peerInfo.ID != peerID {
		http.Error(w, "Peer ID mismatch in multiaddr", http.StatusBadRequest)
		return
	}

	// Try to connect with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err = lh.node.GetHost().Connect(ctx, *peerInfo)
	if err != nil {
		http.Error(w, "Failed to connect: "+err.Error(), http.StatusBadGateway)
		return
	}

	// Track the peer
	lh.node.GetConnectionManager().AddTrackedPeer(peerID, types.NewManualPeerOptions(true))

	response := map[string]interface{}{
		"status":    "connected",
		"message":   "Successfully connected via multiaddr",
		"peer_id":   req.PeerID,
		"multiaddr": req.Multiaddr,
		"timestamp": time.Now(),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)

	log.Printf("Connected to peer %s via multiaddr %s", req.PeerID, req.Multiaddr)
}
