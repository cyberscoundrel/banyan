package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	nodePkg "banyan/node"
)

// RouterHandlers provides HTTP endpoint handlers for URL route management.
// Routes map identifiers to URLs for custom proxy configurations.
type RouterHandlers struct {
	node *nodePkg.Node
}

// NewRouterHandlers creates a new RouterHandlers instance
func NewRouterHandlers(node *nodePkg.Node) *RouterHandlers {
	return &RouterHandlers{
		node: node,
	}
}

// RouteRequest represents a request body for adding or updating a URL route.
type RouteRequest struct {
	Identifier string `json:"identifier"`
	URL        string `json:"url"`
}

// HandleAddRoute handles the /router/add endpoint to add or update a URL route
// mapping in the routing table.
func (rh *RouterHandlers) HandleAddRoute(w http.ResponseWriter, r *http.Request) {
	if rh.node == nil {
		http.Error(w, "Node not available", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed. Use POST", http.StatusMethodNotAllowed)
		return
	}

	var request RouteRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid JSON request body", http.StatusBadRequest)
		return
	}

	if request.Identifier == "" {
		http.Error(w, "identifier is required", http.StatusBadRequest)
		return
	}

	if request.URL == "" {
		http.Error(w, "url is required", http.StatusBadRequest)
		return
	}

	// Add the route to the routing table
	routeTable := rh.node.GetRouteTable()
	routeTable.AddRoute(request.Identifier, request.URL)

	// Return success response
	response := map[string]interface{}{
		"status":     "success",
		"message":    "Route added successfully",
		"identifier": request.Identifier,
		"url":        request.URL,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)

	log.Printf("Added route: %s -> %s", request.Identifier, request.URL)
}

// HandleGetRoutes handles the /router/list endpoint to return all current
// URL route mappings from the routing table.
func (rh *RouterHandlers) HandleGetRoutes(w http.ResponseWriter, r *http.Request) {
	if rh.node == nil {
		http.Error(w, "Node not available", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed. Use GET", http.StatusMethodNotAllowed)
		return
	}

	// Get all routes from the routing table
	routeTable := rh.node.GetRouteTable()
	routes := routeTable.GetAllRoutes()

	response := map[string]interface{}{
		"routes": routes,
		"count":  len(routes),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)

	log.Printf("Served routes: %s %s", r.Method, r.URL.Path)
}
