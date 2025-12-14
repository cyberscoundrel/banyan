package managementserver

import (
	"net"
	"net/http"

	nodePkg "banyan/node"
	"banyan/websocket"
)

// ManagementServer represents the HTTP management/API server
type ManagementServer struct {
	server   *http.Server
	listener net.Listener
	hub      *websocket.Hub
	node     *nodePkg.Node
	mux      *http.ServeMux
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
