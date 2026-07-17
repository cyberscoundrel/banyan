// Package service provides service beacon and locator management for peer discovery
// in a distributed network. It implements a pub/sub-based service discovery mechanism
// where nodes can announce their services (beacons) or discover services offered by
// other peers (locators).
//
// The package supports multiple concurrent service beacons and locators, allowing a
// single node to both provide and consume multiple services simultaneously. Service
// discovery uses libp2p gossipsub for efficient broadcast-based announcements and
// direct peer-to-peer responses for targeted communication.
//
// Key components:
//   - Manager: Coordinates beacon and locator lifecycle and handles peer connections
//   - ServiceBeacon: Announces service availability to the network
//   - ServiceLocator: Discovers and connects to service providers
//
// Security features include optional cryptographic signatures on announcements,
// encrypted peer ID exchange, and transport-based address filtering.
package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"strings"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	"banyan/interfaces"
	"banyan/transport"
	"banyan/types"
)

// LookupResponse represents a response to a peer lookup request.
// It contains the responder's peer information, addresses, and optional
// cryptographic material for secure communication.
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

// Connection and event type constants.
const (
	// ConnTypeHTTPVerified indicates a connection verified via HTTP transport.
	ConnTypeHTTPVerified = "http-verified"
	// EventConnection is emitted when a peer connection is established.
	EventConnection = "connection"
	// EventError is emitted when an error occurs.
	EventError = "error"
	// EventInfo is emitted for informational events.
	EventInfo = "info"
)

// ServiceLocator discovers and connects to service providers for a specific service.
// It operates in either announce mode (responding to lookup requests) or lookup mode
// (actively searching for providers). The locator uses gossipsub for discovery and
// supports encrypted peer ID exchange for privacy.
type ServiceLocator struct {
	serviceManager *Manager
	ServiceKey     crypto.PubKey
	gossipTopic    *pubsub.Topic
	gossipSub      *pubsub.Subscription
	mode           types.ServiceBeaconMode
	limit          int
	ticker         *time.Ticker
	// Ephemeral key used to sign locator requests and receive directed responses
	ephemeralPriv crypto.PrivKey
	ephemeralPub  []byte
}

// Start initializes the locator's ephemeral keypair, begins processing incoming
// messages, and starts periodic lookup requests if in lookup mode.
func (sl *ServiceLocator) Start() error {
	// Generate ephemeral keypair for this locator session if not already present
	if sl.ephemeralPriv == nil {
		if priv, _, err := crypto.GenerateKeyPairWithReader(crypto.Ed25519, 0, rand.Reader); err == nil {
			sl.ephemeralPriv = priv
			if pub := priv.GetPublic(); pub != nil {
				if b, err2 := crypto.MarshalPublicKey(pub); err2 == nil {
					sl.ephemeralPub = b
				}
			}
		} else {
			sl.serviceManager.eventSender(EventError, map[string]interface{}{
				"message": "Failed to generate locator ephemeral key",
				"error":   err.Error(),
			})
		}
	}
	// Start message processing (only when we own the subscription; piggybacked
	// locators receive messages forwarded by the beacon instead).
	if sl.gossipSub != nil {
		go sl.processMessages()
	}

	// Start periodic requests if in lookup mode
	if sl.mode == types.ServiceBeaconModeLookup {
		// Wait for gossipsub mesh to form before sending first message
		// This prevents messages from being lost when nodes start simultaneously
		go func() {
			time.Sleep(1 * time.Second)
			// log.Printf("[LOCATOR] Sending initial lookup request after 1s delay")
			sl.serviceManager.eventSender(EventInfo, map[string]interface{}{
				"message": "Sending initial service lookup request after mesh formation delay",
			})
			sl.sendServiceLookupRequest()
		}()
		// Start periodic requests
		go sl.startPeriodicRequests()
	}

	return nil
}

// Stop halts the locator's periodic requests and cancels message processing.
func (sl *ServiceLocator) Stop() error {
	if sl.ticker != nil {
		sl.ticker.Stop()
		sl.ticker = nil
	}

	if sl.gossipSub != nil {
		sl.gossipSub.Cancel()
	}

	return nil
}

// GetMode returns the current operating mode (announce or lookup).
func (sl *ServiceLocator) GetMode() types.ServiceBeaconMode {
	return sl.mode
}

// SetMode configures the operating mode (announce or lookup).
func (sl *ServiceLocator) SetMode(mode types.ServiceBeaconMode) {
	sl.mode = mode
}

// GetLimit returns the maximum number of service providers to discover.
func (sl *ServiceLocator) GetLimit() int {
	return sl.limit
}

// SetLimit configures the maximum number of service providers to discover.
func (sl *ServiceLocator) SetLimit(limit int) {
	sl.limit = limit
}

// IsRunning reports whether the locator is actively processing requests.
func (sl *ServiceLocator) IsRunning() bool {
	return sl.ticker != nil
}

// processMessages handles incoming PubSub messages
func (sl *ServiceLocator) processMessages() {
	for {
		msg, err := sl.gossipSub.Next(sl.serviceManager.ctx)
		if err != nil {
			sl.serviceManager.eventSender(EventError, map[string]interface{}{
				"message": "Failed to get next message for service locator",
				"error":   err.Error(),
			})
			return
		}

		// Skip our own messages
		if msg.ReceivedFrom == sl.serviceManager.host.ID() {
			continue
		}

		sl.serviceManager.eventSender(EventInfo, map[string]interface{}{
			"message": fmt.Sprintf("[LOCATOR] Received gossipsub message from %s (len=%d)", msg.ReceivedFrom.String()[:8], len(msg.Data)),
		})

		// Process the message
		sl.handleMessage(msg)
	}
}

// handleMessage processes a single PubSub message, dispatching by the "type"
// field to avoid ambiguous struct parsing (ServiceLookupRequest and
// ServiceAnnouncement share the "serviceKey" JSON tag, so blind unmarshal
// would misroute lookup requests through the announcement handler).
func (sl *ServiceLocator) handleMessage(msg *pubsub.Message) {
	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(msg.Data, &envelope); err != nil {
		return
	}

	switch envelope.Type {
	case "service-announcement":
		var announcement types.ServiceAnnouncement
		if err := json.Unmarshal(msg.Data, &announcement); err == nil {
			sl.handleServiceAnnouncement(&announcement, msg.ReceivedFrom)
		}
	case "service-lookup":
		var request types.ServiceLookupRequest
		if err := json.Unmarshal(msg.Data, &request); err == nil {
			sl.handleServiceLookupRequest(&request, msg.ReceivedFrom)
		}
	case "service-response":
		var response types.ServiceResponse
		if err := json.Unmarshal(msg.Data, &response); err == nil {
			sl.serviceManager.ProcessServiceResponse(&response)
		}
	default:
		return
	}

	// Unknown message type
	sl.serviceManager.eventSender(EventError, map[string]interface{}{
		"message": "Unknown message type received by locator",
		"from":    msg.ReceivedFrom.String(),
	})
}

