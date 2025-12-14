package connection

import (
	"context"
	"encoding/json"
	"fmt"
	mathrand "math/rand"
	"net/http"
	"sync"
	"time"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	"banyan/interfaces"
	"banyan/types"
)

// Note: PeerManager interface is now defined in interfaces package

// Manager manages peer connections and tracking
type Manager struct {
	host             host.Host
	ctx              context.Context
	connections      map[peer.ID]*types.ConnectionItem
	connectionsMutex sync.RWMutex
	aliasToPeer      map[string]peer.ID
	usedAliases      map[string]bool
	httpTransport    *http.Transport
	eventBroadcaster interfaces.EventBroadcaster

	// Discovery dependencies
	dht         *dht.IpfsDHT
	gossipTopic *pubsub.Topic
}

// NewManager creates a new connection manager
func NewManager(host host.Host, ctx context.Context, httpTransport *http.Transport,
	dht *dht.IpfsDHT, gossipTopic *pubsub.Topic, eventBroadcaster interfaces.EventBroadcaster) interfaces.PeerManager {
	return &Manager{
		host:             host,
		ctx:              ctx,
		connections:      make(map[peer.ID]*types.ConnectionItem),
		aliasToPeer:      make(map[string]peer.ID),
		usedAliases:      make(map[string]bool),
		httpTransport:    httpTransport,
		eventBroadcaster: eventBroadcaster,
		dht:              dht,
		gossipTopic:      gossipTopic,
	}
}

// sendEvent sends an event via the event broadcaster
func (m *Manager) sendEvent(eventType string, data interface{}) {
	if m.eventBroadcaster != nil {
		m.eventBroadcaster.SendEvent(eventType, data)
	}
}

// AddTrackedPeer adds or updates a tracked peer using the provided options
func (m *Manager) AddTrackedPeer(peerID peer.ID, options types.PeerOptions) *types.ConnectionItem {
	m.connectionsMutex.Lock()
	defer m.connectionsMutex.Unlock()

	// Validate connection type - only allow specific values
	validConnTypes := map[string]bool{
		types.ConnTypeManual:       true,
		types.ConnTypeGossipLookup: true,
		types.ConnTypeHTTPVerified: true,
		types.ConnTypeBackground:   true,
	}
	if !validConnTypes[options.ConnectionType] {
		// Default to background if invalid type provided
		options.ConnectionType = types.ConnTypeBackground
	}

	connItem, exists := m.connections[peerID]
	if !exists {
		alias := m.generateUniqueAliasLocked()
		connItem = &types.ConnectionItem{
			PeerID:         peerID,
			Alias:          alias,
			ServiceKeys:    nil,
			Connected:      time.Now(),
			Status:         types.StatusConnected,
			ConnectionType: options.ConnectionType,
			HTTPCapable:    options.HTTPCapable,
			HTTPTestResult: types.HTTPTestUntested,
		}
		if options.ServiceKey != nil && len(options.ServiceKey) > 0 {
			connItem.ServiceKeys = append(connItem.ServiceKeys, options.ServiceKey)
		}
		m.connections[peerID] = connItem

		if m.aliasToPeer == nil {
			m.aliasToPeer = make(map[string]peer.ID)
		}
		m.aliasToPeer[alias] = peerID

		eventData := map[string]interface{}{
			"peer_id":         peerID.String(),
			"alias":           alias,
			"connection_type": options.ConnectionType,
			"http_capable":    options.HTTPCapable,
			"time":            time.Now(),
		}
		if options.ServiceKey != nil && len(options.ServiceKey) > 0 {
			eventData["service_key"] = fmt.Sprintf("%x", options.ServiceKey[:min(8, len(options.ServiceKey))])
		}

		m.sendEvent(types.EventPeerTracked, eventData)
	} else {
		// For existing connections, only update certain fields if they are "higher priority"
		if options.ConnectionType == types.ConnTypeHTTPVerified || options.ConnectionType == types.ConnTypeManual {
			connItem.ConnectionType = options.ConnectionType
			connItem.HTTPCapable = options.HTTPCapable
		}
		// Update service key: append to list if new
		if options.ServiceKey != nil && len(options.ServiceKey) > 0 {
			found := false
			for _, k := range connItem.ServiceKeys {
				if len(k) == len(options.ServiceKey) {
					same := true
					for i := range k {
						if k[i] != options.ServiceKey[i] {
							same = false
							break
						}
					}
					if same {
						found = true
						break
					}
				}
			}
			if !found {
				connItem.ServiceKeys = append(connItem.ServiceKeys, options.ServiceKey)
			}
		}
		connItem.Status = types.StatusConnected
		connItem.LastDisconnect = nil
	}
	connItem.LastActivity = time.Now()

	return connItem
}

