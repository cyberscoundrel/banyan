// Package discovery provides peer discovery mechanisms for the Banyan network.
// It implements multiple discovery strategies including Distributed Hash Table
// (DHT) for wide-area peer discovery, mDNS for local network discovery, and
// GossipSub-based lookup requests for finding specific peers.
//
// The package handles peer lookup requests and responses over the gossip network,
// supports encrypted peer identity exchange, and integrates with the connection
// manager to establish connections to discovered peers.
package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	dutil "github.com/libp2p/go-libp2p/p2p/discovery/util"

	"banyan/interfaces"
	"banyan/types"
)

// Constants - moved most to types package
const (
	GossipSubTopic = types.GossipSubTopic
)

// Manager coordinates peer discovery using multiple mechanisms. It manages
// DHT-based discovery for finding peers in the wider network, mDNS for local
// network discovery, and GossipSub messaging for peer lookup requests. The
// manager also handles event broadcasting for discovery-related notifications.
type Manager struct {
	host             host.Host
	ctx              context.Context
	dht              *dht.IpfsDHT
	pubsub           *pubsub.PubSub
	eventBroadcaster interfaces.EventBroadcaster
	disableDHT       bool
	noCrypto         bool
	anonLookups      bool
	httpTransport    *http.Transport
}

// NewManager creates a new discovery manager with the given libp2p host,
// context, DHT instance, pubsub system, event broadcaster, and configuration
// options. If disableDHT is true, DHT-based discovery is skipped. If noCrypto
// is true, encryption is disabled for lookup responses. If anonLookups is
// false, anonymous lookup requests are ignored.
func NewManager(host host.Host, ctx context.Context, dht *dht.IpfsDHT, pubsub *pubsub.PubSub, eventBroadcaster interfaces.EventBroadcaster, disableDHT bool, noCrypto bool, anonLookups bool, httpTransport *http.Transport) interfaces.DiscoveryManager {
	return &Manager{
		host:             host,
		ctx:              ctx,
		dht:              dht,
		pubsub:           pubsub,
		eventBroadcaster: eventBroadcaster,
		disableDHT:       disableDHT,
		noCrypto:         noCrypto,
		anonLookups:      anonLookups,
		httpTransport:    httpTransport,
	}
}

// sendEvent sends an event via the event broadcaster if one is configured.
func (m *Manager) sendEvent(eventType string, data interface{}) {
	if m.eventBroadcaster != nil {
		m.eventBroadcaster.SendEvent(eventType, data)
	}
}

// StartPeerDiscovery initializes all enabled peer discovery mechanisms.
// It starts DHT bootstrap and discovery (if enabled) and mDNS discovery.
// The connectionManager is used to add discovered peers to tracking.
func (m *Manager) StartPeerDiscovery(connectionManager interfaces.PeerManager) {
	if m.disableDHT {
		m.sendEvent(types.EventInfo, map[string]interface{}{
			"message": "DHT peer discovery disabled - skipping bootstrap and DHT discovery",
		})
	} else {
		// Bootstrap with default bootstrap nodes in the background
		go func() {
			m.sendEvent(types.EventBootstrap, map[string]interface{}{
				"message": "Starting DHT bootstrap",
			})
			if err := m.dht.Bootstrap(m.ctx); err != nil {
				m.sendEvent(types.EventError, map[string]interface{}{
					"message": "DHT bootstrap failed",
					"error":   err.Error(),
				})
			}
		}()

		// Start DHT-based discovery
		go m.StartDHTDiscovery()
	}

	// Start mDNS discovery
	go m.StartMDNSDiscovery(connectionManager)
}

