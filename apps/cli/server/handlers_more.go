package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"banyan-cli/client"
	"banyan-cli/session"
)

func (s *Server) handleNodeStop(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	inst := s.ActiveInstance()
	if inst == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "no instance selected",
		})
		return
	}

	// Try to stop node process if available
	if inst.NodeProc != nil {
		if err := inst.NodeProc.Stop(); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		inst.NodeProc = nil
	} else if inst.Client != nil {
		// Try remote shutdown
		if err := inst.Client.Shutdown(); err != nil {
			// Fall back to just disconnecting
			inst.Client.Close()
			inst.Connected = false
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"message": "disconnected (remote shutdown not available)",
			})
			return
		}
	}

	if inst.Client != nil {
		inst.Client.Close()
	}
	inst.Connected = false
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	s.wsClientsMu.Lock()
	s.wsClients[conn] = true
	s.wsClientsMu.Unlock()

	defer func() {
		s.wsClientsMu.Lock()
		delete(s.wsClients, conn)
		s.wsClientsMu.Unlock()
		conn.Close()
	}()

	// Keep connection alive and handle pings
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// handleSessionSync provides WebSocket for CLI instance state sync to WebUI
func (s *Server) handleSessionSync(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	// Register with global session - this sends current state immediately
	sess := session.Get()
	sess.AddClient(conn)

	defer func() {
		sess.RemoveClient(conn)
		conn.Close()
	}()

	// Keep connection alive
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (s *Server) handleExecuteCommand(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Command string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	result, err := s.executeCommand(req.Command)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"result":  result,
	})
}

// executeCommand executes a CLI command and returns the result
func (s *Server) executeCommand(input string) (string, error) {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return "", nil
	}

	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	c := s.ActiveClient()

	switch cmd {
	case "status":
		if c == nil {
			return "Not connected", nil
		}
		status, err := c.GetStatus()
		if err != nil {
			return "", err
		}
		data, _ := json.MarshalIndent(status, "", "  ")
		return string(data), nil

	case "peers":
		if c == nil {
			return "Not connected", nil
		}
		peers, err := c.GetConnections()
		if err != nil {
			return "", err
		}
		data, _ := json.MarshalIndent(peers, "", "  ")
		return string(data), nil

	case "services":
		if c == nil {
			return "Not connected", nil
		}
		services, err := c.ListServices()
		if err != nil {
			return "", err
		}
		data, _ := json.MarshalIndent(services, "", "  ")
		return string(data), nil

	case "figs":
		if c == nil {
			return "Not connected", nil
		}
		figs, err := c.GetServiceFigs()
		if err != nil {
			return "", err
		}
		data, _ := json.MarshalIndent(figs, "", "  ")
		return string(data), nil

	case "connect":
		if len(args) == 0 {
			return "", fmt.Errorf("usage: connect <address>")
		}
		address := args[0]
		if !strings.HasPrefix(address, "http") {
			address = "http://" + address
		}
		newClient := client.New(address)
		if _, err := newClient.Health(); err != nil {
			return "", err
		}
		newClient.SubscribeEvents()
		// Create new instance
		inst := &NodeInstance{
			Client:    newClient,
			Address:   address,
			Connected: true,
		}
		idx := s.AddInstance(inst)
		s.instancesMu.Lock()
		s.activeInstanceIdx = idx
		s.instancesMu.Unlock()
		go s.forwardEventsFromClient(newClient)
		return fmt.Sprintf("Connected to %s (instance %d)", address, idx+1), nil

	case "select":
		if len(args) == 0 {
			return "", fmt.Errorf("usage: select <instance_number>")
		}
		var n int
		fmt.Sscanf(args[0], "%d", &n)
		idx := n - 1
		s.instancesMu.Lock()
		if idx < 0 || idx >= len(s.instances) {
			s.instancesMu.Unlock()
			return "", fmt.Errorf("invalid instance index (1-%d)", len(s.instances))
		}
		inst := s.instances[idx]
		if !inst.Connected {
			// Remove disconnected instance
			s.instances = append(s.instances[:idx], s.instances[idx+1:]...)
			if s.activeInstanceIdx >= len(s.instances) {
				s.activeInstanceIdx = len(s.instances) - 1
			}
			s.instancesMu.Unlock()
			// Sync to session after modification
			s.SyncToSession()
			return fmt.Sprintf("Removed disconnected instance %d", n), nil
		}
		s.activeInstanceIdx = idx
		s.instancesMu.Unlock()
		// Sync to session after modification
		s.SyncToSession()
		return fmt.Sprintf("Switched to instance %d: %s", n, inst.Address), nil

	case "instances":
		s.instancesMu.RLock()
		defer s.instancesMu.RUnlock()
		if len(s.instances) == 0 {
			return "No instances", nil
		}
		var sb strings.Builder
		sb.WriteString("Instances:\n")
		for i, inst := range s.instances {
			marker := "  "
			if i == s.activeInstanceIdx {
				marker = "► "
			}
			status := "disconnected"
			if inst.Connected {
				status = "connected"
			}
			sb.WriteString(fmt.Sprintf("%s[%d] %s - %s\n", marker, i+1, inst.Address, status))
		}
		return sb.String(), nil

	case "stop":
		inst := s.ActiveInstance()
		if inst == nil {
			return "No instance selected", nil
		}
		if inst.NodeProc != nil {
			inst.NodeProc.Stop()
		} else if inst.Client != nil {
			inst.Client.Shutdown()
		}
		if inst.Client != nil {
			inst.Client.Close()
		}
		inst.Connected = false
		return "Instance stopped", nil

	case "help":
		return `Available commands:
  connect <address> - Connect to a new node instance
  select <n>        - Switch to instance number n
  instances         - List all instances
  stop              - Stop/disconnect active instance
  status            - Show node status
  peers             - List peers
  services          - List services
  figs              - List figs
  help              - Show this help`, nil

	default:
		return "", fmt.Errorf("unknown command: %s", cmd)
	}
}
