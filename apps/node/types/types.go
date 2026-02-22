package types

import (
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// Protocol IDs
const (
	HTTPProxyProtocol = "/http-proxy/1.0.0"
	GossipSubTopic    = "peer-discovery"
)

// Connection status constants
const (
	StatusConnected    = "connected"
	StatusDisconnected = "disconnected"
	StatusConnecting   = "connecting"
)

// Connection type constants
const (
	ConnTypeManual       = "manual"           // Manually connected via /connect-to-peer/
	ConnTypeGossipLookup = "gossipsub-lookup" // Found via gossipsub lookup response
	ConnTypeHTTPVerified = "http-verified"    // Verified HTTP capability
	ConnTypeBackground   = "background"       // Background discovery (DHT/MDNS)
)

// HTTP test result constants
const (
	HTTPTestSuccess  = "success"
	HTTPTestFailed   = "failed"
	HTTPTestPending  = "pending"
	HTTPTestUntested = "untested"
)

// Event types
const (
	EventPeerConnected    = "peer_connected"
	EventPeerDisconnected = "peer_disconnected"
	EventPeerTracked      = "peer_tracked"
	EventHTTPTest         = "http_test"
	EventGossipMessage    = "gossip_message"
	EventDHTLookup        = "dht_lookup"
	EventProxyRequest     = "proxy_request"
	EventNodeStatus       = "node_status"
	EventNodeStarted      = "node_started"
	EventBootstrap        = "bootstrap"
	EventDiscovery        = "discovery"
	EventConnection       = "connection"
	EventError            = "error"
	EventInfo             = "info"
	EventNATStatus        = "nat_status"
)

// Event system for real-time streaming
type Event struct {
	Type      string      `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data"`
	NodeID    string      `json:"node_id"`
}

// Message types for gossipsub
type LookupRequest struct {
	Type          string    `json:"type"`                    // "lookup"
	Target        string    `json:"target"`                  // PeerID you're looking for(optional)
	From          string    `json:"from"`                    // Your PeerID (optional)
	Addresses     []string  `json:"addresses,omitempty"`     // Multiaddresses of the requester (optional)
	PublicKey     []byte    `json:"publicKey"`               // ECC public key for encryption (optional)
	ServiceKey    []byte    `json:"serviceKey"`              // ECC public key for encryption (optional)
	Encrypted     bool      `json:"encrypted"`               // Whether the addresses are encrypted
	EncryptedData []byte    `json:"encryptedData,omitempty"` // AES encrypted requester data if encrypted=true
	Nonce         []byte    `json:"nonce,omitempty"`         // AES-GCM nonce for encrypted data
	Timestamp     time.Time `json:"timestamp"`
}

type LookupResponse struct {
	Type          string    `json:"type"`                    // "response"
	Target        string    `json:"target"`                  // Original target PeerID(optional)
	From          string    `json:"from"`                    // Responder PeerID
	Addresses     []string  `json:"addresses,omitempty"`     // Multiaddresses of the responder (unencrypted)
	PublicKey     []byte    `json:"publicKey"`               // ECC public key for verification
	Signature     []byte    `json:"signature"`               // Signature of the response
	Encrypted     bool      `json:"encrypted"`               // Whether the addresses are encrypted
	EncryptedData []byte    `json:"encryptedData,omitempty"` // AES encrypted responder data if encrypted=true
	Nonce         []byte    `json:"nonce,omitempty"`         // AES-GCM nonce
	Timestamp     time.Time `json:"timestamp"`
}

// ServiceResponse is used strictly for service locator/beacon exchange.
// It contains only peer ID information (plaintext when crypto disabled, encrypted otherwise).
type ServiceResponse struct {
	Type          string    `json:"type"`                    // "service-response"
	From          string    `json:"from,omitempty"`          // Responder PeerID (only when crypto disabled)
	PublicKey     []byte    `json:"publicKey"`               // Service public key (for decryption and signature verification)
	Signature     []byte    `json:"signature"`               // Signature by service private key over canonical fields
	Encrypted     bool      `json:"encrypted"`               // Always true when crypto enabled
	EncryptedData []byte    `json:"encryptedData,omitempty"` // Encrypted responder peer ID
	Nonce         []byte    `json:"nonce,omitempty"`         // AES-GCM nonce
	Timestamp     time.Time `json:"timestamp"`
	Addresses     []string  `json:"addresses,omitempty"` // Multiaddresses (filtered by transport restrictions if applicable)
}

// ServiceLookupRequest represents a request for a service
type ServiceLookupRequest struct {
	Type           string    `json:"type"`                     // "service-lookup"
	ServiceKey     []byte    `json:"serviceKey"`               // Public key of the service we're looking for
	RequestPath    string    `json:"requestPath,omitempty"`    // Optional: specific path in the fig tree being requested
	From           string    `json:"from,omitempty"`           // Requesting peer ID (unencrypted)
	EncryptedFrom  []byte    `json:"encryptedFrom,omitempty"`  // Encrypted peer ID
	EncryptedNonce []byte    `json:"encryptedNonce,omitempty"` // Nonce for encrypted peer ID
	FromEncrypted  bool      `json:"fromEncrypted"`            // Whether the peer ID is encrypted
	PublicKey      []byte    `json:"publicKey"`                // Public key for encrypted response
	Signature      []byte    `json:"signature,omitempty"`      // Signature by locator ephemeral key
	Timestamp      time.Time `json:"timestamp"`
}

// ServiceAnnouncement represents a service announcement
type ServiceAnnouncement struct {
	Type       string      `json:"type"`       // "service-announcement"
	ServiceKey []byte      `json:"serviceKey"` // Public key of the service
	PeerID     string      `json:"peerID"`     // Peer providing the service
	Data       interface{} `json:"data"`       // Custom service data
	Timestamp  time.Time   `json:"timestamp"`
	Signature  []byte      `json:"signature,omitempty"` // Signature by service private key over announcement fields
}

// GreetingResponse represents the response from the greetings endpoint
type GreetingResponse struct {
	PeerID     string      `json:"peer_id"`
	ServiceKey []byte      `json:"service_key,omitempty"` // Only if this node provides a service
	Data       interface{} `json:"data,omitempty"`        // Custom data
	Timestamp  time.Time   `json:"timestamp"`
	Signature  []byte      `json:"signature,omitempty"` // Signature of the response
}

// AddonDisclosure represents public-facing addon information to include in greetings
type AddonDisclosure struct {
	// Name to display in greetings; if empty, the addon is not disclosed
	Name string `json:"name"`
	// Version formatted as npm dependency versions in package.json, e.g. "^1.2.3", "~0.5.0", "1.2.3"
	Version string `json:"version,omitempty"`
	// Info is optional supplementary information provided by the addon
	Info map[string]interface{} `json:"info,omitempty"`
}

// Connection tracking - enhanced to track peer status and history
type ConnectionItem struct {
	PeerID            peer.ID
	Alias             string   // Unique 4-character shorthand identifier
	ServiceKeys       [][]byte // one peer may serve multiple service keys
	Connected         time.Time
	LastActivity      time.Time
	LastDisconnect    *time.Time
	Status            string // "connected", "disconnected", "connecting"
	ConnectAttempts   int
	ConnectionType    string     // "manual", "gossipsub-lookup", "http-verified", "background"
	HTTPCapable       bool       // Whether this peer has been verified to have HTTP server
	BidirectionalHTTP bool       // Whether bidirectional HTTP communication works
	LastHTTPTest      *time.Time // When we last tested HTTP capability
	HTTPTestResult    string     // "success", "failed", "pending", "untested"
	mutex             sync.RWMutex
	data              map[string]interface{}
}

// PeerOptions contains the settable fields when adding/updating tracked peers
type PeerOptions struct {
	ServiceKey     []byte // single service key to append (optional)
	ConnectionType string // "manual", "gossipsub-lookup", "http-verified", "background"
	HTTPCapable    bool   // Whether this peer has been verified to have HTTP server
}

// Helper functions to create common PeerOptions configurations

// NewManualPeerOptions creates options for manually added peers
func NewManualPeerOptions(httpCapable bool) PeerOptions {
	return PeerOptions{
		ConnectionType: ConnTypeManual,
		HTTPCapable:    httpCapable,
	}
}

// NewServicePeerOptions creates options for service-affiliated peers
func NewServicePeerOptions(serviceKey []byte, httpCapable bool) PeerOptions {
	return PeerOptions{
		ServiceKey:     serviceKey,
		ConnectionType: ConnTypeHTTPVerified,
		HTTPCapable:    httpCapable,
	}
}

// NewBackgroundPeerOptions creates options for background discovered peers
func NewBackgroundPeerOptions() PeerOptions {
	return PeerOptions{
		ConnectionType: ConnTypeBackground,
		HTTPCapable:    false,
	}
}

// NewGossipPeerOptions creates options for gossipsub discovered peers
func NewGossipPeerOptions(httpCapable bool) PeerOptions {
	return PeerOptions{
		ConnectionType: ConnTypeGossipLookup,
		HTTPCapable:    httpCapable,
	}
}

// ServiceBeaconMode represents different modes for service beacons
type ServiceBeaconMode int

const (
	ServiceBeaconModeLookup ServiceBeaconMode = iota
	ServiceBeaconModeAnnounce
	ServiceBeaconModeReplyOnly
)

// RouteEntry represents a routing entry that maps an identifier to a URL
type RouteEntry struct {
	Identifier string    `json:"identifier"`
	URL        string    `json:"url"`
	CreatedAt  time.Time `json:"created_at"`
}

// ServiceRoutingConfig defines routing behavior for a service
type ServiceRoutingConfig struct {
	RoutePrefix  string `json:"routePrefix"`  // Prefix to add to incoming paths (e.g., "/api/v1")
	KeepFullPath bool   `json:"keepFullPath"` // If true, keep full path; if false, strip matched prefix
}

// RouteTable manages the routing table with thread safety
type RouteTable struct {
	routes        map[string]string               // identifier -> URL mapping
	routingConfig map[string]ServiceRoutingConfig // identifier -> routing configuration
	mutex         sync.RWMutex
}

// NewRouteTable creates a new route table
func NewRouteTable() *RouteTable {
	return &RouteTable{
		routes:        make(map[string]string),
		routingConfig: make(map[string]ServiceRoutingConfig),
	}
}

// AddRoute adds or updates a route
func (rt *RouteTable) AddRoute(identifier, url string) {
	rt.mutex.Lock()
	defer rt.mutex.Unlock()
	rt.routes[identifier] = url
}

// AddRouteWithConfig adds or updates a route with routing configuration
func (rt *RouteTable) AddRouteWithConfig(identifier, url string, config ServiceRoutingConfig) {
	rt.mutex.Lock()
	defer rt.mutex.Unlock()
	rt.routes[identifier] = url
	rt.routingConfig[identifier] = config
}

// GetRoute retrieves a URL by identifier
func (rt *RouteTable) GetRoute(identifier string) (string, bool) {
	rt.mutex.RLock()
	defer rt.mutex.RUnlock()
	url, exists := rt.routes[identifier]
	return url, exists
}

// GetRouteConfig retrieves routing configuration for an identifier
func (rt *RouteTable) GetRouteConfig(identifier string) (ServiceRoutingConfig, bool) {
	rt.mutex.RLock()
	defer rt.mutex.RUnlock()
	config, exists := rt.routingConfig[identifier]
	return config, exists
}

// GetAllRoutes returns a copy of all routes
func (rt *RouteTable) GetAllRoutes() map[string]string {
	rt.mutex.RLock()
	defer rt.mutex.RUnlock()
	routes := make(map[string]string, len(rt.routes))
	for k, v := range rt.routes {
		routes[k] = v
	}
	return routes
}

// LoadRoutes loads routes from a map (used for loading from JSON file)
func (rt *RouteTable) LoadRoutes(routes map[string]string) {
	rt.mutex.Lock()
	defer rt.mutex.Unlock()
	for identifier, url := range routes {
		rt.routes[identifier] = url
	}
}

// FigNode represents a node in the key tree for a service.
// Each path node can contain multiple keys to support multiple identities for a path.
type FigNode struct {
	Path              string    `json:"path"`
	Keys              []string  `json:"keys"`
	Children          []FigNode `json:"children,omitempty"`
	AllowedTransports []string  `json:"allowedTransports,omitempty"` // Optional: restricts which transports are allowed for this path
}

// FigFile represents the structure of a fig file used to identify a service and its key tree.
// For now we will only use the root node keys for lookups; hierarchical paths are reserved for future use.
type FigFile struct {
	ServiceAlias string  `json:"serviceAlias"`
	Root         FigNode `json:"root"`
	// Data carries arbitrary metadata for consumers
	Data json.RawMessage `json:"data,omitempty"`
	// Optional security fields:
	// ExpiresAt: RFC3339 timestamp after which this fig must not be accepted
	ExpiresAt time.Time `json:"expiresAt,omitempty"`
	// RequesterPeerID binds the fig to a specific requester peer id
	RequesterPeerID string `json:"requesterPeerId,omitempty"`
	// Nonce binds the fig to a unique request instance (hex-encoded)
	Nonce string `json:"nonce,omitempty"`
	// Signatures contains hex-encoded signatures over the canonical payload
	Signatures []string `json:"signatures,omitempty"`
	// RequiredSigners lists authority public keys (hex-encoded) that must sign the fig template
	RequiredSigners []string `json:"requiredSigners,omitempty"`
	// AuthoritySignatures contains hex-encoded signatures from required authority signers
	AuthoritySignatures []string `json:"authoritySignatures,omitempty"`
}

// BuildCanonicalPayload constructs a deterministic byte slice for signing and verification.
// This method ensures consistent payload generation across all fig file handling.
func (f FigFile) BuildCanonicalPayload() []byte {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(f.ServiceAlias))
	if !f.ExpiresAt.IsZero() {
		b.WriteString("|")
		b.WriteString(f.ExpiresAt.UTC().Format(time.RFC3339))
	}
	if strings.TrimSpace(f.RequesterPeerID) != "" {
		b.WriteString("|")
		b.WriteString(strings.TrimSpace(f.RequesterPeerID))
	}
	if strings.TrimSpace(f.Nonce) != "" {
		b.WriteString("|")
		b.WriteString(strings.TrimSpace(f.Nonce))
	}
	// Include sorted root keys
	keys := append([]string(nil), f.Root.Keys...)
	for i := range keys {
		keys[i] = strings.ToLower(strings.TrimSpace(keys[i]))
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteString("|")
		b.WriteString(k)
	}
	// Include data if present (use compacted JSON for determinism)
	if len(f.Data) > 0 {
		var m interface{}
		if json.Unmarshal(f.Data, &m) == nil {
			compacted, _ := json.Marshal(m)
			b.WriteString("|")
			b.Write(compacted)
		} else {
			b.WriteString("|")
			b.Write(f.Data)
		}
	}
	return []byte(b.String())
}

