package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	nodePkg "banyan/node"
	"banyan/types"
)

// ProxyHandlers provides proxy-related HTTP endpoint handlers
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

// HandleLibp2pProxy proxies requests through libp2p HTTP to other peers
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

	// Construct new URL for peer proxy
	// Use /router/ endpoint if the peer has a route registered, otherwise use direct path
	newPath := fmt.Sprintf("/proxy/peer/%s/%s", selectedPeer.String(), remainingPath)

	// Update request URL and forward to peer proxy handler
	originalPath := r.URL.Path
	if remainingPath != "" {
		originalPath = fmt.Sprintf("/proxy/service/%s/%s", requestBody.CompressedPublicKey, remainingPath)
	} else {
		originalPath = fmt.Sprintf("/proxy/service/%s", requestBody.CompressedPublicKey)
	}
	r.URL.Path = newPath

	// Log the redirect for debugging
	log.Printf("Service key proxy: %s -> %s (service key: %s, peer: %s)", originalPath, newPath, requestBody.CompressedPublicKey, selectedPeer.String())

	// Forward to the peer proxy handler
	ph.HandleLibp2pProxy(w, r)
}

// HandleAliasProxy redirects alias-based requests to the libp2p proxy
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

		// Strip the matched path prefix to get the path to forward to the backend
		if matchedPath != "" && matchedPath != "/" {
			// Remove the matched prefix from the request path
			pathToForward = strings.TrimPrefix(requestPath, matchedPath)
			// Ensure it starts with / or is empty
			if pathToForward != "" && !strings.HasPrefix(pathToForward, "/") {
				pathToForward = "/" + pathToForward
			}
			// Remove leading slash for forwarding (will be added back later)
			pathToForward = strings.TrimPrefix(pathToForward, "/")
		} else {
			// Root path or no match, forward the full path
			pathToForward = remainingPath
		}
		log.Printf("Path to forward: '%s' (stripped '%s' from '%s')", pathToForward, matchedPath, requestPath)
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
	// Use direct path - the receiving peer will handle the request
	newPath := fmt.Sprintf("/proxy/peer/%s/%s", selectedPeer.String(), pathToForward)

	// Update request URL and forward to peer proxy handler
	originalPath := r.URL.Path
	r.URL.Path = newPath

	// Log the redirect for debugging
	log.Printf("Alias proxy: %s -> %s (alias: %s, peer: %s, service key: %s, stripped path: %s)", originalPath, newPath, alias, selectedPeer.String(), selectedKey, pathToForward)

	// Forward to the peer proxy handler
	ph.HandleLibp2pProxy(w, r)
}
