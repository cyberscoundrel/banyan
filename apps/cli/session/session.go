// Package session provides shared instance state between CLI TUI and WebUI
package session

import (
	"encoding/json"
	"sync"

	"github.com/gorilla/websocket"
)

// InstanceInfo represents the connection state of a node instance (no data, just metadata)
type InstanceInfo struct {
	Index      int    `json:"index"`
	Address    string `json:"address"`
	Connected  bool   `json:"connected"`
	Active     bool   `json:"active"`
	Subprocess bool   `json:"subprocess"`
	Name       string `json:"name,omitempty"`
}

// LoggedInstanceInfo represents a logged instance (historical record)
type LoggedInstanceInfo struct {
	Index     int    `json:"index"`
	Address   string `json:"address"`
	Name      string `json:"name,omitempty"`
	LastSeen  string `json:"lastSeen,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
}

// StateMessage is sent to WebUI clients when CLI instance state changes
type StateMessage struct {
	Type            string               `json:"type"` // "instances"
	Instances       []InstanceInfo       `json:"instances"`
	LoggedInstances []LoggedInstanceInfo `json:"loggedInstances,omitempty"`
}

// Command represents an action from WebUI to TUI
type Command struct {
	Action  string // "select", "stop", "connect"
	Index   int    // For select/stop
	Address string // For connect
}

// CommandResult represents the result of a command
type CommandResult struct {
	Success bool
	Message string
	Removed bool // For select - if disconnected instance was removed
	Address string
}

// Session manages shared instance state between TUI and WebUI
type Session struct {
	mu sync.RWMutex

	// Current instance state from CLI
	instances       []InstanceInfo
	loggedInstances []LoggedInstanceInfo

	// WebSocket clients subscribed to state updates
	clients   map[*websocket.Conn]bool
	clientsMu sync.RWMutex

	// Command channel for WebUI -> TUI commands
	cmdChan    chan Command
	resultChan chan CommandResult
}

// Global session instance
var globalSession *Session
var once sync.Once

// Get returns the global session instance
func Get() *Session {
	once.Do(func() {
		globalSession = &Session{
			clients:         make(map[*websocket.Conn]bool),
			instances:       make([]InstanceInfo, 0),
			loggedInstances: make([]LoggedInstanceInfo, 0),
			cmdChan:         make(chan Command, 10),
			resultChan:      make(chan CommandResult, 10),
		}
	})
	return globalSession
}

// SendCommand sends a command to the TUI and waits for result
func (s *Session) SendCommand(cmd Command) CommandResult {
	s.cmdChan <- cmd
	return <-s.resultChan
}

// CommandChan returns the command channel for TUI to listen on
func (s *Session) CommandChan() <-chan Command {
	return s.cmdChan
}

// SendResult sends a command result back to the WebUI handler
func (s *Session) SendResult(result CommandResult) {
	s.resultChan <- result
}

// UpdateInstances updates the instance state and broadcasts to all WebUI clients
func (s *Session) UpdateInstances(instances []InstanceInfo) {
	s.mu.Lock()
	s.instances = instances
	s.mu.Unlock()

	s.broadcast()
}

// UpdateLoggedInstances updates the logged instances state and broadcasts to all WebUI clients
func (s *Session) UpdateLoggedInstances(loggedInstances []LoggedInstanceInfo) {
	s.mu.Lock()
	s.loggedInstances = loggedInstances
	s.mu.Unlock()

	s.broadcast()
}

// GetInstances returns the current instance state
func (s *Session) GetInstances() []InstanceInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return a copy
	result := make([]InstanceInfo, len(s.instances))
	copy(result, s.instances)
	return result
}

// GetLoggedInstances returns the current logged instances state
func (s *Session) GetLoggedInstances() []LoggedInstanceInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return a copy
	result := make([]LoggedInstanceInfo, len(s.loggedInstances))
	copy(result, s.loggedInstances)
	return result
}

// AddClient adds a WebSocket client and sends current state
func (s *Session) AddClient(conn *websocket.Conn) {
	s.clientsMu.Lock()
	s.clients[conn] = true
	s.clientsMu.Unlock()

	// Send current state immediately
	s.sendTo(conn)
}

// RemoveClient removes a WebSocket client
func (s *Session) RemoveClient(conn *websocket.Conn) {
	s.clientsMu.Lock()
	delete(s.clients, conn)
	s.clientsMu.Unlock()
}

// broadcast sends current state to all connected WebUI clients
func (s *Session) broadcast() {
	s.mu.RLock()
	msg := StateMessage{
		Type:            "instances",
		Instances:       s.instances,
		LoggedInstances: s.loggedInstances,
	}
	s.mu.RUnlock()

	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()

	for conn := range s.clients {
		conn.WriteMessage(websocket.TextMessage, data)
	}
}

// sendTo sends current state to a specific client
func (s *Session) sendTo(conn *websocket.Conn) {
	s.mu.RLock()
	msg := StateMessage{
		Type:            "instances",
		Instances:       s.instances,
		LoggedInstances: s.loggedInstances,
	}
	s.mu.RUnlock()

	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	conn.WriteMessage(websocket.TextMessage, data)
}

// ClientCount returns the number of connected WebUI clients
func (s *Session) ClientCount() int {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()
	return len(s.clients)
}
