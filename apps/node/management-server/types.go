package managementserver

import (
	"net"
	"net/http"

	"banyan/management-server/handlers"
	nodePkg "banyan/node"
	"banyan/websocket"
)

// ManagementServer represents the HTTP management and API server for a Banyan node.
// It provides REST endpoints for node management, peer connections, service discovery,
// proxying, tunneling, and WebSocket-based event streaming.
type ManagementServer struct {
	server        *http.Server
	listener      net.Listener
	hub           *websocket.Hub
	node          *nodePkg.Node
	mux           *http.ServeMux
	basicHandlers *handlers.BasicHandlers
}

// GetNode returns the Banyan node instance associated with this management server.
func (ms *ManagementServer) GetNode() *nodePkg.Node {
	return ms.node
}

// GetHub returns the WebSocket hub used for broadcasting real-time events
// to connected WebSocket clients.
func (ms *ManagementServer) GetHub() *websocket.Hub {
	return ms.hub
}

// Mount registers a custom HTTP handler on the management server's mux.
// This allows external packages to add their own endpoints to the API.
func (ms *ManagementServer) Mount(path string, h func(http.ResponseWriter, *http.Request)) {
	if ms.mux != nil {
		ms.mux.HandleFunc(path, h)
	}
}

// SetShutdownFunc sets the callback function to invoke when a shutdown request
// is received via the /node/shutdown endpoint. This endpoint is restricted to
// localhost for security.
func (ms *ManagementServer) SetShutdownFunc(fn func()) {
	if ms.basicHandlers != nil {
		ms.basicHandlers.SetShutdownFunc(fn)
	}
}