// handleServiceAnnouncement processes service announcements
func (sl *ServiceLocator) handleServiceAnnouncement(announcement *types.ServiceAnnouncement, from peer.ID) {
	// Verify this announcement is for our service
	expectedKey, err := crypto.MarshalPublicKey(sl.ServiceKey)
	if err != nil {
		// log.Printf("[LOCATOR] Failed to marshal service key: %v", err)
		return
	}

	if !sl.equal(announcement.ServiceKey, expectedKey) {
		if len(expectedKey) >= 8 && len(announcement.ServiceKey) >= 8 {
			sl.serviceManager.eventSender(EventInfo, map[string]interface{}{
				"message":  "Received announcement for different service key (ignoring)",
				"from":     from.String(),
				"expected": fmt.Sprintf("%x", expectedKey[:8]),
				"received": fmt.Sprintf("%x", announcement.ServiceKey[:8]),
			})
		}
		return // Not for our service
	}

	// log.Printf("[LOCATOR] Received announcement from %s for service key %x (has_peer_id: %v)", from.String()[:8], expectedKey[:8], announcement.PeerID != "")
	sl.serviceManager.eventSender(EventInfo, map[string]interface{}{
		"message":     fmt.Sprintf("[LOCATOR] Received announcement from %s for service key %x (has_peer_id: %v)", from.String()[:8], expectedKey[:8], announcement.PeerID != ""),
		"from":        from.String(),
		"service_key": fmt.Sprintf("%x", expectedKey[:8]),
		"has_peer_id": announcement.PeerID != "",
	})

	// Enforce crypto policy: if crypto required, require valid signature
	if !sl.serviceManager.noCrypto {
		if len(announcement.Signature) == 0 || !sl.serviceManager.verifyServiceAnnouncement(announcement) {
			sl.serviceManager.eventSender(EventError, map[string]interface{}{
				"message": "Rejected service announcement - missing/invalid signature",
				"from":    from.String(),
			})
			return
		}
	}

	sl.serviceManager.eventSender(EventInfo, map[string]interface{}{
		"message":      "Received service announcement",
		"provider":     announcement.PeerID,
		"from":         from.String(),
		"service_data": announcement.Data,
	})

	// Tag peer with this service key and connect if in lookup mode
	if sl.mode == types.ServiceBeaconModeLookup {
		if announcement.PeerID == "" {
			return
		}
		if peerID, err := peer.Decode(announcement.PeerID); err == nil {
			if conn, ok := sl.serviceManager.connectionManager.GetConnectionInfo(peerID); ok && conn != nil {
				sl.serviceManager.connectionManager.AddTrackedPeer(peerID, types.NewServicePeerOptions(expectedKey, conn.HTTPCapable))
			} else {
				sl.serviceManager.connectionManager.AddTrackedPeer(peerID, types.NewServicePeerOptions(expectedKey, true))
				sl.serviceManager.ConnectToServiceProvider(peerID)
			}
		}
	}
}

// handleServiceLookupRequest processes service lookup requests
func (sl *ServiceLocator) handleServiceLookupRequest(request *types.ServiceLookupRequest, from peer.ID) {
	// Only respond if we're in announce mode
	if sl.mode != types.ServiceBeaconModeAnnounce {
		return
	}

	// Verify request is for our service
	expectedKey, _ := crypto.MarshalPublicKey(sl.ServiceKey)
	if !sl.equal(request.ServiceKey, expectedKey) {
		return // Not for our service
	}

	// Check if crypto is disabled and request contains encrypted data
	if sl.serviceManager.noCrypto && request.FromEncrypted {
		sl.serviceManager.eventSender(EventInfo, map[string]interface{}{
			"message": "Skipping encrypted service lookup request - crypto disabled",
			"from":    from.String(),
		})
		return
	}

	sl.serviceManager.eventSender(EventInfo, map[string]interface{}{
		"message": "Received service lookup request",
		"from":    from.String(),
	})

	// Send announcement in response
	sl.sendServiceAnnouncement()
}

// startPeriodicRequests starts sending periodic lookup requests
func (sl *ServiceLocator) startPeriodicRequests() {
	sl.ticker = time.NewTicker(5 * time.Second)
	defer sl.ticker.Stop()

	for range sl.ticker.C {
		sl.sendServiceLookupRequest()
	}
}

// sendServiceLookupRequest sends a lookup request for the service
func (sl *ServiceLocator) sendServiceLookupRequest() {
	serviceKeyBytes, _ := crypto.MarshalPublicKey(sl.ServiceKey)

	request := &types.ServiceLookupRequest{
		Type:       "service-lookup",
		ServiceKey: serviceKeyBytes,
		Timestamp:  time.Now(),
	}

	// Populate peer identity (encrypted or plaintext based on crypto policy)
	if sl.serviceManager.noCrypto {
		// Plaintext mode
		request.From = sl.serviceManager.host.ID().String()
	} else {
		// Encrypted mode: include our ephemeral locator public key for directed encrypted response
		if len(sl.ephemeralPub) > 0 {
			request.PublicKey = sl.ephemeralPub
		}
		if enc, nonce, err := sl.serviceManager.cryptoManager.EncryptPeerIDWithServiceKey(sl.serviceManager.host.ID().String(), sl.ServiceKey); err == nil {
			request.EncryptedFrom = enc
			request.EncryptedNonce = nonce
			request.FromEncrypted = true
		}
	}

	// Always sign the request with the locator ephemeral key when available
	if sl.ephemeralPriv != nil {
		canonical := fmt.Sprintf("%s|%x|%d", request.Type, sha256.Sum256(request.ServiceKey), request.Timestamp.Unix())
		if sig, err := sl.ephemeralPriv.Sign([]byte(canonical)); err == nil {
			request.Signature = sig
		}
	}

	data, err := json.Marshal(request)
	if err != nil {
		sl.serviceManager.eventSender(EventError, map[string]interface{}{
			"message": "Failed to marshal service lookup request",
			"error":   err.Error(),
		})
		return
	}

	// log.Printf("[LOCATOR] Publishing lookup request for service key %x", serviceKeyBytes[:8])
	sl.serviceManager.eventSender(EventInfo, map[string]interface{}{
		"message":     "Publishing service lookup request",
		"service_key": fmt.Sprintf("%x", serviceKeyBytes[:8]),
	})

	if err := sl.gossipTopic.Publish(sl.serviceManager.ctx, data); err != nil {
		sl.serviceManager.eventSender(EventError, map[string]interface{}{
			"message": "Failed to publish service lookup request",
			"error":   err.Error(),
		})
		return
	}

	sl.serviceManager.eventSender(EventInfo, map[string]interface{}{
		"message":     "Sent service lookup request",
		"service_key": fmt.Sprintf("%x", serviceKeyBytes[:8]),
	})
}

// sendServiceAnnouncement sends a service announcement
func (sl *ServiceLocator) sendServiceAnnouncement() {
	serviceKeyBytes, _ := crypto.MarshalPublicKey(sl.ServiceKey)

	announcement := &types.ServiceAnnouncement{
		Type:       "service-announcement",
		ServiceKey: serviceKeyBytes,
		PeerID:     sl.serviceManager.host.ID().String(),
		Timestamp:  time.Now(),
		Data:       map[string]interface{}{},
	}

	data, err := json.Marshal(announcement)
	if err != nil {
		sl.serviceManager.eventSender(EventError, map[string]interface{}{
			"message": "Failed to marshal service announcement",
			"error":   err.Error(),
		})
		return
	}

	if err := sl.gossipTopic.Publish(sl.serviceManager.ctx, data); err != nil {
		sl.serviceManager.eventSender(EventError, map[string]interface{}{
			"message": "Failed to publish service announcement",
			"error":   err.Error(),
		})
		return
	}

	sl.serviceManager.eventSender(EventInfo, map[string]interface{}{
		"message":     "Sent service announcement from locator",
		"service_key": fmt.Sprintf("%x", serviceKeyBytes[:8]),
	})
}