func (m *Manager) generateUniqueAliasLocked() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	const aliasLength = 4

	if m.usedAliases == nil {
		m.usedAliases = make(map[string]bool)
	}

	for {
		alias := make([]byte, aliasLength)
		for i := range alias {
			alias[i] = chars[mathrand.Intn(len(chars))]
		}
		aliasStr := string(alias)

		if !m.usedAliases[aliasStr] {
			m.usedAliases[aliasStr] = true
			return aliasStr
		}
	}
}

// MarkPeerHTTPCapable marks a peer as HTTP capable
func (m *Manager) MarkPeerHTTPCapable(peerID peer.ID, bidirectional bool) {
	m.connectionsMutex.Lock()
	defer m.connectionsMutex.Unlock()

	if conn, exists := m.connections[peerID]; exists {
		conn.HTTPCapable = true
		conn.BidirectionalHTTP = bidirectional
		conn.LastActivity = time.Now()
		now := time.Now()
		conn.LastHTTPTest = &now
		conn.HTTPTestResult = types.HTTPTestSuccess

		m.sendEvent(types.EventHTTPTest, map[string]interface{}{
			"peer_id":       peerID.String(),
			"alias":         conn.Alias,
			"result":        types.HTTPTestSuccess,
			"bidirectional": bidirectional,
			"time":          now,
		})
	}
}

// MarkHTTPTestResult marks the HTTP test result for a peer
func (m *Manager) MarkHTTPTestResult(peerID peer.ID, result string) {
	m.connectionsMutex.Lock()
	defer m.connectionsMutex.Unlock()

	if conn, exists := m.connections[peerID]; exists {
		now := time.Now()
		conn.LastHTTPTest = &now
		conn.HTTPTestResult = result
		conn.LastActivity = now

		m.sendEvent(types.EventHTTPTest, map[string]interface{}{
			"peer_id": peerID.String(),
			"alias":   conn.Alias,
			"result":  result,
			"time":    now,
		})
	}
}

// CleanupOldConnections removes old disconnected connections
func (m *Manager) CleanupOldConnections() {
	m.connectionsMutex.Lock()
	defer m.connectionsMutex.Unlock()

	cutoffTime := time.Now().Add(-30 * time.Minute)
	for peerID, conn := range m.connections {
		if conn.Status == types.StatusDisconnected && conn.LastDisconnect != nil && conn.LastDisconnect.Before(cutoffTime) {
			delete(m.connections, peerID)
			delete(m.aliasToPeer, conn.Alias)
			delete(m.usedAliases, conn.Alias)
		}
	}
}

// GetPeerByAlias returns peer ID by alias
func (m *Manager) GetPeerByAlias(alias string) (peer.ID, bool) {
	m.connectionsMutex.RLock()
	defer m.connectionsMutex.RUnlock()

	peerID, exists := m.aliasToPeer[alias]
	return peerID, exists
}

// GetConnectionsCopy returns a copy of all connections
func (m *Manager) GetConnectionsCopy() map[peer.ID]*types.ConnectionItem {
	m.connectionsMutex.RLock()
	defer m.connectionsMutex.RUnlock()

	conns := make(map[peer.ID]*types.ConnectionItem)
	for k, v := range m.connections {
		connCopy := &types.ConnectionItem{
			PeerID:          v.PeerID,
			Alias:           v.Alias,
			ServiceKeys:     v.ServiceKeys,
			Connected:       v.Connected,
			LastDisconnect:  v.LastDisconnect,
			Status:          v.Status,
			ConnectionType:  v.ConnectionType,
			HTTPCapable:     v.HTTPCapable,
			HTTPTestResult:  v.HTTPTestResult,
			LastHTTPTest:    v.LastHTTPTest,
			ConnectAttempts: v.ConnectAttempts,
		}
		conns[k] = connCopy
	}
	return conns
}

