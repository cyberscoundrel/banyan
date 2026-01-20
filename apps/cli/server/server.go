package server

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"sync"
	"time"

	"banyan-cli/client"
	"banyan-cli/config"
	"banyan-cli/nodeproc"
	"banyan-cli/session"

	"github.com/gorilla/websocket"
)

//go:embed web/dist/*
var webFS embed.FS

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for local development
	},
}

// NodeInstance represents a connected or disconnected node instance
type NodeInstance struct {
	Client     *client.Client
	Address    string
	Connected  bool
	NodeProc   *nodeproc.NodeProcess
	KillOnExit bool
	Name       string
}

// Server represents the web server for the CLI dashboard
type Server struct {
	config *config.Config
	port   int
	server *http.Server

	// Multi-instance support
	instances         []*NodeInstance
	activeInstanceIdx int
	instancesMu       sync.RWMutex

	// WebSocket clients for event broadcasting
	wsClients   map[*websocket.Conn]bool
	wsClientsMu sync.RWMutex

	// Event channel
	events chan client.Event
}

// New creates a new web server
func New(cfg *config.Config, port int) *Server {
	return &Server{
		config:            cfg,
		port:              port,
		instances:         make([]*NodeInstance, 0),
		activeInstanceIdx: -1,
		wsClients:         make(map[*websocket.Conn]bool),
		events:            make(chan client.Event, 100),
	}
}

// ActiveInstance returns the currently active instance, or nil
func (s *Server) ActiveInstance() *NodeInstance {
	s.instancesMu.RLock()
	defer s.instancesMu.RUnlock()
	if s.activeInstanceIdx < 0 || s.activeInstanceIdx >= len(s.instances) {
		return nil
	}
	return s.instances[s.activeInstanceIdx]
}

// ActiveClient returns the client for the active instance, or nil
func (s *Server) ActiveClient() *client.Client {
	inst := s.ActiveInstance()
	if inst == nil || !inst.Connected {
		return nil
	}
	return inst.Client
}

// AddInstance adds a new instance and returns its index
func (s *Server) AddInstance(inst *NodeInstance) int {
	s.instancesMu.Lock()
	s.instances = append(s.instances, inst)
	idx := len(s.instances) - 1
	s.instancesMu.Unlock()
	// Sync to session for instances API
	s.SyncToSession()
	return idx
}

// SyncToSession synchronizes the server's instance state to the global session
// This ensures the instances API endpoint returns consistent data
func (s *Server) SyncToSession() {
	s.instancesMu.RLock()
	defer s.instancesMu.RUnlock()

	instances := make([]session.InstanceInfo, 0, len(s.instances))
	for i, inst := range s.instances {
		instances = append(instances, session.InstanceInfo{
			Index:      i + 1, // 1-based for display
			Address:    inst.Address,
			Connected:  inst.Connected,
			Active:     i == s.activeInstanceIdx,
			Subprocess: inst.KillOnExit,
			Name:       inst.Name,
		})
	}
	session.Get().UpdateInstances(instances)
}

// SaveInstancesToConfig saves the current instances to the config file
func (s *Server) SaveInstancesToConfig() {
	if s.config == nil {
		return
	}
	s.instancesMu.RLock()
	defer s.instancesMu.RUnlock()

	saved := make([]config.SavedInstance, 0, len(s.instances))
	for _, inst := range s.instances {
		// Only save non-subprocess instances
		if !inst.KillOnExit {
			saved = append(saved, config.SavedInstance{
				Address: inst.Address,
				Name:    inst.Name,
			})
		}
	}
	s.config.SaveInstances(saved, s.activeInstanceIdx)
}

// SetClient sets the API client (for backwards compatibility)
func (s *Server) SetClient(c *client.Client) {
	inst := &NodeInstance{
		Client:    c,
		Address:   c.BaseURL(),
		Connected: true,
	}
	idx := s.AddInstance(inst)
	s.instancesMu.Lock()
	s.activeInstanceIdx = idx
	s.instancesMu.Unlock()
	// Start forwarding events from client to web clients
	go s.forwardEventsFromClient(c)
}

