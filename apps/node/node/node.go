// Package node provides the core Banyan network node implementation.
// It wraps a libp2p host with additional capabilities including:
//   - Distributed Hash Table (DHT) for peer discovery
//   - GossipSub for pubsub messaging
//   - HTTP proxy capabilities over libp2p
//   - Service beacon functionality
//   - TCP tunnel support
//   - Event broadcasting
//
// The Node struct is the main entry point for creating and managing a Banyan node.
// Use NewNode to create a new node instance with the desired configuration.
package node

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sync/atomic"

	"github.com/libp2p/go-libp2p"
	p2phttp "github.com/libp2p/go-libp2p-http"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	connectionPkg "banyan/connection"
	cryptoPkg "banyan/crypto"
	discoveryPkg "banyan/discovery"
	eventsPkg "banyan/events"
	"banyan/interfaces"
	httpPkg "banyan/libp2p-http"
	servicePkg "banyan/service"
	tunnelPkg "banyan/tunnel"
	"banyan/types"
)

// HTTPServer defines the interface for the management API server.
// It provides event broadcasting and graceful shutdown capabilities.
// This interface is kept for backwards compatibility; new code should
// use interfaces.HTTPServer instead.
type HTTPServer interface {
	// BroadcastEvent sends an event to all connected clients.
	BroadcastEvent(event types.Event)
	// Close shuts down the server and releases resources.
	Close() error
}

// Config holds all configuration options for a Node.
// All fields are pointers to allow distinguishing between "not set" and "set to zero value".
type Config struct {
	// PrivKeyFile is the path to a PEM file containing the node's private key.
	PrivKeyFile *string
	// ServicesDir is the directory containing services.json and per-service subfolders.
	ServicesDir *string
	// FigsDir is the directory containing fig files.
	FigsDir *string
	// DisableDHT disables DHT peer discovery and lookup when true.
	DisableDHT *bool
	// NoAnnounce prevents the node from announcing via DHT or pubsub when true.
	NoAnnounce *bool
	// ListenMultiaddr is the multiaddr the node listens on (e.g., "/ip4/0.0.0.0/tcp/0").
	ListenMultiaddr *string
	// NATTraversal enables NAT traversal features including hole punching and auto-relay.
	NATTraversal *bool
	// NoCrypto disables encryption on pubsub messages when true.
	NoCrypto *bool
	// AnonLookups allows anonymous lookups and responses when true.
	AnonLookups *bool
	// RouterConfig is the path to a JSON file containing URL routing configuration.
	RouterConfig *string
	// Bootstraps is a list of bootstrap peer multiaddrs for DHT initialization.
	Bootstraps []string
	// AllowExpiredFigs allows loading expired fig files when true (security risk).
	AllowExpiredFigs *bool
	// AllowInsecureFigs allows loading fig files with invalid signatures when true (security risk).
	AllowInsecureFigs *bool
	// BeaconIncludePeerIDAnnouncements includes the beacon's PeerID in public announcements.
	BeaconIncludePeerIDAnnouncements *bool
	// BeaconPeerIDDirectOnly only includes the beacon's PeerID in direct replies to locators.
	BeaconPeerIDDirectOnly *bool
	// OverrideTransportRestrictions allows connections even if transport doesn't match fig restrictions.
	OverrideTransportRestrictions *bool
	// TunnelEnabled enables the TCP tunnel protocol handler.
	TunnelEnabled *bool
	// TunnelAuthToken is the auth token required for tunnel API endpoints.
	TunnelAuthToken *string
	// TunnelAllowedTargets is the list of allowed target hosts for outbound tunnels.
	TunnelAllowedTargets []string
	// AllowUnsafeServiceKeyInjection allows manual service key injection via API (security risk, for testing only).
	AllowUnsafeServiceKeyInjection *bool
}

