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

// EventBroadcaster defines the contract for broadcasting events
type EventBroadcaster interface {
	BroadcastEvent(event types.Event)
	SendEvent(eventType string, data interface{})
	Subscribe(subscriber WebSocketHub)
	Unsubscribe(subscriber WebSocketHub)
	GetSubscriberCount() int
}

// PeerManager defines the contract for managing peer connections
type PeerManager interface {
	// Connection tracking
	AddTrackedPeer(peerID peer.ID, options types.PeerOptions) *types.ConnectionItem
	MarkPeerHTTPCapable(peerID peer.ID, bidirectional bool)
	MarkHTTPTestResult(peerID peer.ID, result string)
	UpdateConnectionStatus(peerID peer.ID, status string)
	GetConnectionInfo(peerID peer.ID) (*types.ConnectionItem, bool)
	GetConnectionsCopy() map[peer.ID]*types.ConnectionItem
	GetPeerByAlias(alias string) (peer.ID, bool)
	CleanupOldConnections()

	// Connection establishment
	ConnectToPeer(peerID peer.ID) error
	ConnectToRequesterDirectly(peerID peer.ID, addresses []string)
	LookupPeer(targetPeerID string, includeFrom bool) error
	LookupPeerViaGossipsub(peerID peer.ID)

	// Connection management
	HandleNewConnection(peerID peer.ID)
	HandleDisconnection(peerID peer.ID)
	CreateBidirectionalConnections()
	CreateBidirectionalConnection(peerID peer.ID)

	// Bidirectional testing
	TestPeerHTTPCapability(peerID peer.ID)
}

// CryptoManager defines the contract for cryptographic operations
type CryptoManager interface {
	// Data encryption with public key ECDH
	EncryptWithPublicKey(data []byte, requesterPubKeyBytes []byte) ([]byte, []byte, error)
	DecryptWithPublicKey(encryptedData, nonce []byte, senderPubKeyBytes []byte) ([]byte, error)
	EncryptWithServiceKey(data []byte, requesterPubKeyBytes []byte, servicePrivKey crypto.PrivKey) ([]byte, []byte, error)
	DecryptWithLocatorKey(encryptedData, nonce []byte, senderServicePubKeyBytes []byte, locatorPrivKey crypto.PrivKey) ([]byte, error)

	// Service-specific encryption with specific service key
	EncryptPeerIDWithServiceKey(peerID string, servicePubKey crypto.PubKey) ([]byte, []byte, error)
	DecryptPeerIDWithSpecificServiceKey(encryptedPeerID, nonce []byte, requesterPubKey []byte, servicePrivKey crypto.PrivKey) (string, error)

	// Multiple service key management
	RegisterServiceKey(keyID string, serviceKey crypto.PrivKey)
	UnregisterServiceKey(keyID string)
	GetServiceKeyByID(keyID string) crypto.PrivKey
	HasServiceKeyByID(keyID string) bool
	ListServiceKeys() []string

	// Legacy single service key support (for backward compatibility)
	GetServiceKey() crypto.PrivKey
	HasServiceKey() bool
	DecryptPeerIDWithServiceKey(encryptedPeerID, nonce []byte, requesterPubKey []byte) (string, error)
}

// DiscoveryManager defines the contract for peer discovery operations
type DiscoveryManager interface {
	StartPeerDiscovery(connectionManager PeerManager)
	StartDHTDiscovery()
	StartMDNSDiscovery(connectionManager PeerManager)
	IsDHTEnabled() bool
	GetDiscoveryMethods() []string
	HandleGossipMessages(cryptoManager CryptoManager, connectionManager PeerManager)
	ProcessGossipMessage(msg *pubsub.Message, cryptoManager CryptoManager, connectionManager PeerManager)
	HandleLookupRequest(req *types.LookupRequest, from peer.ID, cryptoManager CryptoManager, connectionManager PeerManager)
	SendDirectResponse(peerID peer.ID, response *types.LookupResponse)
	ProcessLookupResponse(resp *types.LookupResponse, connectionManager PeerManager)
	VerifyResponseSignature(resp *types.LookupResponse) bool
}

// ServiceManager defines the contract for service management operations
type ServiceManager interface {
	CreateServiceBeacon(servicePrivKey crypto.PrivKey) (ServiceBeacon, error)
	CreateServiceBeaconWithMeta(servicePrivKey crypto.PrivKey, alias string, figTemplates [][]byte) (ServiceBeacon, error)
	StartServiceLocator(servicePubKey crypto.PubKey, mode types.ServiceBeaconMode) error
	ExtractPeerIDFromServiceRequest(req *types.ServiceLookupRequest) string
	ConnectToServiceProvider(peerID peer.ID)
	SendServiceLookupResponse(req *types.ServiceLookupRequest)
	// DEPRECATED: GetServiceBeacon only returns the first beacon. Use ListServiceBeacons() for multi-beacon support.
	GetServiceBeacon() ServiceBeacon
	// Multi-beacon support - use this instead of GetServiceBeacon()
	ListServiceBeacons() []ServiceBeacon
	SendConnectToPeerResponse(peerIDStr string)
	SendDirectServiceResponse(peerID peer.ID, response *types.ServiceResponse)
	ProcessServiceResponse(resp *types.ServiceResponse)
	TestServicePeerBidirectionality(peerID peer.ID, serviceKey []byte)
	ConnectToServiceProviderDirectly(peerID peer.ID, addresses []string)
}

