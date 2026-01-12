package client

import "time"

// NodeStatus represents the status of a node
type NodeStatus struct {
	NodeID               string                 `json:"node_id"`
	TotalConnectedPeers  int                    `json:"total_connected_peers"`
	TrackedPeers         int                    `json:"tracked_peers"`
	HTTPCapablePeers     int                    `json:"http_capable_peers"`
	BidirectionalPeers   int                    `json:"bidirectional_http_peers"`
	ConnectionTypes      map[string]int         `json:"connection_types"`
	ListeningAddrs       []interface{}          `json:"listening_addrs"`
	DHTEnabled           bool                   `json:"dht_enabled"`
	DiscoveryMethods     []string               `json:"discovery_methods"`
	Timestamp            time.Time              `json:"timestamp"`
}

// PeerConnection represents a peer connection
type PeerConnection struct {
	PeerID         string    `json:"peer_id"`
	Alias          string    `json:"alias"`
	Status         string    `json:"status"`
	ConnectionType string    `json:"connection_type"`
	HTTPCapable    bool      `json:"http_capable"`
	HTTPTest       string    `json:"http_test"`
	Connected      bool      `json:"connected"`
	LastActivity   time.Time `json:"last_activity"`
}

// Event represents a WebSocket event from the node
type Event struct {
	Type      string      `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data"`
	NodeID    string      `json:"node_id"`
}

// AddPeerRequest represents a request to add a tracked peer
type AddPeerRequest struct {
	PeerID      string   `json:"peer_id"`
	Addresses   []string `json:"addresses"`
	HTTPCapable bool     `json:"http_capable"`
	Connect     bool     `json:"connect"`
}

// ServiceFig represents a service fig
type ServiceFig struct {
	ServiceAlias string `json:"serviceAlias"`
	ServiceKey   string `json:"service_key"`
}

// FindRequest represents a request to find a service
type FindRequest struct {
	ServiceKey string `json:"serviceKey,omitempty"`
	Alias      string `json:"alias,omitempty"`
}

// ProxyResponse represents a proxy response
type ProxyResponse struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
}

// ServiceBeaconRequest represents a request to start a service beacon
type ServiceBeaconRequest struct {
	FileLocation string `json:"file_location"`
}

// HealthResponse represents a health check response
type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
}

// RouteAddRequest represents a request to add a route
type RouteAddRequest struct {
	Path    string `json:"path"`
	Target  string `json:"target"`
	Methods []string `json:"methods,omitempty"`
}

// Route represents a configured route
type Route struct {
	Path    string   `json:"path"`
	Target  string   `json:"target"`
	Methods []string `json:"methods"`
}

