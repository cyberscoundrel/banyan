package mocks

import (
	"sync"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"

	"banyan/interfaces"
	"banyan/types"
)

type MockServiceBeacon struct {
	mu           sync.RWMutex
	serviceKey   crypto.PrivKey
	mode         types.ServiceBeaconMode
	peers        []peer.ID
	alias        string
	figTemplates [][]byte
	started      bool
}

func NewMockServiceBeacon(serviceKey crypto.PrivKey) *MockServiceBeacon {
	return &MockServiceBeacon{
		serviceKey: serviceKey,
		mode:       types.ServiceBeaconModeLookup,
		peers:      make([]peer.ID, 0),
	}
}

func (b *MockServiceBeacon) Start() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.started = true
	return nil
}

func (b *MockServiceBeacon) Stop() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.started = false
	return nil
}

func (b *MockServiceBeacon) GetServiceKey() crypto.PrivKey {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.serviceKey
}

func (b *MockServiceBeacon) GetMode() types.ServiceBeaconMode {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.mode
}

func (b *MockServiceBeacon) SetMode(mode types.ServiceBeaconMode) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.mode = mode
}

func (b *MockServiceBeacon) GetPeers() []peer.ID {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return append([]peer.ID{}, b.peers...)
}

func (b *MockServiceBeacon) GetAlias() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.alias
}

func (b *MockServiceBeacon) GetFigTemplates() [][]byte {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.figTemplates
}

func (b *MockServiceBeacon) AddPeer(p peer.ID) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.peers = append(b.peers, p)
}

func (b *MockServiceBeacon) IsStarted() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.started
}

func (b *MockServiceBeacon) SetAlias(alias string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.alias = alias
}

func (b *MockServiceBeacon) SetFigTemplates(templates [][]byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.figTemplates = templates
}

type MockServiceManager struct {
	mu                sync.RWMutex
	beacons           []*MockServiceBeacon
	locators          map[string]crypto.PubKey
	serviceResponses  []*types.ServiceResponse
	connectCallback   func(peer.ID)
	responseCallback  func(*types.ServiceResponse)
}

func NewMockServiceManager() *MockServiceManager {
	return &MockServiceManager{
		beacons:          make([]*MockServiceBeacon, 0),
		locators:         make(map[string]crypto.PubKey),
		serviceResponses: make([]*types.ServiceResponse, 0),
	}
}

func (m *MockServiceManager) CreateServiceBeacon(servicePrivKey crypto.PrivKey) (interfaces.ServiceBeacon, error) {
	beacon := NewMockServiceBeacon(servicePrivKey)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.beacons = append(m.beacons, beacon)
	return beacon, nil
}

func (m *MockServiceManager) CreateServiceBeaconWithMeta(servicePrivKey crypto.PrivKey, alias string, figTemplates [][]byte) (interfaces.ServiceBeacon, error) {
	beacon := NewMockServiceBeacon(servicePrivKey)
	beacon.SetAlias(alias)
	beacon.SetFigTemplates(figTemplates)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.beacons = append(m.beacons, beacon)
	return beacon, nil
}

func (m *MockServiceManager) StartServiceLocator(servicePubKey crypto.PubKey, mode types.ServiceBeaconMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	keyBytes, _ := crypto.MarshalPublicKey(servicePubKey)
	keyStr := string(keyBytes)
	m.locators[keyStr] = servicePubKey
	return nil
}

func (m *MockServiceManager) ExtractPeerIDFromServiceRequest(req *types.ServiceLookupRequest) string {
	return req.From
}

func (m *MockServiceManager) ConnectToServiceProvider(peerID peer.ID) {
	m.mu.RLock()
	callback := m.connectCallback
	m.mu.RUnlock()

	if callback != nil {
		callback(peerID)
	}
}

func (m *MockServiceManager) SendServiceLookupResponse(req *types.ServiceLookupRequest) {
}

func (m *MockServiceManager) GetServiceBeacon() interfaces.ServiceBeacon {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.beacons) == 0 {
		return nil
	}
	return m.beacons[0]
}

func (m *MockServiceManager) ListServiceBeacons() []interfaces.ServiceBeacon {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]interfaces.ServiceBeacon, len(m.beacons))
	for i, b := range m.beacons {
		result[i] = b
	}
	return result
}

func (m *MockServiceManager) SendConnectToPeerResponse(peerIDStr string) {
}

func (m *MockServiceManager) SendDirectServiceResponse(peerID peer.ID, response *types.ServiceResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.serviceResponses = append(m.serviceResponses, response)
}

func (m *MockServiceManager) ProcessServiceResponse(resp *types.ServiceResponse) {
	m.mu.RLock()
	callback := m.responseCallback
	m.mu.RUnlock()

	if callback != nil {
		callback(resp)
	}
}

func (m *MockServiceManager) TestServicePeerBidirectionality(peerID peer.ID, serviceKey []byte) {
}

func (m *MockServiceManager) ConnectToServiceProviderDirectly(peerID peer.ID, addresses []string) {
	m.mu.RLock()
	callback := m.connectCallback
	m.mu.RUnlock()

	if callback != nil {
		callback(peerID)
	}
}

func (m *MockServiceManager) SetConnectCallback(fn func(peer.ID)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connectCallback = fn
}

func (m *MockServiceManager) SetResponseCallback(fn func(*types.ServiceResponse)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responseCallback = fn
}

func (m *MockServiceManager) GetServiceResponses() []*types.ServiceResponse {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]*types.ServiceResponse{}, m.serviceResponses...)
}

func (m *MockServiceManager) GetBeaconCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.beacons)
}

func (m *MockServiceManager) GetMockBeacons() []*MockServiceBeacon {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]*MockServiceBeacon{}, m.beacons...)
}
