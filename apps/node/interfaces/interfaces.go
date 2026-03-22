// Package interfaces defines the core contracts for the Banyan node application.
// These interfaces provide abstraction layers that enable loose coupling between components,
// allowing implementations to be swapped without affecting dependent code.
//
// The interfaces are organized into several categories:
//   - Core node operations (Node, NodeBuilder, Configuration)
//   - Peer and connection management (PeerManager, DiscoveryManager)
//   - Cryptographic operations (CryptoManager, PrivateKeyLoader)
//   - Service management (ServiceManager, ServiceBeacon)
//   - HTTP handling (HTTPHandler, HTTPServer)
//   - Event broadcasting (EventBroadcaster, WebSocketHub, WebSocketClient)
//   - Data persistence (PeerRepository, ServiceRepository)
//   - Observability (Metrics, Logger)
//
// Implementations of these interfaces can be found in the internal packages.
package interfaces

import (
	"context"
	"net/http"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"

	"banyan/types"
)

// EventBroadcaster defines the contract for broadcasting events to WebSocket clients.
// It implements a pub/sub pattern where events are broadcast to all registered subscribers.
//
// Implementations must be safe for concurrent use by multiple goroutines.
type EventBroadcaster interface {
	// BroadcastEvent sends an event to all registered WebSocket hubs.
	// The event is distributed to all connected WebSocket clients.
	BroadcastEvent(event types.Event)

	// SendEvent creates and broadcasts an event with the specified type and data.
	// This is a convenience method that wraps BroadcastEvent.
	SendEvent(eventType string, data interface{})

	// Subscribe registers a WebSocketHub to receive broadcast events.
	// Multiple hubs can be subscribed simultaneously.
	Subscribe(subscriber WebSocketHub)

	// Unsubscribe removes a WebSocketHub from receiving broadcast events.
	// After unsubscription, the hub will no longer receive events.
	Unsubscribe(subscriber WebSocketHub)

	// GetSubscriberCount returns the current number of subscribed WebSocket hubs.
	GetSubscriberCount() int
}

// PeerManager defines the contract for managing peer connections in the P2P network.
// It handles connection tracking, establishment, lifecycle management, and HTTP capability testing.
//
// Implementations must be safe for concurrent use by multiple goroutines.
type PeerManager interface {
	// AddTrackedPeer adds a peer to the connection tracking system with the specified options.
	// Returns a ConnectionItem that can be used to monitor the peer's status.
	AddTrackedPeer(peerID peer.ID, options types.PeerOptions) *types.ConnectionItem

	// MarkPeerHTTPCapable marks a peer as capable of HTTP communication.
	// If bidirectional is true, the peer supports two-way HTTP connections.
	MarkPeerHTTPCapable(peerID peer.ID, bidirectional bool)

	// MarkHTTPTestResult records the result of an HTTP capability test for a peer.
	// The result string indicates success or the type of failure.
	MarkHTTPTestResult(peerID peer.ID, result string)

	// UpdateConnectionStatus updates the current connection status of a peer.
	// Common status values include "connected", "disconnected", "connecting".
	UpdateConnectionStatus(peerID peer.ID, status string)

	// GetConnectionInfo retrieves the connection information for a specific peer.
	// Returns the ConnectionItem and true if found, nil and false otherwise.
	GetConnectionInfo(peerID peer.ID) (*types.ConnectionItem, bool)

	// GetConnectionsCopy returns a snapshot of all currently tracked connections.
	// The returned map is a copy and safe to modify.
	GetConnectionsCopy() map[peer.ID]*types.ConnectionItem

	// GetPeerByAlias looks up a peer ID by its human-readable alias.
	// Returns the peer ID and true if found, empty string and false otherwise.
	GetPeerByAlias(alias string) (peer.ID, bool)

	// CleanupOldConnections removes stale connection entries from the tracking system.
	// This should be called periodically to prevent memory leaks.
	CleanupOldConnections()

	// ConnectToPeer initiates a connection to the specified peer.
	// Returns an error if the connection cannot be established.
	ConnectToPeer(peerID peer.ID) error

	// ConnectToRequesterDirectly establishes a direct connection to a peer using provided addresses.
	// This bypasses the DHT and uses explicit multiaddresses.
	ConnectToRequesterDirectly(peerID peer.ID, addresses []string)

	// LookupPeer searches for a peer in the network using available discovery mechanisms.
	// If includeFrom is true, the response includes the discoverer's information.
	LookupPeer(targetPeerID string, includeFrom bool) error

	// LookupPeerViaGossipsub broadcasts a peer lookup request over the gossipsub network.
	// This is useful when DHT lookup fails or is disabled.
	LookupPeerViaGossipsub(peerID peer.ID)

	// HandleNewConnection processes a newly established peer connection.
	// This updates internal state and triggers any necessary handshake logic.
	HandleNewConnection(peerID peer.ID)

	// HandleDisconnection processes a peer disconnection event.
	// This updates connection status and performs cleanup.
	HandleDisconnection(peerID peer.ID)

	// CreateBidirectionalConnections attempts to establish bidirectional HTTP connections
	// with all currently connected peers that support it.
	CreateBidirectionalConnections()

	// CreateBidirectionalConnection attempts to establish a bidirectional HTTP connection
	// with a specific peer.
	CreateBidirectionalConnection(peerID peer.ID)

	// TestPeerHTTPCapability checks if a peer supports HTTP-based communication.
	// This is typically done during connection establishment.
	TestPeerHTTPCapability(peerID peer.ID)
}

