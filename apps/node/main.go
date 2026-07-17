// Command node is the main entry point for running a Banyan network node.
// It initializes a libp2p-based peer-to-peer node with service discovery,
// HTTP proxy capabilities, and a management API server.
//
// Usage:
//
//	node [flags]
//
// Configuration can be provided via command-line flags or a JSON config file
// specified with the -config flag. Command-line flags take precedence over
// config file values.
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"banyan/addon"
	managementserver "banyan/management-server"
	nodePkg "banyan/node"

	"github.com/libp2p/go-libp2p/core/crypto"
)

// BootstrapsValue represents bootstrap peer configuration that can be either
// a file path (string) or an inline array of multiaddr strings.
type BootstrapsValue struct {
	// IsFile indicates whether Path contains a file path (true) or Addrs contains inline addresses (false).
	IsFile bool
	// Path is the file path to a JSON file containing bootstrap peer multiaddrs.
	Path string
	// Addrs is an inline array of bootstrap peer multiaddr strings.
	Addrs []string
}

// UnmarshalJSON implements custom JSON unmarshaling for BootstrapsValue.
// It accepts either a string (interpreted as a file path) or an array of
// strings (interpreted as inline multiaddrs).
func (b *BootstrapsValue) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as string (file path)
	var path string
	if err := json.Unmarshal(data, &path); err == nil {
		b.IsFile = true
		b.Path = path
		return nil
	}

	// Try to unmarshal as array of strings (inline multiaddrs)
	var addrs []string
	if err := json.Unmarshal(data, &addrs); err == nil {
		b.IsFile = false
		b.Addrs = addrs
		return nil
	}

	return fmt.Errorf("bootstraps must be either a string (file path) or an array of strings (multiaddrs)")
}

// NodeConfigFile represents the JSON structure of the node configuration file.
// All fields are optional and map to corresponding command-line flags.
type NodeConfigFile struct {
	// PrivKeyFile is the path to a PEM file containing the node's private key.
	PrivKeyFile *string `json:"privkey,omitempty"`
	// DisableDHT disables DHT peer discovery when true.
	DisableDHT *bool `json:"disableDht,omitempty"`
	// ServicesDir is the directory containing services.json.
	ServicesDir *string `json:"servicesDir,omitempty"`
	// FigsDir is the directory containing fig files.
	FigsDir *string `json:"figsDir,omitempty"`
	// NoAnnounce prevents DHT and pubsub announcements when true.
	NoAnnounce *bool `json:"noAnnounce,omitempty"`
	// AnonLookups allows anonymous lookups when true.
	AnonLookups *bool `json:"anonLookups,omitempty"`
	// ListenMultiaddr is the multiaddr to listen on.
	ListenMultiaddr *string `json:"listen,omitempty"`
	// NoCrypto disables encryption on pubsub messages when true.
	NoCrypto *bool `json:"noCrypto,omitempty"`
	// NATTraversal enables NAT traversal features when true.
	NATTraversal *bool `json:"natTraversal,omitempty"`
	// KuboURL is the URL of a Kubo node to connect to.
	KuboURL *string `json:"kuboUrl,omitempty"`
	// RouterConfig is the path to a JSON router configuration file.
	RouterConfig *string `json:"routerConfig,omitempty"`
	// AddonsDir is the directory containing addons.
	AddonsDir *string `json:"addonsDir,omitempty"`
	// BeaconIncludePeerIDAnnouncements includes PeerID in public announcements.
	BeaconIncludePeerIDAnnouncements *bool `json:"beaconIncludePeerIdAnnouncements,omitempty"`
	// BeaconPeerIDDirectOnly only includes PeerID in direct replies.
	BeaconPeerIDDirectOnly *bool `json:"beaconPeerIdDirectOnly,omitempty"`
	// AllowExpiredFigs allows expired fig files (security risk).
	AllowExpiredFigs *bool `json:"allowExpiredFigs,omitempty"`
	// AllowInsecureFigs allows insecure fig files (security risk).
	AllowInsecureFigs *bool `json:"allowInsecureFigs,omitempty"`
	// OverrideTransportRestrictions bypasses transport restrictions.
	OverrideTransportRestrictions *bool `json:"overrideTransportRestrictions,omitempty"`
	// Bootstraps specifies bootstrap peers (file path or inline array).
	Bootstraps *BootstrapsValue `json:"bootstraps,omitempty"`
	// TunnelEnabled enables the TCP tunnel protocol handler.
	TunnelEnabled *bool `json:"tunnelEnabled,omitempty"`
	// TunnelAuthToken is the auth token for tunnel API endpoints.
	TunnelAuthToken *string `json:"tunnelAuthToken,omitempty"`
	// TunnelAllowedTargets specifies allowed target hosts for tunnels.
	TunnelAllowedTargets []string `json:"tunnelAllowedTargets,omitempty"`
	// AllowUnsafeServiceKeyInjection allows manual service key injection via API (SECURITY RISK).
	AllowUnsafeServiceKeyInjection *bool `json:"allowUnsafeServiceKeyInjection,omitempty"`
	// DataDir is the directory for durable node state (persistence tree).
	DataDir *string `json:"dataDir,omitempty"`
	// PeerstoreDSN selects an external SQL backend for the persistence tree.
	PeerstoreDSN *string `json:"peerstoreDsn,omitempty"`
}

