package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
)

// ConnectToPeer connects to a peer by ID or alias
func (c *Client) ConnectToPeer(peerIDOrAlias string) error {
	_, err := c.doRequest("POST", "/network/connect/"+peerIDOrAlias, nil)
	return err
}

// AddTrackedPeer adds a peer to tracking
func (c *Client) AddTrackedPeer(req AddPeerRequest) (map[string]interface{}, error) {
	body, err := c.doRequest("POST", "/network/peers/add", req)
	if err != nil {
		return nil, err
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return resp, nil
}

// GetAllPeers gets all peer information
func (c *Client) GetAllPeers() (map[string]interface{}, error) {
	body, err := c.doRequest("GET", "/network/peers/all", nil)
	if err != nil {
		return nil, err
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return resp, nil
}

// FindService finds a service by key or alias
func (c *Client) FindService(req FindRequest) (map[string]interface{}, error) {
	body, err := c.doRequest("POST", "/services/find", req)
	if err != nil {
		return nil, err
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return resp, nil
}

// GetServiceFigs gets service figs
func (c *Client) GetServiceFigs() ([]ServiceFig, error) {
	body, err := c.doRequest("GET", "/services/figs", nil)
	if err != nil {
		return nil, err
	}
	var figs []ServiceFig
	if err := json.Unmarshal(body, &figs); err != nil {
		return nil, fmt.Errorf("failed to parse figs: %w", err)
	}
	return figs, nil
}

// ListServices lists configured services
func (c *Client) ListServices() (map[string]interface{}, error) {
	body, err := c.doRequest("GET", "/services/list", nil)
	if err != nil {
		return nil, err
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return resp, nil
}

// StartServiceBeacon starts a service beacon
func (c *Client) StartServiceBeacon(req ServiceBeaconRequest) (map[string]interface{}, error) {
	body, err := c.doRequest("POST", "/services/start", req)
	if err != nil {
		return nil, err
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return resp, nil
}

// StopServiceBeacon stops a service beacon
func (c *Client) StopServiceBeacon(serviceKeyHash string) error {
	path := "/services/stop"
	if serviceKeyHash != "" {
		path = "/services/beacon/kill/" + serviceKeyHash
	}
	_, err := c.doRequest("POST", path, nil)
	return err
}

// ProxyPeerRequest sends a request through a peer proxy
func (c *Client) ProxyPeerRequest(peerID, targetPath, method string, body io.Reader) ([]byte, int, error) {
	url := c.baseURL + "/proxy/peer/" + peerID + "/" + strings.TrimPrefix(targetPath, "/")
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}
	
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("proxy request failed: %w", err)
	}
	defer resp.Body.Close()
	
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response: %w", err)
	}
	
	return respBody, resp.StatusCode, nil
}

// ProxyServiceRequest sends a request through a service key proxy
func (c *Client) ProxyServiceRequest(serviceKey, targetPath, method string, body io.Reader) ([]byte, int, error) {
	url := c.baseURL + "/proxy/service/" + serviceKey + "/" + strings.TrimPrefix(targetPath, "/")
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}
	
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("proxy request failed: %w", err)
	}
	defer resp.Body.Close()
	
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response: %w", err)
	}
	
	return respBody, resp.StatusCode, nil
}

// SubscribeEvents subscribes to WebSocket events
func (c *Client) SubscribeEvents() error {
	wsURL := strings.Replace(c.baseURL, "http://", "ws://", 1)
	wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
	wsURL = wsURL + "/events/subscribe"

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to websocket: %w", err)
	}

	c.wsMu.Lock()
	c.wsConn = conn
	c.connected = true
	c.wsMu.Unlock()

	// Start reading events
	go c.readEvents()

	return nil
}

