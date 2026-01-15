package managementserver

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"

	"banyan/management-server/handlers"
	nodePkg "banyan/node"
	"banyan/types"
	"banyan/websocket"
)

// corsMiddleware adds CORS headers to allow WebUI cross-origin requests
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// Handle preflight OPTIONS request
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// StartManagementServer starts the HTTP management/API server and returns the server instance and URL
func StartManagementServer(node *nodePkg.Node) (*ManagementServer, string, error) {
	// Initialize WebSocket hub for event streaming
	hub := websocket.NewHub()
	go hub.Run()

	// Register the hub with the node's event broadcaster so events from
	// connection/discovery managers are forwarded to WebSocket clients
	if broadcaster := node.GetEventBroadcaster(); broadcaster != nil {
		broadcaster.Subscribe(hub)
		log.Printf("WebSocket hub subscribed to event broadcaster")
	}

	// Type assert to concrete type for now - this is a temporary bridge
	concreteHub, ok := hub.(*websocket.Hub)
	if !ok {
		return nil, "", fmt.Errorf("failed to assert hub type")
	}

	managementServer := &ManagementServer{
		hub:  concreteHub,
		node: node,
	}

	// Create HTTP mux
	mux := http.NewServeMux()

	// Register handlers
	managementServer.registerRoutes(mux)

	// Store mux for dynamic mounts
	managementServer.mux = mux

	// Wrap mux with CORS middleware for WebUI cross-origin requests
	corsHandler := corsMiddleware(mux)

	// Listen on a random available port
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, "", fmt.Errorf("failed to create listener: %w", err)
	}

	managementServer.listener = listener
	managementServer.server = &http.Server{Handler: corsHandler}

	port := listener.Addr().(*net.TCPAddr).Port
	serverURL := fmt.Sprintf("http://localhost:%d", port)

	// Start server in background
	go func() {
		if err := managementServer.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("Management server error: %v", err)
		}
	}()

	log.Printf("Management API server started on port: %d", port)
	log.Printf("Server URL: %s", serverURL)
	log.Println("API Endpoints:")
	log.Println("  Basic:")
	log.Printf("    GET %s/health - Health check", serverURL)
	log.Printf("    ANY %s/api/test - API test endpoint", serverURL)
	log.Printf("    ANY %s/ - Echo endpoint", serverURL)
	log.Println("  Node Management:")
	log.Printf("    GET %s/node/status - Get node status", serverURL)
	log.Printf("    GET %s/node/ping - Ping node", serverURL)
	log.Printf("    POST %s/node/shutdown - Shutdown node (localhost only)", serverURL)
	log.Println("  Network Management:")
	log.Printf("    GET %s/network/connections - Get peer connections", serverURL)
	log.Printf("    POST %s/network/connect/{peerID} - Connect to peer", serverURL)
	log.Printf("    POST %s/network/peers/add - Add tracked peer (localhost only)", serverURL)
	log.Printf("    GET %s/network/peers/all - Get all peer information (localhost only)", serverURL)
	log.Println("  Service Management:")
	log.Printf("    POST %s/services/find - Find service by key/alias", serverURL)
	log.Printf("    GET %s/services/figs - List service figs", serverURL)
	log.Printf("    GET/POST %s/services/beacons/figs - Get signed figs for all active beacons", serverURL)
	log.Printf("    GET/POST %s/services/fig/{serviceKey} - Get signed fig for service key", serverURL)
	log.Printf("    GET %s/services/list - List configured services", serverURL)
	log.Printf("    POST %s/services/start - Start service beacon", serverURL)
	log.Printf("    POST %s/services/stop - Stop service beacon", serverURL)
	log.Printf("    GET %s/services/locators - List service locators", serverURL)
	log.Println("  Proxy:")
	log.Printf("    ANY %s/proxy/peer/{peerID}/{path} - Proxy via peer ID", serverURL)
	log.Printf("    ANY %s/proxy/alias/{alias}/{path} - Proxy via service alias", serverURL)
	log.Printf("    POST %s/proxy/service - Proxy via service key", serverURL)
	log.Println("  Router:")
	log.Printf("    POST %s/router/add - Add URL route mapping", serverURL)
	log.Printf("    GET %s/router/list - List all URL route mappings", serverURL)
	log.Println("  Events:")
	log.Printf("    WS  ws://localhost:%d/events/subscribe - Event subscription", port)
	log.Println("  Legacy endpoints redirect to new structure for backward compatibility")

	return managementServer, serverURL, nil
}