// CryptoManager defines the contract for cryptographic operations in the node.
// It provides ECDH-based encryption/decryption and service key management.
//
// All encryption methods use ECIES (Elliptic Curve Integrated Encryption Scheme)
// with the curve specified by the host's private key.
type CryptoManager interface {
	// EncryptWithPublicKey encrypts data using ECDH with the recipient's public key.
	// Returns the encrypted data, a nonce for decryption, and an error if encryption fails.
	EncryptWithPublicKey(data []byte, requesterPubKeyBytes []byte) ([]byte, []byte, error)

	// DecryptWithPublicKey decrypts data that was encrypted with the local peer's public key.
	// Requires the same nonce that was returned during encryption.
	DecryptWithPublicKey(encryptedData, nonce []byte, senderPubKeyBytes []byte) ([]byte, error)

	// EncryptWithServiceKey encrypts data using a specific service's private key.
	// This is used for service-to-service communication.
	EncryptWithServiceKey(data []byte, requesterPubKeyBytes []byte, servicePrivKey crypto.PrivKey) ([]byte, []byte, error)

	// DecryptWithLocatorKey decrypts data intended for a locator service.
	// Uses the locator's private key for decryption.
	DecryptWithLocatorKey(encryptedData, nonce []byte, senderServicePubKeyBytes []byte, locatorPrivKey crypto.PrivKey) ([]byte, error)

	// EncryptPeerIDWithServiceKey encrypts a peer ID for secure transmission to a service.
	// The service can decrypt this to learn the requester's peer ID.
	EncryptPeerIDWithServiceKey(peerID string, servicePubKey crypto.PubKey) ([]byte, []byte, error)

	// DecryptPeerIDWithSpecificServiceKey decrypts a peer ID using a specific service key.
	// Used when a service receives an encrypted peer ID from a requester.
	DecryptPeerIDWithSpecificServiceKey(encryptedPeerID, nonce []byte, requesterPubKey []byte, servicePrivKey crypto.PrivKey) (string, error)

	// RegisterServiceKey adds a service private key to the key registry.
	// The keyID is used to reference the key in subsequent operations.
	RegisterServiceKey(keyID string, serviceKey crypto.PrivKey)

	// UnregisterServiceKey removes a service key from the registry.
	// After this, the key can no longer be retrieved by its ID.
	UnregisterServiceKey(keyID string)

	// GetServiceKeyByID retrieves a registered service key by its identifier.
	// Returns nil if no key is registered with the given ID.
	GetServiceKeyByID(keyID string) crypto.PrivKey

	// HasServiceKeyByID checks if a service key exists for the given identifier.
	HasServiceKeyByID(keyID string) bool

	// ListServiceKeys returns all registered service key identifiers.
	ListServiceKeys() []string

	// GetServiceKey returns the default (first registered) service key.
	// Deprecated: Use GetServiceKeyByID for multi-service support.
	GetServiceKey() crypto.PrivKey

	// HasServiceKey checks if any service key is registered.
	HasServiceKey() bool

	// DecryptPeerIDWithServiceKey decrypts a peer ID using the default service key.
	// Deprecated: Use DecryptPeerIDWithSpecificServiceKey for multi-service support.
	DecryptPeerIDWithServiceKey(encryptedPeerID, nonce []byte, requesterPubKey []byte) (string, error)
}

