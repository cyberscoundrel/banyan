package client

import (
	"encoding/json"
	"time"
)

// readEvents continuously reads events from the WebSocket connection
func (c *Client) readEvents() {
	defer func() {
		c.wsMu.Lock()
		c.connected = false
		if c.wsConn != nil {
			c.wsConn.Close()
			c.wsConn = nil
		}
		c.wsMu.Unlock()

		// Send a disconnect event so listeners know the connection is gone
		select {
		case c.events <- Event{Type: "websocket_disconnected"}:
		default:
		}
	}()

	for {
		select {
		case <-c.done:
			return
		default:
			c.wsMu.Lock()
			conn := c.wsConn
			c.wsMu.Unlock()

			if conn == nil {
				return
			}

			_, message, err := conn.ReadMessage()
			if err != nil {
				// Send error event for debugging
				select {
				case c.events <- Event{Type: "websocket_error", Data: map[string]string{"error": err.Error()}}:
				default:
				}
				return
			}

			var event Event
			if err := json.Unmarshal(message, &event); err != nil {
				// Send parse error event for debugging with the raw message
				errEvent := Event{
					Type: "parse_error",
					Data: map[string]string{
						"error":   err.Error(),
						"message": string(message),
					},
				}
				select {
				case c.events <- errEvent:
				default:
				}
				continue
			}

			select {
			case c.events <- event:
			default:
				// Channel full, drop old event
				select {
				case <-c.events:
				default:
				}
				c.events <- event
			}
		}
	}
}

// ReconnectWebSocket attempts to reconnect the WebSocket
func (c *Client) ReconnectWebSocket() error {
	c.wsMu.Lock()
	if c.wsConn != nil {
		c.wsConn.Close()
		c.wsConn = nil
		c.connected = false
	}
	c.wsMu.Unlock()

	// Wait a bit before reconnecting
	time.Sleep(time.Second)

	return c.SubscribeEvents()
}

// GetRoutes gets configured routes
func (c *Client) GetRoutes() ([]Route, error) {
	body, err := c.doRequest("GET", "/router/list", nil)
	if err != nil {
		return nil, err
	}
	var routes []Route
	if err := json.Unmarshal(body, &routes); err != nil {
		return nil, err
	}
	return routes, nil
}

// AddRoute adds a new route
func (c *Client) AddRoute(req RouteAddRequest) error {
	_, err := c.doRequest("POST", "/router/add", req)
	return err
}

// GetLocators gets service locators
func (c *Client) GetLocators() (map[string]interface{}, error) {
	body, err := c.doRequest("GET", "/services/locators", nil)
	if err != nil {
		return nil, err
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// StartLocator starts a service locator
func (c *Client) StartLocator(serviceKey string) (map[string]interface{}, error) {
	body, err := c.doRequest("POST", "/services/locator/start", map[string]string{"serviceKey": serviceKey})
	if err != nil {
		return nil, err
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}
