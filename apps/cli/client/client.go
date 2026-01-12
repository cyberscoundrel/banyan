package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Client is the API client for communicating with a Banyan node
type Client struct {
	baseURL    string
	httpClient *http.Client
	wsConn     *websocket.Conn
	wsMu       sync.Mutex
	events     chan Event
	done       chan struct{}
	connected  bool
}

// New creates a new API client
func New(baseURL string) *Client {
	// Ensure baseURL doesn't have trailing slash
	baseURL = strings.TrimSuffix(baseURL, "/")

	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		events: make(chan Event, 100),
		done:   make(chan struct{}),
	}
}

// BaseURL returns the base URL of the client
func (c *Client) BaseURL() string {
	return c.baseURL
}

// IsConnected returns whether the client is connected to the websocket
func (c *Client) IsConnected() bool {
	c.wsMu.Lock()
	defer c.wsMu.Unlock()
	return c.connected
}

// Events returns the events channel
func (c *Client) Events() <-chan Event {
	return c.events
}

// Close closes the client connections
func (c *Client) Close() {
	close(c.done)
	c.wsMu.Lock()
	if c.wsConn != nil {
		c.wsConn.Close()
		c.connected = false
	}
	c.wsMu.Unlock()
}

// doRequest performs an HTTP request and returns the response body
func (c *Client) doRequest(method, path string, body interface{}) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// Health checks the node health
func (c *Client) Health() (*HealthResponse, error) {
	body, err := c.doRequest("GET", "/health", nil)
	if err != nil {
		return nil, err
	}
	// Use a more lenient parsing that accepts any JSON with status field
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse health response: %w (body: %s)", err, string(body))
	}
	status, _ := raw["status"].(string)
	return &HealthResponse{Status: status}, nil
}

// GetStatus gets the node status
func (c *Client) GetStatus() (*NodeStatus, error) {
	body, err := c.doRequest("GET", "/node/status", nil)
	if err != nil {
		return nil, err
	}
	// Use lenient parsing to handle timestamp format differences
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse status: %w (body: %s)", err, string(body))
	}

	status := &NodeStatus{}
	if v, ok := raw["node_id"].(string); ok {
		status.NodeID = v
	}
	if v, ok := raw["total_connected_peers"].(float64); ok {
		status.TotalConnectedPeers = int(v)
	}
	if v, ok := raw["tracked_peers"].(float64); ok {
		status.TrackedPeers = int(v)
	}
	if v, ok := raw["http_capable_peers"].(float64); ok {
		status.HTTPCapablePeers = int(v)
	}
	if v, ok := raw["bidirectional_http_peers"].(float64); ok {
		status.BidirectionalPeers = int(v)
	}
	if v, ok := raw["connection_types"].(map[string]interface{}); ok {
		status.ConnectionTypes = make(map[string]int)
		for k, val := range v {
			if n, ok := val.(float64); ok {
				status.ConnectionTypes[k] = int(n)
			}
		}
	}
	if v, ok := raw["listening_addrs"].([]interface{}); ok {
		status.ListeningAddrs = v
	}
	if v, ok := raw["dht_enabled"].(bool); ok {
		status.DHTEnabled = v
	}
	if v, ok := raw["discovery_methods"].([]interface{}); ok {
		for _, m := range v {
			if s, ok := m.(string); ok {
				status.DiscoveryMethods = append(status.DiscoveryMethods, s)
			}
		}
	}
	return status, nil
}

// GetConnections gets all peer connections
func (c *Client) GetConnections() ([]PeerConnection, error) {
	body, err := c.doRequest("GET", "/network/connections", nil)
	if err != nil {
		return nil, err
	}

	// The node returns a wrapper object with "connections" array
	var wrapper struct {
		TotalTracked int                      `json:"total_tracked"`
		Connections  []map[string]interface{} `json:"connections"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		// Try parsing as flat array for backward compatibility
		var raw []map[string]interface{}
		if err2 := json.Unmarshal(body, &raw); err2 != nil {
			return nil, fmt.Errorf("failed to parse connections: %w (body: %.500s)", err, string(body))
		}
		wrapper.Connections = raw
	}

	var connections []PeerConnection
	for _, r := range wrapper.Connections {
		conn := PeerConnection{}
		if v, ok := r["peer_id"].(string); ok {
			conn.PeerID = v
		}
		if v, ok := r["alias"].(string); ok {
			conn.Alias = v
		}
		if v, ok := r["status"].(string); ok {
			conn.Status = v
		}
		if v, ok := r["connection_type"].(string); ok {
			conn.ConnectionType = v
		}
		if v, ok := r["http_capable"].(bool); ok {
			conn.HTTPCapable = v
		}
		if v, ok := r["http_test_result"].(string); ok {
			conn.HTTPTest = v
		} else if v, ok := r["http_test"].(string); ok {
			conn.HTTPTest = v
		}
		if v, ok := r["connected"].(bool); ok {
			conn.Connected = v
		}
		if v, ok := r["libp2p_connected"].(bool); ok {
			conn.Connected = v
		}
		connections = append(connections, conn)
	}
	return connections, nil
}

// Shutdown requests the node to shut down gracefully
// This only works when the client is connecting to localhost
func (c *Client) Shutdown() error {
	resp, err := c.httpClient.Post(c.baseURL+"/node/shutdown", "application/json", nil)
	if err != nil {
		return fmt.Errorf("shutdown request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("shutdown can only be initiated from localhost")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("shutdown failed: %s", string(body))
	}

	return nil
}
