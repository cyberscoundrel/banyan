package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"banyan/common"
)

// BasicHandlers provides basic HTTP endpoint handlers
type BasicHandlers struct{}

// NewBasicHandlers creates a new BasicHandlers instance
func NewBasicHandlers() *BasicHandlers {
	return &BasicHandlers{}
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