// SetNodeProcess sets the node process manager (for backwards compatibility)
func (s *Server) SetNodeProcess(np *nodeproc.NodeProcess) {
	inst := s.ActiveInstance()
	if inst != nil {
		inst.NodeProc = np
	}
}

// Start starts the web server
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/peers", s.handlePeers)
	mux.HandleFunc("/api/connect", s.handleConnect)
	mux.HandleFunc("/api/services", s.handleServices)
	mux.HandleFunc("/api/figs", s.handleFigs)
	mux.HandleFunc("/api/find", s.handleFind)
	mux.HandleFunc("/api/serve/start", s.handleServeStart)
	mux.HandleFunc("/api/serve/stop", s.handleServeStop)
	mux.HandleFunc("/api/peer/connect", s.handlePeerConnect)
	mux.HandleFunc("/api/peer/add", s.handlePeerAdd)
	mux.HandleFunc("/api/proxy", s.handleProxy)
	mux.HandleFunc("/api/routes", s.handleRoutes)
	mux.HandleFunc("/api/node/start", s.handleNodeStart)
	mux.HandleFunc("/api/node/stop", s.handleNodeStop)
	mux.HandleFunc("/api/execute", s.handleExecuteCommand)

	// Instance management routes
	mux.HandleFunc("/api/instances", s.handleInstances)
	mux.HandleFunc("/api/instances/select", s.handleSelectInstance)
	mux.HandleFunc("/api/instances/stop", s.handleStopInstance)

	// WebSocket for events from nodes
	mux.HandleFunc("/api/events", s.handleWebSocket)

	// WebSocket for CLI session instance state sync
	mux.HandleFunc("/api/session", s.handleSessionSync)

	// Serve static web files
	webDist, err := fs.Sub(webFS, "web/dist")
	if err != nil {
		// If embedded files don't exist, serve a placeholder
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<!DOCTYPE html>
<html><head><title>Banyan CLI</title></head>
<body><h1>Banyan CLI Web UI</h1>
<p>Web UI not built. Run 'npm run build' in apps/cli/web</p></body></html>`))
		})
	} else {
		fileServer := http.FileServer(http.FS(webDist))
		mux.Handle("/", fileServer)
	}

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: mux,
	}

	log.Printf("Starting web server on http://localhost:%d", s.port)

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Web server error: %v", err)
		}
	}()

	// Start background health checker
	go s.healthCheckLoop()

	return nil
}

// healthCheckLoop periodically checks the health of all instances
func (s *Server) healthCheckLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		s.checkAllInstanceHealth()
	}
}

// checkAllInstanceHealth checks if all instances are still reachable
func (s *Server) checkAllInstanceHealth() {
	s.instancesMu.Lock()
	defer s.instancesMu.Unlock()

	for i, inst := range s.instances {
		if inst.Client == nil {
			continue
		}

		_, err := inst.Client.Health()
		wasConnected := inst.Connected
		inst.Connected = (err == nil)

		// Log state changes
		if wasConnected && !inst.Connected {
			log.Printf("Instance %d (%s) is now unreachable", i+1, inst.Address)
		} else if !wasConnected && inst.Connected {
			log.Printf("Instance %d (%s) is now reachable", i+1, inst.Address)
			// Reconnect websocket
			inst.Client.SubscribeEvents()
			go s.forwardEventsFromClient(inst.Client)
		}
	}
}

// Stop stops the web server
func (s *Server) Stop() error {
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.server.Shutdown(ctx)
	}
	return nil
}

// forwardEventsFromClient forwards events from a specific client to web socket clients
func (s *Server) forwardEventsFromClient(c *client.Client) {
	if c == nil {
		return
	}
	for event := range c.Events() {
		s.broadcastEvent(event)
	}
}

// broadcastEvent sends an event to all connected WebSocket clients
func (s *Server) broadcastEvent(event client.Event) {
	s.wsClientsMu.RLock()
	defer s.wsClientsMu.RUnlock()

	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	for conn := range s.wsClients {
		conn.WriteMessage(websocket.TextMessage, data)
	}
}

// BroadcastEvent allows external code to broadcast events
func (s *Server) BroadcastEvent(event client.Event) {
	s.broadcastEvent(event)
}