// Command line flags for node configuration.
// These can be overridden by a config file; CLI flags take precedence.
var (
	configFile      = flag.String("config", "", "Path to JSON configuration file")
	privKeyFile     = flag.String("privkey", "", "Path to PEM file containing private key for peer identity and AES encryption")
	disableDHT      = flag.Bool("disable-dht", false, "Disable DHT peer discovery and lookup (useful for local testing)")
	servicesDir     = flag.String("services-dir", "", "Directory containing services.json and per-service subfolders (overrides default ./services relative to executable)")
	figsDir         = flag.String("figs-dir", "", "Directory containing fig files (overrides default ./figs relative to executable)")
	noAnnounce      = flag.Bool("no-announce", false, "Do not announce this node's presence via DHT or over pubsub if service key is provided")
	anonLookups     = flag.Bool("anon-lookups", false, "Allow anonymous lookups and responses")
	listenMultiaddr = flag.String("listen", "/ip4/0.0.0.0/tcp/0", "Multiaddr to listen on")
	noCrypto        = flag.Bool("no-crypto", false, "Disable encryption on pubsub messages")
	natTraversal    = flag.Bool("nat-traversal", false, "Enable NAT traversal")
	kuboURL         = flag.String("kubo-url", "", "Kubo URL to connect to")
	routerConfig    = flag.String("router-config", "", "Path to JSON file containing router configuration (identifier to URL mappings)")
	addonsDir       = flag.String("addons-dir", "", "Directory containing addons.json and addon executables (overrides default)")
	// Beacon behavior flags
	beaconIncludePeerIDAnnouncements = flag.Bool("beacon-include-peerid-announcements", true, "Include beacon's PeerID in public announcements")
	beaconPeerIDDirectOnly           = flag.Bool("beacon-peerid-direct-only", false, "Only include beacon's PeerID in direct replies to locators")
	// Fig security/override flags
	allowExpiredFigs  = flag.Bool("allow-expired-figs", false, "Allow loading fig files even if expired (SECURITY RISK)")
	allowInsecureFigs = flag.Bool("allow-insecure-figs", false, "Allow loading fig files with invalid or missing signatures (SECURITY RISK)")
	// Transport restriction flags
	overrideTransportRestrictions = flag.Bool("override-transport-restrictions", false, "Allow connections even if transport doesn't match fig restrictions")
	// Tunnel configuration flags
	tunnelEnabled   = flag.Bool("tunnel-enabled", true, "Enable TCP tunnel protocol handler for peer-to-peer TCP tunneling")
	tunnelAuthToken = flag.String("tunnel-auth-token", "", "Auth token required for tunnel API endpoints (optional)")
	// Unsafe testing flags
	allowUnsafeServiceKeyInjection = flag.Bool("allow-unsafe-service-key-injection", false, "Allow manual injection of service keys via API (SECURITY RISK - for testing only)")
	// Persistence tree flags
	dataDir      = flag.String("data-dir", "", "Directory for durable node state such as the peer persistence tree (overrides default ./data relative to executable)")
	peerstoreDSN = flag.String("peerstore-dsn", "", "DSN for an external SQL persistence-tree backend (unset uses the embedded SQLite store under --data-dir)")
)

