package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	nodePkg "banyan/node"
	"banyan/types"
)

// ProxyHandlers provides HTTP endpoint handlers for proxying requests through
// libp2p to other peers using peer ID, service key, or service alias.
type ProxyHandlers struct {
	node           *nodePkg.Node
	broadcastEvent func(types.Event)
}

// NewProxyHandlers creates a new ProxyHandlers instance
func NewProxyHandlers(node *nodePkg.Node, broadcastEvent func(types.Event)) *ProxyHandlers {
	return &ProxyHandlers{
		node:           node,
		broadcastEvent: broadcastEvent,
	}
}

// HandleLibp2pProxy handles the /proxy/peer/{peerID}/{path} endpoint to proxy
// HTTP requests through libp2p to a specific peer. It validates peer connectivity
// and HTTP capability before forwarding the request.
func (ph *ProxyHandlers) HandleLibp2pProxy(w http.ResponseWriter, r *http.Request) {
	if ph.node == nil {
		http.Error(w, "LibP2P node not available", http.StatusServiceUnavailable)
		return
	}

	// Parse the path: /proxy/peer/{peerID}/{targetPath}
	pathParts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/proxy/peer/"), "/", 2)
	if len(pathParts) < 1 || pathParts[0] == "" {
		http.Error(w, "Peer ID not specified in path", http.StatusBadRequest)
		return
	}

	peerIDStr := pathParts[0]
	targetPath := "/"
	if len(pathParts) > 1 && pathParts[1] != "" {
		targetPath = "/" + pathParts[1]
	}

	// Add query parameters if any
	if r.URL.RawQuery != "" {
		targetPath += "?" + r.URL.RawQuery
	}

	// Validate peer ID format
	peerID, err := peer.Decode(peerIDStr)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid peer ID format: %s", peerIDStr), http.StatusBadRequest)
		log.Printf("Invalid peer ID format: %s, error: %v", peerIDStr, err)
		return
	}

	// Check if we're connected to the peer
	connectedness := ph.node.GetHost().Network().Connectedness(peerID)
	if connectedness != network.Connected {
		http.Error(w, fmt.Sprintf("Not connected to peer %s (connectedness: %s)", peerIDStr, connectedness), http.StatusBadGateway)
		log.Printf("Not connected to peer %s for proxy request", peerIDStr)
		return
	}

	// Check if this peer is tracked and has HTTP capability
	connItem, tracked := ph.node.GetConnectionManager().GetConnectionInfo(peerID)
	isHTTPCapable := tracked && connItem.HTTPCapable

	// Only block if peer was previously tested and failed HTTP capability test
	if tracked && !isHTTPCapable && connItem.HTTPTestResult == "failed" {
		http.Error(w, fmt.Sprintf("Peer %s is not HTTP capable (previously failed test)", peerIDStr), http.StatusBadGateway)
		log.Printf("Peer %s is tracked but previously failed HTTP capability test", peerIDStr)
		return
	}

	log.Printf("Proxying request through libp2p to peer %s, path: %s", peerIDStr, targetPath)

	// Use the node's HTTP transport to make the request
	client := &http.Client{Transport: ph.node.GetHTTPTransport()}

	// Construct the libp2p URL
	libp2pURL := fmt.Sprintf("libp2p://%s%s", peerIDStr, targetPath)

	// Create the proxy request
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	proxyReq, err := http.NewRequestWithContext(ctx, r.Method, libp2pURL, r.Body)
	if err != nil {
		http.Error(w, "Failed to create proxy request", http.StatusInternalServerError)
		log.Printf("Failed to create proxy request: %v", err)
		return
	}

	// Copy headers
	for key, values := range r.Header {
		for _, value := range values {
			proxyReq.Header.Add(key, value)
		}
	}

	// Inject the local node's peer identity so the remote node can
	// forward it to the downstream service.
	proxyReq.Header.Set("X-Libp2p-PeerID", ph.node.GetHost().ID().String())

	// Make the request through libp2p
	resp, err := client.Do(proxyReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to proxy request to peer %s: %v", peerIDStr, err), http.StatusBadGateway)
		log.Printf("Failed to proxy request to peer %s: %v", peerIDStr, err)

		// Mark peer as potentially not HTTP capable if this was an HTTP error
		if tracked {
			ph.node.GetConnectionManager().MarkHTTPTestResult(peerID, "failed")
		}

		return
	}
	defer resp.Body.Close()

	log.Printf("Successfully proxied request to peer %s, got status: %d", peerIDStr, resp.StatusCode)

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Set status code
	w.WriteHeader(resp.StatusCode)

	// Stream response body directly
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		log.Printf("Error streaming response body from peer %s: %v", peerIDStr, err)
	}

	// If this was successful and the peer wasn't previously marked as HTTP capable, mark it now
	if !tracked || !isHTTPCapable {
		ph.node.GetConnectionManager().MarkPeerHTTPCapable(peerID, false) // We don't know about bidirectional yet
	}

	// Broadcast proxy event
	ph.broadcastEvent(types.Event{
		Type:      "proxy_request",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"target_peer": peerIDStr,
			"target_path": targetPath,
			"method":      r.Method,
			"status_code": resp.StatusCode,
			"success":     resp.StatusCode < 400,
		},
		NodeID: ph.node.GetHost().ID().String(),
	})
}