// StartDHTDiscovery starts periodic DHT-based peer discovery. It advertises
// the node's presence on the GossipSub topic and periodically searches for
// other peers advertising on the same topic, attempting to connect to any
// discovered peers.
func (m *Manager) StartDHTDiscovery() {
	routingDiscovery := drouting.NewRoutingDiscovery(m.dht)
	dutil.Advertise(m.ctx, routingDiscovery, GossipSubTopic)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			// Find peers interested in the same topic
			peerChan, err := routingDiscovery.FindPeers(m.ctx, GossipSubTopic)
			if err != nil {
				m.sendEvent(types.EventError, map[string]interface{}{
					"message": "DHT peer discovery failed",
					"error":   err.Error(),
				})
				continue
			}

			// Connect to discovered peers
			for p := range peerChan {
				if p.ID == m.host.ID() {
					continue
				}

				// Try to connect
				err := m.host.Connect(m.ctx, p)
				if err != nil {
					m.sendEvent(types.EventError, map[string]interface{}{
						"message": "Failed to connect to discovered peer",
						"peer_id": p.ID.String(),
						"error":   err.Error(),
					})
				} else {
					m.sendEvent(types.EventDiscovery, map[string]interface{}{
						"message": "Connected to peer via DHT discovery",
						"peer_id": p.ID.String(),
					})
				}
			}
		}
	}
}

// peerDiscoveryNotifee implements the mDNS notification interface for
// receiving peer discovery events.
type peerDiscoveryNotifee struct {
	PeerChan          chan peer.AddrInfo
	discoveryManager  *Manager
	connectionManager interfaces.PeerManager
}

func (n *peerDiscoveryNotifee) HandlePeerFound(pi peer.AddrInfo) {
	n.PeerChan <- pi
}

// StartMDNSDiscovery starts mDNS-based local network peer discovery.
// Discovered peers are automatically connected and added to the connection
// manager as background connections.
func (m *Manager) StartMDNSDiscovery(connectionManager interfaces.PeerManager) {
	notifee := &peerDiscoveryNotifee{
		PeerChan:          make(chan peer.AddrInfo),
		discoveryManager:  m,
		connectionManager: connectionManager,
	}

	ser := mdns.NewMdnsService(m.host, "libp2p-proxy", notifee)
	if err := ser.Start(); err != nil {
		m.sendEvent(types.EventError, map[string]interface{}{
			"message": "Failed to start mDNS discovery",
			"error":   err.Error(),
		})
		return
	}

	m.sendEvent(types.EventInfo, map[string]interface{}{
		"message": "Started mDNS peer discovery",
	})

	for {
		select {
		case <-m.ctx.Done():
			return
		case pi := <-notifee.PeerChan:
			if pi.ID == m.host.ID() {
				continue
			}

			err := m.host.Connect(m.ctx, pi)
			if err != nil {
				m.sendEvent(types.EventError, map[string]interface{}{
					"message": "Failed to connect to mDNS discovered peer",
					"peer_id": pi.ID.String(),
					"error":   err.Error(),
				})
			} else {
				m.sendEvent(types.EventDiscovery, map[string]interface{}{
					"message": "Connected to peer via mDNS discovery",
					"peer_id": pi.ID.String(),
				})

				// Add to tracking as background connection
				connectionManager.AddTrackedPeer(pi.ID, types.NewBackgroundPeerOptions())
			}
		}
	}
}

// IsDHTEnabled returns whether DHT-based discovery is enabled.
func (m *Manager) IsDHTEnabled() bool {
	return !m.disableDHT
}

// GetDiscoveryMethods returns a list of currently active discovery method names.
func (m *Manager) GetDiscoveryMethods() []string {
	methods := []string{"mDNS"}
	if !m.disableDHT {
		methods = append(methods, "DHT")
	}
	return methods
}

// HandleGossipMessages is a placeholder for gossip message handling setup.
// Actual message processing is delegated to ProcessGossipMessage.
func (m *Manager) HandleGossipMessages(cryptoManager interfaces.CryptoManager, connectionManager interfaces.PeerManager) {
	// This would typically be called from the main gossip subscription handler
	// The actual message processing is done in ProcessGossipMessage
}