// DiscoveryManager defines the contract for peer discovery operations.
// It supports multiple discovery mechanisms including DHT and mDNS.
type DiscoveryManager interface {
	// StartPeerDiscovery begins all configured peer discovery mechanisms.
	// The connectionManager is used to handle discovered peers.
	StartPeerDiscovery(connectionManager PeerManager)

	// StartDHTDiscovery starts the Kademlia DHT-based peer discovery.
	// This is the primary mechanism for finding peers in large networks.
	StartDHTDiscovery()

	// StartMDNSDiscovery starts local network peer discovery via mDNS.
	// This is useful for development and local network scenarios.
	StartMDNSDiscovery(connectionManager PeerManager)

	// IsDHTEnabled returns true if DHT discovery is active and available.
	IsDHTEnabled() bool

	// GetDiscoveryMethods returns a list of active discovery method names.
	GetDiscoveryMethods() []string

	// HandleGossipMessages processes incoming gossipsub messages for peer discovery.
	// This runs as a continuous message handler loop.
	HandleGossipMessages(cryptoManager CryptoManager, connectionManager PeerManager)

	// ProcessGossipMessage handles a single gossipsub message for peer discovery.
	// Extracts peer information and updates connection state.
	ProcessGossipMessage(msg *pubsub.Message, cryptoManager CryptoManager, connectionManager PeerManager)

	// HandleLookupRequest processes an incoming peer lookup request.
	// Responds with known peer information if available.
	HandleLookupRequest(req *types.LookupRequest, from peer.ID, cryptoManager CryptoManager, connectionManager PeerManager)

	// SendDirectResponse sends a lookup response directly to a specific peer.
	// Used when responding to individual lookup requests.
	SendDirectResponse(peerID peer.ID, response *types.LookupResponse)

	// ProcessLookupResponse handles an incoming lookup response.
	// Updates local peer information based on the response.
	ProcessLookupResponse(resp *types.LookupResponse, connectionManager PeerManager)

	// VerifyResponseSignature validates the cryptographic signature on a lookup response.
	// Returns true if the signature is valid and the response is authentic.
	VerifyResponseSignature(resp *types.LookupResponse) bool
}

// ServiceManager defines the contract for service management operations.
// It handles service beacon lifecycle, service discovery, and service communication.
type ServiceManager interface {
	// CreateServiceBeacon creates a new service beacon with the given private key.
	// The beacon announces the service's availability to the network.
	CreateServiceBeacon(servicePrivKey crypto.PrivKey) (ServiceBeacon, error)

	// CreateServiceBeaconWithMeta creates a service beacon with metadata.
	// The alias provides a human-readable name; figTemplates contain configuration data.
	CreateServiceBeaconWithMeta(servicePrivKey crypto.PrivKey, alias string, figTemplates [][]byte) (ServiceBeacon, error)

	// StartServiceLocator begins listening for service requests as a locator.
	// The mode determines how the locator responds to service lookups.
	StartServiceLocator(servicePubKey crypto.PubKey, mode types.ServiceBeaconMode) error

	// ExtractPeerIDFromServiceRequest retrieves the peer ID from a service lookup request.
	// This is used to identify the requester during service establishment.
	ExtractPeerIDFromServiceRequest(req *types.ServiceLookupRequest) string

	// ConnectToServiceProvider initiates a connection to a discovered service provider.
	ConnectToServiceProvider(peerID peer.ID)

	// SendServiceLookupResponse sends a response to a service lookup request.
	// This is called by service providers to accept connections.
	SendServiceLookupResponse(req *types.ServiceLookupRequest)

	// GetServiceBeacon returns the first registered service beacon.
	// Deprecated: Use ListServiceBeacons() for multi-beacon support.
	GetServiceBeacon() ServiceBeacon

	// ListServiceBeacons returns all registered service beacons.
	// Use this instead of GetServiceBeacon() when multiple services are running.
	ListServiceBeacons() []ServiceBeacon

	// SendConnectToPeerResponse sends a connection response to a peer by ID string.
	SendConnectToPeerResponse(peerIDStr string)

	// SendDirectServiceResponse sends a service response directly to a specific peer.
	SendDirectServiceResponse(peerID peer.ID, response *types.ServiceResponse)

	// ProcessServiceResponse handles an incoming service response.
	// This completes the service discovery handshake.
	ProcessServiceResponse(resp *types.ServiceResponse)

	// TestServicePeerBidirectionality tests if a service peer supports bidirectional connections.
	TestServicePeerBidirectionality(peerID peer.ID, serviceKey []byte)

	// ConnectToServiceProviderDirectly establishes a direct connection using provided addresses.
	// Bypasses discovery mechanisms and uses explicit addresses.
	ConnectToServiceProviderDirectly(peerID peer.ID, addresses []string)
}

