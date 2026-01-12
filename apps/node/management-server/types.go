package managementserver

import (
	"net"
	"net/http"

	"banyan/management-server/handlers"
	nodePkg "banyan/node"
	"banyan/websocket"
)

// ManagementServer represents the HTTP management/API server
type ManagementServer struct {
	server        *http.Server
	listener      net.Listener
	hub           *websocket.Hub
	node          *nodePkg.Node
	mux           *http.ServeMux
	basicHandlers *handlers.BasicHandlers
}

// GetNode returns the libp2p node instance
func (ms *ManagementServer) GetNode() *nodePkg.Node {
	return ms.node
}

// GetHub returns the WebSocket hub instance
func (ms *ManagementServer) GetHub() *websocket.Hub {
	return ms.hub
}

// Mount registers a handler on the management server mux.
func (ms *ManagementServer) Mount(path string, h func(http.ResponseWriter, *http.Request)) {
	if ms.mux != nil {
		ms.mux.HandleFunc(path, h)
	}
}

// SetShutdownFunc sets the function to call when shutdown is requested via API
func (ms *ManagementServer) SetShutdownFunc(fn func()) {
	if ms.basicHandlers != nil {
		ms.basicHandlers.SetShutdownFunc(fn)
	}
}