// HandleServiceKeyProxy handles the /proxy/service/{key}/{path} endpoint to proxy
// requests to a peer identified by service key. It requires a POST request with
// a JSON body containing the compressed_public_key field.
func (ph *ProxyHandlers) HandleServiceKeyProxy(w http.ResponseWriter, r *http.Request) {
	if ph.node == nil {
		http.Error(w, "LibP2P node not available", http.StatusServiceUnavailable)
		return
	}

	// Only allow POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed. Only POST requests are supported.", http.StatusMethodNotAllowed)
		return
	}

	// Read and parse request body
	var requestBody struct {
		CompressedPublicKey string `json:"compressed_public_key"`
	}

	var remainingPath string
	if r.URL.Path != "" {
		remainingPath = strings.TrimPrefix(r.URL.Path, "/proxy/service/")
	}

	err := json.NewDecoder(r.Body).Decode(&requestBody)
	if err != nil {
		http.Error(w, "Invalid request body. Expected JSON with compressed_public_key field.", http.StatusBadRequest)
		return
	}

	if requestBody.CompressedPublicKey == "" {
		http.Error(w, "Missing compressed_public_key in request body", http.StatusBadRequest)
		return
	}

	// Find the first peer in connection items with the service key
	connections := ph.node.GetConnectionManager().GetConnectionsCopy()
	found := false
	var selectedPeer peer.ID
	for _, conn := range connections {
		for _, svcKey := range conn.ServiceKeys {
			if fmt.Sprintf("%x", svcKey) == strings.ToLower(strings.TrimSpace(requestBody.CompressedPublicKey)) {
				selectedPeer = conn.PeerID
				found = true
				break
			}
		}
		if found {
			continue
		}
	}
	if !found {
		http.Error(w, fmt.Sprintf("No connected peers for service key '%s'. Ensure locator is running and peers are connected.", requestBody.CompressedPublicKey), http.StatusBadGateway)
		return
	}

	var originalPath string
	if remainingPath != "" {
		originalPath = fmt.Sprintf("/proxy/service/%s/%s", requestBody.CompressedPublicKey, remainingPath)
	} else {
		originalPath = fmt.Sprintf("/proxy/service/%s", requestBody.CompressedPublicKey)
	}

	// Route through the receiving peer's route table to reach the backend service
	newPath := fmt.Sprintf("/proxy/peer/%s/router/%s/%s", selectedPeer.String(), requestBody.CompressedPublicKey, remainingPath)
	r.URL.Path = newPath

	log.Printf("Service key proxy: %s -> %s (service key: %s, peer: %s)", originalPath, newPath, requestBody.CompressedPublicKey, selectedPeer.String())

	// Forward to the peer proxy handler
	ph.HandleLibp2pProxy(w, r)
}

