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

	"github.com/libp2p/go-libp2p"
	p2phttp "github.com/libp2p/go-libp2p-http"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
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
	"banyan/types"
)

// HTTPServer interface for the management API server - keeping here for backwards compatibility
// but recommend using interfaces.HTTPServer for new code
type HTTPServer interface {
	BroadcastEvent(event types.Event)
	Close() error
}

// Configuration holds node configuration
type Config struct {
	PrivKeyFile     *string
	ServicesDir     *string
	FigsDir         *string
	DisableDHT      *bool
	NoAnnounce      *bool
	ListenMultiaddr *string
	NATTraversal    *bool
	NoCrypto        *bool
	AnonLookups     *bool
	RouterConfig    *string
	Bootstraps      []string // Optional bootstrap peer multiaddrs
	// Fig validation flags
	AllowExpiredFigs  *bool
	AllowInsecureFigs *bool
	// Beacon behavior flags
	BeaconIncludePeerIDAnnouncements *bool
	BeaconPeerIDDirectOnly           *bool
	// Transport restriction flags
	OverrideTransportRestrictions *bool // Allow connections even if transport doesn't match fig restrictions
}

// Node represents our libp2p node with HTTP proxy capabilities
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

	// Service-specific components
	serviceBeacon interfaces.ServiceBeacon // legacy single beacon (first)

	// Event broadcasting
	eventBroadcaster interfaces.EventBroadcaster

	// Router table for URL routing
	routeTable *types.RouteTable

	// Configuration
	config *Config

	// key loader for services
	keyLoader PrivateKeyLoader
}

// LoadPrivateKeyFromPEM loads a private key from a PEM file
type PrivateKeyLoader func(filename string) (crypto.PrivKey, error)

// NewNode creates a new libp2p node with all managers
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
	if config.NATTraversal != nil && *config.NATTraversal {
		opts = append(opts,
			libp2p.EnableHolePunching(),
			libp2p.EnableAutoRelay(),
			libp2p.NATPortMap(),
		)
		fmt.Println("NAT traversal enabled: hole punching + auto-relay + UPnP")
	}

	// Create libp2p host
	h, err := libp2p.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create libp2p host: %w", err)
	}

	// Initialize DHT conditionally
	var kademliaDHT *dht.IpfsDHT
	disableDHT := config.DisableDHT != nil && *config.DisableDHT
	if !disableDHT {
		kademliaDHT, err = dht.New(ctx, h)
		if err != nil {
			return nil, fmt.Errorf("failed to create DHT: %w", err)
		}

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

// Start initializes and starts all node services
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

// Close shuts down the node
func (n *Node) Close() {
	if n.httpServer != nil {
		n.httpServer.Close()
	}
	if n.httpListener != nil {
		n.httpListener.Close()
	}
	n.host.Close()
}

// SendEvent sends an event to all subscribers via the event broadcaster
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

// handleGossipMessages handles incoming gossip messages
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

// logConnectionDetails logs connection details for NAT traversal debugging
func (n *Node) logConnectionDetails(conn network.Conn) {
	n.SendEvent("connection_details", map[string]interface{}{
		"remote_peer":   conn.RemotePeer().String(),
		"remote_addr":   conn.RemoteMultiaddr().String(),
		"local_addr":    conn.LocalMultiaddr().String(),
		"connection_id": conn.ID(),
		"timestamp":     time.Now(),
	})
}

// GetHost returns the libp2p host
func (n *Node) GetHost() host.Host {
	return n.host
}

// GetConnectionManager returns the connection manager
func (n *Node) GetConnectionManager() interfaces.PeerManager {
	return n.connectionManager
}

// GetServiceManager returns the service manager
func (n *Node) GetServiceManager() interfaces.ServiceManager {
	return n.serviceManager
}

// GetHTTPHandler returns the HTTP handler
func (n *Node) GetHTTPHandler() interfaces.HTTPHandler {
	return n.httpHandler
}

// GetDiscoveryManager returns the discovery manager
func (n *Node) GetDiscoveryManager() interfaces.DiscoveryManager {
	return n.discoveryManager
}

// GetHTTPTransport returns the HTTP transport
func (n *Node) GetHTTPTransport() *http.Transport {
	return n.httpTransport
}

// SetHTTPServer sets the HTTP server for event broadcasting
func (n *Node) SetHTTPServer(server HTTPServer) {
	n.httpServer = server
}

// SetEventBroadcaster sets the event broadcaster for the node
func (n *Node) SetEventBroadcaster(broadcaster interfaces.EventBroadcaster) {
	n.eventBroadcaster = broadcaster
}

// GetEventBroadcaster returns the event broadcaster
func (n *Node) GetEventBroadcaster() interfaces.EventBroadcaster {
	return n.eventBroadcaster
}

// GetRouteTable returns the route table
func (n *Node) GetRouteTable() *types.RouteTable {
	return n.routeTable
}

// GetConfig returns the node configuration
func (n *Node) GetConfig() *Config {
	return n.config
}

// loadAndStartServices loads services.json from the services directory and starts service beacons
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

// loadRouteConfig loads route configuration from a JSON file
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