// GetConnectionInfo returns connection info for a specific peer
func (m *Manager) GetConnectionInfo(peerID peer.ID) (*types.ConnectionItem, bool) {
	m.connectionsMutex.RLock()
	defer m.connectionsMutex.RUnlock()

	conn, exists := m.connections[peerID]
	return conn, exists
}

// UpdateConnectionStatus updates the connection status for a peer
func (m *Manager) UpdateConnectionStatus(peerID peer.ID, status string) {
	m.connectionsMutex.Lock()
	defer m.connectionsMutex.Unlock()

	if conn, exists := m.connections[peerID]; exists {
		conn.Status = status
		conn.LastActivity = time.Now()

		if status == types.StatusDisconnected {
			now := time.Now()
			conn.LastDisconnect = &now
		} else if status == types.StatusConnected {
			conn.LastDisconnect = nil
			conn.ConnectAttempts = 0
		}
	}
}

// ConnectToPeer attempts to connect to a peer using DHT and gossipsub lookup
func (m *Manager) ConnectToPeer(peerID peer.ID) error {
	if m.host.Network().Connectedness(peerID) == network.Connected {
		return nil
	}

	_ = m.AddTrackedPeer(peerID, types.NewManualPeerOptions(true))
	m.UpdateConnectionStatus(peerID, types.StatusConnecting)

	// Try DHT lookup first (if enabled and available)
	if m.dht != nil {
		m.sendEvent(types.EventDHTLookup, map[string]interface{}{
			"message": "Attempting DHT lookup for peer",
			"peer_id": peerID.String(),
		})

		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()

		peerInfo, err := m.dht.FindPeer(ctx, peerID)
		if err == nil {
			m.sendEvent(types.EventDHTLookup, map[string]interface{}{
				"message": "Found peer via DHT",
				"peer_id": peerID.String(),
				"addrs":   peerInfo.Addrs,
			})

			if err := m.host.Connect(ctx, peerInfo); err == nil {
				m.sendEvent(types.EventConnection, map[string]interface{}{
					"message": "Successfully connected to peer via DHT",
					"peer_id": peerID.String(),
				})
				return nil
			}
		}
	}

	// Fallback to gossipsub lookup
	m.LookupPeerViaGossipsub(peerID)
	return nil
}

// LookupPeer searches for a peer using gossipsub and DHT
func (m *Manager) LookupPeer(targetPeerID string, includeFrom bool) error {
	req := types.LookupRequest{
		Type:      "lookup",
		Target:    targetPeerID,
		Timestamp: time.Now(),
	}

	pubKey := m.host.Peerstore().PubKey(m.host.ID())
	if pubKey != nil {
		pubKeyBytes, err := crypto.MarshalPublicKey(pubKey)
		if err == nil {
			req.PublicKey = pubKeyBytes
		}
	}

	if includeFrom {
		req.From = m.host.ID().String()
		// Include our addresses for discovery gossip lookups
		addresses := make([]string, 0, len(m.host.Addrs()))
		for _, addr := range m.host.Addrs() {
			addresses = append(addresses, fmt.Sprintf("%s/p2p/%s", addr, m.host.ID()))
		}
		req.Addresses = addresses
	}

	reqData, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal lookup request: %w", err)
	}

	if err := m.gossipTopic.Publish(m.ctx, reqData); err != nil {
		return fmt.Errorf("failed to publish lookup request: %w", err)
	}

	if m.dht != nil {
		go func() {
			targetPeer, err := peer.Decode(targetPeerID)
			if err != nil {
				return
			}

			peerInfo, err := m.dht.FindPeer(m.ctx, targetPeer)
			if err != nil {
				m.sendEvent(types.EventError, map[string]interface{}{
					"message": "DHT lookup failed",
					"error":   err.Error(),
					"peer_id": targetPeerID,
				})
				return
			}

			m.sendEvent(types.EventDHTLookup, map[string]interface{}{
				"message": "Found peer via DHT",
				"peer_id": targetPeerID,
			})
			if err := m.host.Connect(m.ctx, peerInfo); err != nil {
				m.sendEvent(types.EventError, map[string]interface{}{
					"message": "Failed to connect to DHT-found peer",
					"error":   err.Error(),
					"peer_id": targetPeerID,
				})
			}
		}()
	}

	m.sendEvent(types.EventDHTLookup, map[string]interface{}{
		"message": "Published lookup request via gossipsub",
		"target":  targetPeerID,
	})

	return nil
}

