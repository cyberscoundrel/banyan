package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	nodePkg "banyan/node"
)

// LibP2PHandlers provides LibP2P-specific HTTP endpoint handlers
type LibP2PHandlers struct {
	node *nodePkg.Node
}

// NewLibP2PHandlers creates a new LibP2PHandlers instance
func NewLibP2PHandlers(node *nodePkg.Node) *LibP2PHandlers {
	return &LibP2PHandlers{
		node: node,
	}
}

// HandleStatus gets the status directly from the libp2p node
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
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(status)

	log.Printf("Served libp2p status: %s %s", r.Method, r.URL.Path)
}

// HandleConnections gets the connections directly from the libp2p node
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

// HandleConnectToPeer connects to a peer via the libp2p node using the connection manager
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
	peerIDStr := strings.TrimPrefix(r.URL.Path, "/libp2p/connect-to-peer/")
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