// BuildAuthorityPayload constructs a deterministic byte slice for authority signature verification.
// This payload is used for pre-signing fig templates by governance/CA authorities before
// request-specific details (nonce, requester peer ID) are added.
// The payload intentionally excludes nonce, requester peer ID, authority signatures, and service signatures
// to avoid circular dependencies and enable template signing.
func (f FigFile) BuildAuthorityPayload() []byte {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(f.ServiceAlias))
	if !f.ExpiresAt.IsZero() {
		b.WriteString("|")
		b.WriteString(f.ExpiresAt.UTC().Format(time.RFC3339))
	}
	// Include sorted required signers
	if len(f.RequiredSigners) > 0 {
		signers := append([]string(nil), f.RequiredSigners...)
		for i := range signers {
			signers[i] = strings.ToLower(strings.TrimSpace(signers[i]))
		}
		sort.Strings(signers)
		for _, s := range signers {
			b.WriteString("|")
			b.WriteString(s)
		}
	}
	// Include sorted root keys
	keys := append([]string(nil), f.Root.Keys...)
	for i := range keys {
		keys[i] = strings.ToLower(strings.TrimSpace(keys[i]))
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteString("|")
		b.WriteString(k)
	}
	return []byte(b.String())
}

// FindKeysForPath returns the most specific matching keys for a given request path.
// It performs hierarchical path matching, prioritizing exact matches over prefix matches.
// Falls back to Root.Keys if no path-specific match is found.
// Returns an empty slice (not nil) if no keys are available.
func (f FigFile) FindKeysForPath(requestPath string) []string {
	keys, _ := f.FindKeysAndMatchedPath(requestPath)
	return keys
}