// ServiceBeacon defines the contract for a service beacon that announces service availability.
// Beacons are used by service providers to advertise their presence on the network.
type ServiceBeacon interface {
	// Start begins the beacon's announcement loop.
	// The beacon will periodically broadcast its presence to the network.
	Start() error

	// Stop halts the beacon and cleans up resources.
	Stop() error

	// GetServiceKey returns the private key associated with this service.
	GetServiceKey() crypto.PrivKey

	// GetMode returns the current operating mode of the beacon.
	GetMode() types.ServiceBeaconMode

	// SetMode changes the beacon's operating mode.
	SetMode(mode types.ServiceBeaconMode)

	// GetPeers returns the list of peers currently connected to this service.
	GetPeers() []peer.ID

	// GetAlias returns the human-readable name for this service.
	GetAlias() string

	// GetFigTemplates returns the configuration templates associated with this service.
	// These templates are shared with connecting peers.
	GetFigTemplates() [][]byte
}

// HTTPHandler defines the contract for HTTP request handling in the P2P network.
// Only peer-to-peer safe endpoints are exposed through this interface.
//
// Security note: Management endpoints like HandleConnectToPeer are intentionally
// excluded and should only be accessible via the management server on localhost.
type HTTPHandler interface {
	// StartP2PProtocolServer starts the HTTP server for P2P protocol communication.
	StartP2PProtocolServer()

	// HandlePing responds to peer health check requests.
	// Returns a simple response indicating the node is responsive.
	HandlePing(w http.ResponseWriter, r *http.Request)

	// HandleGreetings handles the initial greeting exchange between peers.
	// Returns peer information and capabilities.
	HandleGreetings(w http.ResponseWriter, r *http.Request)

	// HandleServiceFigs returns service configuration templates to requesting peers.
	HandleServiceFigs(w http.ResponseWriter, r *http.Request)

	// HandleStatus returns the current node status and statistics.
	HandleStatus(w http.ResponseWriter, r *http.Request)

	// HandleConnectionsStatus returns information about current peer connections.
	HandleConnectionsStatus(w http.ResponseWriter, r *http.Request)

	// HandleLibp2pHTTPProxy handles HTTP requests proxied through libp2p streams.
	HandleLibp2pHTTPProxy(w http.ResponseWriter, r *http.Request)

	// HandleRouter is the main routing handler for P2P HTTP requests.
	HandleRouter(w http.ResponseWriter, r *http.Request)

	// MountP2P dynamically registers a new P2P endpoint handler at the specified path.
	// This allows addons to extend the P2P API.
	MountP2P(path string, h func(http.ResponseWriter, *http.Request))

	// SetAddonDisclosureProvider sets a provider function that returns addon disclosures.
	// These disclosures are included in greeting responses.
	SetAddonDisclosureProvider(provider func() []types.AddonDisclosure)
}