// main is the entry point for the Banyan node.
// It loads configuration, creates and starts the node, initializes the management
// API server, and runs until interrupted.
func main() {
	flag.Parse()

	ctx := context.Background()

	// Load configuration from file if provided, then merge with command-line flags
	config, err := loadConfig()
	if err != nil {
		fmt.Printf("Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Create the libp2p node with modular structure
	node, err := nodePkg.NewNode(ctx, config, loadPrivateKeyFromPEM)
	if err != nil {
		fmt.Printf("Failed to create node: %v\n", err)
		os.Exit(1)
	}
	defer node.Close()

	// Note: Event broadcaster setup would be done here in a production system
	// For now, keeping the existing direct event system

	// Print the peer ID
	fmt.Printf("Libp2p node started with Peer ID: %s\n", node.GetHost().ID().String())

	// Print configuration status
	if noCrypto != nil && *noCrypto {
		fmt.Println("Crypto DISABLED - gossipsub messages will not be encrypted")
	}
	if anonLookups != nil && *anonLookups {
		fmt.Println("Anonymous lookups ENABLED - peer identity not required for responses")
	}
	if allowExpiredFigs != nil && *allowExpiredFigs {
		fmt.Println("WARNING: Allowing EXPIRED fig files (SECURITY RISK)")
	}
	if allowInsecureFigs != nil && *allowInsecureFigs {
		fmt.Println("WARNING: Allowing INSECURE fig files with invalid/missing signatures (SECURITY RISK)")
	}
	if allowUnsafeServiceKeyInjection != nil && *allowUnsafeServiceKeyInjection {
		fmt.Println("WARNING: Allowing UNSAFE service key injection via API (SECURITY RISK - for testing only)")
	}

	// Start management API server
	managementServer, serverURL, err := managementserver.StartManagementServer(node)
	if err != nil {
		log.Fatalf("Failed to start management server: %v", err)
	}
	node.SetHTTPServer(managementServer)

	fmt.Printf("Management API server available at: %s\n", serverURL)

	fmt.Printf("Node started at: %v\n", time.Now())
	fmt.Printf("Listening addresses:\n")
	for _, addr := range node.GetHost().Addrs() {
		fmt.Printf("  %s/p2p/%s\n", addr, node.GetHost().ID())
	}

	// Start all node services
	if err := node.Start(); err != nil {
		fmt.Printf("Failed to start node services: %v\n", err)
		os.Exit(1)
	}

	// Wire and start addon manager
	am := addon.NewManager(ctx, nil).WithMounts(
		managementServer.Mount,
		node.GetHTTPHandler().MountP2P,
	)
	// Expose manager globally for components that need to orchestrate alias resolution
	addon.SetGlobalManager(am)
	// Pass management server URL to addons for direct HTTP access
	am = am.WithManagerURL(serverURL)
	if addonsDir != nil && *addonsDir != "" {
		am = am.WithDirectory(*addonsDir)
	}
	// Provide addon disclosures to greetings
	node.GetHTTPHandler().SetAddonDisclosureProvider(am.DisclosureProvider())

	if err := am.LoadAndStart(); err != nil {
		fmt.Printf("Addon manager error: %v\n", err)
	}

	fmt.Println("Node is running. Press Ctrl+C to stop.")

	// Keep the node running
	select {}
}

// loadPrivateKeyFromPEM loads a private key from a PEM file.
// It supports EC PRIVATE KEY, PRIVATE KEY (PKCS8), and RSA PRIVATE KEY blocks.
// The key is converted to a libp2p-compatible format.
func loadPrivateKeyFromPEM(filename string) (crypto.PrivKey, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	var key interface{}
	var parseErr error

	switch block.Type {
	case "EC PRIVATE KEY":
		key, parseErr = x509.ParseECPrivateKey(block.Bytes)
	case "PRIVATE KEY":
		key, parseErr = x509.ParsePKCS8PrivateKey(block.Bytes)
	case "RSA PRIVATE KEY":
		key, parseErr = x509.ParsePKCS1PrivateKey(block.Bytes)
	default:
		return nil, fmt.Errorf("unsupported PEM type: %s", block.Type)
	}

	if parseErr != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", parseErr)
	}

	// Handle Ed25519 keys specially - crypto.KeyPairFromStdKey needs a pointer
	if ed25519Key, ok := key.(ed25519.PrivateKey); ok {
		key = &ed25519Key
	}

	privKey, _, err := crypto.KeyPairFromStdKey(key)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to libp2p key: %w", err)
	}

	return privKey, nil
}

// loadBootstraps loads bootstrap peer multiaddrs from either an inline array
// or a file path specified in BootstrapsValue. Returns nil if no bootstraps
// are configured.
func loadBootstraps(bootstrapsValue *BootstrapsValue) ([]string, error) {
	if bootstrapsValue == nil {
		return nil, nil
	}

	if bootstrapsValue.IsFile {
		// Load from file
		data, err := os.ReadFile(bootstrapsValue.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to read bootstraps file %s: %w", bootstrapsValue.Path, err)
		}

		var addrs []string
		if err := json.Unmarshal(data, &addrs); err != nil {
			return nil, fmt.Errorf("failed to parse bootstraps file %s: %w", bootstrapsValue.Path, err)
		}

		fmt.Printf("Loaded %d bootstrap peers from file: %s\n", len(addrs), bootstrapsValue.Path)
		return addrs, nil
	}

	// Use inline array
	if len(bootstrapsValue.Addrs) > 0 {
		fmt.Printf("Using %d bootstrap peers from config file\n", len(bootstrapsValue.Addrs))
	}
	return bootstrapsValue.Addrs, nil
}

