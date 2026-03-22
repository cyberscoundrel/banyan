// Package websocket provides a WebSocket hub for real-time event broadcasting
// to connected clients.
//
// The hub maintains a registry of active WebSocket connections and provides
// a thread-safe mechanism for broadcasting events to all connected clients.
// It handles client registration, unregistration, and message distribution
// through channel-based communication.
//
// The package implements the interfaces.WebSocketHub interface and is designed
// to be used with the gorilla/websocket library for handling WebSocket
// connections in HTTP servers.
//
// Example usage:
//
//	hub := NewHub()
//	go hub.Run()
//	hub.BroadcastEvent(event)
package websocket

import (
	"fmt"
	"log"
	"sync"
	"time"

	"banyan/interfaces"
	"banyan/types"

	"github.com/gorilla/websocket"
)

// Client represents a connected WebSocket client.
// It wraps a gorilla/websocket connection and provides channels for
// sending events back to the client. Each client has a unique identifier
// and is managed by a Hub instance.
type Client struct {
	conn *websocket.Conn
	send chan types.Event
	hub  *Hub
	id   string
}

// Hub maintains the set of active WebSocket clients and broadcasts events to them.
// It provides thread-safe client management through channel-based operations,
// allowing concurrent registration, unregistration, and broadcasting.
// The Hub should be started with Run() before accepting connections.
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan types.Event
	register   chan *Client
	unregister chan *Client
	mutex      sync.RWMutex
}

// NewHub creates and returns a new WebSocket hub instance.
// The returned hub implements interfaces.WebSocketHub and must be
// started with Run() in a separate goroutine before use.
func NewHub() interfaces.WebSocketHub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan types.Event),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// Run starts the hub's main event loop. This method blocks indefinitely
// and should be invoked in a separate goroutine. It handles client
// registration, unregistration, and broadcasts events to all connected clients.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mutex.Lock()
			h.clients[client] = true
			h.mutex.Unlock()
			log.Printf("WebSocket client connected: %s", client.id)

		case client := <-h.unregister:
			h.mutex.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mutex.Unlock()
			log.Printf("WebSocket client disconnected: %s", client.id)

		case event := <-h.broadcast:
			h.mutex.RLock()
			for client := range h.clients {
				select {
				case client.send <- event:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mutex.RUnlock()
		}
	}
}

// BroadcastEvent queues an event to be sent to all connected WebSocket clients.
// The event is sent asynchronously through the hub's broadcast channel.
func (h *Hub) BroadcastEvent(event types.Event) {
	h.broadcast <- event
}

// NewClient creates a new Client from a WebSocket connection.
// The connection must be a *gorilla/websocket.Conn. The client is assigned
// a unique identifier but is not automatically registered with the hub.
func (h *Hub) NewClient(conn interface{}) interfaces.WebSocketClient {
	wsConn := conn.(*websocket.Conn)
	clientID := fmt.Sprintf("client-%d", time.Now().UnixNano())
	client := &Client{
		conn: wsConn,
		send: make(chan types.Event, 256),
		hub:  h,
		id:   clientID,
	}
	return client
}

// Register adds a client to the hub for event broadcasting.
// The client will receive all events broadcast to the hub until unregistered.
func (h *Hub) Register(client interfaces.WebSocketClient) {
	h.register <- client.(*Client)
}

// Unregister removes a client from the hub and closes its send channel.
// After unregistration, the client will no longer receive broadcast events.
func (h *Hub) Unregister(client interfaces.WebSocketClient) {
	h.unregister <- client.(*Client)
}

// GetID returns the unique identifier assigned to this client.
func (c *Client) GetID() string {
	return c.id
}

// GetSendChannel returns the channel used to send events to this client.
// Events written to this channel will be forwarded to the WebSocket connection.
func (c *Client) GetSendChannel() chan types.Event {
	return c.send
}

// GetConnection returns the underlying WebSocket connection as an interface{}.
// The caller should type assert to *gorilla/websocket.Conn for usage.
func (c *Client) GetConnection() interface{} {
	return c.conn
}
