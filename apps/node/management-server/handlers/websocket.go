package handlers

import (
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	wsHub "banyan/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for development
	},
}

// WebSocketHandlers provides HTTP endpoint handlers for WebSocket connections
// that enable real-time event streaming to clients.
type WebSocketHandlers struct {
	hub *wsHub.Hub
}

// NewWebSocketHandlers creates a new WebSocketHandlers instance
func NewWebSocketHandlers(hub *wsHub.Hub) *WebSocketHandlers {
	return &WebSocketHandlers{
		hub: hub,
	}
}

// HandleEventSubscribe handles the /events/subscribe WebSocket endpoint for
// clients to subscribe to real-time events from the node including peer connections,
// service discoveries, and proxy requests.
func (wh *WebSocketHandlers) HandleEventSubscribe(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade WebSocket connection: %v", err)
		return
	}

	client := wh.hub.NewClient(conn)
	wh.hub.Register(client)

	// Handle the client connection
	go func() {
		// Type assert the connection to *websocket.Conn
		wsConn, ok := client.GetConnection().(*websocket.Conn)
		if !ok {
			log.Printf("Invalid websocket connection type")
			return
		}

		defer func() {
			wh.hub.Unregister(client)
			wsConn.Close()
		}()

		// Send events to client
		for event := range client.GetSendChannel() {
			wsConn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := wsConn.WriteJSON(event); err != nil {
				log.Printf("Failed to send event to client %s: %v", client.GetID(), err)
				return
			}
		}
		wsConn.WriteMessage(websocket.CloseMessage, []byte{})
	}()

	// Keep connection alive and handle close
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}
	}
}