// Node represents a Banyan libp2p node with HTTP proxy capabilities.
// It encapsulates all components needed for peer-to-peer communication,
// service discovery, and HTTP request handling.
type Node struct {
	// Core libp2p components
	host          host.Host
	ctx           context.Context
	dht           *dht.IpfsDHT
	pubsub        *pubsub.PubSub
	gossipTopic   *pubsub.Topic
	gossipSub     *pubsub.Subscription
	httpListener  net.Listener
	httpTransport *http.Transport
	httpServer    HTTPServer

	// Managers (using interfaces for decoupling)
	connectionManager interfaces.PeerManager
	cryptoManager     interfaces.CryptoManager
	discoveryManager  interfaces.DiscoveryManager
	serviceManager    interfaces.ServiceManager
	httpHandler       interfaces.HTTPHandler
	tunnelHandler     *tunnelPkg.Handler

	// Service-specific components
	serviceBeacon interfaces.ServiceBeacon // legacy single beacon (first)

	// Event broadcasting
	eventBroadcaster interfaces.EventBroadcaster

	// Router table for URL routing
	routeTable *types.RouteTable

	// NAT traversal status tracking
	natReachability atomic.Int32 // stores network.Reachability values
	hasRelayAddr    atomic.Bool
	dhtReady        atomic.Bool

	// Configuration
	config *Config

	// key loader for services
	keyLoader PrivateKeyLoader
}

// PrivateKeyLoader is a function type for loading private keys from PEM files.
// It takes a filename and returns a libp2p private key or an error.
type PrivateKeyLoader func(filename string) (crypto.PrivKey, error)