// ProcessGossipMessage processes a single gossip message for peer discovery.
// It handles lookup requests and responses, decrypting encrypted data if
// necessary and establishing connections to discovered peers.
func (m *Manager) ProcessGossipMessage(msg *pubsub.Message, cryptoManager interfaces.CryptoManager, connectionManager interfaces.PeerManager) {
	// Try to parse as lookup request first
	var lookupReq types.LookupRequest
	if err := json.Unmarshal(msg.Data, &lookupReq); err == nil && lookupReq.Type == "lookup" {
		m.HandleLookupRequest(&lookupReq, msg.ReceivedFrom, cryptoManager, connectionManager)
		return
	}

	// Try to parse as lookup response
	var lookupResp types.LookupResponse
	if err := json.Unmarshal(msg.Data, &lookupResp); err == nil && lookupResp.Type == "response" {
		// If encrypted, decrypt responder's addresses using responder's public key
		if lookupResp.Encrypted && len(lookupResp.EncryptedData) > 0 {
			if m.noCrypto {
				m.sendEvent(types.EventInfo, map[string]interface{}{
					"message": "Skipping encrypted lookup response - crypto disabled",
					"from":    msg.ReceivedFrom.String(),
				})
				return
			}
			if cryptoManager != nil && len(lookupResp.PublicKey) > 0 && len(lookupResp.Nonce) > 0 {
				if plaintext, err := cryptoManager.DecryptWithPublicKey(lookupResp.EncryptedData, lookupResp.Nonce, lookupResp.PublicKey); err == nil {
					var addrs []string
					_ = json.Unmarshal(plaintext, &addrs)
					lookupResp.Addresses = addrs
				} else {
					m.sendEvent(types.EventError, map[string]interface{}{
						"message": "Failed to decrypt lookup response",
						"error":   err.Error(),
					})
					return
				}
			}
		}
		m.ProcessLookupResponse(&lookupResp, connectionManager)
		return
	}

	m.sendEvent(types.EventGossipMessage, map[string]interface{}{
		"message": "Received unknown gossip message type",
		"from":    msg.ReceivedFrom.String(),
	})
}

// HandleLookupRequest processes incoming peer lookup requests from the
// gossip network. It validates the request, optionally checks for anonymous
// lookup restrictions, and sends a response containing the peer's public key
// and optionally encrypted peer ID back to the requester.
func (m *Manager) HandleLookupRequest(req *types.LookupRequest, from peer.ID, cryptoManager interfaces.CryptoManager, connectionManager interfaces.PeerManager) {
	m.sendEvent(types.EventGossipMessage, map[string]interface{}{
		"message": "Received lookup request",
		"from":    from.String(),
		"target":  req.Target,
	})

	// If target is specified and it's not us, ignore
	if req.Target != "" && req.Target != m.host.ID().String() {
		return
	}

	// Check if anonymous lookups are disabled and no peer identity provided
	if !m.anonLookups && req.From == "" && len(req.PublicKey) == 0 {
		m.sendEvent(types.EventInfo, map[string]interface{}{
			"message": "Ignoring anonymous lookup request - anon lookups disabled",
			"from":    from.String(),
		})
		return
	}

	// Do not rely on provided addresses in gossip; use discovery to connect if needed

	// Prepare response
	response := &types.LookupResponse{
		Type:      "response",
		Target:    req.Target,
		From:      m.host.ID().String(),
		Timestamp: time.Now(),
	}

	// Include our public key
	pubKey := m.host.Peerstore().PubKey(m.host.ID())
	if pubKey != nil {
		pubKeyBytes, err := crypto.MarshalPublicKey(pubKey)
		if err == nil {
			response.PublicKey = pubKeyBytes
		}
	}

	// Do not include addresses in gossip responses. If encryption requested, encrypt our PeerID for the requester to resolve.
	if len(req.PublicKey) > 0 && cryptoManager != nil && !m.noCrypto {
		pidBytes := []byte(m.host.ID().String())
		encryptedData, nonce, err := cryptoManager.EncryptWithPublicKey(pidBytes, req.PublicKey)
		if err == nil {
			response.Encrypted = true
			response.EncryptedData = encryptedData
			response.Nonce = nonce
		}
	} else if m.noCrypto {
		response.From = m.host.ID().String()
	}

	// Sign the response
	responseData := fmt.Sprintf("%s:%s:%s", response.Type, response.From, response.Target)
	if signature, err := m.host.Peerstore().PrivKey(m.host.ID()).Sign([]byte(responseData)); err == nil {
		response.Signature = signature
	}

	// Send response directly to requester if possible
	if req.From != "" {
		requesterPeerID, err := peer.Decode(req.From)
		if err == nil {
			m.SendDirectResponse(requesterPeerID, response)
		}
	}
}

