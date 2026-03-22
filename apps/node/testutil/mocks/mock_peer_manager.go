package mocks

import (
	"sync"

	"github.com/libp2p/go-libp2p/core/peer"

	"banyan/types"
)

type MockPeerManager struct {
	mu               sync.RWMutex
	peers            map[peer.ID]*types.ConnectionItem
	aliases          map[string]peer.ID
	httpCapable      map[peer.ID]bool
	httpTestResults  map[peer.ID]string
	connectErrors    map[peer.ID]error
	connectedPeers   map[peer.ID]bool
	connectCallback  func(peer.ID) error
}

func NewMockPeerManager() *MockPeerManager {
	return &MockPeerManager{
		peers:           make(map[peer.ID]*types.ConnectionItem),
		aliases:         make(map[string]peer.ID),
		httpCapable:     make(map[peer.ID]bool),
		httpTestResults: make(map[peer.ID]string),
		connectErrors:   make(map[peer.ID]error),
		connectedPeers:  make(map[peer.ID]bool),
	}
}

func (m *MockPeerManager) AddTrackedPeer(peerID peer.ID, options types.PeerOptions) *types.ConnectionItem {
	m.mu.Lock()
	defer m.mu.Unlock()

	item := &types.ConnectionItem{
		PeerID:         peerID,
		ConnectionType: options.ConnectionType,
		HTTPCapable:    options.HTTPCapable,
		Status:         types.StatusConnected,
	}
	if options.ServiceKey != nil {
		item.ServiceKeys = [][]byte{options.ServiceKey}
	}
	m.peers[peerID] = item
	m.connectedPeers[peerID] = true

	return item
}

func (m *MockPeerManager) MarkPeerHTTPCapable(peerID peer.ID, bidirectional bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.httpCapable[peerID] = true
	if item, ok := m.peers[peerID]; ok {
		item.HTTPCapable = true
		item.BidirectionalHTTP = bidirectional
	}
}

func (m *MockPeerManager) MarkHTTPTestResult(peerID peer.ID, result string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.httpTestResults[peerID] = result
	if item, ok := m.peers[peerID]; ok {
		item.HTTPTestResult = result
	}
}

func (m *MockPeerManager) UpdateConnectionStatus(peerID peer.ID, status string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if item, ok := m.peers[peerID]; ok {
		item.Status = status
	}
	m.connectedPeers[peerID] = (status == types.StatusConnected)
}

func (m *MockPeerManager) GetConnectionInfo(peerID peer.ID) (*types.ConnectionItem, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	item, ok := m.peers[peerID]
	return item, ok
}

func (m *MockPeerManager) GetConnectionsCopy() map[peer.ID]*types.ConnectionItem {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[peer.ID]*types.ConnectionItem, len(m.peers))
	for k, v := range m.peers {
		result[k] = v
	}
	return result
}

func (m *MockPeerManager) GetPeerByAlias(alias string) (peer.ID, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	peerID, ok := m.aliases[alias]
	return peerID, ok
}

func (m *MockPeerManager) CleanupOldConnections() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, item := range m.peers {
		if item.Status == types.StatusDisconnected {
			delete(m.peers, id)
			delete(m.connectedPeers, id)
		}
	}
}

func (m *MockPeerManager) ConnectToPeer(peerID peer.ID) error {
	if m.connectCallback != nil {
		return m.connectCallback(peerID)
	}
	if err, ok := m.connectErrors[peerID]; ok {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connectedPeers[peerID] = true
	return nil
}

func (m *MockPeerManager) ConnectToRequesterDirectly(peerID peer.ID, addresses []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connectedPeers[peerID] = true
}

func (m *MockPeerManager) LookupPeer(targetPeerID string, includeFrom bool) error {
	return nil
}

func (m *MockPeerManager) LookupPeerViaGossipsub(peerID peer.ID) {
}

func (m *MockPeerManager) HandleNewConnection(peerID peer.ID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connectedPeers[peerID] = true
	if _, ok := m.peers[peerID]; !ok {
		m.peers[peerID] = &types.ConnectionItem{
			PeerID: peerID,
			Status: types.StatusConnected,
		}
	}
}

func (m *MockPeerManager) HandleDisconnection(peerID peer.ID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connectedPeers[peerID] = false
	if item, ok := m.peers[peerID]; ok {
		item.Status = types.StatusDisconnected
	}
}

func (m *MockPeerManager) CreateBidirectionalConnections() {
}

func (m *MockPeerManager) CreateBidirectionalConnection(peerID peer.ID) {
}

func (m *MockPeerManager) TestPeerHTTPCapability(peerID peer.ID) {
}

func (m *MockPeerManager) SetConnectError(peerID peer.ID, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connectErrors[peerID] = err
}

func (m *MockPeerManager) SetConnectCallback(fn func(peer.ID) error) {
	m.connectCallback = fn
}

func (m *MockPeerManager) IsConnected(peerID peer.ID) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connectedPeers[peerID]
}

func (m *MockPeerManager) SetAlias(peerID peer.ID, alias string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.aliases[alias] = peerID
	if item, ok := m.peers[peerID]; ok {
		item.Alias = alias
	}
}

func (m *MockPeerManager) GetPeerCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.peers)
}
