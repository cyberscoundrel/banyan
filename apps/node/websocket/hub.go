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

// Client represents a WebSocket client connection
type Client struct {
	conn *websocket.Conn
	send chan types.Event
	hub  *Hub
	id   string
}

// Hub maintains the set of active clients and broadcasts events to them
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan types.Event
	register   chan *Client
	unregister chan *Client
	mutex      sync.RWMutex
}

// NewHub creates a new WebSocket hub
func NewHub() interfaces.WebSocketHub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan types.Event),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// Run starts the hub's main loop for managing clients and broadcasting events
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

// BroadcastEvent sends an event to all connected WebSocket clients
func (h *Hub) BroadcastEvent(event types.Event) {
	h.broadcast <- event
}

// NewClient creates a new WebSocket client
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

// Register registers a client with the hub
func (h *Hub) Register(client interfaces.WebSocketClient) {
	h.register <- client.(*Client)
}

// Unregister unregisters a client from the hub
func (h *Hub) Unregister(client interfaces.WebSocketClient) {
	h.unregister <- client.(*Client)
}

// GetID returns the client's ID
func (c *Client) GetID() string {
	return c.id
}

// GetSendChannel returns the client's send channel
func (c *Client) GetSendChannel() chan types.Event {
	return c.send
}

// GetConnection returns the client's WebSocket connection
func (c *Client) GetConnection() interface{} {
	return c.conn
}