// ServiceBeacon defines the contract for service beacons
type ServiceBeacon interface {
	Start() error
	Stop() error
	GetServiceKey() crypto.PrivKey
	GetMode() types.ServiceBeaconMode
	SetMode(mode types.ServiceBeaconMode)
	GetPeers() []peer.ID
	// Metadata for alias and fig templates associated to this beacon
	GetAlias() string
	GetFigTemplates() [][]byte
}

// HTTPHandler defines the contract for HTTP request handling (peer-to-peer safe endpoints only)
type HTTPHandler interface {
	StartP2PProtocolServer()
	HandlePing(w http.ResponseWriter, r *http.Request)
	HandleGreetings(w http.ResponseWriter, r *http.Request)
	HandleServiceFigs(w http.ResponseWriter, r *http.Request)
	HandleStatus(w http.ResponseWriter, r *http.Request)
	HandleConnectionsStatus(w http.ResponseWriter, r *http.Request)
	HandleLibp2pHTTPProxy(w http.ResponseWriter, r *http.Request)
	HandleRouter(w http.ResponseWriter, r *http.Request)
	// NOTE: HandleConnectToPeer is intentionally NOT in this interface
	// It should only be accessible via management server (localhost only)
	// Dynamic mount support for addon endpoints
	MountP2P(path string, h func(http.ResponseWriter, *http.Request))
	// SetAddonDisclosureProvider sets a provider function returning addon disclosures for greetings
	SetAddonDisclosureProvider(provider func() []types.AddonDisclosure)
}

// HTTPServer defines the contract for the management API server
type HTTPServer interface {
	BroadcastEvent(event types.Event)
	Close() error
	GetNode() Node
}

// WebSocketHub defines the contract for WebSocket event broadcasting
type WebSocketHub interface {
	BroadcastEvent(event types.Event)
	NewClient(conn interface{}) WebSocketClient
	Register(client WebSocketClient)
	Unregister(client WebSocketClient)
	Run()
}

// WebSocketClient defines the contract for WebSocket clients
type WebSocketClient interface {
	GetID() string
	GetSendChannel() chan types.Event
	GetConnection() interface{}
}

// Node defines the contract for the main libp2p node
type Node interface {
	// Core node operations
	Start() error
	Close()
	GetHost() host.Host

	// Manager access
	GetConnectionManager() PeerManager
	GetServiceManager() ServiceManager
	GetHTTPHandler() HTTPHandler
	GetDiscoveryManager() DiscoveryManager
	GetHTTPTransport() *http.Transport

	// HTTP server management
	SetHTTPServer(server HTTPServer)

	// Event system
	SendEvent(eventType string, data interface{})
}

// Configuration defines the contract for node configuration
type Configuration interface {
	GetPrivKeyFile() *string
	GetServicesDir() *string
	GetDisableDHT() *bool
	GetNoAnnounce() *bool
	GetListenMultiaddr() *string
	GetNATTraversal() *bool
	Validate() error
}

// Repository interfaces for potential future data persistence
type PeerRepository interface {
	SavePeer(peerID peer.ID, options types.PeerOptions) error
	GetPeer(peerID peer.ID) (*types.ConnectionItem, error)
	GetAllPeers() (map[peer.ID]*types.ConnectionItem, error)
	DeletePeer(peerID peer.ID) error
}

// ServiceRepository for service-related data
type ServiceRepository interface {
	SaveService(serviceKey []byte, peerID peer.ID) error
	FindServiceProviders(serviceKey []byte) ([]peer.ID, error)
	DeleteService(serviceKey []byte, peerID peer.ID) error
}

// PrivateKeyLoader defines the contract for loading private keys
type PrivateKeyLoader interface {
	LoadFromFile(filename string) (crypto.PrivKey, error)
	LoadFromBytes(data []byte) (crypto.PrivKey, error)
}

// Metrics defines the contract for system metrics
type Metrics interface {
	IncrementCounter(name string, labels map[string]string)
	SetGauge(name string, value float64, labels map[string]string)
	RecordHistogram(name string, value float64, labels map[string]string)
	GetMetrics() map[string]interface{}
}

// Logger defines the contract for logging
type Logger interface {
	Debug(msg string, fields map[string]interface{})
	Info(msg string, fields map[string]interface{})
	Warn(msg string, fields map[string]interface{})
	Error(msg string, fields map[string]interface{})
	Fatal(msg string, fields map[string]interface{})
}

// NodeBuilder defines the contract for building nodes
type NodeBuilder interface {
	WithConfiguration(config Configuration) NodeBuilder
	WithEventBroadcaster(broadcaster EventBroadcaster) NodeBuilder
	WithLogger(logger Logger) NodeBuilder
	WithMetrics(metrics Metrics) NodeBuilder
	WithKeyLoader(loader PrivateKeyLoader) NodeBuilder
	Build(ctx context.Context) (Node, error)
}