// SendDirectResponse sends a lookup response directly to a specific peer
// using libp2p HTTP transport to the peer's /discovery/response endpoint.
func (m *Manager) SendDirectResponse(peerID peer.ID, response *types.LookupResponse) {
	// Send via libp2p HTTP to the peer's protocol server
	responseData, err := json.Marshal(response)
	if err != nil {
		m.sendEvent(types.EventError, map[string]interface{}{
			"message": "Failed to marshal lookup response",
			"error":   err.Error(),
		})
		return
	}
	client := &http.Client{Transport: m.httpTransport}
	url := fmt.Sprintf("libp2p://%s/discovery/response", peerID)
	req, err := http.NewRequestWithContext(m.ctx, "POST", url, bytes.NewReader(responseData))
	if err != nil {
		m.sendEvent(types.EventError, map[string]interface{}{
			"message": "Failed to create direct discovery response request",
			"error":   err.Error(),
			"to":      peerID.String(),
		})
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		m.sendEvent(types.EventError, map[string]interface{}{
			"message": "Failed to send direct discovery response",
			"error":   err.Error(),
			"to":      peerID.String(),
		})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		m.sendEvent(types.EventError, map[string]interface{}{
			"message": "Direct discovery response returned non-2xx",
			"status":  resp.StatusCode,
			"to":      peerID.String(),
		})
		return
	}
}

// ProcessLookupResponse processes a lookup response received from another
// peer. It verifies the response signature if present and attempts to connect
// to the responding peer via the connection manager.
func (m *Manager) ProcessLookupResponse(resp *types.LookupResponse, connectionManager interfaces.PeerManager) {
	m.sendEvent(types.EventGossipMessage, map[string]interface{}{
		"message": "Received lookup response",
		"from":    resp.From,
		"target":  resp.Target,
	})

	// Verify signature if present
	if len(resp.Signature) > 0 && !m.VerifyResponseSignature(resp) {
		m.sendEvent(types.EventError, map[string]interface{}{
			"message": "Invalid signature in lookup response",
			"from":    resp.From,
		})
		return
	}

	// Try to connect to responder via discovery (addresses removed from protocol)
	if resp.From != "" {
		responderPeerID, err := peer.Decode(resp.From)
		if err == nil {
			_ = connectionManager.ConnectToPeer(responderPeerID)
		}
	}
}

// VerifyResponseSignature verifies the cryptographic signature of a lookup
// response to ensure it was sent by the claimed peer. Returns true if the
// signature is valid, false otherwise.
func (m *Manager) VerifyResponseSignature(resp *types.LookupResponse) bool {
	if len(resp.PublicKey) == 0 || len(resp.Signature) == 0 {
		return false
	}

	// Unmarshal the public key
	pubKey, err := crypto.UnmarshalPublicKey(resp.PublicKey)
	if err != nil {
		return false
	}

	// Create the response data that was signed
	responseData := fmt.Sprintf("%s:%s:%s", resp.Type, resp.From, resp.Target)

	// Verify the signature
	valid, err := pubKey.Verify([]byte(responseData), resp.Signature)
	if err != nil {
		return false
	}

	return valid
}