// FindKeysAndMatchedPath returns both the keys and the matched path for a given request path.
// This is useful for path stripping in proxies.
// Returns (keys, matchedPath) where matchedPath is the path in the fig tree that matched.
func (f FigFile) FindKeysAndMatchedPath(requestPath string) ([]string, string) {
	// Normalize the request path
	requestPath = strings.TrimSpace(requestPath)
	if requestPath == "" {
		requestPath = "/"
	}
	// Ensure path starts with /
	if !strings.HasPrefix(requestPath, "/") {
		requestPath = "/" + requestPath
	}

	// Try to find path-specific keys
	pathKeys, matchedPath := f.Root.findKeysAndMatchedPath(requestPath)

	// If path-specific keys found, return them
	if len(pathKeys) > 0 {
		return pathKeys, matchedPath
	}

	// Fall back to root keys
	if len(f.Root.Keys) > 0 {
		return f.Root.Keys, "/"
	}

	// Return empty slice if no keys available
	return []string{}, ""
}

// findKeysAndMatchedPath is a recursive helper that searches the fig tree for the most specific
// matching path and returns both the keys and the matched path.
// Returns (keys, matchedPath) where matchedPath is the path in the fig tree that matched.
func (n FigNode) findKeysAndMatchedPath(requestPath string) ([]string, string) {
	// Normalize node path
	nodePath := strings.TrimSpace(n.Path)
	if nodePath == "" {
		nodePath = "/"
	}

	// Check for exact match first
	if nodePath == requestPath {
		if len(n.Keys) > 0 {
			return n.Keys, nodePath
		}
		// Exact match but no keys, continue searching children
	}

	// Check if this node's path is a prefix of the request path
	// For prefix matching, ensure proper path boundaries
	isPrefix := false
	if nodePath == "/" {
		// Root always matches as prefix
		isPrefix = true
	} else if strings.HasPrefix(requestPath, nodePath) {
		// Check that it's a proper path prefix (followed by / or end of string)
		if requestPath == nodePath {
			isPrefix = true
		} else if len(requestPath) > len(nodePath) && requestPath[len(nodePath)] == '/' {
			isPrefix = true
		}
	}

	if !isPrefix {
		return []string{}, ""
	}

	// This node is a prefix match. Search children for more specific matches.
	var bestMatch []string
	var bestMatchPath string
	bestMatchLen := len(nodePath)

	for _, child := range n.Children {
		childKeys, childMatchPath := child.findKeysAndMatchedPath(requestPath)
		if len(childKeys) > 0 {
			// Child found a match. Check if it's more specific.
			if len(childMatchPath) > bestMatchLen {
				bestMatch = childKeys
				bestMatchPath = childMatchPath
				bestMatchLen = len(childMatchPath)
			}
		}
	}

	// If a child had a better match, return it
	if len(bestMatch) > 0 {
		return bestMatch, bestMatchPath
	}

	// Otherwise, return this node's keys if it has any
	if len(n.Keys) > 0 {
		return n.Keys, nodePath
	}

	// No keys at this level
	return []string{}, ""
}

