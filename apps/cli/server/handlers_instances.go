package server

import (
	"encoding/json"
	"net/http"

	"banyan-cli/session"
)

// InstanceInfo represents instance info for the API
type InstanceInfo struct {
	Index      int    `json:"index"`
	Address    string `json:"address"`
	Connected  bool   `json:"connected"`
	Active     bool   `json:"active"`
	Subprocess bool   `json:"subprocess"`
	Name       string `json:"name,omitempty"`
}

func (s *Server) handleInstances(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Get instances from session (shared state from TUI)
	sess := session.Get()
	sessInstances := sess.GetInstances()

	instances := make([]InstanceInfo, 0, len(sessInstances))
	activeIdx := 0
	for _, inst := range sessInstances {
		instances = append(instances, InstanceInfo{
			Index:      inst.Index,
			Address:    inst.Address,
			Connected:  inst.Connected,
			Active:     inst.Active,
			Subprocess: inst.Subprocess,
			Name:       inst.Name,
		})
		if inst.Active {
			activeIdx = inst.Index
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"instances":    instances,
		"active_index": activeIdx,
		"total":        len(instances),
	})
}

func (s *Server) handleSelectInstance(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Index int `json:"index"` // 1-based
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	// Send command to TUI via session
	sess := session.Get()
	result := sess.SendCommand(session.Command{
		Action: "select",
		Index:  req.Index,
	})

	if !result.Success {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": result.Message})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"removed": result.Removed,
		"index":   req.Index,
		"address": result.Address,
	})
}

func (s *Server) handleStopInstance(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Index int `json:"index"` // 1-based, 0 means active
	}
	json.NewDecoder(r.Body).Decode(&req)

	// Send command to TUI via session
	sess := session.Get()
	result := sess.SendCommand(session.Command{
		Action: "stop",
		Index:  req.Index,
	})

	if !result.Success {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": result.Message})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"index":   req.Index,
		"message": result.Message,
	})
}
