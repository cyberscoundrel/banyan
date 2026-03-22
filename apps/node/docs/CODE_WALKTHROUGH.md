# Banyan Node Code Walkthrough

This document provides a comprehensive walkthrough of the Banyan node codebase, organized by directory and component.

## Table of Contents

1. [Overview](#overview)
2. [Data Structures](#data-structures)
3. [Component Interactions](#component-interactions)
4. [Directory-by-Directory Walkthrough](#directory-by-directory-walkthrough)
5. [Usage Example](#usage-example)

---

## Overview

The Banyan node is a libp2p-based peer-to-peer networking application that provides:

- **Peer Discovery**: DHT, mDNS, and GossipSub-based peer discovery mechanisms
- **Service Discovery**: Beacon/locator pattern for service advertisement and discovery
- **HTTP Proxying**: Libp2p HTTP transport for peer-to-peer HTTP communication
- **TCP Tunneling**: Peer-to-peer TCP tunneling for arbitrary TCP services
- **Event Broadcasting**: Real-time event streaming via WebSockets
- **Management API**: HTTP endpoints for node management and monitoring
- **Addon System**: Extensible addon architecture via JSON-RPC

### Architecture Diagram

```
flowchart TD
    subgraph ENTRY["Entry Point"]
        MAIN["main.go<br/>CLI parsing, config loading"]
    end

    subgraph NODE["Node Core - node/node.go"]
        direction TB
        HOST["libp2p Host"]
        DHT["DHT"]
        PUBSUB["PubSub"]
    end

    subgraph MANAGERS["Managers"]
        direction LR
        CONN["Connection<br/>Peer tracking"]
        DISC["Discovery<br/>DHT, mDNS"]
        SERV["Service<br/>Beacons, Locators"]
        CRYP["Crypto<br/>Encryption"]
    end

    subgraph PROTOCOL["Libp2p Protocol Handlers"]
        direction LR
        PING["/ping"]
        GREET["/greetings"]
        FIGS["/serviceFigs"]
    end

    subgraph SERVERS["HTTP Servers"]
        direction TB
        MGMT["Management API<br/>localhost:9999"]
        WS["WebSocket Hub<br/>Real-time events"]
    end

    subgraph ADDONS["Addon System"]
        ADDON["JSON-RPC Addons"]
    end

    MAIN --> NODE
    NODE --> MANAGERS
    MANAGERS --> PROTOCOL
    MANAGERS --> SERVERS
    NODE --> ADDONS
```

### Data Flow Summary
```
Client Request Flow:
  Client --> Management Server HTTP --> Node --> Manager --> Libp2p Protocol Handler --> Response

Event Broadcast Flow:
  Managers --> Event Broadcaster --> WebSocket Hub --> Connected Clients
```

### Key Design Principles

1. **Interface-based design**: All major components implement interfaces defined in `interfaces/interfaces.go`
2. **Dependency injection**: Components receive dependencies through constructors
3. **Event-driven architecture**: Real-time events broadcast to WebSocket clients
4. **Concurrent-safe**: Thread-safe data structures with proper synchronization

---

## Data Structures

Core types are defined in `types/types.go`.

### Protocol Constants

```go
const (
    HTTPProxyProtocol = "/http-proxy/1.0.0"  // libp2p protocol id
    GossipSubTopic    = "peer-discovery"      // pubsub topic name
)

const (
    StatusConnected    = "connected"
    StatusDisconnected = "disconnected"
    StatusConnecting   = "connecting"
)

const (
    ConnTypeManual       = "manual"              // Manual connection via API
    ConnTypeGossipLookup = "gossipsub-lookup"    // Discovered via gossipsub
    ConnTypeHTTPVerified = "http-verified"       // Verified HTTP capability
    ConnTypeBackground   = "background"          // DHT/mDNS discovery
)
```

### Event System

```go
type Event struct {
    Type      string      `json:"type"`
    Timestamp time.Time   `json:"timestamp"`
    Data      interface{} `json:"data"`
    NodeID    string      `json:"node_id"`
}

const (
    EventPeerConnected    = "peer_connected"
    EventPeerDisconnected = "peer_disconnected"
    EventPeerTracked      = "peer_tracked"
    EventHTTPTest         = "http_test"
    EventGossipMessage    = "gossip_message"
    EventDHTLookup        = "dht_lookup"
    EventProxyRequest     = "proxy_request"
    EventNodeStarted      = "node_started"
    EventBootstrap        = "bootstrap"
    EventDiscovery        = "discovery"
    EventConnection       = "connection"
    EventError            = "error"
    EventInfo             = "info"
    EventNATStatus        = "nat_status"
)
```

### Connection Tracking

```go
type ConnectionItem struct {
    PeerID           peer.ID
    Alias            string        // 4-char shorthand identifier
    ServiceKeys      [][]byte      // Service public keys this peer provides
    Connected        time.Time     // When connection was established
    LastActivity     time.Time     // Most recent interaction
    LastDisconnect   *time.Time    // Previous disconnection time
    Status           string        // "connected", "disconnected", "connecting"
    ConnectAttempts  int           // Number of connection attempts
    ConnectionType   string        // How connection was established
    HTTPCapable      bool          // Has verified HTTP server
    BidirectionalHTTP bool         // Two-way HTTP works
    LastHTTPTest     *time.Time    // Last HTTP capability test
    HTTPTestResult   string        // "success", "failed", "pending", "untested"
}
```

### Peer Options

```go
type PeerOptions struct {
    ServiceKey     []byte
    ConnectionType string
    HTTPCapable    bool
}

func NewManualPeerOptions(httpCapable bool) PeerOptions
func NewServicePeerOptions(serviceKey []byte, httpCapable bool) PeerOptions
func NewBackgroundPeerOptions() PeerOptions
func NewGossipPeerOptions(httpCapable bool) PeerOptions
```

### Lookup Messages

```go
type LookupRequest struct {
    Type          string    `json:"type"`
    Target        string    `json:"target"`
    From          string    `json:"from"`
    Addresses     []string  `json:"addresses,omitempty"`
    PublicKey     []byte    `json:"publicKey"`
    ServiceKey    []byte    `json:"serviceKey"`
    Encrypted     bool      `json:"encrypted"`
    EncryptedData []byte    `json:"encryptedData,omitempty"`
    Nonce         []byte    `json:"nonce,omitempty"`
    Timestamp     time.Time `json:"timestamp"`
}

type LookupResponse struct {
    Type          string    `json:"type"`
    Target        string    `json:"target"`
    From          string    `json:"from"`
    Addresses     []string  `json:"addresses,omitempty"`
    PublicKey     []byte    `json:"publicKey"`
    Signature     []byte    `json:"signature"`
    Encrypted     bool      `json:"encrypted"`
    EncryptedData []byte    `json:"encryptedData,omitempty"`
    Nonce         []byte    `json:"nonce,omitempty"`
    Timestamp     time.Time `json:"timestamp"`
}
```

### Service Discovery Types

```go
type ServiceBeaconMode int

const (
    ServiceBeaconModeLookup    ServiceBeaconMode = iota  // Looking for services
    ServiceBeaconModeAnnounce                            // Announcing availability
    ServiceBeaconModeReplyOnly                           // Only respond to queries
)

type ServiceLookupRequest struct {
    Type           string    `json:"type"`
    ServiceKey     []byte    `json:"serviceKey"`
    RequestPath    string    `json:"requestPath,omitempty"`
    From           string    `json:"from,omitempty"`
    EncryptedFrom  []byte    `json:"encryptedFrom,omitempty"`
    EncryptedNonce []byte    `json:"encryptedNonce,omitempty"`
    FromEncrypted  bool      `json:"fromEncrypted"`
    PublicKey      []byte    `json:"publicKey"`
    Signature      []byte    `json:"signature,omitempty"`
    Timestamp      time.Time `json:"timestamp"`
}

type ServiceResponse struct {
    Type          string    `json:"type"`
    From          string    `json:"from,omitempty"`
    PublicKey     []byte    `json:"publicKey"`
    Signature     []byte    `json:"signature"`
    Encrypted     bool      `json:"encrypted"`
    EncryptedData []byte    `json:"encryptedData,omitempty"`
    Nonce         []byte    `json:"nonce,omitempty"`
    Timestamp     time.Time `json:"timestamp"`
    Addresses     []string  `json:"addresses,omitempty"`
}

type ServiceAnnouncement struct {
    Type       string      `json:"type"`
    ServiceKey []byte      `json:"serviceKey"`
    PeerID     string      `json:"peerID"`
    Data       interface{} `json:"data"`
    Timestamp  time.Time   `json:"timestamp"`
    Signature  []byte      `json:"signature,omitempty"`
}
```

### Fig File (Service Identity)

```go
type FigFile struct {
    ServiceAlias        string          `json:"serviceAlias"`
    Root                FigNode         `json:"root"`
    Data                json.RawMessage `json:"data,omitempty"`
    ExpiresAt           time.Time       `json:"expiresAt,omitempty"`
    RequesterPeerID     string          `json:"requesterPeerId,omitempty"`
    Nonce               string          `json:"nonce,omitempty"`
    Signatures          []string        `json:"signatures,omitempty"`
    RequiredSigners     []string        `json:"requiredSigners,omitempty"`
    AuthoritySignatures []string        `json:"authoritySignatures,omitempty"`
}

type FigNode struct {
    Path              string   `json:"path"`
    Keys              []string `json:"keys"`
    Children          []FigNode `json:"children,omitempty"`
    AllowedTransports []string `json:"allowedTransports,omitempty"`
}

func (f FigFile) BuildCanonicalPayload() []byte
func (f FigFile) FindKeysForPath(requestPath string) []string
```

### Route Table

```go
type RouteTable struct {
    routes        map[string]string
    routingConfig map[string]ServiceRoutingConfig
}

type ServiceRoutingConfig struct {
    RoutePrefix  string `json:"routePrefix"`
    KeepFullPath bool   `json:"keepFullPath"`
}
```

---

## Component Interactions

### System Initialization Flow

```
main.go
    |
    +-> loadConfig() -> nodePkg.Config
    |
    +-> nodePkg.NewNode(ctx, config, keyLoader)
    |       |
    |       +-> libp2p.New() -> host.Host
    |       |
    |       +-> dht.New() -> *dht.IpfsDHT (optional)
    |       |
    |       +-> pubsub.NewGossipSub() -> *pubsub.PubSub
    |       |
    |       +-> eventsPkg.NewBroadcaster()
    |       |
    |       +-> connectionPkg.NewManager()
    |       |
    |       +-> cryptoPkg.NewManager()
    |       |
    |       +-> discoveryPkg.NewManager()
    |       |
    |       +-> servicePkg.NewManager()
    |       |
    |       +-> httpPkg.NewHandler()
    |       |
    |       +-> tunnelPkg.NewHandler()
    |
    +-> managementserver.StartManagementServer(node)
    |       |
    |       +-> websocket.NewHub() + registerRoutes()
    |
    +-> node.Start()
    |       |
    |       +-> discoveryManager.StartPeerDiscovery()
    |       |
    |       +-> handleGossipMessages() (goroutine)
    |       |
    |       +-> httpHandler.StartP2PProtocolServer() (goroutine)
    |       |
    |       +-> loadAndStartServices()
    |
    +-> addon.NewManager().LoadAndStart()
```

### Peer Discovery Flow

```
+-----------------+     +-----------------+     +-----------------+
|  DHT Discovery  |     |  mDNS Discovery |     | GossipSub Topic |
|  (Periodic)     |     |  (Local Net)    |     | (Broadcast)     |
+--------+--------+     +--------+--------+     +--------+--------+
         |                       |                       |
         |                       |                       |
         v                       v                       v
+-----------------------------------------------------------------+
|                     discovery/manager.go                        |
|                                                                 |
|  StartDHTDiscovery():    StartMDNSDiscovery():  HandleLookup:  |
|  - Advertise on topic    - Listen for mDNS      - Verify sig   |
|  - FindPeers() loop      - Connect on found     - Send response|
|  - Connect to peers      - AddTrackedPeer                      |
+-----------------------------------------------------------------+
         |                       |                       |
         +-----------------------+-----------------------+
                                 v
+-----------------------------------------------------------------+
|                    connection/manager.go                        |
|                                                                 |
|  AddTrackedPeer() -> Create ConnectionItem with alias          |
|  HandleNewConnection() -> Update status, test HTTP capability  |
|  TestPeerHTTPCapability() -> GET libp2p://{peerID}/ping        |
+-----------------------------------------------------------------+
```

### Service Discovery Flow

```
+-----------------------------------------------------------------+
|                      Service Provider                           |
|                                                                 |
|  servicePkg.CreateServiceBeacon(privKey)                       |
|       |                                                         |
|       v                                                         |
|  ServiceBeacon.Start()                                          |
|       |                                                         |
|       +-> Join topic "service-{hash(servicePubKey)[:8]}"       |
|       |                                                         |
|       +-> Periodic sendServiceAnnouncement()                    |
|           - Broadcast ServiceAnnouncement with signature        |
|                                                                 |
|  On ServiceLookupRequest:                                        |
|       |                                                         |
|       +-> Verify request signature                              |
|       |                                                         |
|       +-> Decrypt requester peer ID                             |
|       |                                                         |
|       +-> Send ServiceResponse (encrypted)                      |
+-----------------------------------------------------------------+
                              |
                              | GossipSub
                              v
+-----------------------------------------------------------------+
|                      Service Locator                            |
|                                                                 |
|  servicePkg.StartServiceLocator(pubKey, mode)                   |
|       |                                                         |
|       v                                                         |
|  ServiceLocator.Start()                                         |
|       |                                                         |
|       +-> Generate ephemeral keypair                            |
|       |                                                         |
|       +-> Periodic sendServiceLookupRequest()                   |
|           - Encrypt peer ID with service key                    |
|           - Sign with ephemeral key                             |
|                                                                 |
|  On ServiceAnnouncement:                                        |
|       |                                                         |
|       +-> Verify signature                                      |
|       |                                                         |
|       +-> AddTrackedPeer with service key                       |
|       |                                                         |
|       +-> ConnectToServiceProvider()                            |
+-----------------------------------------------------------------+
```

---

## Directory-by-Directory Walkthrough

### types/ - Core Type Definitions

**Purpose**: Defines all core data structures, constants, and type definitions used throughout the application.

**Key Files**:
- `types.go` - Main type definitions

**Key Types**:

```go
type ConnectionItem struct { ... }
type PeerOptions struct { ... }

type LookupRequest struct { ... }
type LookupResponse struct { ... }

type ServiceLookupRequest struct { ... }
type ServiceResponse struct { ... }
type ServiceAnnouncement struct { ... }
type ServiceBeaconMode int

type FigFile struct { ... }
type FigNode struct { ... }

type RouteTable struct { ... }
type ServiceRoutingConfig struct { ... }

type Event struct { ... }
```

**Code Snippet - Peer Options Pattern**:

```go
func NewManualPeerOptions(httpCapable bool) PeerOptions {
    return PeerOptions{
        ConnectionType: ConnTypeManual,
        HTTPCapable:    httpCapable,
    }
}

func NewServicePeerOptions(serviceKey []byte, httpCapable bool) PeerOptions {
    return PeerOptions{
        ServiceKey:     serviceKey,
        ConnectionType: ConnTypeHTTPVerified,
        HTTPCapable:    httpCapable,
    }
}
```

---

### interfaces/ - Interface Contracts

**Purpose**: Defines interface contracts that enable loose coupling between components.

**Key Files**:
- `interfaces.go` - All interface definitions

**Key Interfaces**:

```go
type EventBroadcaster interface {
    BroadcastEvent(event types.Event)
    SendEvent(eventType string, data interface{})
    Subscribe(subscriber WebSocketHub)
    Unsubscribe(subscriber WebSocketHub)
}

type PeerManager interface {
    AddTrackedPeer(peerID peer.ID, options types.PeerOptions) *types.ConnectionItem
    MarkPeerHTTPCapable(peerID peer.ID, bidirectional bool)
    UpdateConnectionStatus(peerID peer.ID, status string)
    GetConnectionInfo(peerID peer.ID) (*types.ConnectionItem, bool)
    GetConnectionsCopy() map[peer.ID]*types.ConnectionItem
    ConnectToPeer(peerID peer.ID) error
    LookupPeer(targetPeerID string, includeFrom bool) error
    HandleNewConnection(peerID peer.ID)
    HandleDisconnection(peerID peer.ID)
    TestPeerHTTPCapability(peerID peer.ID)
}

type CryptoManager interface {
    EncryptWithPublicKey(data, requesterPubKeyBytes []byte) ([]byte, []byte, error)
    DecryptWithPublicKey(encryptedData, nonce, senderPubKeyBytes []byte) ([]byte, error)
    RegisterServiceKey(keyID string, serviceKey crypto.PrivKey)
    GetServiceKeyByID(keyID string) crypto.PrivKey
}

type DiscoveryManager interface {
    StartPeerDiscovery(connectionManager PeerManager)
    StartDHTDiscovery()
    StartMDNSDiscovery(connectionManager PeerManager)
    HandleGossipMessages(cryptoManager CryptoManager, connectionManager PeerManager)
    ProcessGossipMessage(msg *pubsub.Message, ...)
}

type ServiceManager interface {
    CreateServiceBeacon(servicePrivKey crypto.PrivKey) (ServiceBeacon, error)
    StartServiceLocator(servicePubKey crypto.PubKey, mode types.ServiceBeaconMode) error
    ProcessServiceResponse(resp *types.ServiceResponse)
}

type Node interface {
    Start() error
    Close()
    GetHost() host.Host
    GetConnectionManager() PeerManager
    GetServiceManager() ServiceManager
    GetHTTPHandler() HTTPHandler
    GetDiscoveryManager() DiscoveryManager
}
```

---

### crypto/ - Cryptographic Operations

**Purpose**: Provides AES-GCM encryption with ECDH-style key derivation for secure peer communication.

**Key Files**:
- `manager.go` - CryptoManager implementation

**Key Functions**:

```go
type Manager struct {
    host        host.Host
    serviceKey  crypto.PrivKey
    serviceKeys map[string]crypto.PrivKey
}

func (m *Manager) EncryptWithPublicKey(data, requesterPubKeyBytes []byte) ([]byte, []byte, error) {
    ourPrivKeyBytes, _ := crypto.MarshalPrivateKey(ourPrivKey)
    keyMaterial := append(ourPrivKeyBytes, requesterPubKeyBytes...)
    keyHash := sha256.Sum256(keyMaterial)
    aesKey := keyHash[:]
    
    block, _ := aes.NewCipher(aesKey)
    gcm, _ := cipher.NewGCM(block)
    
    nonce := make([]byte, gcm.NonceSize())
    io.ReadFull(rand.Reader, nonce)
    
    ciphertext := gcm.Seal(nil, nonce, data, nil)
    return ciphertext, nonce, nil
}

func (m *Manager) RegisterServiceKey(keyID string, serviceKey crypto.PrivKey)
func (m *Manager) GetServiceKeyByID(keyID string) crypto.PrivKey
```

**Encryption Flow**:
1. Derive shared key: `SHA256(localPrivKey || remotePubKey)`
2. Create AES-256-GCM cipher
3. Generate random nonce
4. Encrypt data with GCM seal

---

### discovery/ - Peer Discovery

**Purpose**: Implements multiple peer discovery mechanisms (DHT, mDNS, GossipSub).

**Key Files**:
- `manager.go` - DiscoveryManager implementation

**Key Components**:

```go
type Manager struct {
    host             host.Host
    dht              *dht.IpfsDHT
    pubsub           *pubsub.PubSub
    eventBroadcaster interfaces.EventBroadcaster
    disableDHT       bool
    noCrypto         bool
    anonLookups      bool
}

func (m *Manager) StartDHTDiscovery() {
    routingDiscovery := drouting.NewRoutingDiscovery(m.dht)
    dutil.Advertise(m.ctx, routingDiscovery, GossipSubTopic)
    
    ticker := time.NewTicker(30 * time.Second)
    for range ticker.C {
        peerChan, _ := routingDiscovery.FindPeers(m.ctx, GossipSubTopic)
        for p := range peerChan {
            m.host.Connect(m.ctx, p)
        }
    }
}

func (m *Manager) StartMDNSDiscovery(connectionManager interfaces.PeerManager) {
    notifee := &peerDiscoveryNotifee{...}
    ser := mdns.NewMdnsService(m.host, "libp2p-proxy", notifee)
    ser.Start()
}

func (m *Manager) ProcessGossipMessage(msg *pubsub.Message, ...) {
    var lookupReq types.LookupRequest
    if err := json.Unmarshal(msg.Data, &lookupReq); err == nil {
        m.HandleLookupRequest(&lookupReq, ...)
    }
}
```

---

### connection/ - Connection Tracking

**Purpose**: Manages peer connection state, aliases, and HTTP capability testing.

**Key Files**:
- `manager.go` - PeerManager implementation

**Key Functions**:

```go
type Manager struct {
    host             host.Host
    connections      map[peer.ID]*types.ConnectionItem
    aliasToPeer      map[string]peer.ID
    usedAliases      map[string]bool
    dht              *dht.IpfsDHT
    gossipTopic      *pubsub.Topic
}

func (m *Manager) AddTrackedPeer(peerID peer.ID, options types.PeerOptions) *types.ConnectionItem {
    alias := m.generateUniqueAliasLocked()
    connItem := &types.ConnectionItem{
        PeerID:         peerID,
        Alias:          alias,
        Status:         types.StatusConnected,
        ConnectionType: options.ConnectionType,
        HTTPCapable:    options.HTTPCapable,
    }
    m.connections[peerID] = connItem
    return connItem
}

func (m *Manager) TestPeerHTTPCapability(peerID peer.ID) {
    client := &http.Client{Transport: m.httpTransport}
    url := fmt.Sprintf("libp2p://%s/ping", peerID)
    resp, err := client.Get(url)
}

func (m *Manager) ConnectToPeer(peerID peer.ID) error {
    if m.dht != nil {
        peerInfo, err := m.dht.FindPeer(m.ctx, peerID)
        if err == nil {
            return m.host.Connect(m.ctx, peerInfo)
        }
    }
    m.LookupPeerViaGossipsub(peerID)
    return nil
}
```

---

### service/ - Service Beacon/Locator

**Purpose**: Implements service discovery using beacon (provider) and locator (consumer) patterns.

**Key Files**:
- `manager.go` - ServiceManager, ServiceBeacon, ServiceLocator

**ServiceBeacon (Provider)**:

```go
type ServiceBeacon struct {
    ServicePrivKey crypto.PrivKey
    gossipTopic    *pubsub.Topic
    mode           types.ServiceBeaconMode
    Alias          string
    FigTemplates   [][]byte
}

func (sb *ServiceBeacon) Start() error {
    serviceTopic, _ := sb.serviceManager.pubsub.Join(topicName)
    serviceSub, _ := serviceTopic.Subscribe()
    
    go sb.startPeriodicAnnouncements()
    go sb.processMessages()
}

func (sb *ServiceBeacon) handleServiceLookupRequest(req *types.ServiceLookupRequest, from peer.ID) {
    peerID := sb.serviceManager.ExtractPeerIDFromServiceRequestWithKey(req, sb.ServicePrivKey)
    
    resp := &types.ServiceResponse{Type: "service-response", Timestamp: time.Now()}
    
    if len(req.PublicKey) > 0 {
        enc, nonce, _ := sb.serviceManager.cryptoManager.EncryptWithServiceKey(
            []byte(responderPeerID), req.PublicKey, sb.ServicePrivKey)
        resp.Encrypted = true
        resp.EncryptedData = enc
        resp.Nonce = nonce
    }
    
    sb.serviceManager.SendDirectServiceResponse(p, resp)
}
```

**ServiceLocator (Consumer)**:

```go
type ServiceLocator struct {
    ServiceKey  crypto.PubKey
    gossipTopic *pubsub.Topic
    mode        types.ServiceBeaconMode
    ephemeralPriv crypto.PrivKey
}

func (sl *ServiceLocator) Start() error {
    priv, _, _ := crypto.GenerateKeyPairWithReader(crypto.Ed25519, 0, rand.Reader)
    sl.ephemeralPriv = priv
    
    go sl.processMessages()
    
    if sl.mode == types.ServiceBeaconModeLookup {
        go sl.startPeriodicRequests()
    }
}

func (sl *ServiceLocator) sendServiceLookupRequest() {
    request := &types.ServiceLookupRequest{
        Type:       "service-lookup",
        ServiceKey: serviceKeyBytes,
        Timestamp:  time.Now(),
    }
    
    enc, nonce, _ := sl.serviceManager.cryptoManager.EncryptPeerIDWithServiceKey(
        sl.serviceManager.host.ID().String(), sl.ServiceKey)
    request.EncryptedFrom = enc
    request.EncryptedNonce = nonce
    request.FromEncrypted = true
    
    sl.gossipTopic.Publish(sl.serviceManager.ctx, data)
}
```

---

### events/ - Event Broadcasting

**Purpose**: Centralized event distribution to WebSocket clients.

**Key Files**:
- `broadcaster.go` - EventBroadcaster implementation

```go
type Broadcaster struct {
    subscribers []interfaces.WebSocketHub
    mutex       sync.RWMutex
    nodeID      string
}

func (b *Broadcaster) BroadcastEvent(event types.Event) {
    b.mutex.RLock()
    defer b.mutex.RUnlock()
    
    for _, subscriber := range b.subscribers {
        go subscriber.BroadcastEvent(event)
    }
}

func (b *Broadcaster) SendEvent(eventType string, data interface{}) {
    event := types.Event{
        Type:      eventType,
        Timestamp: time.Now(),
        Data:      data,
        NodeID:    b.nodeID,
    }
    b.BroadcastEvent(event)
}
```

---

### tunnel/ - TCP Tunneling

**Purpose**: Provides peer-to-peer TCP tunneling over libp2p streams.

**Key Files**:
- `tunnel.go` - Handler implementation

**Protocol**:
```
1. Initiator -> Responder: TunnelRequest{TargetHost, TargetPort}
2. Responder -> Initiator: TunnelResponse{Success, Error}
3. If Success: Bidirectional stream relay
```

```go
const ProtocolID = protocol.ID("/banyan/tcp-tunnel/1.0.0")

type Handler struct {
    host           host.Host
    allowedTargets []string
}

func (h *Handler) handleIncomingStream(s network.Stream) {
    var reqLen uint32
    binary.Read(s, binary.BigEndian, &reqLen)
    
    reqBuf := make([]byte, reqLen)
    io.ReadFull(s, reqBuf)
    
    if !h.isAllowedTarget(targetHost) {
        h.writeResponse(s, false, "target not allowed")
        return
    }
    
    conn, _ := net.DialTimeout("tcp", targetAddr, 10*time.Second)
    h.writeResponse(s, true, "")
    
    go io.Copy(conn, s)
    go io.Copy(s, conn)
}

func (h *Handler) OpenTunnel(peerID peer.ID, targetHost string, targetPort uint16) (network.Stream, error) {
    s, _ := h.host.NewStream(h.ctx, peerID, ProtocolID)
    
    binary.Write(s, binary.BigEndian, uint32(len(reqBuf)))
    s.Write(reqBuf)
    
    return s, nil
}
```

---

### transport/ - Transport Filtering

**Purpose**: Filter multiaddresses by transport protocol (TCP, QUIC, WebSocket, etc.).

**Key Files**:
- `filter.go` - Transport filtering utilities

```go
const (
    TransportTCP  = "tcp"
    TransportQUIC = "quic"
    TransportWS   = "ws"
    TransportWSS  = "wss"
    TransportI2P  = "i2p"
    TransportNym  = "nym"
    TransportTor  = "onion"
)

func ExtractTransport(addr multiaddr.Multiaddr) (string, error) {
    protocols := addr.Protocols()
    for _, proto := range protocols {
        switch proto.Name {
        case "quic", "quic-v1":
            return TransportQUIC, nil
        case "ws":
            return TransportWS, nil
        case "onion", "onion3":
            return TransportTor, nil
        }
    }
}

func FilterMultiaddrsByTransport(addrs []multiaddr.Multiaddr, allowed []string) []multiaddr.Multiaddr {
    filtered := make([]multiaddr.Multiaddr, 0)
    for _, addr := range addrs {
        transport, _ := ExtractTransport(addr)
        if allowedSet[transport] {
            filtered = append(filtered, addr)
        }
    }
    return filtered
}
```

---

### websocket/ - WebSocket Hub

**Purpose**: Manages WebSocket client connections for real-time event streaming.

**Key Files**:
- `hub.go` - Hub and Client implementations

```go
type Hub struct {
    clients    map[*Client]bool
    broadcast  chan types.Event
    register   chan *Client
    unregister chan *Client
}

func (h *Hub) Run() {
    for {
        select {
        case client := <-h.register:
            h.clients[client] = true
        case client := <-h.unregister:
            delete(h.clients, client)
            close(client.send)
        case event := <-h.broadcast:
            for client := range h.clients {
                client.send <- event
            }
        }
    }
}

type Client struct {
    conn *websocket.Conn
    send chan types.Event
    id   string
}
```

---

### management-server/ - HTTP Management API

**Purpose**: Provides HTTP endpoints for node management, monitoring, and control.

**Key Files**:
- `server.go` - Server setup and route registration
- `handlers/` - Individual endpoint handlers

**Endpoints**:

```
Basic:
  GET  /health                    - Health check
  GET  /node/status               - Node status
  POST /node/shutdown             - Shutdown node

Network:
  GET  /network/connections       - List peer connections
  POST /network/connect/{peerID}  - Connect to peer
  POST /network/peers/add         - Add tracked peer

Services:
  POST /services/find             - Find service by key/alias
  GET  /services/figs             - Get service figs
  GET  /services/list             - List services
  POST /services/start            - Start service beacon
  GET  /services/locators         - List locators

Proxy:
  ANY  /proxy/peer/{peerID}/{path}  - Proxy via peer ID
  ANY  /proxy/alias/{alias}/{path}  - Proxy via alias

Router:
  POST /router/add                - Add route
  GET  /router/list               - List routes

Tunnel:
  POST /tunnel/open               - Open tunnel
  GET  /tunnel/list               - List tunnels

Events:
  WS   /events/subscribe          - WebSocket events
```

---

### libp2p-http/ - P2P HTTP Handlers

**Purpose**: HTTP handlers exposed to other peers via libp2p HTTP transport.

**Key Files**:
- `handler.go` - P2P HTTP handler implementation

**Key Endpoints** (exposed to peers):

```go
func (h *Handler) StartP2PProtocolServer() {
    listener, _ := gostream.Listen(h.host, p2phttp.DefaultP2PProtocol)
    mux := http.NewServeMux()
    
    mux.HandleFunc("/ping", h.HandlePing)
    mux.HandleFunc("/greetings", h.HandleGreetings)
    mux.HandleFunc("/status", h.HandleStatus)
    mux.HandleFunc("/connections", h.HandleConnectionsStatus)
    mux.HandleFunc("/services/figs", h.HandleServiceFigs)
    mux.HandleFunc("/discovery/response", h.HandleDiscoveryResponse)
    mux.HandleFunc("/services/response", h.HandleServiceResponse)
    mux.HandleFunc("/router/", h.HandleRouter)
    
    server := &http.Server{Handler: mux}
    server.Serve(listener)
}

func (h *Handler) HandleGreetings(w http.ResponseWriter, r *http.Request) {
    response := types.GreetingResponse{
        PeerID:    h.host.ID().String(),
        ServiceKey: servicePubKeyBytes,
        Data:      serviceInfo,
        Timestamp: time.Now(),
    }
    response.Signature, _ = h.host.Peerstore().PrivKey(h.host.ID()).Sign(payload)
    json.NewEncoder(w).Encode(response)
}
```

---

### addon/ - Addon System

**Purpose**: External addon process management with JSON-RPC communication.

**Key Files**:
- `manager.go` - Addon manager and RPC handling
- `sdk/sdk.go` - Addon SDK for external processes
- `sdk/mux.go` - HTTP multiplexer for addons

**Communication Protocol**:

```
Host <-> Addon (JSON-RPC 2.0 over stdin/stdout)

Host -> Addon:
  - "host_ready" notification on startup
  - "addon_handle_http" request for HTTP forwarding
  - "alias_resolve" request for alias resolution

Addon -> Host:
  - "addon_register_endpoint" to register HTTP handlers
  - "addon_disclose" to provide greeting disclosure info
```

```go
type Manager struct {
    procs       map[string]*addonProcess
    mgmtMount   func(path string, h func(http.ResponseWriter, *http.Request))
    p2pMount    func(path string, h func(http.ResponseWriter, *http.Request))
}

func (m *Manager) LoadAndStart() error {
    data, _ := os.ReadFile(filepath.Join(addonsDir, "addons.json"))
    json.Unmarshal(data, &entries)
    
    for _, ent := range entries {
        m.startAddon(addonsDir, ent)
    }
}

func (m *Manager) handleAddonRequest(p *addonProcess, req *rpcRequest) {
    switch req.Method {
    case "addon_register_endpoint":
        var rp registerEndpointParams
        json.Unmarshal(req.Params, &rp)
        
        if rp.Kind == "local" {
            m.mgmtMount(basePath, handler)
        } else {
            m.p2pMount(basePath, handler)
        }
    case "addon_disclose":
        m.disclosures[p.name] = disclosure
    }
}
```

---

### common/ - Shared Types

**Purpose**: Simple shared types for HTTP responses.

**Key Files**:
- `types.go` - Response type

```go
type Response struct {
    Message   string              `json:"message"`
    Timestamp time.Time           `json:"timestamp"`
    Method    string              `json:"method"`
    Path      string              `json:"path"`
    Headers   map[string][]string `json:"headers"`
}
```

---

### node/ - Main Node Implementation

**Purpose**: Core node implementation that orchestrates all components.

**Key Files**:
- `node.go` - Node struct and lifecycle management

**Node Structure**:

```go
type Node struct {
    host          host.Host
    dht           *dht.IpfsDHT
    pubsub        *pubsub.PubSub
    gossipTopic   *pubsub.Topic
    
    connectionManager interfaces.PeerManager
    cryptoManager     interfaces.CryptoManager
    discoveryManager  interfaces.DiscoveryManager
    serviceManager    interfaces.ServiceManager
    httpHandler       interfaces.HTTPHandler
    tunnelHandler     *tunnelPkg.Handler
    
    eventBroadcaster interfaces.EventBroadcaster
    routeTable       *types.RouteTable
    config           *Config
}

type Config struct {
    PrivKeyFile    *string
    ServicesDir    *string
    DisableDHT     *bool
    NoAnnounce     *bool
    NATTraversal   *bool
    NoCrypto       *bool
    Bootstraps     []string
}
```

**Node Creation**:

```go
func NewNode(ctx context.Context, config *Config, keyLoader PrivateKeyLoader) (*Node, error) {
    opts := []libp2p.Option{
        libp2p.ListenAddrStrings(*config.ListenMultiaddr),
    }
    if config.NATTraversal {
        opts = append(opts,
            libp2p.EnableHolePunching(),
            libp2p.EnableAutoRelayWithPeerSource(peerSource),
            libp2p.NATPortMap())
    }
    h, _ := libp2p.New(opts...)
    
    kademliaDHT, _ := dht.New(ctx, h)
    kademliaDHT.Bootstrap(ctx)
    
    ps, _ := pubsub.NewGossipSub(ctx, h)
    topic, _ := ps.Join("peer-discovery")
    
    node.connectionManager = connectionPkg.NewManager(...)
    node.cryptoManager = cryptoPkg.NewManager(...)
    node.discoveryManager = discoveryPkg.NewManager(...)
    node.serviceManager = servicePkg.NewManager(...)
    node.httpHandler = httpPkg.NewHandler(...)
    
    h.Network().Notify(&network.NotifyBundle{
        ConnectedF:    func(n network.Network, conn network.Conn) { ... },
        DisconnectedF: func(n network.Network, conn network.Conn) { ... },
    })
}
```

**Node Startup**:

```go
func (n *Node) Start() error {
    n.discoveryManager.StartPeerDiscovery(n.connectionManager)
    go n.handleGossipMessages()
    go n.httpHandler.StartP2PProtocolServer()
    n.loadAndStartServices()
    n.SendEvent("node_started", map[string]interface{}{...})
}
```

---

## Usage Example

### Starting a Node

```go
package main

import (
    "context"
    "crypto/ed25519"
    "crypto/x509"
    "encoding/pem"
    "fmt"
    "os"
    
    nodePkg "banyan/node"
    
    "github.com/libp2p/go-libp2p/core/crypto"
)

func loadPrivateKeyFromPEM(filename string) (crypto.PrivKey, error) {
    data, _ := os.ReadFile(filename)
    block, _ := pem.Decode(data)
    
    key, _ := x509.ParsePKCS8PrivateKey(block.Bytes)
    if ed25519Key, ok := key.(ed25519.PrivateKey); ok {
        key = &ed25519Key
    }
    
    privKey, _, _ := crypto.KeyPairFromStdKey(key)
    return privKey, nil
}

func main() {
    ctx := context.Background()
    
    listenAddr := "/ip4/0.0.0.0/tcp/0"
    config := &nodePkg.Config{
        ListenMultiaddr: &listenAddr,
        NATTraversal:    ptrBool(true),
        DisableDHT:      ptrBool(false),
    }
    
    node, err := nodePkg.NewNode(ctx, config, loadPrivateKeyFromPEM)
    if err != nil {
        panic(err)
    }
    defer node.Close()
    
    fmt.Printf("Node started with Peer ID: %s\n", node.GetHost().ID())
    
    if err := node.Start(); err != nil {
        panic(err)
    }
    
    select {}
}

func ptrBool(v bool) *bool { return &v }
```

### Connecting to a Peer

```go
peerID, _ := peer.Decode("12D3KooW...")
err := node.GetConnectionManager().ConnectToPeer(peerID)
```

### Starting a Service Beacon

```go
privKey, _, _ := crypto.GenerateEd25519Key(rand.Reader)
beacon, _ := node.GetServiceManager().CreateServiceBeacon(privKey)
beacon.Start()
```

### Subscribing to Events

```go
// WebSocket client connection
conn, _, _ := websocket.DefaultDialer.Dial("ws://localhost:8080/events/subscribe", nil)

for {
    _, msg, _ := conn.ReadMessage()
    var event types.Event
    json.Unmarshal(msg, &event)
    fmt.Printf("Event: %s - %v\n", event.Type, event.Data)
}
```

---

## Summary

The Banyan node is a well-structured peer-to-peer application built on libp2p with:

1. **Clean separation of concerns** through interfaces
2. **Multiple discovery mechanisms** (DHT, mDNS, GossipSub)
3. **Service discovery** via beacon/locator pattern
4. **Secure communication** with encrypted message exchange
5. **Extensibility** through the addon system
6. **Real-time monitoring** via WebSocket events

The codebase follows Go best practices with:
- Interface-based dependency injection
- Concurrent-safe data structures
- Clear package boundaries
- Comprehensive event logging
