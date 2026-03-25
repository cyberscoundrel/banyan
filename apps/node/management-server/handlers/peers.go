package handlers

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	nodePkg "banyan/node"
	"banyan/types"
)

type PeerHandlers struct {
	node *nodePkg.Node
}

func NewPeerHandlers(node *nodePkg.Node) *PeerHandlers {
	return &PeerHandlers{
		node: node,
	}
}

func isLocalhost(r *http.Request) bool {
	host := r.Host
	if colonIndex := strings.Index(host, ":"); colonIndex != -1 {
		host = host[:colonIndex]
	}
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func (ph *PeerHandlers) HandleAddTrackedPeer(w http.ResponseWriter, r *http.Request) {
	if !isLocalhost(r) {
		http.Error(w, "Access denied: This endpoint is only accessible from localhost", http.StatusForbidden)
		return
	}

	if ph.node == nil {
		http.Error(w, "LibP2P node not available", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed. Use POST", http.StatusMethodNotAllowed)
		return
	}

	var request struct {
		PeerID      string   `json:"peer_id"`
		Addresses   []string `json:"addresses"`
		Connect     bool     `json:"connect,omitempty"`
		HTTPCapable bool     `json:"http_capable,omitempty"`
		ServiceKey  string   `json:"service_key,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid JSON request body", http.StatusBadRequest)
		return
	}

	if request.PeerID == "" {
		http.Error(w, "peer_id is required", http.StatusBadRequest)
		return
	}

	if len(request.Addresses) == 0 {
		http.Error(w, "at least one address is required", http.StatusBadRequest)
		return
	}

	peerID, err := peer.Decode(request.PeerID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid peer ID format: %v", err), http.StatusBadRequest)
		return
	}

	var validAddrs []multiaddr.Multiaddr
	for _, addrStr := range request.Addresses {
		addr, err := multiaddr.NewMultiaddr(addrStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid multiaddress '%s': %v", addrStr, err), http.StatusBadRequest)
			return
		}
		validAddrs = append(validAddrs, addr)
	}

	var peerOptions types.PeerOptions
	if request.ServiceKey != "" {
		config := ph.node.GetConfig()
		if config == nil || config.AllowUnsafeServiceKeyInjection == nil || !*config.AllowUnsafeServiceKeyInjection {
			http.Error(w, "service_key injection requires --allow-unsafe-service-key-injection flag (SECURITY RISK - for testing only)", http.StatusForbidden)
			return
		}
		serviceKeyBytes, err := hex.DecodeString(request.ServiceKey)
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid service_key format (expected hex): %v", err), http.StatusBadRequest)
			return
		}
		peerOptions = types.NewServicePeerOptions(serviceKeyBytes, request.HTTPCapable)
	} else {
		peerOptions = types.NewManualPeerOptions(request.HTTPCapable)
	}

	_ = ph.node.GetConnectionManager().AddTrackedPeer(peerID, peerOptions)

	response := map[string]interface{}{
		"message":         "Peer added to tracking successfully",
		"peer_id":         peerID.String(),
		"addresses":       request.Addresses,
		"http_capable":    request.HTTPCapable,
		"connection_type": peerOptions.ConnectionType,
		"timestamp":       time.Now(),
	}
	if request.ServiceKey != "" {
		response["service_key"] = request.ServiceKey[:min(8, len(request.ServiceKey))] + "..."
	}

	if request.Connect {
		connected := false
		var connectionError string

		for _, addr := range validAddrs {
			peerInfo, err := peer.AddrInfoFromP2pAddr(addr)
				if err != nil {
					peerInfo = &peer.AddrInfo{
						ID:    peerID,
						Addrs: []multiaddr.Multiaddr{addr},
					}
				}

				ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
				err = ph.node.GetHost().Connect(ctx, *peerInfo)
				cancel()

				if err == nil {
					connected = true
					break
				} else {
					connectionError = err.Error()
				}
			}

			response["connection_attempted"] = true
			response["connected"] = connected
			if !connected && connectionError != "" {
				response["connection_error"] = connectionError
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)

		log.Printf("Added tracked peer: %s (connect=%v, success=%v)", peerID.String(), request.Connect, response["connected"])
	}

func (ph *PeerHandlers) HandleGetAllPeers(w http.ResponseWriter, r *http.Request) {
	if !isLocalhost(r) {
		http.Error(w, "Access denied: This endpoint is only accessible from localhost", http.StatusForbidden)
		return
	}

	if ph.node == nil {
		http.Error(w, "LibP2P node not available", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed. Use GET", http.StatusMethodNotAllowed)
		return
	}

	connectedPeers := ph.node.GetHost().Network().Peers()
	connectedPeerMap := make(map[peer.ID]bool)
	for _, p := range connectedPeers {
		connectedPeerMap[p] = true
	}

	connectionsMap := ph.node.GetConnectionManager().GetConnectionsCopy()

	var trackedPeers []map[string]interface{}
	var untrackedPeers []map[string]interface{}

	for peerID, connItem := range connectionsMap {
		var serviceKeysArray []string

		for _, serviceKey := range connItem.ServiceKeys {
			serviceKeysArray = append(serviceKeysArray, fmt.Sprintf("%x", serviceKey))
		}
		connInfo := map[string]interface{}{
			"peer_id":            peerID.String(),
			"alias":              connItem.Alias,
			"status":             connItem.Status,
			"connected_time":     connItem.Connected,
			"last_activity":      connItem.LastActivity,
			"service_keys":       serviceKeysArray,
			"connect_attempts":   connItem.ConnectAttempts,
			"connection_type":    connItem.ConnectionType,
			"http_capable":       connItem.HTTPCapable,
			"bidirectional_http": connItem.BidirectionalHTTP,
			"http_test_result":   connItem.HTTPTestResult,
			"libp2p_connected":   connectedPeerMap[peerID],
			"tracked":            true,
		}

		if connItem.LastDisconnect != nil {
			connInfo["last_disconnect"] = *connItem.LastDisconnect
		}

		if connItem.LastHTTPTest != nil {
			connInfo["last_http_test"] = *connItem.LastHTTPTest
		}

		if connectedPeerMap[peerID] {
			if conn := ph.node.GetHost().Network().ConnsToPeer(peerID); len(conn) > 0 {
				var addrs []string
				for _, c := range conn {
					addrs = append(addrs, c.RemoteMultiaddr().String())
				}
				connInfo["addresses"] = addrs
			}
		}

		trackedPeers = append(trackedPeers, connInfo)

		delete(connectedPeerMap, peerID)
	}

	for peerID := range connectedPeerMap {
		connInfo := map[string]interface{}{
			"peer_id":          peerID.String(),
			"libp2p_connected": true,
			"tracked":          false,
			"connection_type":  "untracked",
		}

		if conn := ph.node.GetHost().Network().ConnsToPeer(peerID); len(conn) > 0 {
			var addrs []string
			for _, c := range conn {
				addrs = append(addrs, c.RemoteMultiaddr().String())
			}
			connInfo["addresses"] = addrs
		}

		untrackedPeers = append(untrackedPeers, connInfo)
	}

	response := map[string]interface{}{
		"tracked_peers":   trackedPeers,
		"untracked_peers": untrackedPeers,
		"total_tracked":   len(trackedPeers),
		"total_untracked": len(untrackedPeers),
		"total_connected": len(connectedPeers),
		"total_tracked_connected": func() int {
			count := 0
			for _, peer := range trackedPeers {
				if peer["libp2p_connected"].(bool) {
					count++
				}
			}
			return count
		}(),
		"timestamp": time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)

	log.Printf("Served all peers info: %d tracked, %d untracked", len(trackedPeers), len(untrackedPeers))
}
