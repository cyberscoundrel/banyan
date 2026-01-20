package server

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"banyan-cli/client"
	"banyan-cli/nodeproc"
)

func (s *Server) handlePeerConnect(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	c := s.ActiveClient()
	if c == nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "not connected"})
		return
	}

	var req struct {
		PeerID string `json:"peerId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if err := c.ConnectToPeer(req.PeerID); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (s *Server) handlePeerAdd(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	c := s.ActiveClient()
	if c == nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "not connected"})
		return
	}

	var req client.AddPeerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	result, err := c.AddTrackedPeer(req)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	c := s.ActiveClient()
	if c == nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "not connected"})
		return
	}

	var req struct {
		Type   string `json:"type"` // "peer" or "service"
		ID     string `json:"id"`
		Path   string `json:"path"`
		Method string `json:"method"`
		Body   string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if req.Method == "" {
		req.Method = "GET"
	}

	var body io.Reader
	if req.Body != "" {
		body = strings.NewReader(req.Body)
	}

	var respBody []byte
	var statusCode int
	var err error

	switch req.Type {
	case "peer":
		respBody, statusCode, err = c.ProxyPeerRequest(req.ID, req.Path, req.Method, body)
	case "service":
		respBody, statusCode, err = c.ProxyServiceRequest(req.ID, req.Path, req.Method, body)
	default:
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid proxy type"})
		return
	}

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"statusCode": statusCode,
		"body":       string(respBody),
	})
}

func (s *Server) handleRoutes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	c := s.ActiveClient()
	if c == nil {
		json.NewEncoder(w).Encode([]interface{}{})
		return
	}

	switch r.Method {
	case http.MethodGet:
		routes, err := c.GetRoutes()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(routes)

	case http.MethodPost:
		var req client.RouteAddRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		if err := c.AddRoute(req); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]bool{"success": true})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleNodeStart(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Parse request body for optional parameters
	var req struct {
		NodePath   string   `json:"nodePath"`
		ConfigPath string   `json:"configPath"`
		ExtraArgs  []string `json:"extraArgs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}

	// Determine executable path
	execPath := req.NodePath
	if execPath == "" {
		execPath = s.config.NodeExecutable
	}
	if execPath == "" {
		execPath = findNodeExecutable()
	}
	if execPath == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "node executable not found. Specify nodePath or place banyan-* executable in same directory"})
		return
	}

	// Determine config path
	configPath := req.ConfigPath
	if configPath == "" {
		configPath = s.config.NodeConfigPath
	}

	np := nodeproc.New(execPath, configPath, req.ExtraArgs)
	if err := np.Start(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	url, err := np.WaitForManagementURL(30 * time.Second)
	if err != nil {
		np.Stop()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	// Auto-connect to the started node and create instance
	c := client.New(url)
	connected := false
	if _, err := c.Health(); err == nil {
		c.SubscribeEvents()
		// Create new instance for this node
		inst := &NodeInstance{
			Client:     c,
			Address:    url,
			Connected:  true,
			NodeProc:   np,
			KillOnExit: true,
		}
		idx := s.AddInstance(inst)
		s.instancesMu.Lock()
		s.activeInstanceIdx = idx
		s.instancesMu.Unlock()
		go s.forwardEventsFromClient(c)
		connected = true
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"url":       url,
		"connected": connected,
	})
}

// findNodeExecutable looks for any executable starting with "banyan" in the same directory
func findNodeExecutable() string {
	exePath, err := os.Executable()
	if err != nil {
		return ""
	}
	exePath, _ = filepath.EvalSymlinks(exePath)
	dir := filepath.Dir(exePath)
	exeName := filepath.Base(exePath)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == exeName {
			continue
		}
		nameLower := strings.ToLower(name)
		if !strings.HasPrefix(nameLower, "banyan") {
			continue
		}
		// On Windows, must end with .exe
		if runtime.GOOS == "windows" && !strings.HasSuffix(nameLower, ".exe") {
			continue
		}
		candidate := filepath.Join(dir, name)
		if runtime.GOOS != "windows" {
			info, err := entry.Info()
			if err != nil || info.Mode()&0111 == 0 {
				continue
			}
		}
		return candidate
	}
	return ""
}