// LookupPeerViaGossipsub performs a gossipsub-based peer lookup
func (m *Manager) LookupPeerViaGossipsub(peerID peer.ID) {
	m.sendEvent(types.EventDHTLookup, map[string]interface{}{
		"message": "Starting gossipsub lookup for peer",
		"peer_id": peerID.String(),
	})

	err := m.LookupPeer(peerID.String(), true)
	if err != nil {
		m.sendEvent(types.EventError, map[string]interface{}{
			"message": "Failed to start gossipsub lookup",
			"error":   err.Error(),
			"peer_id": peerID.String(),
		})
	}
}

// ConnectToRequesterDirectly attempts to connect directly to a peer using provided multiaddresses
func (m *Manager) ConnectToRequesterDirectly(peerID peer.ID, addresses []string) {
	m.sendEvent(types.EventConnection, map[string]interface{}{
		"message":   "Attempting direct connection to lookup requester",
		"peer_id":   peerID.String(),
		"addresses": addresses,
	})

	if m.host.Network().Connectedness(peerID) == network.Connected {
		m.sendEvent(types.EventInfo, map[string]interface{}{
			"message": "Already connected to lookup requester",
			"peer_id": peerID.String(),
		})
		m.AddTrackedPeer(peerID, types.NewGossipPeerOptions(true))
		return
	}

	for _, addrStr := range addresses {
		addr, err := multiaddr.NewMultiaddr(addrStr)
		if err != nil {
			m.sendEvent(types.EventError, map[string]interface{}{
				"message": "Invalid multiaddress from lookup requester",
				"error":   err.Error(),
				"address": addrStr,
				"peer_id": peerID.String(),
			})
			continue
		}

		peerInfo, err := peer.AddrInfoFromP2pAddr(addr)
		if err != nil {
			m.sendEvent(types.EventError, map[string]interface{}{
				"message": "Failed to parse peer info from address",
				"error":   err.Error(),
				"address": addrStr,
				"peer_id": peerID.String(),
			})
			continue
		}

		if peerInfo.ID != peerID {
			m.sendEvent(types.EventError, map[string]interface{}{
				"message":  "Peer ID mismatch in provided address",
				"expected": peerID.String(),
				"actual":   peerInfo.ID.String(),
				"address":  addrStr,
			})
			continue
		}

		ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
		err = m.host.Connect(ctx, *peerInfo)
		cancel()

		if err != nil {
			m.sendEvent(types.EventError, map[string]interface{}{
				"message": "Failed to connect to lookup requester via provided address",
				"error":   err.Error(),
				"address": addrStr,
				"peer_id": peerID.String(),
			})
			continue
		}

		m.sendEvent(types.EventConnection, map[string]interface{}{
			"message": "Successfully connected to lookup requester via direct address",
			"peer_id": peerID.String(),
			"address": addrStr,
		})

		m.AddTrackedPeer(peerID, types.NewGossipPeerOptions(true))
		return
	}

	m.sendEvent(types.EventError, map[string]interface{}{
		"message": "Failed to connect to lookup requester via any provided address",
		"peer_id": peerID.String(),
	})
}

// HandleNewConnection handles new incoming connections
func (m *Manager) HandleNewConnection(peerID peer.ID) {
	m.sendEvent(types.EventPeerConnected, map[string]interface{}{
		"peer_id": peerID.String(),
		"time":    time.Now(),
	})

	connItem, tracked := m.GetConnectionInfo(peerID)
	if tracked {
		m.UpdateConnectionStatus(peerID, types.StatusConnected)

		if connItem.HTTPCapable || connItem.ConnectionType == types.ConnTypeManual {
			time.AfterFunc(500*time.Millisecond, func() {
				go m.CreateBidirectionalConnection(peerID)
			})
		}

		m.sendEvent(types.EventInfo, map[string]interface{}{
			"message":         "Tracked peer reconnected",
			"peer_id":         peerID.String(),
			"alias":           connItem.Alias,
			"connection_type": connItem.ConnectionType,
		})
	} else {
		m.sendEvent(types.EventInfo, map[string]interface{}{
			"message": "Background connection from peer - not tracking for HTTP",
			"peer_id": peerID.String(),
		})
	}
}

