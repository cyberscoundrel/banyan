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

// Types - would normally be in shared package
// Use types.ServiceLookupRequest instead of local definition

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

// Constants
const (
	ConnTypeHTTPVerified = "http-verified"
	EventConnection      = "connection"
	EventError           = "error"
	EventInfo            = "info"
)

// ServiceLocator handles service discovery for a specific service
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

// Start starts the service locator's operations
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
	// Start message processing
	go sl.processMessages()

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

// Stop stops the service locator
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

// GetMode returns the locator mode
func (sl *ServiceLocator) GetMode() types.ServiceBeaconMode {
	return sl.mode
}

// SetMode sets the locator mode
func (sl *ServiceLocator) SetMode(mode types.ServiceBeaconMode) {
	sl.mode = mode
}

// GetLimit returns the service limit
func (sl *ServiceLocator) GetLimit() int {
	return sl.limit
}

// SetLimit sets the service limit
func (sl *ServiceLocator) SetLimit(limit int) {
	sl.limit = limit
}

// IsRunning returns whether the locator is running
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

		// Process the message
		sl.handleMessage(msg)
	}
}

// handleMessage processes a single PubSub message
func (sl *ServiceLocator) handleMessage(msg *pubsub.Message) {
	// Try to parse as ServiceAnnouncement
	var announcement types.ServiceAnnouncement
	if err := json.Unmarshal(msg.Data, &announcement); err == nil {
		sl.handleServiceAnnouncement(&announcement, msg.ReceivedFrom)
		return
	}

	// Try to parse as ServiceLookupRequest
	var request types.ServiceLookupRequest
	if err := json.Unmarshal(msg.Data, &request); err == nil {
		sl.handleServiceLookupRequest(&request, msg.ReceivedFrom)
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

	peerIDShort := from.String()
	if len(peerIDShort) > 8 {
		peerIDShort = peerIDShort[:8]
	}
	// log.Printf("[LOCATOR] Received announcement from %s for service key %x (has_peer_id: %v)", peerIDShort, expectedKey[:8], announcement.PeerID != "")
	sl.serviceManager.eventSender(EventInfo, map[string]interface{}{
		"message":     "Received service announcement for our service key",
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
		// If we already have a connection item for this peer, append the service key; else add tracked peer
		if announcement.PeerID != "" {
			if peerID, err := peer.Decode(announcement.PeerID); err == nil {
				if conn, ok := sl.serviceManager.connectionManager.GetConnectionInfo(peerID); ok && conn != nil {
					// Append service key if not present
					sl.serviceManager.connectionManager.AddTrackedPeer(peerID, types.NewServicePeerOptions(expectedKey, conn.HTTPCapable))
				} else {
					// Track new and try to connect via discovery
					sl.serviceManager.connectionManager.AddTrackedPeer(peerID, types.NewServicePeerOptions(expectedKey, true))
					sl.serviceManager.ConnectToServiceProvider(peerID)
				}
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

// ServiceBeacon announces service availability
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
}

// Start starts the service beacon
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

// Stop stops the service beacon
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

// GetServiceKey returns the service private key
func (sb *ServiceBeacon) GetServiceKey() crypto.PrivKey {
	return sb.ServicePrivKey
}

// GetMode returns the beacon mode
func (sb *ServiceBeacon) GetMode() types.ServiceBeaconMode {
	return sb.mode
}

// SetMode sets the beacon mode
func (sb *ServiceBeacon) SetMode(mode types.ServiceBeaconMode) {
	sb.mode = mode
}

// GetPeers returns the list of connected peers
func (sb *ServiceBeacon) GetPeers() []peer.ID {
	return sb.peers
}

// GetAlias returns the configured alias for this beacon (may be empty)
func (sb *ServiceBeacon) GetAlias() string { return sb.Alias }

// GetFigTemplates returns raw fig template bytes associated to this beacon
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
	}
}

// handleMessage processes a single PubSub message
func (sb *ServiceBeacon) handleMessage(msg *pubsub.Message) {
	// Try to parse as ServiceLookupRequest
	var request types.ServiceLookupRequest
	if err := json.Unmarshal(msg.Data, &request); err == nil {
		sb.handleServiceLookupRequest(&request, msg.ReceivedFrom)
		return
	}

	// Unknown message type or not relevant to beacon
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

	peerIDShort := from.String()
	if len(peerIDShort) > 8 {
		peerIDShort = peerIDShort[:8]
	}
	// log.Printf("[BEACON] Received lookup request from %s for service key %x", peerIDShort, serviceKeyBytes[:8])
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

// Manager handles service beacon and locator functionality
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

// NewManager creates a new service manager
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

// CreateServiceBeacon creates a new service beacon for the given service private key
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

// CreateServiceBeaconWithMeta creates a service beacon and attaches alias and fig templates
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

// StartServiceLocator starts a service locator for the given service public key
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
		return fmt.Errorf("service locator already exists for this service")
	}

	// Join the service topic
	serviceTopic, err := m.pubsub.Join(topicName)
	if err != nil {
		return fmt.Errorf("failed to join service topic: %w", err)
	}

	// Subscribe to the service topic
	serviceSub, err := serviceTopic.Subscribe()
	if err != nil {
		return fmt.Errorf("failed to subscribe to service topic: %w", err)
	}

	// log.Printf("[LOCATOR] Subscribed to topic %s", topicName)
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
		limit:          10, // Default limit of 10 services
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

// ExtractPeerIDFromServiceRequest extracts the peer ID from a service request using legacy single service key
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

// ExtractPeerIDFromServiceRequestWithKey extracts the peer ID from a service request using a specific service key
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

// ConnectToServiceProvider connects to a service provider
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

// SendServiceLookupResponse sends a response to a service lookup request
func (m *Manager) SendServiceLookupResponse(req *types.ServiceLookupRequest) {
	// This method would handle responding to service lookup requests
	// Implementation would depend on the specific service logic
	m.eventSender(EventInfo, map[string]interface{}{
		"message": "Sending service lookup response",
		"to":      req.From,
	})
}

// GetServiceBeacon returns the current service beacon
// DEPRECATED: This only returns the first beacon. Use ListServiceBeacons() for multi-beacon support.
// This method is kept for backward compatibility but should not be used in new code.
func (m *Manager) GetServiceBeacon() interfaces.ServiceBeacon {
	return m.serviceBeacon
}

// ListServiceBeacons returns all active service beacons
func (m *Manager) ListServiceBeacons() []interfaces.ServiceBeacon {
	res := make([]interfaces.ServiceBeacon, 0, len(m.serviceBeacons))
	for _, b := range m.serviceBeacons {
		res = append(res, b)
	}
	return res
}

// SendConnectToPeerResponse sends a response to connect to peer request
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

// SendDirectServiceResponse sends a response directly to a peer
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

// SendDirectDiscoveryResponse sends a discovery (LookupResponse) directly to a peer
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

// ProcessServiceResponse processes a direct service response, decrypts peer ID, and connects
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
		// If response includes addresses, use direct connection
		if len(resp.Addresses) > 0 {
			m.ConnectToServiceProviderDirectly(pid, resp.Addresses)
		} else {
			// Fall back to peer discovery
			m.ConnectToServiceProvider(pid)
		}
	} else {
		m.eventSender(EventError, map[string]interface{}{"message": "invalid responder peer id", "peer_id": peerIDStr})
	}
}

// TestServicePeerBidirectionality tests bidirectional connection with a service peer
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

// ConnectToServiceProviderDirectly attempts to connect directly to a service provider using provided addresses
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