// HTTPServer defines the contract for the management API server.
// This server provides administrative interfaces and WebSocket support.
type HTTPServer interface {
	// BroadcastEvent sends an event to all connected WebSocket clients.
	BroadcastEvent(event types.Event)

	// Close gracefully shuts down the HTTP server and releases resources.
	Close() error

	// GetNode returns the Node instance associated with this server.
	GetNode() Node
}

// WebSocketHub defines the contract for WebSocket event distribution.
// Hubs manage client connections and broadcast events to all connected clients.
type WebSocketHub interface {
	// BroadcastEvent sends an event to all connected WebSocket clients.
	BroadcastEvent(event types.Event)

	// NewClient creates a new WebSocket client from a connection.
	// The client is not automatically registered; call Register to add it.
	NewClient(conn interface{}) WebSocketClient

	// Register adds a client to the hub for event distribution.
	Register(client WebSocketClient)

	// Unregister removes a client from the hub.
	// The client will no longer receive broadcast events.
	Unregister(client WebSocketClient)

	// Run starts the hub's event processing loop.
	// This should be called in a separate goroutine.
	Run()
}

// WebSocketClient defines the contract for individual WebSocket connections.
// Each client represents a single connected WebSocket user.
type WebSocketClient interface {
	// GetID returns the unique identifier for this client connection.
	GetID() string

	// GetSendChannel returns the channel used to send events to this client.
	// Events written to this channel are transmitted over the WebSocket.
	GetSendChannel() chan types.Event

	// GetConnection returns the underlying WebSocket connection object.
	// The return type is interface{} to support different WebSocket implementations.
	GetConnection() interface{}
}

// Node defines the contract for the main libp2p node.
// It provides access to all core functionality and manager components.
type Node interface {
	// Start initializes and starts all node components.
	// This includes networking, discovery, and protocol handlers.
	Start() error

	// Close gracefully shuts down the node and all its components.
	Close()

	// GetHost returns the underlying libp2p host.
	// The host provides direct access to libp2p networking functionality.
	GetHost() host.Host

	// GetConnectionManager returns the peer connection manager.
	GetConnectionManager() PeerManager

	// GetServiceManager returns the service management interface.
	GetServiceManager() ServiceManager

	// GetHTTPHandler returns the P2P HTTP protocol handler.
	GetHTTPHandler() HTTPHandler

	// GetDiscoveryManager returns the peer discovery manager.
	GetDiscoveryManager() DiscoveryManager

	// GetHTTPTransport returns the HTTP transport for making peer-to-peer HTTP requests.
	GetHTTPTransport() *http.Transport

	// SetHTTPServer associates an HTTP server with this node.
	// The server provides management and WebSocket interfaces.
	SetHTTPServer(server HTTPServer)

	// SendEvent broadcasts an event to all event subscribers.
	SendEvent(eventType string, data interface{})
}

// Configuration defines the contract for node configuration.
// Implementations may load configuration from files, environment variables, or other sources.
type Configuration interface {
	// GetPrivKeyFile returns the path to the private key file.
	// Returns nil if no key file is specified (a new key will be generated).
	GetPrivKeyFile() *string

	// GetServicesDir returns the directory containing service key files.
	// Each file in this directory should contain a serialized service private key.
	GetServicesDir() *string

	// GetDisableDHT returns true if DHT discovery should be disabled.
	GetDisableDHT() *bool

	// GetNoAnnounce returns true if the node should not announce itself to the network.
	GetNoAnnounce() *bool

	// GetListenMultiaddr returns the multiaddress the node should listen on.
	GetListenMultiaddr() *string

	// GetNATTraversal returns true if NAT traversal (UPnP/NAT-PMP) should be enabled.
	GetNATTraversal() *bool

	// Validate checks the configuration for errors and inconsistencies.
	// Returns an error describing the first validation failure.
	Validate() error
}