// loadConfig loads configuration from a file (if provided) and merges with
// command-line flags. Command-line flags take precedence over config file values.
// Returns a node.Config ready for use with NewNode.
func loadConfig() (*nodePkg.Config, error) {
	var fileConfig NodeConfigFile

	// Load config file if provided
	if configFile != nil && *configFile != "" {
		data, err := os.ReadFile(*configFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}

		if err := json.Unmarshal(data, &fileConfig); err != nil {
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}

		fmt.Printf("Loaded configuration from: %s\n", *configFile)
	}

	// Helper function to determine which value to use (CLI flag takes precedence)
	// For string flags: use CLI if explicitly set (not empty), otherwise use file config
	stringValue := func(cliFlag *string, fileValue *string) *string {
		// Check if CLI flag was explicitly set by comparing to default
		if cliFlag != nil && *cliFlag != "" {
			return cliFlag
		}
		return fileValue
	}

	// Track which flags were explicitly set by the user
	explicitFlags := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) {
		explicitFlags[f.Name] = true
	})

	// For bool flags: use CLI if explicitly set, otherwise use file config
	boolValue := func(flagName string, cliFlag *bool, fileValue *bool, defaultVal bool) *bool {
		if explicitFlags[flagName] {
			return cliFlag
		}
		if fileValue != nil {
			return fileValue
		}
		return &defaultVal
	}

	// Load bootstrap peers (from inline array or file)
	bootstraps, err := loadBootstraps(fileConfig.Bootstraps)
	if err != nil {
		return nil, fmt.Errorf("failed to load bootstraps: %w", err)
	}

	// Build the final config, with CLI flags overriding file config
	config := &nodePkg.Config{
		PrivKeyFile:                      stringValue(privKeyFile, fileConfig.PrivKeyFile),
		ServicesDir:                      stringValue(servicesDir, fileConfig.ServicesDir),
		FigsDir:                          stringValue(figsDir, fileConfig.FigsDir),
		ListenMultiaddr:                  stringValue(listenMultiaddr, fileConfig.ListenMultiaddr),
		RouterConfig:                     stringValue(routerConfig, fileConfig.RouterConfig),
		DisableDHT:                       boolValue("disable-dht", disableDHT, fileConfig.DisableDHT, false),
		NoAnnounce:                       boolValue("no-announce", noAnnounce, fileConfig.NoAnnounce, false),
		AnonLookups:                      boolValue("anon-lookups", anonLookups, fileConfig.AnonLookups, false),
		NoCrypto:                         boolValue("no-crypto", noCrypto, fileConfig.NoCrypto, false),
		NATTraversal:                     boolValue("nat-traversal", natTraversal, fileConfig.NATTraversal, false),
		AllowExpiredFigs:                 boolValue("allow-expired-figs", allowExpiredFigs, fileConfig.AllowExpiredFigs, false),
		AllowInsecureFigs:                boolValue("allow-insecure-figs", allowInsecureFigs, fileConfig.AllowInsecureFigs, false),
		BeaconIncludePeerIDAnnouncements: boolValue("beacon-include-peerid-announcements", beaconIncludePeerIDAnnouncements, fileConfig.BeaconIncludePeerIDAnnouncements, true),
		BeaconPeerIDDirectOnly:           boolValue("beacon-peerid-direct-only", beaconPeerIDDirectOnly, fileConfig.BeaconPeerIDDirectOnly, false),
		OverrideTransportRestrictions:    boolValue("override-transport-restrictions", overrideTransportRestrictions, fileConfig.OverrideTransportRestrictions, false),
		Bootstraps:                       bootstraps, // Bootstraps can only be set via config file
		// Tunnel configuration
		TunnelEnabled:                   boolValue("tunnel-enabled", tunnelEnabled, fileConfig.TunnelEnabled, true),
		TunnelAuthToken:                 stringValue(tunnelAuthToken, fileConfig.TunnelAuthToken),
		TunnelAllowedTargets:            fileConfig.TunnelAllowedTargets, // Can only be set via config file
		AllowUnsafeServiceKeyInjection:  boolValue("allow-unsafe-service-key-injection", allowUnsafeServiceKeyInjection, fileConfig.AllowUnsafeServiceKeyInjection, false),
		DataDir:                         stringValue(dataDir, fileConfig.DataDir),
		PeerstoreDSN:                    stringValue(peerstoreDSN, fileConfig.PeerstoreDSN),
	}

	// Handle addonsDir separately since it's not part of node.Config
	if fileConfig.AddonsDir != nil && *fileConfig.AddonsDir != "" && (addonsDir == nil || *addonsDir == "") {
		addonsDir = fileConfig.AddonsDir
	}

	return config, nil
}