// HandleAliasProxy handles the /proxy/alias/{alias}/{path} endpoint to proxy
// requests to a peer identified by service alias. It supports hierarchical path
// matching when fig data is available for the alias.
func (ph *ProxyHandlers) HandleAliasProxy(w http.ResponseWriter, r *http.Request) {
	if ph.node == nil {
		http.Error(w, "LibP2P node not available", http.StatusServiceUnavailable)
		return
	}

	// Extract alias and path from URL
	// URL format: /proxy/alias/{alias}/{path}
	path := strings.TrimPrefix(r.URL.Path, "/proxy/alias/")
	if path == "" || path == "/" {
		http.Error(w, "Missing alias in path. Format: /proxy/alias/{alias}/{path}", http.StatusBadRequest)
		return
	}

	// Find the first slash to separate alias from remaining path
	slashIndex := strings.Index(path, "/")
	var alias, remainingPath string

	if slashIndex == -1 {
		// No remaining path, just alias
		alias = path
		remainingPath = ""
	} else {
		alias = path[:slashIndex]
		remainingPath = path[slashIndex+1:]
	}

	if alias == "" {
		http.Error(w, "Empty alias in path. Format: /proxy/alias/{alias}/{path}", http.StatusBadRequest)
		return
	}

	// Allow optional addon filter via query param addon=NAME
	desiredAddon := strings.TrimSpace(r.URL.Query().Get("addon"))

	// Enforce prior alias discovery via /find: alias must map to service keys
	serviceKeys, ok := GetServiceKeysForAlias(alias, desiredAddon)
	if !ok || len(serviceKeys) == 0 {
		msg := fmt.Sprintf("Alias '%s' not loaded. Call /find first to load service keys.", alias)
		if desiredAddon != "" {
			msg = fmt.Sprintf("Alias '%s' not loaded for addon '%s'. Call /find with {\"alias\":\"%s\",\"addon\":\"%s\"} first.", alias, desiredAddon, alias, desiredAddon)
		}
		http.Error(w, msg, http.StatusPreconditionRequired)
		return
	}

	// Try to get fig data for hierarchical path matching
	figData, hasFigData := GetFigDataForAlias(alias, desiredAddon)
	var matchedKeys []string
	var matchedPath string
	var pathToForward string

	if hasFigData {
		// Use hierarchical path matching to find the most specific keys for this path
		requestPath := "/" + remainingPath
		matchedKeys, matchedPath = figData.FindKeysAndMatchedPath(requestPath)
		log.Printf("Hierarchical path match for alias '%s' path '%s': found %d keys, matched path '%s'", alias, requestPath, len(matchedKeys), matchedPath)

		// Always forward the full original path to preserve backend routing
		pathToForward = remainingPath
		log.Printf("Path to forward: '%s'", pathToForward)
	} else {
		// Fallback to all service keys if fig data not available
		matchedKeys = serviceKeys
		pathToForward = remainingPath
		log.Printf("No fig data for alias '%s', using all %d service keys", alias, len(matchedKeys))
	}

	if len(matchedKeys) == 0 {
		http.Error(w, fmt.Sprintf("No keys found for path '%s' in alias '%s'", "/"+remainingPath, alias), http.StatusNotFound)
		return
	}

	// Choose a peer connected for any of the matched keys (peers are tracked when locator connects)
	var selectedPeer peer.ID
	var selectedKey string
	connections := ph.node.GetConnectionManager().GetConnectionsCopy()
	found := false
	for pid, conn := range connections {
		// Compare against matched keys for this path
		for _, keyHex := range matchedKeys {
			for _, svcKey := range conn.ServiceKeys {
				if fmt.Sprintf("%x", svcKey) == strings.ToLower(strings.TrimSpace(keyHex)) {
					selectedPeer = pid
					selectedKey = keyHex
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if found {
			break
		}
	}

	if !found {
		// As a fallback, allow existing alias->peer mapping if present
		if aliasPeer, exists := ph.node.GetConnectionManager().GetPeerByAlias(alias); exists {
			selectedPeer = aliasPeer
			found = true
			// Use the first matched key as the service key
			if len(matchedKeys) > 0 {
				selectedKey = matchedKeys[0]
			}
		}
	}

	if !found {
		http.Error(w, fmt.Sprintf("No connected peers for alias '%s'. Ensure locator is running and peers are connected.", alias), http.StatusBadGateway)
		return
	}

	// Construct new URL for peer proxy using the stripped path
	// Route through the receiving peer's route table to reach the backend service
	newPath := fmt.Sprintf("/proxy/peer/%s/router/%s/%s", selectedPeer.String(), selectedKey, pathToForward)

	// Update request URL and forward to peer proxy handler
	originalPath := r.URL.Path
	r.URL.Path = newPath

	// Log the redirect for debugging
	log.Printf("Alias proxy: %s -> %s (alias: %s, peer: %s, service key: %s, stripped path: %s)", originalPath, newPath, alias, selectedPeer.String(), selectedKey, pathToForward)

	// Forward to the peer proxy handler
	ph.HandleLibp2pProxy(w, r)
}

// HandleServiceKeyPrefixProxy handles /proxy/service-key/{prefix}/{path}
// It resolves a key prefix to a full service key and routes through the route table.
// This is similar to git's abbreviated commit hashes - a minimum of 8 characters is required.
func (ph *ProxyHandlers) HandleServiceKeyPrefixProxy(w http.ResponseWriter, r *http.Request) {
	if ph.node == nil {
		http.Error(w, "LibP2P node not available", http.StatusServiceUnavailable)
		return
	}

	// Extract prefix and path from URL: /proxy/service-key/{prefix}/{path}
	path := strings.TrimPrefix(r.URL.Path, "/proxy/service-key/")
	if path == "" || path == "/" {
		http.Error(w, "Missing service key prefix. Format: /proxy/service-key/{prefix}/{path}", http.StatusBadRequest)
		return
	}

	parts := strings.SplitN(path, "/", 2)

	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Missing service key prefix", http.StatusBadRequest)
		return
	}

	prefix := strings.ToLower(strings.TrimSpace(parts[0]))
	remainingPath := ""
	if len(parts) > 1 {
		remainingPath = parts[1]
	}

	if len(prefix) < 8 {
		http.Error(w, "Service key prefix must be at least 8 characters", http.StatusBadRequest)
		return
	}

	// Search all connections for matching service keys
	connections := ph.node.GetConnectionManager().GetConnectionsCopy()
	var matchingKeys []string
	keyToPeer := make(map[string]peer.ID)

	for pid, conn := range connections {
		for _, svcKey := range conn.ServiceKeys {
			keyHex := fmt.Sprintf("%x", svcKey)
			if strings.HasPrefix(strings.ToLower(keyHex), prefix) {
				matchingKeys = append(matchingKeys, keyHex)
				keyToPeer[keyHex] = pid
			}
		}
	}

	if len(matchingKeys) == 0 {
		http.Error(w, fmt.Sprintf("No service key found matching prefix '%s'", prefix), http.StatusNotFound)
		return
	}

	if len(matchingKeys) > 1 {
		http.Error(w, fmt.Sprintf("Ambiguous prefix '%s' matches %d keys: %v", prefix, len(matchingKeys), matchingKeys), http.StatusBadRequest)
		return
	}

	// Exactly one match
	selectedKey := matchingKeys[0]
	selectedPeer := keyToPeer[selectedKey]

	log.Printf("Service key prefix proxy: resolved prefix '%s' to key '%s' on peer %s", prefix, selectedKey, selectedPeer.String())

	// Route through the receiving peer's route table to reach the backend service
	newPath := fmt.Sprintf("/proxy/peer/%s/router/%s/%s", selectedPeer.String(), selectedKey, remainingPath)
	r.URL.Path = newPath

	ph.HandleLibp2pProxy(w, r)
}

// HandleAliasBroadcast handles /proxy/alias/broadcast/{alias}/{path} to fan out
// a request to ALL connected peers that have the matched service key. It uses
// the same hierarchical path matching as HandleAliasProxy to resolve the path
// to a service key, then fans out concurrently.
func (ph *ProxyHandlers) HandleAliasBroadcast(w http.ResponseWriter, r *http.Request) {
	if ph.node == nil {
		http.Error(w, "LibP2P node not available", http.StatusServiceUnavailable)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/proxy/alias/broadcast/")
	if path == "" || path == "/" {
		http.Error(w, "Missing alias in path. Format: /proxy/alias/broadcast/{alias}/{path}", http.StatusBadRequest)
		return
	}

	slashIndex := strings.Index(path, "/")
	var alias, remainingPath string
	if slashIndex == -1 {
		alias = path
		remainingPath = ""
	} else {
		alias = path[:slashIndex]
		remainingPath = path[slashIndex+1:]
	}

	if alias == "" {
		http.Error(w, "Empty alias in path", http.StatusBadRequest)
		return
	}

	desiredAddon := strings.TrimSpace(r.URL.Query().Get("addon"))
	serviceKeys, ok := GetServiceKeysForAlias(alias, desiredAddon)
	if !ok || len(serviceKeys) == 0 {
		http.Error(w, fmt.Sprintf("Alias '%s' not loaded. Call /find first to load service keys.", alias), http.StatusPreconditionRequired)
		return
	}

	figData, hasFigData := GetFigDataForAlias(alias, desiredAddon)
	var matchedKeys []string
	var pathToForward string

	if hasFigData {
		requestPath := "/" + remainingPath
		matchedKeys, _ := figData.FindKeysAndMatchedPath(requestPath)
		log.Printf("Broadcast path match for alias '%s' path '%s': found %d keys", alias, requestPath, len(matchedKeys))

		pathToForward = remainingPath
	} else {
		matchedKeys = serviceKeys
		pathToForward = remainingPath
	}

	if len(matchedKeys) == 0 {
		http.Error(w, fmt.Sprintf("No keys found for path '%s' in alias '%s'", "/"+remainingPath, alias), http.StatusNotFound)
		return
	}

	peers := ph.findPeersForKeys(matchedKeys)
	if len(peers) == 0 {
		http.Error(w, fmt.Sprintf("No connected peers for alias '%s'", alias), http.StatusBadGateway)
		return
	}

	ph.fanOutToPeers(w, r, peers, matchedKeys, pathToForward, alias)
}

// HandleServiceKeyPrefixBroadcast handles /proxy/service-key/broadcast/{prefix}/{path}
// to fan out a request to all peers that have a matching service key.
func (ph *ProxyHandlers) HandleServiceKeyPrefixBroadcast(w http.ResponseWriter, r *http.Request) {
	if ph.node == nil {
		http.Error(w, "LibP2P node not available", http.StatusServiceUnavailable)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/proxy/service-key/broadcast/")
	if path == "" || path == "/" {
		http.Error(w, "Missing service key prefix", http.StatusBadRequest)
		return
	}

	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Missing service key prefix", http.StatusBadRequest)
		return
	}

	prefix := strings.ToLower(strings.TrimSpace(parts[0]))
	remainingPath := ""
	if len(parts) > 1 {
		remainingPath = parts[1]
	}

	if len(prefix) < 8 {
		http.Error(w, "Service key prefix must be at least 8 characters", http.StatusBadRequest)
		return
	}

	connections := ph.node.GetConnectionManager().GetConnectionsCopy()
	var matchedKeys []string
	keyToPeers := make(map[string][]peer.ID)

	for pid, conn := range connections {
		for _, svcKey := range conn.ServiceKeys {
			keyHex := fmt.Sprintf("%x", svcKey)
			if strings.HasPrefix(strings.ToLower(keyHex), prefix) {
				matchedKeys = append(matchedKeys, keyHex)
				keyToPeers[keyHex] = append(keyToPeers[keyHex], pid)
			}
		}
	}

	if len(matchedKeys) == 0 {
		http.Error(w, fmt.Sprintf("No service key found matching prefix '%s'", prefix), http.StatusNotFound)
		return
	}

	var peers []peer.ID
	seen := make(map[string]bool)
	for _, pids := range keyToPeers {
		for _, pid := range pids {
			pidStr := pid.String()
			if !seen[pidStr] {
				peers = append(peers, pid)
				seen[pidStr] = true
			}
		}
	}

	log.Printf("Service key broadcast: prefix '%s' matched %d keys on %d peers", prefix, len(matchedKeys), len(peers))
	ph.fanOutToPeers(w, r, peers, matchedKeys, remainingPath, prefix)
}

// findPeersForKeys returns all unique connected peers that have any of the given service keys.
func (ph *ProxyHandlers) findPeersForKeys(keys []string) []peer.ID {
	connections := ph.node.GetConnectionManager().GetConnectionsCopy()
	var peers []peer.ID
	seen := make(map[string]bool)

	for pid, conn := range connections {
		for _, keyHex := range keys {
			for _, svcKey := range conn.ServiceKeys {
				if fmt.Sprintf("%x", svcKey) == strings.ToLower(strings.TrimSpace(keyHex)) {
					pidStr := pid.String()
					if !seen[pidStr] {
						peers = append(peers, pid)
						seen[pidStr] = true
					}
				}
			}
		}
	}

	return peers
}

// fanOutToPeers fans out an HTTP request to multiple peers concurrently and returns
// an aggregated response. For each peer, it picks the first matched key and routes
// through the peer's route table.
func (ph *ProxyHandlers) fanOutToPeers(w http.ResponseWriter, r *http.Request, peers []peer.ID, matchedKeys []string, pathToForward string, label string) {
	type fanResult struct {
		peerID  peer.ID
		status  int
		body    []byte
		headers http.Header
		err     error
	}

	results := make([]fanResult, len(peers))
	var wg sync.WaitGroup

	for i, pid := range peers {
		wg.Add(1)
		go func(idx int, p peer.ID) {
			defer wg.Done()
			selectedKey := matchedKeys[0]
			newPath := fmt.Sprintf("/proxy/peer/%s/router/%s/%s", p.String(), selectedKey, pathToForward)

			proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, newPath, r.Body)
			if err != nil {
				results[idx] = fanResult{peerID: p, err: err}
				return
			}

			for key, values := range r.Header {
				for _, value := range values {
					proxyReq.Header.Add(key, value)
				}
			}
			proxyReq.Header.Set("X-Libp2p-PeerID", ph.node.GetHost().ID().String())
			proxyReq.Header.Del("X-Banyan-Broadcast")

			client := &http.Client{Transport: ph.node.GetHTTPTransport()}
			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
			defer cancel()

			resp, err := client.Do(proxyReq.WithContext(ctx))
			if err != nil {
				results[idx] = fanResult{peerID: p, err: err}
				return
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			results[idx] = fanResult{peerID: p, status: resp.StatusCode, body: body, headers: resp.Header}
		}(i, pid)
	}

	wg.Wait()

	allFailed := true
	for _, res := range results {
		if res.err == nil && res.status < 500 {
			allFailed = false
			break
		}
	}

	if allFailed {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		errs := make([]string, 0)
		for _, res := range results {
			if res.err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", res.peerID, res.err))
			} else {
				errs = append(errs, fmt.Sprintf("%s: HTTP %d", res.peerID, res.status))
			}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":      "all peers failed",
			"label":      label,
			"peer_count": len(peers),
			"errors":     errs,
		})
		return
	}

	var successCount int
	var failCount int
	var lastSuccessHeaders http.Header
	var lastSuccessStatus int
	var lastSuccessBody []byte

	for _, res := range results {
		if res.err != nil {
			failCount++
			continue
		}
		if res.status >= 400 {
			failCount++
			continue
		}
		successCount++
		lastSuccessHeaders = res.headers
		lastSuccessStatus = res.status
		lastSuccessBody = res.body
	}

	log.Printf("Broadcast to %s: %d/%d peers succeeded", label, successCount, len(peers))

	for key, values := range lastSuccessHeaders {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.Header().Set("X-Banyan-Broadcast-Results", fmt.Sprintf("%d/%d", successCount, len(peers)))
	w.WriteHeader(lastSuccessStatus)
	w.Write(lastSuccessBody)
}