// findKeysForPath is a recursive helper that searches the fig tree for the most specific
// matching path. It uses prefix matching where a node's path is a prefix of the request path.
// Returns empty slice if no match found.
func (n FigNode) findKeysForPath(requestPath string) []string {
	// Normalize node path
	nodePath := strings.TrimSpace(n.Path)
	if nodePath == "" {
		nodePath = "/"
	}

	// Check for exact match first
	if nodePath == requestPath {
		if len(n.Keys) > 0 {
			return n.Keys
		}
		// Exact match but no keys, continue searching children
	}

	// Check if this node's path is a prefix of the request path
	// For prefix matching, ensure proper path boundaries
	isPrefix := false
	if nodePath == "/" {
		// Root always matches as prefix
		isPrefix = true
	} else if strings.HasPrefix(requestPath, nodePath) {
		// Check that it's a proper path prefix (followed by / or end of string)
		if requestPath == nodePath {
			isPrefix = true
		} else if len(requestPath) > len(nodePath) && requestPath[len(nodePath)] == '/' {
			isPrefix = true
		}
	}

	if !isPrefix {
		return []string{}
	}

	// This node is a prefix match. Search children for more specific matches.
	var bestMatch []string
	bestMatchLen := len(nodePath)

	for _, child := range n.Children {
		childKeys := child.findKeysForPath(requestPath)
		if len(childKeys) > 0 {
			// Child found a match. Check if it's more specific.
			childPath := strings.TrimSpace(child.Path)
			if childPath == "" {
				childPath = "/"
			}
			if len(childPath) > bestMatchLen {
				bestMatch = childKeys
				bestMatchLen = len(childPath)
			}
		}
	}

	// If a child had a better match, return it
	if len(bestMatch) > 0 {
		return bestMatch
	}

	// Otherwise, return this node's keys if it has any
	if len(n.Keys) > 0 {
		return n.Keys
	}

	// No keys at this level
	return []string{}
}