// getNodeAddresses returns the node's addresses as strings
func (sl *ServiceLocator) getNodeAddresses() []string {
	addresses := make([]string, 0, len(sl.serviceManager.host.Addrs()))
	for _, addr := range sl.serviceManager.host.Addrs() {
		addresses = append(addresses, fmt.Sprintf("%s/p2p/%s", addr, sl.serviceManager.host.ID()))
	}
	return addresses
}

// equal compares two byte slices for equality
func (sl *ServiceLocator) equal(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ServiceBeacon announces service availability to the network.
// It periodically broadcasts service announcements and responds to lookup requests
// from peers seeking the service. Beacons can optionally include peer IDs in
// announcements and support transport-restricted address filtering via fig files.
type ServiceBeacon struct {
	serviceManager *Manager
	ServicePrivKey crypto.PrivKey
	gossipTopic    *pubsub.Topic
	gossipSub      *pubsub.Subscription
	mode           types.ServiceBeaconMode
	ticker         *time.Ticker
	peers          []peer.ID
	// Metadata
	Alias        string
	FigTemplates [][]byte
	FigFile      *types.FigFile // Optional: fig file for transport restrictions and path-based routing
	// Behavior flags
	includePeerIDInAnnouncements   bool
	onlyIncludePeerIDInDirectReply bool
	// Piggyback: locator sharing this beacon's GossipSub subscription so both
	// can operate on the same topic without a second Subscribe() call.
	piggybackLocator *ServiceLocator
}

// Start begins periodic announcements and starts processing incoming lookup requests.
func (sb *ServiceBeacon) Start() error {
	// Start ticker for periodic announcements (every 5 seconds until peers found)
	sb.ticker = time.NewTicker(5 * time.Second)

	// Start message processing
	go sb.processMessages()

	// Send immediate announcement on startup (don't wait 5 seconds)
	// Wait for gossipsub mesh to form before sending first message
	// This prevents messages from being lost when nodes start simultaneously
	if sb.mode == types.ServiceBeaconModeAnnounce {
		go func() {
			time.Sleep(1 * time.Second)
			// log.Printf("[BEACON] Sending initial announcement after 1s delay")
			sb.serviceManager.eventSender(EventInfo, map[string]interface{}{
				"message": "Sending initial service announcement after mesh formation delay",
			})
			sb.sendServiceAnnouncement()
		}()
	}

	// Start periodic announcements
	go sb.startPeriodicAnnouncements()

	return nil
}

// Stop halts periodic announcements and cancels message processing.
func (sb *ServiceBeacon) Stop() error {
	if sb.ticker != nil {
		sb.ticker.Stop()
		sb.ticker = nil
	}

	if sb.gossipSub != nil {
		sb.gossipSub.Cancel()
	}

	return nil
}

// GetServiceKey returns the service's private key used for signing announcements.
func (sb *ServiceBeacon) GetServiceKey() crypto.PrivKey {
	return sb.ServicePrivKey
}

// GetMode returns the current operating mode.
func (sb *ServiceBeacon) GetMode() types.ServiceBeaconMode {
	return sb.mode
}

// SetMode configures the operating mode.
func (sb *ServiceBeacon) SetMode(mode types.ServiceBeaconMode) {
	sb.mode = mode
}

// GetPeers returns the list of peers currently connected to this beacon.
func (sb *ServiceBeacon) GetPeers() []peer.ID {
	return sb.peers
}

// GetAlias returns the human-readable alias configured for this beacon.
// Returns an empty string if no alias was set.
func (sb *ServiceBeacon) GetAlias() string { return sb.Alias }

// GetFigTemplates returns the raw fig template bytes associated with this beacon.
// Fig templates define transport restrictions and routing rules.
func (sb *ServiceBeacon) GetFigTemplates() [][]byte { return sb.FigTemplates }

// processMessages handles incoming PubSub messages
func (sb *ServiceBeacon) processMessages() {
	for {
		msg, err := sb.gossipSub.Next(sb.serviceManager.ctx)
		if err != nil {
			sb.serviceManager.eventSender(EventError, map[string]interface{}{
				"message": "Failed to get next message for service beacon",
				"error":   err.Error(),
			})
			return
		}

		// Skip our own messages
		if msg.ReceivedFrom == sb.serviceManager.host.ID() {
			continue
		}

		// Process the message
		sb.handleMessage(msg)

		// Forward to piggybacked locator so it can handle announcements,
		// responses, etc. on the same subscription.
		if sb.piggybackLocator != nil {
			sb.piggybackLocator.handleMessage(msg)
		}
	}
}

// handleMessage processes a single PubSub message using type-based dispatch.
func (sb *ServiceBeacon) handleMessage(msg *pubsub.Message) {
	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(msg.Data, &envelope); err != nil {
		return
	}

	switch envelope.Type {
	case "service-lookup":
		var request types.ServiceLookupRequest
		if err := json.Unmarshal(msg.Data, &request); err == nil {
			sb.handleServiceLookupRequest(&request, msg.ReceivedFrom)
		}
	}
}

// handleServiceLookupRequest processes service lookup requests
func (sb *ServiceBeacon) handleServiceLookupRequest(request *types.ServiceLookupRequest, from peer.ID) {
	// Check if request is for our service
	serviceKeyBytes, err := crypto.MarshalPublicKey(sb.ServicePrivKey.GetPublic())
	if err != nil {
		// log.Printf("[BEACON] Failed to marshal service key: %v", err)
		return
	}

	if !sb.equal(request.ServiceKey, serviceKeyBytes) {
		if len(serviceKeyBytes) >= 8 && len(request.ServiceKey) >= 8 {
			sb.serviceManager.eventSender(EventInfo, map[string]interface{}{
				"message":  "Received lookup request for different service key (ignoring)",
				"from":     from.String(),
				"expected": fmt.Sprintf("%x", serviceKeyBytes[:8]),
				"received": fmt.Sprintf("%x", request.ServiceKey[:8]),
			})
		}
		return // Not for our service
	}

	// log.Printf("[BEACON] Received lookup request from %s for service key %x", from.String()[:8], serviceKeyBytes[:8])
	sb.serviceManager.eventSender(EventInfo, map[string]interface{}{
		"message":     "Received service lookup request for our service key",
		"from":        from.String(),
		"service_key": fmt.Sprintf("%x", serviceKeyBytes[:8]),
	})

	// Verify locator request signature if provided and crypto enabled
	if !sb.serviceManager.noCrypto {
		if len(request.Signature) == 0 || len(request.PublicKey) == 0 {
			sb.serviceManager.eventSender(EventInfo, map[string]interface{}{
				"message": "Skipping unsigned/anonymous service lookup request",
				"from":    from.String(),
			})
			return
		}
		canonical := fmt.Sprintf("%s|%x|%d", request.Type, sha256.Sum256(request.ServiceKey), request.Timestamp.Unix())
		if pub, err := crypto.UnmarshalPublicKey(request.PublicKey); err == nil {
			ok, verr := pub.Verify([]byte(canonical), request.Signature)
			if verr != nil || !ok {
				sb.serviceManager.eventSender(EventInfo, map[string]interface{}{
					"message": "Invalid signature on service lookup request",
					"from":    from.String(),
				})
				return
			}
		} else {
			return
		}
	}

	// Enforce crypto policy
	if sb.serviceManager.noCrypto && request.FromEncrypted {
		sb.serviceManager.eventSender(EventInfo, map[string]interface{}{
			"message": "Skipping encrypted service lookup request - crypto disabled",
			"from":    from.String(),
		})
		return
	}
	if !sb.serviceManager.noCrypto && !request.FromEncrypted {
		sb.serviceManager.eventSender(EventInfo, map[string]interface{}{
			"message": "Skipping unencrypted service lookup request - crypto required",
			"from":    from.String(),
		})
		return
	}

	// Extract peer ID using our specific service key for decryption
	peerID := sb.serviceManager.ExtractPeerIDFromServiceRequestWithKey(request, sb.ServicePrivKey)

	sb.serviceManager.eventSender(EventInfo, map[string]interface{}{
		"message":           "Received service lookup request for beacon",
		"from":              from.String(),
		"decrypted_peer_id": peerID,
	})

	// Build minimal response: only peer IDs; optionally encrypt our PeerID for requester
	responderPeerID := sb.serviceManager.host.ID().String()
	makeResponse := func() *types.ServiceResponse {
		resp := &types.ServiceResponse{
			Type:      "service-response",
			Timestamp: time.Now(),
		}
		// Encrypt our PeerID into EncryptedData when crypto enabled and requester provided pubkey
		if len(request.PublicKey) > 0 && !sb.serviceManager.noCrypto {
			// Use the service private key for encryption, not the peer identity key
			if enc, nonce, err := sb.serviceManager.cryptoManager.EncryptWithServiceKey([]byte(responderPeerID), request.PublicKey, sb.ServicePrivKey); err == nil {
				resp.Encrypted = true
				resp.EncryptedData = enc
				resp.Nonce = nonce
			}
		} else if sb.serviceManager.noCrypto {
			// Plaintext fallback only when crypto disabled
			resp.From = responderPeerID
		}
		// Include service public key for decryption on the locator side
		servicePub := sb.ServicePrivKey.GetPublic()
		if b, err := crypto.MarshalPublicKey(servicePub); err == nil {
			resp.PublicKey = b
		}

		// Filter and include addresses based on transport restrictions
		addresses := sb.getFilteredAddresses(request.RequestPath)
		if len(addresses) > 0 {
			resp.Addresses = addresses
		}

		// Sign the response with the service private key over canonical fields: Type|sha256(PublicKey)|Timestamp
		canonical := fmt.Sprintf("%s|%x|%d", resp.Type, sha256.Sum256(resp.PublicKey), resp.Timestamp.Unix())
		if sig, err := sb.ServicePrivKey.Sign([]byte(canonical)); err == nil {
			resp.Signature = sig
		}
		return resp
	}

	// If requester PeerID provided (decrypted), send direct; else publish on service topic
	if peerID != "" {
		resp := makeResponse()
		if p, err := peer.Decode(peerID); err == nil {
			sb.serviceManager.SendDirectServiceResponse(p, resp)
		}
	} else {
		// Publish to service topic
		resp := makeResponse()
		data, err := json.Marshal(resp)
		if err == nil {
			_ = sb.gossipTopic.Publish(sb.serviceManager.ctx, data)
		}
	}
}

// startPeriodicAnnouncements starts periodic service announcements
func (sb *ServiceBeacon) startPeriodicAnnouncements() {
	for range sb.ticker.C {
		if sb.mode == types.ServiceBeaconModeAnnounce {
			sb.sendServiceAnnouncement()
		}
	}
}

// sendServiceAnnouncement sends a service announcement
func (sb *ServiceBeacon) sendServiceAnnouncement() {
	serviceKeyBytes, _ := crypto.MarshalPublicKey(sb.ServicePrivKey.GetPublic())

	// Determine if we include peerID based on flags
	peerIDStr := ""
	if sb.includePeerIDInAnnouncements && !sb.onlyIncludePeerIDInDirectReply {
		peerIDStr = sb.serviceManager.host.ID().String()
	}

	announcement := &types.ServiceAnnouncement{
		Type:       "service-announcement",
		ServiceKey: serviceKeyBytes,
		PeerID:     peerIDStr,
		Timestamp:  time.Now(),
		Data:       map[string]interface{}{},
	}

	// Sign announcement
	if sig, err := sb.serviceManager.signServiceAnnouncement(announcement, sb.ServicePrivKey); err == nil {
		announcement.Signature = sig
	}

	data, err := json.Marshal(announcement)
	if err != nil {
		sb.serviceManager.eventSender(EventError, map[string]interface{}{
			"message": "Failed to marshal service announcement",
			"error":   err.Error(),
		})
		return
	}

	if err := sb.gossipTopic.Publish(sb.serviceManager.ctx, data); err != nil {
		sb.serviceManager.eventSender(EventError, map[string]interface{}{
			"message": "Failed to publish service announcement",
			"error":   err.Error(),
		})
		return
	}

	// log.Printf("[BEACON] Published announcement for service key %x", serviceKeyBytes[:8])
	sb.serviceManager.eventSender(EventInfo, map[string]interface{}{
		"message":     "Sent service announcement",
		"service_key": fmt.Sprintf("%x", serviceKeyBytes[:8]),
	})
}

// getNodeAddresses returns the node's addresses as strings
func (sb *ServiceBeacon) getNodeAddresses() []string {
	addresses := make([]string, 0, len(sb.serviceManager.host.Addrs()))
	for _, addr := range sb.serviceManager.host.Addrs() {
		addresses = append(addresses, fmt.Sprintf("%s/p2p/%s", addr, sb.serviceManager.host.ID()))
	}
	return addresses
}

// getFilteredAddresses returns the node's addresses filtered by transport restrictions
// from the fig file for the given request path. If no fig file is set or no restrictions
// apply, returns all addresses.
func (sb *ServiceBeacon) getFilteredAddresses(requestPath string) []string {
	// Get all node addresses
	allAddresses := sb.getNodeAddresses()

	// If no fig file is set, return all addresses
	if sb.FigFile == nil {
		return allAddresses
	}

	// Determine which path to use (default to root if not specified)
	pathToCheck := requestPath
	if pathToCheck == "" {
		pathToCheck = "/"
	}

	// Find the matching node in the fig tree
	node, found := sb.FigFile.FindNodeForPath(pathToCheck)
	if !found || node == nil {
		// No matching node found, return all addresses
		return allAddresses
	}

	// If no transport restrictions on this node, return all addresses
	if len(node.AllowedTransports) == 0 {
		return allAddresses
	}

	// Filter addresses by allowed transports using the transport package
	filtered := transport.FilterMultiaddrStringsByTransport(allAddresses, node.AllowedTransports)

	sb.serviceManager.eventSender(EventInfo, map[string]interface{}{
		"message":            "Filtered addresses by transport restrictions",
		"request_path":       requestPath,
		"allowed_transports": node.AllowedTransports,
		"total_addresses":    len(allAddresses),
		"filtered_addresses": len(filtered),
	})

	return filtered
}

// equal compares two byte slices for equality
func (sb *ServiceBeacon) equal(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Manager coordinates service beacon and locator lifecycle, peer connections,
// and cryptographic operations. It supports multiple concurrent beacons and locators,
// allowing a single node to provide and consume multiple services simultaneously.
type Manager struct {
	host              host.Host
	ctx               context.Context
	pubsub            *pubsub.PubSub
	eventSender       func(string, interface{})
	cryptoManager     interfaces.CryptoManager
	connectionManager interfaces.PeerManager
	// Back-compat single-beacon reference (first beacon if any)
	serviceBeacon *ServiceBeacon
	// Multi-beacon support
	serviceBeacons  []*ServiceBeacon
	serviceLocators map[string]*ServiceLocator
	httpTransport   *http.Transport
	noCrypto        bool
	// Beacon behavior flags (applied to new beacons)
	beaconIncludePeerIDAnnouncements bool
	beaconPeerIDDirectOnly           bool
	// Transport restriction flags
	overrideTransportRestrictions bool
}

// signServiceAnnouncement signs the announcement with the service private key
func (m *Manager) signServiceAnnouncement(a *types.ServiceAnnouncement, servicePrivKey crypto.PrivKey) ([]byte, error) {
	// Build canonical message: type|serviceKeyHex|peerID|timestamp|addressesJSON
	// Serialize Data if map, else empty
	var addrJSON string
	if a.Data != nil {
		if mapp, ok := a.Data.(map[string]interface{}); ok {
			if raw, ok2 := mapp["addresses"]; ok2 {
				if b, err := json.Marshal(raw); err == nil {
					addrJSON = string(b)
				}
			}
		}
	}
	msg := fmt.Sprintf("%s|%x|%s|%d|%s", a.Type, sha256.Sum256(a.ServiceKey), a.PeerID, a.Timestamp.Unix(), addrJSON)
	return servicePrivKey.Sign([]byte(msg))
}

// verifyServiceAnnouncement verifies the announcement signature using the embedded service public key
func (m *Manager) verifyServiceAnnouncement(a *types.ServiceAnnouncement) bool {
	if len(a.Signature) == 0 {
		return false
	}
	// Rebuild the signed message with same canonicalization
	var addrJSON string
	if a.Data != nil {
		if mapp, ok := a.Data.(map[string]interface{}); ok {
			if raw, ok2 := mapp["addresses"]; ok2 {
				if b, err := json.Marshal(raw); err == nil {
					addrJSON = string(b)
				}
			}
		}
	}
	msg := fmt.Sprintf("%s|%x|%s|%d|%s", a.Type, sha256.Sum256(a.ServiceKey), a.PeerID, a.Timestamp.Unix(), addrJSON)
	// Unmarshal service public key
	pubKey, err := crypto.UnmarshalPublicKey(a.ServiceKey)
	if err != nil {
		return false
	}
	ok, err := pubKey.Verify([]byte(msg), a.Signature)
	return err == nil && ok
}

// NewManager creates a new service manager with the given configuration.
// Parameters:
//   - host: libp2p host for peer communication
//   - ctx: context for lifecycle management
//   - pubsub: gossipsub instance for service announcements
//   - cryptoManager: handles encryption and decryption operations
//   - connectionManager: manages peer connections
//   - httpTransport: HTTP transport for direct peer responses
//   - eventSender: callback for emitting events
//   - noCrypto: disables cryptographic verification when true
//   - beaconIncludePeerIDAnnouncements: includes peer ID in broadcast announcements
//   - beaconPeerIDDirectOnly: only includes peer ID in direct responses
//   - overrideTransportRestrictions: allows fallback to peer discovery on connection failure
func NewManager(host host.Host, ctx context.Context, pubsub *pubsub.PubSub,
	cryptoManager interfaces.CryptoManager, connectionManager interfaces.PeerManager, httpTransport *http.Transport, eventSender func(string, interface{}), noCrypto bool, beaconIncludePeerIDAnnouncements bool, beaconPeerIDDirectOnly bool, overrideTransportRestrictions bool) interfaces.ServiceManager {
	return &Manager{
		host:                             host,
		ctx:                              ctx,
		pubsub:                           pubsub,
		eventSender:                      eventSender,
		cryptoManager:                    cryptoManager,
		connectionManager:                connectionManager,
		serviceBeacons:                   make([]*ServiceBeacon, 0),
		serviceLocators:                  make(map[string]*ServiceLocator),
		httpTransport:                    httpTransport,
		noCrypto:                         noCrypto,
		beaconIncludePeerIDAnnouncements: beaconIncludePeerIDAnnouncements,
		beaconPeerIDDirectOnly:           beaconPeerIDDirectOnly,
		overrideTransportRestrictions:    overrideTransportRestrictions,
	}
}

// CreateServiceBeacon creates a new service beacon for announcing service availability.
// The beacon uses the provided private key to sign announcements and creates a unique
// gossipsub topic based on the service public key hash.
func (m *Manager) CreateServiceBeacon(servicePrivKey crypto.PrivKey) (interfaces.ServiceBeacon, error) {
	// Create a topic name based on the hash of the service public key
	servicePubKey := servicePrivKey.GetPublic()
	servicePubKeyBytes, err := crypto.MarshalPublicKey(servicePubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal service public key: %w", err)
	}

	// Create topic name from hash of service public key
	hash := sha256.Sum256(servicePubKeyBytes)
	topicName := fmt.Sprintf("service-%x", hash[:8]) // Use first 8 bytes of hash

	// log.Printf("[BEACON] Creating beacon for service key %x on topic %s", servicePubKeyBytes[:8], topicName)
	m.eventSender(EventInfo, map[string]interface{}{
		"message":     "Creating beacon for service key",
		"topic":       topicName,
		"service_key": fmt.Sprintf("%x", servicePubKeyBytes[:8]),
	})

	// Join the service topic
	serviceTopic, err := m.pubsub.Join(topicName)
	if err != nil {
		return nil, fmt.Errorf("failed to join service topic: %w", err)
	}

	// Subscribe to the service topic
	serviceSub, err := serviceTopic.Subscribe()
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to service topic: %w", err)
	}

	// log.Printf("[BEACON] Subscribed to topic %s", topicName)
	m.eventSender(EventInfo, map[string]interface{}{
		"message": "Beacon subscribed to service topic",
		"topic":   topicName,
	})

	beacon := &ServiceBeacon{
		serviceManager:                 m,
		ServicePrivKey:                 servicePrivKey,
		gossipTopic:                    serviceTopic,
		gossipSub:                      serviceSub,
		mode:                           types.ServiceBeaconModeAnnounce,
		peers:                          make([]peer.ID, 0),
		includePeerIDInAnnouncements:   m.beaconIncludePeerIDAnnouncements,
		onlyIncludePeerIDInDirectReply: m.beaconPeerIDDirectOnly,
	}

	m.serviceBeacons = append(m.serviceBeacons, beacon)
	if m.serviceBeacon == nil {
		m.serviceBeacon = beacon
	}
	return beacon, nil
}

// CreateServiceBeaconWithMeta creates a service beacon with additional metadata.
// The alias provides a human-readable identifier, and fig templates define
// transport restrictions and routing rules for the service.
func (m *Manager) CreateServiceBeaconWithMeta(servicePrivKey crypto.PrivKey, alias string, figTemplates [][]byte) (interfaces.ServiceBeacon, error) {
	b, err := m.CreateServiceBeacon(servicePrivKey)
	if err != nil {
		return nil, err
	}
	sb := b.(*ServiceBeacon)
	sb.Alias = alias
	sb.FigTemplates = figTemplates
	return sb, nil
}

// StartServiceLocator creates and starts a locator for discovering service providers.
// The locator joins the service's gossipsub topic and begins processing messages
// according to the specified mode (announce or lookup).
func (m *Manager) StartServiceLocator(servicePubKey crypto.PubKey, mode types.ServiceBeaconMode) error {
	servicePubKeyBytes, err := crypto.MarshalPublicKey(servicePubKey)
	if err != nil {
		return fmt.Errorf("failed to marshal service public key: %w", err)
	}

	// Create topic name from hash of service public key
	hash := sha256.Sum256(servicePubKeyBytes)
	topicName := fmt.Sprintf("service-%x", hash[:8])

	// log.Printf("[LOCATOR] Creating locator for service key %x on topic %s (mode: %s)", servicePubKeyBytes[:8], topicName, mode)
	m.eventSender(EventInfo, map[string]interface{}{
		"message":     "Creating locator for service key",
		"topic":       topicName,
		"service_key": fmt.Sprintf("%x", servicePubKeyBytes[:8]),
		"mode":        mode,
	})

	// Check if we already have a locator for this service
	if _, exists := m.serviceLocators[topicName]; exists {
		m.eventSender(EventInfo, map[string]interface{}{
			"message":     "[LOCATOR] locator already exists, skipping",
			"topic":       topicName,
			"service_key": fmt.Sprintf("%x", servicePubKeyBytes[:8]),
		})
		return fmt.Errorf("service locator already exists for this service")
	}

	// Check if a beacon already owns this topic's subscription. GossipSub
	// only allows one subscription per topic, so the locator piggybacks on
	// the beacon's subscription and receives messages forwarded by it.
	var ownerBeacon *ServiceBeacon
	for _, b := range m.serviceBeacons {
		bPubBytes, _ := crypto.MarshalPublicKey(b.ServicePrivKey.GetPublic())
		bHash := sha256.Sum256(bPubBytes)
		if fmt.Sprintf("service-%x", bHash[:8]) == topicName {
			ownerBeacon = b
			break
		}
	}

	if ownerBeacon != nil {
		m.eventSender(EventInfo, map[string]interface{}{
			"message":     "Locator piggybacking on existing beacon subscription",
			"topic":       topicName,
			"service_key": fmt.Sprintf("%x", servicePubKeyBytes[:8]),
		})

		locator := &ServiceLocator{
			serviceManager: m,
			ServiceKey:     servicePubKey,
			gossipTopic:    ownerBeacon.gossipTopic, // reuse beacon's topic for publishing
			gossipSub:      nil,                     // no own subscription; beacon forwards messages
			mode:           mode,
			limit:          10,
		}
		m.serviceLocators[topicName] = locator
		ownerBeacon.piggybackLocator = locator
		return locator.Start()
	}

	// Normal path: no beacon owns this topic, create own subscription
	serviceTopic, err := m.pubsub.Join(topicName)
	if err != nil {
		return fmt.Errorf("failed to join service topic: %w", err)
	}

	serviceSub, err := serviceTopic.Subscribe()
	if err != nil {
		return fmt.Errorf("failed to subscribe to service topic: %w", err)
	}

	m.eventSender(EventInfo, map[string]interface{}{
		"message": fmt.Sprintf("[LOCATOR] Subscribed to topic %s", topicName),
	})
	m.eventSender(EventInfo, map[string]interface{}{
		"message": "Locator subscribed to service topic",
		"topic":   topicName,
	})

	locator := &ServiceLocator{
		serviceManager: m,
		ServiceKey:     servicePubKey,
		gossipTopic:    serviceTopic,
		gossipSub:      serviceSub,
		mode:           mode,
		limit:          10,
	}

	m.serviceLocators[topicName] = locator

	// Start the locator
	if err := locator.Start(); err != nil {
		return fmt.Errorf("failed to start service locator: %w", err)
	}

	m.eventSender(EventInfo, map[string]interface{}{
		"message":     "Started service locator",
		"topic":       topicName,
		"mode":        mode,
		"service_key": fmt.Sprintf("%x", servicePubKeyBytes[:8]),
	})

	return nil
}

// ExtractPeerIDFromServiceRequest extracts the peer ID from a service lookup request
// using the legacy single service key. Returns an empty string if extraction fails
// or crypto is disabled for encrypted requests.
func (m *Manager) ExtractPeerIDFromServiceRequest(req *types.ServiceLookupRequest) string {
	if req.FromEncrypted && len(req.EncryptedFrom) > 0 && len(req.EncryptedNonce) > 0 {
		// Skip encrypted data if crypto is disabled
		if m.noCrypto {
			return ""
		}
		// Decrypt the peer ID using legacy service key
		if m.cryptoManager.HasServiceKey() {
			peerID, err := m.cryptoManager.DecryptPeerIDWithServiceKey(req.EncryptedFrom, req.EncryptedNonce, req.PublicKey)
			if err != nil {
				m.eventSender(EventError, map[string]interface{}{
					"message": "Failed to decrypt peer ID from service request",
					"error":   err.Error(),
				})
				return ""
			}
			return peerID
		}
		return ""
	}
	return req.From
}

// ExtractPeerIDFromServiceRequestWithKey extracts the peer ID from a service lookup request
// using a specific service private key for decryption. This allows multiple beacons
// to decrypt requests targeted at their specific service.
func (m *Manager) ExtractPeerIDFromServiceRequestWithKey(req *types.ServiceLookupRequest, servicePrivKey crypto.PrivKey) string {
	if req.FromEncrypted && len(req.EncryptedFrom) > 0 && len(req.EncryptedNonce) > 0 {
		// Skip encrypted data if crypto is disabled
		if m.noCrypto {
			return ""
		}
		// Decrypt the peer ID using the specific service key
		if servicePrivKey != nil {
			peerID, err := m.cryptoManager.DecryptPeerIDWithSpecificServiceKey(req.EncryptedFrom, req.EncryptedNonce, req.PublicKey, servicePrivKey)
			if err != nil {
				m.eventSender(EventError, map[string]interface{}{
					"message": "Failed to decrypt peer ID from service request with specific key",
					"error":   err.Error(),
				})
				return ""
			}
			return peerID
		}
		return ""
	}
	return req.From
}

// ConnectToServiceProvider establishes a connection to a service provider peer
// using peer discovery. If already connected, it verifies and tracks the connection.
func (m *Manager) ConnectToServiceProvider(peerID peer.ID) {
	// Check if already connected
	if m.host.Network().Connectedness(peerID) == network.Connected {
		m.eventSender(EventInfo, map[string]interface{}{
			"message": "Already connected to service provider",
			"peer_id": peerID.String(),
		})
		return
	}

	// Try to connect
	err := m.host.Connect(m.ctx, peer.AddrInfo{ID: peerID})
	if err != nil {
		m.eventSender(EventError, map[string]interface{}{
			"message": "Failed to connect to service provider",
			"peer_id": peerID.String(),
			"error":   err.Error(),
		})
		return
	}

	m.eventSender(EventConnection, map[string]interface{}{
		"message": "Connected to service provider",
		"peer_id": peerID.String(),
	})

	// Add to tracking as HTTP verified peer
	// For service connections, we should include the service key if available from the locator context
	// However, at this point we don't have the specific service key context, so use the regular method
	m.connectionManager.AddTrackedPeer(peerID, types.PeerOptions{
		ConnectionType: ConnTypeHTTPVerified,
		HTTPCapable:    true,
	})

	// Test bidirectional connection
	go m.connectionManager.CreateBidirectionalConnection(peerID)
}

// SendServiceLookupResponse sends a response to a service lookup request.
// This method is intended for custom service response handling.
func (m *Manager) SendServiceLookupResponse(req *types.ServiceLookupRequest) {
	// This method would handle responding to service lookup requests
	// Implementation would depend on the specific service logic
	m.eventSender(EventInfo, map[string]interface{}{
		"message": "Sending service lookup response",
		"to":      req.From,
	})
}

// GetServiceBeacon returns the first service beacon.
//
// Deprecated: This method only returns the first beacon and does not support
// multi-beacon scenarios. Use ListServiceBeacons() for new code.
func (m *Manager) GetServiceBeacon() interfaces.ServiceBeacon {
	return m.serviceBeacon
}

// ListServiceBeacons returns all active service beacons managed by this manager.
func (m *Manager) ListServiceBeacons() []interfaces.ServiceBeacon {
	res := make([]interfaces.ServiceBeacon, 0, len(m.serviceBeacons))
	for _, b := range m.serviceBeacons {
		res = append(res, b)
	}
	return res
}

// SendConnectToPeerResponse sends a discovery response to a peer requesting connection.
// The response includes the local node's addresses and public key.
func (m *Manager) SendConnectToPeerResponse(peerIDStr string) {
	// Parse peer ID
	peerID, err := peer.Decode(peerIDStr)
	if err != nil {
		m.eventSender(EventError, map[string]interface{}{
			"message": "Invalid peer ID in connect-to-peer request",
			"peer_id": peerIDStr,
			"error":   err.Error(),
		})
		return
	}

	// Create response with our addresses
	response := &types.LookupResponse{
		Type:      "response",
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

	// Include our addresses
	addresses := make([]string, 0, len(m.host.Addrs()))
	for _, addr := range m.host.Addrs() {
		addresses = append(addresses, fmt.Sprintf("%s/p2p/%s", addr, m.host.ID()))
	}
	response.Addresses = addresses

	// Sign the response
	responseData := fmt.Sprintf("%s:%s:%s", response.Type, response.From, response.Target)
	if signature, err := m.host.Peerstore().PrivKey(m.host.ID()).Sign([]byte(responseData)); err == nil {
		response.Signature = signature
	}

	// Send discovery response (LookupResponse) to the peer
	m.SendDirectDiscoveryResponse(peerID, response)
}

// SendDirectServiceResponse sends a service response directly to a specific peer
// using libp2p HTTP transport. The response is marshaled to JSON and posted
// to the peer's service response endpoint.
func (m *Manager) SendDirectServiceResponse(peerID peer.ID, response *types.ServiceResponse) {
	responseData, err := json.Marshal(response)
	if err != nil {
		m.eventSender(EventError, map[string]interface{}{
			"message": "Failed to marshal service response",
			"error":   err.Error(),
		})
		return
	}

	m.eventSender(EventInfo, map[string]interface{}{
		"message": "Sending direct service response",
		"to":      peerID.String(),
		"size":    len(responseData),
	})
	// Send via libp2p HTTP to the peer's protocol server
	client := &http.Client{Transport: m.httpTransport}
	url := fmt.Sprintf("libp2p://%s/services/response", peerID)
	req, err := http.NewRequestWithContext(m.ctx, "POST", url, bytes.NewReader(responseData))
	if err != nil {
		m.eventSender(EventError, map[string]interface{}{
			"message": "Failed to create direct service response request",
			"error":   err.Error(),
			"to":      peerID.String(),
		})
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		m.eventSender(EventError, map[string]interface{}{
			"message": "Failed to send direct service response",
			"error":   err.Error(),
			"to":      peerID.String(),
		})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		m.eventSender(EventError, map[string]interface{}{
			"message": "Direct service response returned non-2xx",
			"status":  resp.StatusCode,
			"to":      peerID.String(),
		})
		return
	}
}

// SendDirectDiscoveryResponse sends a discovery response directly to a specific peer
// using libp2p HTTP transport.
func (m *Manager) SendDirectDiscoveryResponse(peerID peer.ID, response *types.LookupResponse) {
	responseData, err := json.Marshal(response)
	if err != nil {
		m.eventSender(EventError, map[string]interface{}{
			"message": "Failed to marshal discovery response",
			"error":   err.Error(),
		})
		return
	}

	m.eventSender(EventInfo, map[string]interface{}{
		"message": "Sending direct discovery response",
		"to":      peerID.String(),
		"size":    len(responseData),
	})
	// Send via libp2p HTTP to the peer's protocol server
	client := &http.Client{Transport: m.httpTransport}
	url := fmt.Sprintf("libp2p://%s/discovery/response", peerID)
	req, err := http.NewRequestWithContext(m.ctx, "POST", url, bytes.NewReader(responseData))
	if err != nil {
		m.eventSender(EventError, map[string]interface{}{
			"message": "Failed to create direct discovery response request",
			"error":   err.Error(),
			"to":      peerID.String(),
		})
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		m.eventSender(EventError, map[string]interface{}{
			"message": "Failed to send direct discovery response",
			"error":   err.Error(),
			"to":      peerID.String(),
		})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		m.eventSender(EventError, map[string]interface{}{
			"message": "Direct discovery response returned non-2xx",
			"status":  resp.StatusCode,
			"to":      peerID.String(),
		})
		return
	}
}

// ProcessServiceResponse handles an incoming service response by verifying the
// signature, decrypting the responder's peer ID, and establishing a connection.
// It attempts decryption using all active locators' ephemeral keys.
func (m *Manager) ProcessServiceResponse(resp *types.ServiceResponse) {
	// Determine responder peer id
	peerIDStr := strings.TrimSpace(resp.From)
	if m.noCrypto {
		if peerIDStr == "" {
			m.eventSender(EventError, map[string]interface{}{"message": "missing responder peer id in plaintext response"})
			return
		}
	} else {
		// Verify signature with provided service public key
		if len(resp.PublicKey) == 0 || len(resp.Signature) == 0 {
			m.eventSender(EventError, map[string]interface{}{"message": "missing signature or public key in service response"})
			return
		}
		if pub, err := crypto.UnmarshalPublicKey(resp.PublicKey); err == nil {
			canonical := fmt.Sprintf("%s|%x|%d", resp.Type, sha256.Sum256(resp.PublicKey), resp.Timestamp.Unix())
			ok, verr := pub.Verify([]byte(canonical), resp.Signature)
			if verr != nil || !ok {
				m.eventSender(EventError, map[string]interface{}{"message": "invalid signature on service response"})
				return
			}
		} else {
			m.eventSender(EventError, map[string]interface{}{"message": "invalid service public key in response"})
			return
		}
		// Encrypted case: try all active locators to decrypt responder peer id
		if len(resp.EncryptedData) == 0 || len(resp.Nonce) == 0 || len(resp.PublicKey) == 0 {
			m.eventSender(EventError, map[string]interface{}{"message": "missing encryption fields in service response"})
			return
		}
		// Iterate locators
		for _, loc := range m.serviceLocators {
			if loc == nil || loc.ephemeralPriv == nil {
				continue
			}
			if plaintext, err := m.cryptoManager.DecryptWithLocatorKey(resp.EncryptedData, resp.Nonce, resp.PublicKey, loc.ephemeralPriv); err == nil {
				peerIDStr = string(plaintext)
				break
			}
		}
		if peerIDStr == "" {
			m.eventSender(EventError, map[string]interface{}{"message": "failed to decrypt responder peer id from service response"})
			return
		}
	}
	// Connect using peer discovery or provided addresses
	if pid, err := peer.Decode(peerIDStr); err == nil {
		if len(resp.Addresses) > 0 {
			m.ConnectToServiceProviderDirectly(pid, resp.Addresses)
		} else {
			m.ConnectToServiceProvider(pid)
		}

		// Associate the service key from the response with this connection so that
		// alias resolve (which filters by ServiceKeys) can find this peer immediately
		// instead of waiting for a subsequent beacon announcement.
		if len(resp.PublicKey) > 0 {
			m.connectionManager.AddTrackedPeer(pid, types.NewServicePeerOptions(resp.PublicKey, true))
		}
	} else {
		m.eventSender(EventError, map[string]interface{}{"message": "invalid responder peer id", "peer_id": peerIDStr})
	}
}

// TestServicePeerBidirectionality verifies bidirectional HTTP connectivity with a
// service peer by making a ping request via libp2p HTTP transport.
func (m *Manager) TestServicePeerBidirectionality(peerID peer.ID, serviceKey []byte) {
	m.eventSender(EventConnection, map[string]interface{}{
		"message":     "Testing bidirectional connection with service peer",
		"peer_id":     peerID.String(),
		"service_key": fmt.Sprintf("%x", serviceKey[:8]),
	})

	// Test HTTP capability by making a request
	client := &http.Client{Transport: m.httpTransport}
	url := fmt.Sprintf("libp2p://%s/ping", peerID)

	ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		m.eventSender(EventError, map[string]interface{}{
			"message": "Failed to create test request for service peer",
			"error":   err.Error(),
			"peer_id": peerID.String(),
		})
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		m.eventSender(EventError, map[string]interface{}{
			"message": "Failed to test service peer bidirectionality",
			"error":   err.Error(),
			"peer_id": peerID.String(),
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		m.eventSender(EventConnection, map[string]interface{}{
			"message": "Successfully established bidirectional connection with service peer",
			"peer_id": peerID.String(),
		})
	} else {
		m.eventSender(EventError, map[string]interface{}{
			"message": "Service peer test failed with status",
			"status":  resp.StatusCode,
			"peer_id": peerID.String(),
		})
	}
}

// ConnectToServiceProviderDirectly establishes a connection using explicitly provided
// multiaddresses rather than peer discovery. If all direct connection attempts fail
// and overrideTransportRestrictions is enabled, it falls back to peer discovery.
func (m *Manager) ConnectToServiceProviderDirectly(peerID peer.ID, addresses []string) {
	m.eventSender(EventConnection, map[string]interface{}{
		"message":   "Attempting direct connection to service provider",
		"peer_id":   peerID.String(),
		"addresses": addresses,
	})

	// Check if already connected
	if m.host.Network().Connectedness(peerID) == network.Connected {
		m.eventSender(EventInfo, map[string]interface{}{
			"message": "Already connected to service provider",
			"peer_id": peerID.String(),
		})
		// Add to tracking as HTTP verified peer
		m.connectionManager.AddTrackedPeer(peerID, types.PeerOptions{
			ConnectionType: ConnTypeHTTPVerified,
			HTTPCapable:    true,
		})
		// Test bidirectional connection
		go m.connectionManager.CreateBidirectionalConnection(peerID)
		return
	}

	// Log transport information for each address
	for _, addrStr := range addresses {
		if addr, err := multiaddr.NewMultiaddr(addrStr); err == nil {
			transportInfo := transport.GetTransportInfo(addr)
			m.eventSender(EventInfo, map[string]interface{}{
				"message":   "Address transport info",
				"address":   addrStr,
				"transport": transportInfo,
			})
		}
	}

	// Try connecting to each provided address
	connectionSuccess := false
	for _, addrStr := range addresses {
		addr, err := multiaddr.NewMultiaddr(addrStr)
		if err != nil {
			m.eventSender(EventError, map[string]interface{}{
				"message": "Invalid multiaddress from service provider",
				"error":   err.Error(),
				"address": addrStr,
				"peer_id": peerID.String(),
			})
			continue
		}

		peerInfo, err := peer.AddrInfoFromP2pAddr(addr)
		if err != nil {
			m.eventSender(EventError, map[string]interface{}{
				"message": "Failed to parse peer info from address",
				"error":   err.Error(),
				"address": addrStr,
				"peer_id": peerID.String(),
			})
			continue
		}

		// Verify the peer ID matches
		if peerInfo.ID != peerID {
			m.eventSender(EventError, map[string]interface{}{
				"message":  "Peer ID mismatch in provided address",
				"expected": peerID.String(),
				"actual":   peerInfo.ID.String(),
				"address":  addrStr,
			})
			continue
		}

		// Attempt connection
		ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
		err = m.host.Connect(ctx, *peerInfo)
		cancel()

		if err != nil {
			m.eventSender(EventError, map[string]interface{}{
				"message": "Failed to connect to service provider via provided address",
				"error":   err.Error(),
				"address": addrStr,
				"peer_id": peerID.String(),
			})
			continue
		}

		// Success!
		connectionSuccess = true
		m.eventSender(EventConnection, map[string]interface{}{
			"message": "Successfully connected to service provider via direct address",
			"peer_id": peerID.String(),
			"address": addrStr,
		})

		// Add to tracking as HTTP verified peer
		m.connectionManager.AddTrackedPeer(peerID, types.PeerOptions{
			ConnectionType: ConnTypeHTTPVerified,
			HTTPCapable:    true,
		})

		// Test bidirectional connection
		go m.connectionManager.CreateBidirectionalConnection(peerID)
		return
	}

	// If no direct connection succeeded and override flag is set, fall back to peer discovery
	if !connectionSuccess {
		if m.overrideTransportRestrictions {
			m.eventSender(EventInfo, map[string]interface{}{
				"message": "Direct connection failed, falling back to peer discovery (override enabled)",
				"peer_id": peerID.String(),
			})
			m.ConnectToServiceProvider(peerID)
		} else {
			m.eventSender(EventError, map[string]interface{}{
				"message": "Failed to connect to service provider via any provided address",
				"peer_id": peerID.String(),
			})
		}
	}
}
