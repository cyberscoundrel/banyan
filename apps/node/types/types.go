// Package types defines the core data structures, constants, and types used throughout
// the banyan node application. This package contains protocol definitions, event types,
// peer connection management structures, service discovery types, and routing configuration.
//
// The package provides:
//   - Protocol identifiers and connection status constants
//   - Event system types for real-time streaming
//   - Gossipsub message types for peer discovery
//   - Service discovery and announcement structures
//   - Connection tracking with HTTP capability verification
//   - Route table management for service routing
//   - Fig file structures for hierarchical key tree management
package types

import (
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// Protocol identifiers and topics used for libp2p communication.
// These constants define the protocol versions and pubsub topics for peer interaction.
const (
	// HTTPProxyProtocol is the libp2p protocol ID for HTTP proxy communication.
	HTTPProxyProtocol = "/http-proxy/1.0.0"
	// GossipSubTopic is the pubsub topic name used for peer discovery via gossipsub.
	GossipSubTopic = "peer-discovery"
)

// Connection status values representing the current state of a peer connection.
// These constants are used to track and report peer connection lifecycle states.
const (
	// StatusConnected indicates an active connection to the peer.
	StatusConnected = "connected"
	// StatusDisconnected indicates no active connection to the peer.
	StatusDisconnected = "disconnected"
	// StatusConnecting indicates a connection attempt is in progress.
	StatusConnecting = "connecting"
)

// Connection type values indicating how a peer connection was established.
// These constants help identify the discovery mechanism and trust level of connections.
const (
	// ConnTypeManual indicates a peer was connected manually via the /connect-to-peer/ endpoint.
	ConnTypeManual = "manual"
	// ConnTypeGossipLookup indicates a peer was discovered via gossipsub lookup response.
	ConnTypeGossipLookup = "gossipsub-lookup"
	// ConnTypeHTTPVerified indicates a peer connection with verified HTTP capability.
	ConnTypeHTTPVerified = "http-verified"
	// ConnTypeBackground indicates a peer was discovered through background mechanisms (DHT/MDNS).
	ConnTypeBackground = "background"
)

// HTTP test result values representing the outcome of HTTP capability verification.
// These constants track whether a peer's HTTP server has been tested and its accessibility.
const (
	// HTTPTestSuccess indicates the peer's HTTP server is accessible and responding.
	HTTPTestSuccess = "success"
	// HTTPTestFailed indicates the peer's HTTP server is not accessible.
	HTTPTestFailed = "failed"
	// HTTPTestPending indicates an HTTP capability test is in progress.
	HTTPTestPending = "pending"
	// HTTPTestUntested indicates no HTTP capability test has been performed yet.
	HTTPTestUntested = "untested"
)

// Event type values used in the event system for real-time streaming.
// These constants identify different categories of events that can occur during node operation.
const (
	// EventPeerConnected is emitted when a new peer connection is established.
	EventPeerConnected = "peer_connected"
	// EventPeerDisconnected is emitted when a peer connection is terminated.
	EventPeerDisconnected = "peer_disconnected"
	// EventPeerTracked is emitted when a peer is added to the connection tracker.
	EventPeerTracked = "peer_tracked"
	// EventHTTPTest is emitted when an HTTP capability test completes.
	EventHTTPTest = "http_test"
	// EventGossipMessage is emitted when a message is received via gossipsub.
	EventGossipMessage = "gossip_message"
	// EventDHTLookup is emitted when a DHT lookup operation occurs.
	EventDHTLookup = "dht_lookup"
	// EventProxyRequest is emitted when a proxy request is processed.
	EventProxyRequest = "proxy_request"
	// EventNodeStatus is emitted for general node status updates.
	EventNodeStatus = "node_status"
	// EventNodeStarted is emitted when the node has completed startup.
	EventNodeStarted = "node_started"
	// EventBootstrap is emitted during the bootstrap process.
	EventBootstrap = "bootstrap"
	// EventDiscovery is emitted when peer discovery events occur.
	EventDiscovery = "discovery"
	// EventConnection is emitted for connection-related events.
	EventConnection = "connection"
	// EventError is emitted when an error occurs.
	EventError = "error"
	// EventInfo is emitted for informational events.
	EventInfo = "info"
	// EventNATStatus is emitted when NAT status changes.
	EventNATStatus = "nat_status"
)

// Event represents an event in the real-time streaming system.
// Events are used to notify clients about peer connections, status changes,
// and other significant occurrences during node operation.
type Event struct {
	// Type identifies the category of event (e.g., "peer_connected", "http_test").
	Type string `json:"type"`
	// Timestamp records when the event occurred.
	Timestamp time.Time `json:"timestamp"`
	// Data contains event-specific payload data. The structure varies by event type.
	Data interface{} `json:"data"`
	// NodeID identifies the node that generated this event.
	NodeID string `json:"node_id"`
}

// LookupRequest represents a peer lookup request sent via gossipsub.
// It is used to discover peers and optionally establish encrypted communication channels.
type LookupRequest struct {
	// Type is the message type, always "lookup" for lookup requests.
	Type string `json:"type"`
	// Target is the optional PeerID being searched for.
	Target string `json:"target"`
	// From is the optional PeerID of the requester.
	From string `json:"from"`
	// Addresses contains the multiaddresses of the requester when sent unencrypted.
	Addresses []string `json:"addresses,omitempty"`
	// PublicKey is the ECC public key used for encryption of responses.
	PublicKey []byte `json:"publicKey"`
	// ServiceKey is the ECC public key identifying the service being looked up.
	ServiceKey []byte `json:"serviceKey"`
	// Encrypted indicates whether the addresses field contains encrypted data.
	Encrypted bool `json:"encrypted"`
	// EncryptedData contains AES-GCM encrypted requester data when Encrypted is true.
	EncryptedData []byte `json:"encryptedData,omitempty"`
	// Nonce is the AES-GCM nonce used for decrypting EncryptedData.
	Nonce []byte `json:"nonce,omitempty"`
	// Timestamp records when the request was created.
	Timestamp time.Time `json:"timestamp"`
}

// LookupResponse represents a response to a peer lookup request sent via gossipsub.
// It contains the responder's connection information and supports encrypted data exchange.
type LookupResponse struct {
	// Type is the message type, always "response" for lookup responses.
	Type string `json:"type"`
	// Target is the original target PeerID from the lookup request.
	Target string `json:"target"`
	// From is the PeerID of the responder.
	From string `json:"from"`
	// Addresses contains the multiaddresses of the responder when sent unencrypted.
	Addresses []string `json:"addresses,omitempty"`
	// PublicKey is the ECC public key for signature verification.
	PublicKey []byte `json:"publicKey"`
	// Signature is the cryptographic signature of the response payload.
	Signature []byte `json:"signature"`
	// Encrypted indicates whether the addresses field contains encrypted data.
	Encrypted bool `json:"encrypted"`
	// EncryptedData contains AES-GCM encrypted responder data when Encrypted is true.
	EncryptedData []byte `json:"encryptedData,omitempty"`
	// Nonce is the AES-GCM nonce used for decrypting EncryptedData.
	Nonce []byte `json:"nonce,omitempty"`
	// Timestamp records when the response was created.
	Timestamp time.Time `json:"timestamp"`
}

// ServiceResponse represents a response from a service beacon exchange.
// It is used strictly for service locator/beacon communication and contains
// peer ID information (plaintext when crypto disabled, encrypted otherwise).
type ServiceResponse struct {
	// Type is the message type, always "service-response".
	Type string `json:"type"`
	// From is the responder's PeerID, included only when crypto is disabled.
	From string `json:"from,omitempty"`
	// PublicKey is the service's public key for decryption and signature verification.
	PublicKey []byte `json:"publicKey"`
	// Signature is the cryptographic signature by the service private key.
	Signature []byte `json:"signature"`
	// Encrypted indicates whether the data is encrypted; always true when crypto is enabled.
	Encrypted bool `json:"encrypted"`
	// EncryptedData contains the encrypted responder peer ID when Encrypted is true.
	EncryptedData []byte `json:"encryptedData,omitempty"`
	// Nonce is the AES-GCM nonce for decrypting EncryptedData.
	Nonce []byte `json:"nonce,omitempty"`
	// Timestamp records when the response was created.
	Timestamp time.Time `json:"timestamp"`
	// Addresses contains multiaddresses, filtered by transport restrictions if applicable.
	Addresses []string `json:"addresses,omitempty"`
}

// ServiceLookupRequest represents a request to locate a service in the network.
// It supports both plaintext and encrypted peer identification for privacy.
type ServiceLookupRequest struct {
	// Type is the message type, always "service-lookup".
	Type string `json:"type"`
	// ServiceKey is the public key of the service being looked up.
	ServiceKey []byte `json:"serviceKey"`
	// RequestPath optionally specifies a particular path in the service's fig tree.
	RequestPath string `json:"requestPath,omitempty"`
	// From is the requesting peer ID when sent unencrypted.
	From string `json:"from,omitempty"`
	// EncryptedFrom contains the encrypted peer ID when FromEncrypted is true.
	EncryptedFrom []byte `json:"encryptedFrom,omitempty"`
	// EncryptedNonce is the nonce for decrypting EncryptedFrom.
	EncryptedNonce []byte `json:"encryptedNonce,omitempty"`
	// FromEncrypted indicates whether the peer ID is encrypted.
	FromEncrypted bool `json:"fromEncrypted"`
	// PublicKey is the requester's public key for encrypting the response.
	PublicKey []byte `json:"publicKey"`
	// Signature is the signature by the locator's ephemeral key.
	Signature []byte `json:"signature,omitempty"`
	// Timestamp records when the request was created.
	Timestamp time.Time `json:"timestamp"`
}

// ServiceAnnouncement represents a broadcast announcement of a service's availability.
// Services use this to advertise their presence and capabilities to the network.
type ServiceAnnouncement struct {
	// Type is the message type, always "service-announcement".
	Type string `json:"type"`
	// ServiceKey is the public key identifying the service.
	ServiceKey []byte `json:"serviceKey"`
	// PeerID is the peer ID providing the service.
	PeerID string `json:"peerID"`
	// Data contains custom service-specific data or metadata.
	Data interface{} `json:"data"`
	// Timestamp records when the announcement was created.
	Timestamp time.Time `json:"timestamp"`
	// Signature is the cryptographic signature by the service's private key.
	Signature []byte `json:"signature,omitempty"`
}

// GreetingResponse represents the response from the HTTP greetings endpoint.
// It provides basic node identification and optional service discovery information.
type GreetingResponse struct {
	// PeerID is the libp2p peer ID of the responding node.
	PeerID string `json:"peer_id"`
	// ServiceKey is the public key if this node provides a service.
	ServiceKey []byte `json:"service_key,omitempty"`
	// Data contains custom data provided by the node or its addons.
	Data interface{} `json:"data,omitempty"`
	// Timestamp records when the greeting was generated.
	Timestamp time.Time `json:"timestamp"`
	// Signature is the cryptographic signature of the response payload.
	Signature []byte `json:"signature,omitempty"`
}

// AddonDisclosure represents public-facing information about an addon to include in greetings.
// Addons can use this structure to advertise their presence and capabilities to connecting peers.
type AddonDisclosure struct {
	// Name is the display name for the addon in greetings. If empty, the addon is not disclosed.
	Name string `json:"name"`
	// Version is the addon version formatted as npm dependency versions (e.g., "^1.2.3", "~0.5.0").
	Version string `json:"version,omitempty"`
	// Info contains optional supplementary information provided by the addon.
	Info map[string]interface{} `json:"info,omitempty"`
}

// ConnectionItem represents a tracked peer connection with comprehensive status information.
// It tracks connection lifecycle, HTTP capability, and service associations for each peer.
type ConnectionItem struct {
	// PeerID is the libp2p peer identifier.
	PeerID peer.ID
	// Alias is a unique 4-character shorthand identifier for display purposes.
	Alias string
	// ServiceKeys contains the service public keys this peer provides (one peer may serve multiple services).
	ServiceKeys [][]byte
	// Connected records when the current connection was established.
	Connected time.Time
	// LastActivity records the timestamp of the most recent interaction with this peer.
	LastActivity time.Time
	// LastDisconnect records when the previous connection was terminated, if any.
	LastDisconnect *time.Time
	// Status is the current connection state: "connected", "disconnected", or "connecting".
	Status string
	// ConnectAttempts counts how many connection attempts have been made.
	ConnectAttempts int
	// ConnectionType indicates how the connection was established.
	ConnectionType string
	// HTTPCapable indicates whether this peer has a verified HTTP server.
	HTTPCapable bool
	// BidirectionalHTTP indicates whether bidirectional HTTP communication works.
	BidirectionalHTTP bool
	// LastHTTPTest records when the HTTP capability was last tested.
	LastHTTPTest *time.Time
	// HTTPTestResult is the outcome of the last HTTP test.
	HTTPTestResult string
	// mutex protects concurrent access to the connection item's data.
	mutex sync.RWMutex
	// data stores arbitrary key-value metadata for the connection.
	data map[string]interface{}
}

// PeerOptions contains configurable fields when adding or updating tracked peers.
// It provides a builder-style interface for specifying peer connection attributes.
type PeerOptions struct {
	// ServiceKey is a single service key to associate with the peer (optional).
	ServiceKey []byte
	// ConnectionType indicates how the peer connection was established.
	ConnectionType string
	// HTTPCapable indicates whether the peer has a verified HTTP server.
	HTTPCapable bool
}

// NewManualPeerOptions creates PeerOptions for manually added peers.
// The httpCapable parameter indicates whether the peer has HTTP capability.
func NewManualPeerOptions(httpCapable bool) PeerOptions {
	return PeerOptions{
		ConnectionType: ConnTypeManual,
		HTTPCapable:    httpCapable,
	}
}

// NewServicePeerOptions creates PeerOptions for service-affiliated peers.
// The serviceKey identifies the service, and httpCapable indicates HTTP capability.
func NewServicePeerOptions(serviceKey []byte, httpCapable bool) PeerOptions {
	return PeerOptions{
		ServiceKey:     serviceKey,
		ConnectionType: ConnTypeHTTPVerified,
		HTTPCapable:    httpCapable,
	}
}

// NewBackgroundPeerOptions creates PeerOptions for peers discovered through
// background mechanisms such as DHT or MDNS.
func NewBackgroundPeerOptions() PeerOptions {
	return PeerOptions{
		ConnectionType: ConnTypeBackground,
		HTTPCapable:    false,
	}
}

// NewGossipPeerOptions creates PeerOptions for peers discovered via gossipsub.
// The httpCapable parameter indicates whether the peer has HTTP capability.
func NewGossipPeerOptions(httpCapable bool) PeerOptions {
	return PeerOptions{
		ConnectionType: ConnTypeGossipLookup,
		HTTPCapable:    httpCapable,
	}
}

// ServiceBeaconMode represents the operational mode for service beacon behavior.
// It determines how a service beacon interacts with the network.
type ServiceBeaconMode int

const (
	// ServiceBeaconModeLookup indicates the beacon is actively looking for services.
	ServiceBeaconModeLookup ServiceBeaconMode = iota
	// ServiceBeaconModeAnnounce indicates the beacon is announcing service availability.
	ServiceBeaconModeAnnounce
	// ServiceBeaconModeReplyOnly indicates the beacon only responds to direct queries.
	ServiceBeaconModeReplyOnly
)

// RouteEntry represents a single routing entry that maps an identifier to a target URL.
// It is used for service routing configuration and persistence.
type RouteEntry struct {
	// Identifier is the unique key for this route (typically a service identifier).
	Identifier string `json:"identifier"`
	// URL is the target address for this route.
	URL string `json:"url"`
	// CreatedAt records when this route entry was created.
	CreatedAt time.Time `json:"created_at"`
}

// ServiceRoutingConfig defines routing behavior for a service endpoint.
// It controls how incoming request paths are transformed before forwarding.
type ServiceRoutingConfig struct {
	// RoutePrefix is the prefix to add to incoming paths (e.g., "/api/v1").
	RoutePrefix string `json:"routePrefix"`
	// KeepFullPath determines whether to keep the full path or strip the matched prefix.
	KeepFullPath bool `json:"keepFullPath"`
}

// RouteTable manages service routing entries with thread-safe operations.
// It maintains a mapping of identifiers to URLs and associated routing configurations.
type RouteTable struct {
	// routes maps identifiers to their target URLs.
	routes map[string]string
	// routingConfig maps identifiers to their routing configurations.
	routingConfig map[string]ServiceRoutingConfig
	// mutex protects concurrent access to routes and routingConfig.
	mutex sync.RWMutex
}

// NewRouteTable creates and returns a new empty RouteTable instance.
func NewRouteTable() *RouteTable {
	return &RouteTable{
		routes:        make(map[string]string),
		routingConfig: make(map[string]ServiceRoutingConfig),
	}
}

// AddRoute adds or updates a route mapping the identifier to the specified URL.
func (rt *RouteTable) AddRoute(identifier, url string) {
	rt.mutex.Lock()
	defer rt.mutex.Unlock()
	rt.routes[identifier] = url
}

// AddRouteWithConfig adds or updates a route with an associated routing configuration.
func (rt *RouteTable) AddRouteWithConfig(identifier, url string, config ServiceRoutingConfig) {
	rt.mutex.Lock()
	defer rt.mutex.Unlock()
	rt.routes[identifier] = url
	rt.routingConfig[identifier] = config
}

// GetRoute retrieves the URL for the given identifier. Returns the URL and true if found,
// or an empty string and false if the identifier does not exist.
func (rt *RouteTable) GetRoute(identifier string) (string, bool) {
	rt.mutex.RLock()
	defer rt.mutex.RUnlock()
	url, exists := rt.routes[identifier]
	return url, exists
}

// GetRouteConfig retrieves the routing configuration for the given identifier.
// Returns the configuration and true if found, or an empty configuration and false.
func (rt *RouteTable) GetRouteConfig(identifier string) (ServiceRoutingConfig, bool) {
	rt.mutex.RLock()
	defer rt.mutex.RUnlock()
	config, exists := rt.routingConfig[identifier]
	return config, exists
}

// GetAllRoutes returns a copy of all route mappings in the table.
func (rt *RouteTable) GetAllRoutes() map[string]string {
	rt.mutex.RLock()
	defer rt.mutex.RUnlock()
	routes := make(map[string]string, len(rt.routes))
	for k, v := range rt.routes {
		routes[k] = v
	}
	return routes
}

// LoadRoutes loads multiple routes from a map, typically used when loading from a JSON file.
// Existing routes with the same identifiers will be overwritten.
func (rt *RouteTable) LoadRoutes(routes map[string]string) {
	rt.mutex.Lock()
	defer rt.mutex.Unlock()
	for identifier, url := range routes {
		rt.routes[identifier] = url
	}
}

// FigNode represents a node in the hierarchical key tree for a service.
// Each path node can contain multiple keys to support multiple identities for a path,
// enabling fine-grained access control and service routing.
type FigNode struct {
	// Path is the URL path this node represents in the tree.
	Path string `json:"path"`
	// Keys contains the service keys associated with this path.
	Keys []string `json:"keys"`
	// Children contains child nodes for hierarchical path matching.
	Children []FigNode `json:"children,omitempty"`
	// AllowedTransports optionally restricts which transport protocols are allowed for this path.
	AllowedTransports []string `json:"allowedTransports,omitempty"`
}

// FigFile represents the structure of a fig file used to identify a service and its key tree.
// It provides hierarchical key management for service access control and supports optional
// security features including expiration, peer binding, and multi-signature verification.
// Currently, only root node keys are used for lookups; hierarchical paths are reserved for future use.
type FigFile struct {
	// ServiceAlias is the human-readable alias for the service.
	ServiceAlias string `json:"serviceAlias"`
	// Root is the root node of the service's key tree.
	Root FigNode `json:"root"`
	// Data carries arbitrary metadata for consumers.
	Data json.RawMessage `json:"data,omitempty"`
	// ExpiresAt is the RFC3339 timestamp after which this fig must not be accepted.
	ExpiresAt time.Time `json:"expiresAt,omitempty"`
	// RequesterPeerID binds the fig to a specific requester peer ID.
	RequesterPeerID string `json:"requesterPeerId,omitempty"`
	// Nonce binds the fig to a unique request instance (hex-encoded).
	Nonce string `json:"nonce,omitempty"`
	// Signatures contains hex-encoded signatures over the canonical payload.
	Signatures []string `json:"signatures,omitempty"`
	// RequiredSigners lists authority public keys (hex-encoded) that must sign the fig template.
	RequiredSigners []string `json:"requiredSigners,omitempty"`
	// AuthoritySignatures contains hex-encoded signatures from required authority signers.
	AuthoritySignatures []string `json:"authoritySignatures,omitempty"`
}

// BuildCanonicalPayload constructs a deterministic byte slice for signing and verification.
// The payload includes service alias, expiration, requester peer ID, nonce, sorted root keys,
// and compacted data. This ensures consistent payload generation across all fig file handling.
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
// request-specific details (nonce, requester peer ID) are added. The payload intentionally
// excludes nonce, requester peer ID, authority signatures, and service signatures to avoid
// circular dependencies and enable template signing.
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
// It performs hierarchical path matching, prioritizing exact matches over prefix matches,
// and falls back to Root.Keys if no path-specific match is found.
// Returns an empty slice (not nil) if no keys are available.
func (f FigFile) FindKeysForPath(requestPath string) []string {
	keys, _ := f.FindKeysAndMatchedPath(requestPath)
	return keys
}

// FindKeysAndMatchedPath returns both the keys and the matched path for a given request path.
// This is useful for path stripping in proxies. Returns (keys, matchedPath) where matchedPath
// is the path in the fig tree that matched, or an empty string if no match was found.
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
// matching path. It returns both the keys and the matched path in the fig tree.
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
// matching path using prefix matching. Returns an empty slice if no match is found.
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
// and all descendant nodes in the fig tree.
func (f *FigFile) CollectAllServiceKeys() []string {
	if f.Root.Path == "" && len(f.Root.Keys) == 0 && len(f.Root.Children) == 0 {
		return []string{}
	}
	return f.Root.collectKeysRecursive()
}

// collectKeysRecursive recursively collects all keys from this node and all
// descendant nodes in the tree.
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
// It uses prefix matching where a node's path is a prefix of the request path.
// Returns the matching node and true if found, or nil and false if no match.
func (f *FigFile) FindNodeForPath(requestPath string) (*FigNode, bool) {
	return f.Root.findNodeForPath(requestPath)
}

// findNodeForPath is a recursive helper that searches the fig tree for the most specific
// matching node using prefix matching.
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