// registerRoutes sets up all HTTP routes with their respective handlers
func (ms *ManagementServer) registerRoutes(mux *http.ServeMux) {
	// Initialize handlers
	basicHandlers := handlers.NewBasicHandlers()
	libp2pHandlers := handlers.NewLibP2PHandlers(ms.node)
	proxyHandlers := handlers.NewProxyHandlers(ms.node, ms.BroadcastEvent)
	peerHandlers := handlers.NewPeerHandlers(ms.node)
	wsHandlers := handlers.NewWebSocketHandlers(ms.hub)
	serviceHandlers := handlers.NewServiceHandlers(ms.node, ms.BroadcastEvent)
	routerHandlers := handlers.NewRouterHandlers(ms.node)
	tunnelHandlers := handlers.NewTunnelHandlers(ms.node)

	// Basic endpoints
	mux.HandleFunc("/", basicHandlers.HandleRequest)
	mux.HandleFunc("/api/test", basicHandlers.HandleAPITest)
	mux.HandleFunc("/health", basicHandlers.HandleHealth)

	// Node management endpoints
	mux.HandleFunc("/node/status", libp2pHandlers.HandleStatus)
	mux.HandleFunc("/node/ping", libp2pHandlers.HandleStatus) // Reuse status handler for ping
	mux.HandleFunc("/node/shutdown", basicHandlers.HandleShutdown)

	// Store basicHandlers so we can set the shutdown func later
	ms.basicHandlers = basicHandlers

	// Network management endpoints
	mux.HandleFunc("/network/connections", libp2pHandlers.HandleConnections)
	mux.HandleFunc("/network/connect/", libp2pHandlers.HandleConnectToPeer)
	mux.HandleFunc("/network/connect-multiaddr", libp2pHandlers.HandleConnectWithMultiaddr)
	mux.HandleFunc("/network/peers/add", peerHandlers.HandleAddTrackedPeer)
	mux.HandleFunc("/network/peers/all", peerHandlers.HandleGetAllPeers)

	// Service management endpoints
	mux.HandleFunc("/services/find", serviceHandlers.HandleFind)
	mux.HandleFunc("/services/figs", serviceHandlers.HandleFigs)
	mux.HandleFunc("/services/figs/verbose", serviceHandlers.HandleFigs)
	mux.HandleFunc("/services/beacons/figs", serviceHandlers.HandleBeaconFigs)
	mux.HandleFunc("/services/fig/", serviceHandlers.HandleServiceFigByKey)
	mux.HandleFunc("/services/list", serviceHandlers.HandleServices)
	mux.HandleFunc("/services/list/verbose", serviceHandlers.HandleServices)
	mux.HandleFunc("/services/start", serviceHandlers.HandleServe)
	mux.HandleFunc("/services/stop", serviceHandlers.HandleBeaconKill)
	mux.HandleFunc("/services/locators", serviceHandlers.HandleLocators)
	mux.HandleFunc("/services/locator/start", serviceHandlers.HandleLocatorStart)
	mux.HandleFunc("/services/locator/suspend/", serviceHandlers.HandleLocatorSuspend)
	mux.HandleFunc("/services/locator/revive/", serviceHandlers.HandleLocatorRevive)
	mux.HandleFunc("/services/locator/destroy/", serviceHandlers.HandleLocatorDestroy)
	// Beacon endpoints - support both with and without service key hash
	mux.HandleFunc("/services/beacon/suspend/", serviceHandlers.HandleBeaconSuspend) // with hash
	mux.HandleFunc("/services/beacon/suspend", serviceHandlers.HandleBeaconSuspend)  // without hash (legacy)
	mux.HandleFunc("/services/beacon/kill/", serviceHandlers.HandleBeaconKill)       // with hash
	mux.HandleFunc("/services/beacon/kill", serviceHandlers.HandleBeaconKill)        // without hash (legacy)

	// Proxy endpoints
	mux.HandleFunc("/proxy/peer/", proxyHandlers.HandleLibp2pProxy)
	mux.HandleFunc("/proxy/alias/", proxyHandlers.HandleAliasProxy)
	mux.HandleFunc("/proxy/service/", proxyHandlers.HandleServiceKeyProxy)

	// Router management endpoints
	mux.HandleFunc("/router/add", routerHandlers.HandleAddRoute)
	mux.HandleFunc("/router/list", routerHandlers.HandleGetRoutes)

	// Tunnel management endpoints (for proxy addon and peer-to-peer TCP tunneling)
	mux.HandleFunc("/tunnel/open", tunnelHandlers.HandleOpenTunnel)
	mux.HandleFunc("/tunnel/close/", tunnelHandlers.HandleCloseTunnel)
	mux.HandleFunc("/tunnel/list", tunnelHandlers.HandleListTunnels)

	// WebSocket endpoint for event streaming
	mux.HandleFunc("/events/subscribe", wsHandlers.HandleEventSubscribe)

	// Backward compatibility redirects - old endpoints redirect to new ones
	ms.registerLegacyRedirects(mux)
}