// PeerRepository defines the contract for peer data persistence.
// Implementations may store peer data in databases, files, or memory.
type PeerRepository interface {
	// SavePeer stores peer information with the given options.
	SavePeer(peerID peer.ID, options types.PeerOptions) error

	// GetPeer retrieves stored information for a specific peer.
	// Returns an error if the peer is not found.
	GetPeer(peerID peer.ID) (*types.ConnectionItem, error)

	// GetAllPeers retrieves all stored peer information.
	GetAllPeers() (map[peer.ID]*types.ConnectionItem, error)

	// DeletePeer removes a peer from the repository.
	DeletePeer(peerID peer.ID) error
}

// ServiceRepository defines the contract for service data persistence.
// Used to store mappings between service keys and provider peer IDs.
type ServiceRepository interface {
	// SaveService registers a peer as a provider for a specific service.
	SaveService(serviceKey []byte, peerID peer.ID) error

	// FindServiceProviders returns all peers that provide a specific service.
	FindServiceProviders(serviceKey []byte) ([]peer.ID, error)

	// DeleteService removes a peer's registration for a specific service.
	DeleteService(serviceKey []byte, peerID peer.ID) error
}

// PrivateKeyLoader defines the contract for loading cryptographic private keys.
// Supports loading from files or raw byte data.
type PrivateKeyLoader interface {
	// LoadFromFile reads and deserializes a private key from a file.
	LoadFromFile(filename string) (crypto.PrivKey, error)

	// LoadFromBytes deserializes a private key from raw byte data.
	LoadFromBytes(data []byte) (crypto.PrivKey, error)
}

// Metrics defines the contract for collecting and reporting system metrics.
// Implementations may export metrics to Prometheus, StatsD, or other systems.
type Metrics interface {
	// IncrementCounter increments a counter metric by 1.
	// Labels provide dimensional data for filtering and grouping.
	IncrementCounter(name string, labels map[string]string)

	// SetGauge sets a gauge metric to a specific value.
	// Gauges represent point-in-time values like connection count.
	SetGauge(name string, value float64, labels map[string]string)

	// RecordHistogram records an observation in a histogram metric.
	// Histograms are useful for latency and size distributions.
	RecordHistogram(name string, value float64, labels map[string]string)

	// GetMetrics returns all current metric values as a map.
	// The format depends on the implementation.
	GetMetrics() map[string]interface{}
}

// Logger defines the contract for structured logging.
// Implementations may output to various destinations with different formats.
type Logger interface {
	// Debug logs a message at debug level.
	// Debug messages are for detailed diagnostic information.
	Debug(msg string, fields map[string]interface{})

	// Info logs a message at info level.
	// Info messages are for general operational information.
	Info(msg string, fields map[string]interface{})

	// Warn logs a message at warning level.
	// Warning messages indicate potential issues that don't prevent operation.
	Warn(msg string, fields map[string]interface{})

	// Error logs a message at error level.
	// Error messages indicate failures that affect operation.
	Error(msg string, fields map[string]interface{})

	// Fatal logs a message at fatal level and terminates the program.
	// Fatal messages indicate unrecoverable errors.
	Fatal(msg string, fields map[string]interface{})
}

// NodeBuilder defines the contract for constructing Node instances.
// Implements the builder pattern for flexible node configuration.
//
// Example usage:
//
//	node, err := builder.
//	    WithConfiguration(config).
//	    WithLogger(logger).
//	    Build(ctx)
type NodeBuilder interface {
	// WithConfiguration sets the configuration for the node.
	// Returns the builder for method chaining.
	WithConfiguration(config Configuration) NodeBuilder

	// WithEventBroadcaster sets the event broadcaster for the node.
	// Returns the builder for method chaining.
	WithEventBroadcaster(broadcaster EventBroadcaster) NodeBuilder

	// WithLogger sets the logger for the node.
	// Returns the builder for method chaining.
	WithLogger(logger Logger) NodeBuilder

	// WithMetrics sets the metrics collector for the node.
	// Returns the builder for method chaining.
	WithMetrics(metrics Metrics) NodeBuilder

	// WithKeyLoader sets the private key loader for the node.
	// Returns the builder for method chaining.
	WithKeyLoader(loader PrivateKeyLoader) NodeBuilder

	// Build creates and returns a new Node instance with the configured options.
	// The context is used for cancellation during node initialization.
	Build(ctx context.Context) (Node, error)
}
