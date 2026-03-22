package mocks

import (
	"sync"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"

	"banyan/types"
)

type MockDiscoveryManager struct {
	mu                 sync.RWMutex
	dhtEnabled         bool
	discoveryMethods   []string
	started            bool
	lookupHandler      func(*types.LookupRequest, peer.ID)
	responseHandler    func(*types.LookupResponse)
	gossipHandler      func(*pubsub.Message)
	sentResponses      []*types.LookupResponse
	verifiedSignatures map[string]bool
}

func NewMockDiscoveryManager() *MockDiscoveryManager {
	return &MockDiscoveryManager{
		discoveryMethods:   []string{"mdns"},
		verifiedSignatures: make(map[string]bool),
		sentResponses:      make([]*types.LookupResponse, 0),
	}
}

func (m *MockDiscoveryManager) StartPeerDiscovery(connectionManager interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = true
}

func (m *MockDiscoveryManager) StartDHTDiscovery() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dhtEnabled = true
	if !contains(m.discoveryMethods, "dht") {
		m.discoveryMethods = append(m.discoveryMethods, "dht")
	}
}

func (m *MockDiscoveryManager) StartMDNSDiscovery(connectionManager interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !contains(m.discoveryMethods, "mdns") {
		m.discoveryMethods = append(m.discoveryMethods, "mdns")
	}
}

func (m *MockDiscoveryManager) IsDHTEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.dhtEnabled
}

func (m *MockDiscoveryManager) GetDiscoveryMethods() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]string{}, m.discoveryMethods...)
}

func (m *MockDiscoveryManager) HandleGossipMessages(cryptoManager interface{}, connectionManager interface{}) {
}

func (m *MockDiscoveryManager) ProcessGossipMessage(msg *pubsub.Message, cryptoManager interface{}, connectionManager interface{}) {
	m.mu.RLock()
	handler := m.gossipHandler
	m.mu.RUnlock()

	if handler != nil {
		handler(msg)
	}
}

func (m *MockDiscoveryManager) HandleLookupRequest(req *types.LookupRequest, from peer.ID, cryptoManager interface{}, connectionManager interface{}) {
	m.mu.RLock()
	handler := m.lookupHandler
	m.mu.RUnlock()

	if handler != nil {
		handler(req, from)
	}
}

func (m *MockDiscoveryManager) SendDirectResponse(peerID peer.ID, response *types.LookupResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sentResponses = append(m.sentResponses, response)
}

func (m *MockDiscoveryManager) ProcessLookupResponse(resp *types.LookupResponse, connectionManager interface{}) {
	m.mu.RLock()
	handler := m.responseHandler
	m.mu.RUnlock()

	if handler != nil {
		handler(resp)
	}
}

func (m *MockDiscoveryManager) VerifyResponseSignature(resp *types.LookupResponse) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(resp.Signature) == 0 {
		return false
	}

	if verified, ok := m.verifiedSignatures[string(resp.Signature)]; ok {
		return verified
	}

	return true
}

func (m *MockDiscoveryManager) SetLookupHandler(fn func(*types.LookupRequest, peer.ID)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lookupHandler = fn
}

func (m *MockDiscoveryManager) SetResponseHandler(fn func(*types.LookupResponse)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responseHandler = fn
}

func (m *MockDiscoveryManager) SetGossipHandler(fn func(*pubsub.Message)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gossipHandler = fn
}

func (m *MockDiscoveryManager) SetSignatureVerified(sig []byte, verified bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.verifiedSignatures[string(sig)] = verified
}

func (m *MockDiscoveryManager) GetSentResponses() []*types.LookupResponse {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]*types.LookupResponse{}, m.sentResponses...)
}

func (m *MockDiscoveryManager) IsStarted() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.started
}

func (m *MockDiscoveryManager) SetDHTEnabled(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dhtEnabled = enabled
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