// HandleDisconnection handles peer disconnections
func (m *Manager) HandleDisconnection(peerID peer.ID) {
	m.sendEvent(types.EventPeerDisconnected, map[string]interface{}{
		"peer_id": peerID.String(),
		"time":    time.Now(),
	})

	m.UpdateConnectionStatus(peerID, types.StatusDisconnected)

	time.AfterFunc(30*time.Minute, func() {
		m.CleanupOldConnections()
	})
}

// CreateBidirectionalConnections attempts to create connections back to all tracked HTTP-capable peers
func (m *Manager) CreateBidirectionalConnections() {
	connections := m.GetConnectionsCopy()
	httpCapablePeers := make([]peer.ID, 0)
	for peerID, connItem := range connections {
		if connItem.HTTPCapable || connItem.ConnectionType == types.ConnTypeManual {
			httpCapablePeers = append(httpCapablePeers, peerID)
		}
	}

	m.sendEvent(types.EventDiscovery, map[string]interface{}{
		"message": "Testing bidirectional HTTP connections with HTTP-capable peers",
		"count":   len(httpCapablePeers),
	})

	for _, peerID := range httpCapablePeers {
		go m.CreateBidirectionalConnection(peerID)
	}
}

// CreateBidirectionalConnection creates a connection back to a specific HTTP-capable peer
func (m *Manager) CreateBidirectionalConnection(peerID peer.ID) {
	connItem, exists := m.GetConnectionInfo(peerID)
	if !exists {
		m.sendEvent(types.EventError, map[string]interface{}{
			"message": "Skipping bidirectional test for untracked peer",
			"peer_id": peerID.String(),
		})
		return
	}

	if connItem.HTTPTestResult == types.HTTPTestSuccess &&
		connItem.LastHTTPTest != nil &&
		time.Since(*connItem.LastHTTPTest) < 30*time.Second {
		m.sendEvent(types.EventInfo, map[string]interface{}{
			"message": "Recently tested bidirectional connection with peer",
			"peer_id": peerID.String(),
		})
		return
	}

	m.sendEvent(types.EventConnection, map[string]interface{}{
		"message": "Testing bidirectional HTTP connection to tracked peer",
		"peer_id": peerID.String(),
	})

	if m.host.Network().Connectedness(peerID) != network.Connected {
		m.sendEvent(types.EventError, map[string]interface{}{
			"message": "Not connected to peer",
			"peer_id": peerID.String(),
		})
		m.MarkHTTPTestResult(peerID, types.HTTPTestFailed)
		return
	}

	m.MarkHTTPTestResult(peerID, types.HTTPTestPending)
	m.TestPeerHTTPCapability(peerID)
}

// TestPeerHTTPCapability tests if a peer has HTTP server capability
func (m *Manager) TestPeerHTTPCapability(peerID peer.ID) {
	client := &http.Client{Transport: m.httpTransport}

	url := fmt.Sprintf("libp2p://%s/ping", peerID)
	ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		m.sendEvent(types.EventError, map[string]interface{}{
			"message": "Failed to create ping request",
			"error":   err.Error(),
			"peer_id": peerID.String(),
		})
		m.MarkHTTPTestResult(peerID, types.HTTPTestFailed)
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		m.sendEvent(types.EventError, map[string]interface{}{
			"message": "Failed to ping peer",
			"error":   err.Error(),
			"peer_id": peerID.String(),
		})
		m.MarkHTTPTestResult(peerID, types.HTTPTestFailed)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		m.sendEvent(types.EventError, map[string]interface{}{
			"message": "Ping request failed with status",
			"status":  resp.StatusCode,
			"peer_id": peerID.String(),
		})
		m.MarkHTTPTestResult(peerID, types.HTTPTestFailed)
		return
	}

	m.sendEvent(types.EventConnection, map[string]interface{}{
		"message": "Successfully established bidirectional HTTP connection with peer",
		"peer_id": peerID.String(),
	})

	m.MarkPeerHTTPCapable(peerID, true)
}
