package handlers

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"banyan/common"
)

// BasicHandlers provides basic HTTP endpoint handlers
type BasicHandlers struct {
	shutdownFunc func()
}

// NewBasicHandlers creates a new BasicHandlers instance
func NewBasicHandlers() *BasicHandlers {
	return &BasicHandlers{}
}

// SetShutdownFunc sets the function to call for node shutdown
func (bh *BasicHandlers) SetShutdownFunc(fn func()) {
	bh.shutdownFunc = fn
}

// HandleRequest handles the root endpoint with echo functionality
func (bh *BasicHandlers) HandleRequest(w http.ResponseWriter, r *http.Request) {
	response := common.Response{
		Message:   "Hello from HTTP management server!",
		Timestamp: time.Now(),
		Method:    r.Method,
		Path:      r.URL.Path,
		Headers:   r.Header,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode response: %v", err)
	}

	log.Printf("Handled request: %s %s", r.Method, r.URL.Path)
}

// HandleAPITest handles the API test endpoint
func (bh *BasicHandlers) HandleAPITest(w http.ResponseWriter, r *http.Request) {
	response := common.Response{
		Message:   "API test successful!",
		Timestamp: time.Now(),
		Method:    r.Method,
		Path:      r.URL.Path,
		Headers:   r.Header,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode response: %v", err)
	}

	log.Printf("API test request: %s %s", r.Method, r.URL.Path)
}

// HandleHealth handles the health check endpoint
func (bh *BasicHandlers) HandleHealth(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now(),
		"service":   "libp2p-proxy-server",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode health response: %v", err)
	}

	log.Printf("Health check: %s %s", r.Method, r.URL.Path)
}

// HandleShutdown handles graceful shutdown requests
// This endpoint requires POST and can only be called from localhost for security
func (bh *BasicHandlers) HandleShutdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Security: Only allow shutdown from localhost
	remoteAddr := r.RemoteAddr
	host, _, _ := net.SplitHostPort(remoteAddr)
	if host != "127.0.0.1" && host != "::1" && host != "localhost" {
		http.Error(w, "Shutdown can only be initiated from localhost", http.StatusForbidden)
		log.Printf("Shutdown request rejected from non-localhost: %s", remoteAddr)
		return
	}

	log.Printf("Shutdown requested from %s", remoteAddr)

	response := map[string]interface{}{
		"status":    "shutting_down",
		"message":   "Node shutdown initiated",
		"timestamp": time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)

	// Trigger shutdown in a goroutine so we can respond first
	go func() {
		time.Sleep(100 * time.Millisecond) // Give time for response to be sent
		if bh.shutdownFunc != nil {
			bh.shutdownFunc()
		} else {
			log.Println("No shutdown function configured, exiting directly")
			os.Exit(0)
		}
	}()
}