// NewNode creates a new Banyan node with the specified configuration.
// It initializes all libp2p components including the host, DHT, pubsub,
// and various managers for connections, discovery, services, and HTTP handling.
//
// The keyLoader parameter is used to load private keys for the node identity
// and for service beacons. If config.PrivKeyFile is set, it will be used
// to load the node's identity key.
//
// Returns an error if the libp2p host, DHT, or pubsub initialization fails.
func NewNode(ctx context.Context, config *Config, keyLoader PrivateKeyLoader) (*Node, error) {
	var opts []libp2p.Option

	// Add listening addresses
	if config.ListenMultiaddr != nil && *config.ListenMultiaddr != "" {
		opts = append(opts, libp2p.ListenAddrStrings(*config.ListenMultiaddr))
	} else {
		opts = append(opts, libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"))
	}

	// Load custom private key if provided
	if config.PrivKeyFile != nil && *config.PrivKeyFile != "" {
		privKey, err := keyLoader(*config.PrivKeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load private key: %w", err)
		}
		opts = append(opts, libp2p.Identity(privKey))
	}

	// Configure NAT traversal if enabled
	var relayDHT *dht.IpfsDHT
	var relayHost host.Host
	if config.NATTraversal != nil && *config.NATTraversal {
		peerSource := func(ctx context.Context, num int) <-chan peer.AddrInfo {
			ch := make(chan peer.AddrInfo, num)
			go func() {
				defer close(ch)
				if relayDHT == nil || relayHost == nil {
					return
				}
				for _, p := range relayDHT.RoutingTable().ListPeers() {
					addrs := relayHost.Peerstore().Addrs(p)
					if len(addrs) == 0 {
						continue
					}
					select {
					case ch <- peer.AddrInfo{ID: p, Addrs: addrs}:
					case <-ctx.Done():
						return
					}
				}
			}()
			return ch
		}

		opts = append(opts,
			libp2p.EnableHolePunching(),
			libp2p.EnableAutoRelayWithPeerSource(peerSource),
			libp2p.NATPortMap(),
		)
		fmt.Println("NAT traversal enabled: hole punching + auto-relay + UPnP")
	}

	// Create libp2p host
	h, err := libp2p.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create libp2p host: %w", err)
	}
	relayHost = h

	// Initialize DHT conditionally
	var kademliaDHT *dht.IpfsDHT
	disableDHT := config.DisableDHT != nil && *config.DisableDHT
	if !disableDHT {
		kademliaDHT, err = dht.New(ctx, h)
		if err != nil {
			return nil, fmt.Errorf("failed to create DHT: %w", err)
		}
		relayDHT = kademliaDHT

		// Determine which bootstrap peers to use
		var bootstrapPeers []peer.AddrInfo
		if len(config.Bootstraps) > 0 {
			// Use custom bootstrap peers from config
			fmt.Printf("\n=== Using Custom Bootstrap Peers ===\n")
			for _, addrStr := range config.Bootstraps {
				maddr, err := multiaddr.NewMultiaddr(addrStr)
				if err != nil {
					fmt.Printf("Warning: Invalid bootstrap multiaddr %s: %v\n", addrStr, err)
					continue
				}

				peerInfo, err := peer.AddrInfoFromP2pAddr(maddr)
				if err != nil {
					fmt.Printf("Warning: Could not parse peer info from %s: %v\n", addrStr, err)
					continue
				}

				bootstrapPeers = append(bootstrapPeers, *peerInfo)
			}
			fmt.Printf("Loaded %d custom bootstrap peers\n", len(bootstrapPeers))
		} else {
			// Use default bootstrap peers
			bootstrapPeers = dht.GetDefaultBootstrapPeerAddrInfos()
			fmt.Printf("\n=== Using Default Bootstrap Peers ===\n")
		}

		// DEBUG: Log the bootstrap peers being used
		fmt.Printf("\n=== DHT Bootstrap Debug Info ===\n")
		fmt.Printf("Using %d bootstrap peers:\n", len(bootstrapPeers))
		for i, peerInfo := range bootstrapPeers {
			fmt.Printf("  [%d] ID: %s\n", i, peerInfo.ID.String())
			for _, addr := range peerInfo.Addrs {
				fmt.Printf("      Addr: %s\n", addr.String())
			}
		}

		// DEBUG: Log supported protocols
		fmt.Printf("\nNode supported protocols: %v\n", h.Mux().Protocols())

		// DEBUG: Test connectivity to each bootstrap peer
		fmt.Printf("\n=== Testing Bootstrap Peer Connectivity ===\n")
		successCount := 0
		for i, peerInfo := range bootstrapPeers {
			fmt.Printf("[%d] Testing connection to %s...\n", i, peerInfo.ID.String())

			testCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := h.Connect(testCtx, peerInfo)
			cancel()

			if err != nil {
				fmt.Printf("    ❌ FAILED: %v\n", err)
			} else {
				fmt.Printf("    ✓ SUCCESS: Connected to %s\n", peerInfo.ID.String())
				successCount++
				// Disconnect to avoid keeping test connections
				h.Network().ClosePeer(peerInfo.ID)
			}
		}
		fmt.Printf("\nConnectivity Summary: %d/%d bootstrap peers reachable\n", successCount, len(bootstrapPeers))
		fmt.Printf("================================\n\n")

		// Now proceed with actual bootstrap
		fmt.Println("Starting DHT bootstrap process...")
		if err = kademliaDHT.Bootstrap(ctx); err != nil {
			return nil, fmt.Errorf("failed to bootstrap DHT: %w", err)
		}
	}

	// Initialize PubSub (GossipSub)
	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		return nil, fmt.Errorf("failed to create pubsub: %w", err)
	}

	// Join the gossipsub topic
	const gossipSubTopic = "peer-discovery"
	topic, err := ps.Join(gossipSubTopic)
	if err != nil {
		return nil, fmt.Errorf("failed to join topic: %w", err)
	}

	// Subscribe to the topic
	sub, err := topic.Subscribe()
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to topic: %w", err)
	}

	// Create HTTP transport for libp2p HTTP client
	httpTransport := &http.Transport{}
	httpTransport.RegisterProtocol("libp2p", p2phttp.NewTransport(h))

	// Determine configuration flags
	noCrypto := config.NoCrypto != nil && *config.NoCrypto
	anonLookups := config.AnonLookups != nil && *config.AnonLookups

	// Initialize route table
	routeTable := types.NewRouteTable()

	// Load routes from config file if provided
	if config.RouterConfig != nil && *config.RouterConfig != "" {
		if err := loadRouteConfig(*config.RouterConfig, routeTable); err != nil {
			return nil, fmt.Errorf("failed to load router config: %w", err)
		}
	}

	// Create the node
	node := &Node{
		host:          h,
		ctx:           ctx,
		dht:           kademliaDHT,
		pubsub:        ps,
		gossipTopic:   topic,
		gossipSub:     sub,
		httpTransport: httpTransport,
		routeTable:    routeTable,
		config:        config,
		keyLoader:     keyLoader,
	}

	// Initialize event broadcaster first - create a real broadcaster for the managers
	node.eventBroadcaster = eventsPkg.NewBroadcaster(h.ID().String())

	// Initialize managers with interfaces
	node.connectionManager = connectionPkg.NewManager(h, ctx, httpTransport, kademliaDHT, topic, node.eventBroadcaster)

	// Create crypto manager (no default service key; per-service keys will be loaded later)
	node.cryptoManager = cryptoPkg.NewManager(h, nil)

	// Create discovery manager with crypto and anon lookup settings
	node.discoveryManager = discoveryPkg.NewManager(h, ctx, kademliaDHT, ps, node.eventBroadcaster, disableDHT, noCrypto, anonLookups, httpTransport)

	// Create service manager with crypto setting - use function wrapper for event sender
	eventSenderFunc := func(eventType string, data interface{}) {
		node.SendEvent(eventType, data)
	}
	// Beacon flags default values
	includePeerID := config.BeaconIncludePeerIDAnnouncements != nil && *config.BeaconIncludePeerIDAnnouncements
	peerIDDirectOnly := config.BeaconPeerIDDirectOnly != nil && *config.BeaconPeerIDDirectOnly
	overrideTransport := config.OverrideTransportRestrictions != nil && *config.OverrideTransportRestrictions
	node.serviceManager = servicePkg.NewManager(h, ctx, ps, node.cryptoManager, node.connectionManager, httpTransport, eventSenderFunc, noCrypto, includePeerID, peerIDDirectOnly, overrideTransport)

	// Create HTTP handler
	node.httpHandler = httpPkg.NewHandler(h, ctx, httpTransport, node.connectionManager, node.serviceManager, node.cryptoManager, node.eventBroadcaster, routeTable, noCrypto)

	// Create TCP tunnel handler for peer-to-peer TCP tunneling (if enabled)
	tunnelEnabled := config.TunnelEnabled == nil || *config.TunnelEnabled
	if tunnelEnabled {
		node.tunnelHandler = tunnelPkg.NewHandler(h, ctx, func(format string, v ...interface{}) {
			fmt.Printf("[tunnel] "+format+"\n", v...)
		})
		node.tunnelHandler.SetRouteLookup(func(identifier string) (string, bool) {
			return routeTable.GetRoute(identifier)
		})
		// Set allowed targets from config
		if len(config.TunnelAllowedTargets) > 0 {
			node.tunnelHandler.SetAllowedTargets(config.TunnelAllowedTargets)
		}
	}

	// Subscribe to libp2p event bus for NAT reachability and address changes
	if config.NATTraversal != nil && *config.NATTraversal {
		evtSub, err := h.EventBus().Subscribe([]interface{}{
			new(event.EvtLocalReachabilityChanged),
			new(event.EvtLocalAddressesUpdated),
		})
		if err == nil {
			go func() {
				defer evtSub.Close()
				for evt := range evtSub.Out() {
					switch e := evt.(type) {
					case event.EvtLocalReachabilityChanged:
						node.natReachability.Store(int32(e.Reachability))
						node.SendEvent(types.EventNATStatus, map[string]interface{}{
							"reachability": e.Reachability.String(),
							"relay_addr":   node.HasRelayAddr(),
							"timestamp":    time.Now(),
						})
					case event.EvtLocalAddressesUpdated:
						hasRelay := checkForRelayAddrs(h.Addrs())
						node.hasRelayAddr.Store(hasRelay)
						node.SendEvent(types.EventNATStatus, map[string]interface{}{
							"reachability": network.Reachability(node.natReachability.Load()).String(),
							"relay_addr":   hasRelay,
							"relay_addrs":  getRelayAddrs(h.Addrs()),
							"timestamp":    time.Now(),
						})
					}
				}
			}()
		}
	}

	// Start DHT readiness monitoring goroutine
	if kademliaDHT != nil {
		go func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			timeout := time.After(5 * time.Minute)
			for {
				select {
				case <-ticker.C:
					if kademliaDHT.RoutingTable().Size() > 0 {
						node.dhtReady.Store(true)
						node.SendEvent(types.EventNATStatus, map[string]interface{}{
							"dht_ready":    true,
							"routing_size": kademliaDHT.RoutingTable().Size(),
							"timestamp":    time.Now(),
						})
						return
					}
				case <-timeout:
					return
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	// Multi-service beacons will be loaded in Start() via services loader

	// Set up connection handlers
	h.Network().Notify(&network.NotifyBundle{
		ConnectedF: func(n network.Network, conn network.Conn) {
			node.connectionManager.HandleNewConnection(conn.RemotePeer())
			if config.NATTraversal != nil && *config.NATTraversal {
				node.logConnectionDetails(conn)
			}
		},
		DisconnectedF: func(n network.Network, conn network.Conn) {
			node.connectionManager.HandleDisconnection(conn.RemotePeer())
		},
	})

	return node, nil
}

// Start initializes and starts all node services including peer discovery,
// gossip message handling, the libp2p protocol server, and service beacons.
// It should be called after NewNode to begin normal node operation.
// Returns an error if service loading or beacon startup fails.
func (n *Node) Start() error {
	// Start discovery mechanisms
	n.discoveryManager.StartPeerDiscovery(n.connectionManager)

	// Start gossip message handling
	go n.handleGossipMessages()

	// Start libp2p protocol server
	go n.httpHandler.StartP2PProtocolServer()

	// Load services directory configuration and start service beacons
	if err := n.loadAndStartServices(); err != nil {
		return err
	}

	n.SendEvent("node_started", map[string]interface{}{
		"peer_id":   n.host.ID().String(),
		"timestamp": time.Now(),
		"addrs":     n.host.Addrs(),
	})

	return nil
}

// Close shuts down the node by closing the HTTP server, listener, and libp2p host.
// It releases all resources held by the node.
func (n *Node) Close() {
	if n.httpServer != nil {
		n.httpServer.Close()
	}
	if n.httpListener != nil {
		n.httpListener.Close()
	}
	n.host.Close()
}

// SendEvent broadcasts an event to all subscribers via the event broadcaster.
// It also sends the event via the HTTP server if one is configured,
// for backwards compatibility with HTTP-based event clients.
func (n *Node) SendEvent(eventType string, data interface{}) {
	if n.eventBroadcaster != nil {
		n.eventBroadcaster.SendEvent(eventType, data)
	}

	// Backwards compatibility: also send via HTTP server if available
	if n.httpServer != nil {
		event := types.Event{
			Type:      eventType,
			Timestamp: time.Now(),
			Data:      data,
			NodeID:    n.host.ID().String(),
		}
		n.httpServer.BroadcastEvent(event)
	}
}

// GetNATReachability returns the current NAT reachability status as a string.
// Possible values include "Unknown", "Public", and "Private".
func (n *Node) GetNATReachability() string {
	return network.Reachability(n.natReachability.Load()).String()
}

// HasRelayAddr returns true if the node has at least one relay (/p2p-circuit) address,
// indicating that the node can be reached via circuit relay.
func (n *Node) HasRelayAddr() bool {
	return n.hasRelayAddr.Load()
}

// GetRelayAddrs returns the list of relay (/p2p-circuit) addresses as strings.
// Returns an empty slice if no relay addresses are available.
func (n *Node) GetRelayAddrs() []string {
	return getRelayAddrs(n.host.Addrs())
}

// IsDHTReady returns true if the DHT routing table has at least one peer,
// indicating that the DHT is operational for peer lookups.
func (n *Node) IsDHTReady() bool {
	return n.dhtReady.Load()
}

// GetDHTRoutingTableSize returns the number of peers in the DHT routing table.
func (n *Node) GetDHTRoutingTableSize() int {
	if n.dht == nil {
		return 0
	}
	return n.dht.RoutingTable().Size()
}

// IsNATTraversalEnabled returns whether NAT traversal is configured.
func (n *Node) IsNATTraversalEnabled() bool {
	return n.config.NATTraversal != nil && *n.config.NATTraversal
}

// checkForRelayAddrs returns true if any of the provided multiaddrs contain
// the /p2p-circuit component, indicating relay capability.
func checkForRelayAddrs(addrs []multiaddr.Multiaddr) bool {
	for _, addr := range addrs {
		if strings.Contains(addr.String(), "/p2p-circuit") {
			return true
		}
	}
	return false
}

// getRelayAddrs extracts and returns all relay addresses from the provided
// multiaddr list as strings.
func getRelayAddrs(addrs []multiaddr.Multiaddr) []string {
	var result []string
	for _, addr := range addrs {
		if strings.Contains(addr.String(), "/p2p-circuit") {
			result = append(result, addr.String())
		}
	}
	return result
}

// handleGossipMessages processes incoming gossip messages from the pubsub subscription.
// It filters out messages from itself and forwards remaining messages to the
// discovery manager for processing.
func (n *Node) handleGossipMessages() {
	for {
		msg, err := n.gossipSub.Next(n.ctx)
		if err != nil {
			n.SendEvent("error", map[string]interface{}{
				"message": "Failed to get next gossip message",
				"error":   err.Error(),
			})
			continue
		}

		// Skip our own messages
		if msg.ReceivedFrom == n.host.ID() {
			continue
		}

		// Process the message through discovery manager
		n.discoveryManager.ProcessGossipMessage(msg, n.cryptoManager, n.connectionManager)
	}
}

// logConnectionDetails emits a connection_details event with information about
// a new peer connection, useful for NAT traversal debugging.
func (n *Node) logConnectionDetails(conn network.Conn) {
	n.SendEvent("connection_details", map[string]interface{}{
		"remote_peer":   conn.RemotePeer().String(),
		"remote_addr":   conn.RemoteMultiaddr().String(),
		"local_addr":    conn.LocalMultiaddr().String(),
		"connection_id": conn.ID(),
		"timestamp":     time.Now(),
	})
}

// GetHost returns the underlying libp2p host.
func (n *Node) GetHost() host.Host {
	return n.host
}

// GetConnectionManager returns the connection manager for this node.
func (n *Node) GetConnectionManager() interfaces.PeerManager {
	return n.connectionManager
}

// GetServiceManager returns the service manager for this node.
func (n *Node) GetServiceManager() interfaces.ServiceManager {
	return n.serviceManager
}

// GetHTTPHandler returns the HTTP handler for this node.
func (n *Node) GetHTTPHandler() interfaces.HTTPHandler {
	return n.httpHandler
}

// GetDiscoveryManager returns the discovery manager for this node.
func (n *Node) GetDiscoveryManager() interfaces.DiscoveryManager {
	return n.discoveryManager
}

// GetHTTPTransport returns the HTTP transport configured for libp2p URLs.
func (n *Node) GetHTTPTransport() *http.Transport {
	return n.httpTransport
}

// SetHTTPServer sets the HTTP server for event broadcasting.
func (n *Node) SetHTTPServer(server HTTPServer) {
	n.httpServer = server
}

// SetEventBroadcaster sets the event broadcaster for the node.
func (n *Node) SetEventBroadcaster(broadcaster interfaces.EventBroadcaster) {
	n.eventBroadcaster = broadcaster
}

// GetEventBroadcaster returns the event broadcaster for this node.
func (n *Node) GetEventBroadcaster() interfaces.EventBroadcaster {
	return n.eventBroadcaster
}

// GetRouteTable returns the URL route table for this node.
func (n *Node) GetRouteTable() *types.RouteTable {
	return n.routeTable
}

// GetConfig returns the node configuration.
func (n *Node) GetConfig() *Config {
	return n.config
}

// GetTunnelHandler returns the TCP tunnel handler for this node,
// or nil if tunneling is not enabled.
func (n *Node) GetTunnelHandler() *tunnelPkg.Handler {
	return n.tunnelHandler
}

// loadAndStartServices loads services.json from the services directory and starts
// service beacons for each configured service. It loads service keys, registers
// routes, and creates beacons for service discovery.
func (n *Node) loadAndStartServices() error {
	// Determine services dir
	var servicesDir string
	if n.config.ServicesDir != nil && *n.config.ServicesDir != "" {
		servicesDir = *n.config.ServicesDir
	} else {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("failed to determine executable path: %w", err)
		}
		servicesDir = filepath.Join(filepath.Dir(exe), "services")
	}

	// Ensure absolute
	absServicesDir, err := filepath.Abs(servicesDir)
	if err != nil {
		return fmt.Errorf("failed to resolve services directory: %w", err)
	}

	// If directory doesn't exist, nothing to load
	if _, err := os.Stat(absServicesDir); err != nil {
		return nil
	}

	// Load services.json
	cfgPath := filepath.Join(absServicesDir, "services.json")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		// No config file: skip
		return nil
	}

	// Parse as map alias -> entry
	type servicesEntry struct {
		Directory    string   `json:"directory"`
		PEM          string   `json:"pem"`
		PEMs         []string `json:"pems"`
		Fig          string   `json:"fig"`
		Figs         []string `json:"figs"`
		RouteURL     string   `json:"routeUrl"`     // URL to route requests to
		RoutePrefix  string   `json:"routePrefix"`  // Prefix to add to paths
		KeepFullPath bool     `json:"keepFullPath"` // Whether to keep full path
	}

	entries := make(map[string]servicesEntry)
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("failed to parse services.json: %w", err)
	}

	// Helper: resolve path relative to base
	resolvePath := func(base, p string) string {
		if p == "" {
			return ""
		}
		if filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(base, p)
	}

	for alias, entry := range entries {
		base := absServicesDir
		if strings.TrimSpace(entry.Directory) != "" {
			base = filepath.Join(absServicesDir, entry.Directory)
		}

		// Collect pem paths
		pemPaths := make([]string, 0)
		if strings.TrimSpace(entry.PEM) != "" {
			pemPaths = append(pemPaths, resolvePath(base, entry.PEM))
		}
		for _, p := range entry.PEMs {
			if strings.TrimSpace(p) != "" {
				pemPaths = append(pemPaths, resolvePath(base, p))
			}
		}

		// Collect fig templates
		figPaths := make([]string, 0)
		if strings.TrimSpace(entry.Fig) != "" {
			figPaths = append(figPaths, resolvePath(base, entry.Fig))
		}
		for _, f := range entry.Figs {
			if strings.TrimSpace(f) != "" {
				figPaths = append(figPaths, resolvePath(base, f))
			}
		}

		figTemplates := make([][]byte, 0, len(figPaths))
		for _, fp := range figPaths {
			b, err := os.ReadFile(fp)
			if err == nil && len(b) > 0 {
				figTemplates = append(figTemplates, b)
			}
		}

		for i, pemPath := range pemPaths {
			// Load key
			priv, err := n.keyLoader(pemPath)
			if err != nil {
				n.SendEvent("error", map[string]interface{}{
					"message": "Failed to load service key",
					"alias":   alias,
					"path":    pemPath,
					"error":   err.Error(),
				})
				continue
			}

			// Register the service key with the crypto manager
			// Create a unique key ID using alias and index for multiple keys per alias
			keyID := fmt.Sprintf("%s-%d", alias, i)
			n.cryptoManager.RegisterServiceKey(keyID, priv)

			// Register route if routeUrl is specified
			if entry.RouteURL != "" {
				// Get the service key hex for use as identifier
				pubKey := priv.GetPublic()
				pubKeyBytes, _ := crypto.MarshalPublicKey(pubKey)
				serviceKeyHex := fmt.Sprintf("%x", pubKeyBytes)

				// Create routing config
				routingConfig := types.ServiceRoutingConfig{
					RoutePrefix:  entry.RoutePrefix,
					KeepFullPath: entry.KeepFullPath,
				}

				// Register the route with the route table
				n.routeTable.AddRouteWithConfig(serviceKeyHex, entry.RouteURL, routingConfig)

				n.SendEvent("service_route_registered", map[string]interface{}{
					"alias":        alias,
					"serviceKey":   serviceKeyHex,
					"routeUrl":     entry.RouteURL,
					"routePrefix":  entry.RoutePrefix,
					"keepFullPath": entry.KeepFullPath,
				})
			}

			// Create beacon with meta
			beacon, err := n.serviceManager.CreateServiceBeaconWithMeta(priv, alias, figTemplates)
			if err != nil {
				n.SendEvent("error", map[string]interface{}{
					"message": "Failed to create service beacon",
					"alias":   alias,
					"error":   err.Error(),
				})
				continue
			}
			if n.config.NoAnnounce == nil || !*n.config.NoAnnounce {
				if err := beacon.Start(); err != nil {
					n.SendEvent("error", map[string]interface{}{
						"message": "Failed to start service beacon",
						"alias":   alias,
						"error":   err.Error(),
					})
					continue
				}
			}
			// Track first beacon for legacy field
			if n.serviceBeacon == nil {
				n.serviceBeacon = beacon
			}
			n.SendEvent("service_beacon_started", map[string]interface{}{
				"alias": alias,
				"keyID": keyID,
			})
		}
	}

	return nil
}

// loadRouteConfig loads route configuration from a JSON file and populates
// the provided route table with identifier to URL mappings.
func loadRouteConfig(filename string, routeTable *types.RouteTable) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read router config file: %w", err)
	}

	var routes map[string]string
	if err := json.Unmarshal(data, &routes); err != nil {
		return fmt.Errorf("failed to parse router config JSON: %w", err)
	}

	routeTable.LoadRoutes(routes)
	return nil
}