// CollectAllServiceKeys recursively collects all service keys from the root node
// and all descendant nodes in the fig tree
func (f *FigFile) CollectAllServiceKeys() []string {
	if f.Root.Path == "" && len(f.Root.Keys) == 0 && len(f.Root.Children) == 0 {
		return []string{}
	}
	return f.Root.collectKeysRecursive()
}

// collectKeysRecursive recursively collects all keys from this node and all
// descendant nodes in the tree
func (n *FigNode) collectKeysRecursive() []string {
	keys := make([]string, 0)

	// Add keys from current node
	keys = append(keys, n.Keys...)

	// Recursively collect keys from all children
	for _, child := range n.Children {
		keys = append(keys, child.collectKeysRecursive()...)
	}

	return keys
}

// FindNodeForPath finds the most specific FigNode that matches the given request path.
// Returns the matching node and true if found, or nil and false if no match.
// Uses prefix matching where a node's path is a prefix of the request path.
func (f *FigFile) FindNodeForPath(requestPath string) (*FigNode, bool) {
	return f.Root.findNodeForPath(requestPath)
}

// findNodeForPath is a recursive helper that searches the fig tree for the most specific
// matching node. It uses prefix matching where a node's path is a prefix of the request path.
func (n *FigNode) findNodeForPath(requestPath string) (*FigNode, bool) {
	// Normalize node path
	nodePath := strings.TrimSpace(n.Path)
	if nodePath == "" {
		nodePath = "/"
	}

	// Normalize request path
	reqPath := strings.TrimSpace(requestPath)
	if reqPath == "" {
		reqPath = "/"
	}

	// Ensure paths start with /
	if !strings.HasPrefix(nodePath, "/") {
		nodePath = "/" + nodePath
	}
	if !strings.HasPrefix(reqPath, "/") {
		reqPath = "/" + reqPath
	}

	// Check if this node matches
	var bestMatch *FigNode
	var bestMatchFound bool

	if reqPath == nodePath || strings.HasPrefix(reqPath, nodePath+"/") || nodePath == "/" {
		// This node is a potential match
		bestMatch = n
		bestMatchFound = true
	}

	// Check children for more specific matches
	for i := range n.Children {
		if childMatch, found := n.Children[i].findNodeForPath(reqPath); found {
			// Child has a more specific match
			bestMatch = childMatch
			bestMatchFound = true
		}
	}

	return bestMatch, bestMatchFound
}