// registerLegacyRedirects sets up HTTP redirects from old endpoints to new ones for backward compatibility
func (ms *ManagementServer) registerLegacyRedirects(mux *http.ServeMux) {
	// Create redirect handler function
	redirect := func(oldPath, newPath string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// Preserve the remaining path after the old prefix
			remainingPath := strings.TrimPrefix(r.URL.Path, oldPath)
			targetURL := newPath + remainingPath

			// Preserve query parameters
			if r.URL.RawQuery != "" {
				targetURL += "?" + r.URL.RawQuery
			}

			log.Printf("Redirecting deprecated endpoint %s to %s", r.URL.Path, targetURL)
			http.Redirect(w, r, targetURL, http.StatusMovedPermanently)
		}
	}

	// Register redirects for exact path matches
	mux.HandleFunc("/libp2p/status", redirect("/libp2p/status", "/node/status"))
	mux.HandleFunc("/libp2p/connections", redirect("/libp2p/connections", "/network/connections"))
	mux.HandleFunc("/find", redirect("/find", "/services/find"))
	mux.HandleFunc("/figs", redirect("/figs", "/services/figs"))
	mux.HandleFunc("/services", redirect("/services", "/services/list"))
	mux.HandleFunc("/serve", redirect("/serve", "/services/start"))
	mux.HandleFunc("/ping", redirect("/ping", "/node/ping"))
	mux.HandleFunc("/locators", redirect("/locators", "/services/locators"))

	// Register redirects for path prefixes
	mux.HandleFunc("/libp2p/connect-to-peer/", redirect("/libp2p/connect-to-peer/", "/network/connect/"))
	mux.HandleFunc("/libp2p/proxy/", redirect("/libp2p/proxy/", "/proxy/peer/"))
	mux.HandleFunc("/libp2p/peers/", redirect("/libp2p/peers/", "/network/peers/"))
	mux.HandleFunc("/out/", redirect("/out/", "/proxy/alias/"))
	mux.HandleFunc("/skout/", redirect("/skout/", "/proxy/service/"))
	mux.HandleFunc("/locator/", redirect("/locator/", "/services/locator/"))
	mux.HandleFunc("/beacon/", redirect("/beacon/", "/services/beacon/"))
}

// Close shuts down the management server
func (ms *ManagementServer) Close() error {
	if ms.server != nil {
		return ms.server.Shutdown(context.Background())
	}
	return nil
}

// BroadcastEvent sends an event to all connected WebSocket clients
func (ms *ManagementServer) BroadcastEvent(event types.Event) {
	ms.hub.BroadcastEvent(event)
}

// ProxyTarget returns the proxy target URL (for compatibility)
func (ms *ManagementServer) ProxyTarget() string {
	// Since we're now integrated, we don't really have a separate proxy target
	// Return empty string or a placeholder
	return "integrated"
}
