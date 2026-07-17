package libp2phttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/test"

	"banyan/interfaces"
	"banyan/types"
)

func newTestHandler(t *testing.T) (*Handler, crypto.PrivKey) {
	t.Helper()

	priv, _, err := test.RandTestKeyPair(crypto.Ed25519, 256)
	if err != nil {
		t.Fatalf("Failed to generate test keys: %v", err)
	}

	host, err := libp2p.New(libp2p.Identity(priv))
	if err != nil {
		t.Fatalf("Failed to create libp2p host: %v", err)
	}
	t.Cleanup(func() { host.Close() })

	connectionManager := &mockPeerManager{
		connections: make(map[peer.ID]*types.ConnectionItem),
	}
	serviceManager := &mockServiceManager{
		beacons: make([]interfaces.ServiceBeacon, 0),
	}
	cryptoManager := &mockCryptoManager{}
	eventBroadcaster := &mockEventBroadcaster{}
	routeTable := types.NewRouteTable()

	handler := NewHandler(
		host,
		context.Background(),
		nil,
		connectionManager,
		serviceManager,
		cryptoManager,
		eventBroadcaster,
		routeTable,
		false,
		nil,
	).(*Handler)

	return handler, priv
}

func TestHandlePing(t *testing.T) {
	handler, _ := newTestHandler(t)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"GET request", http.MethodGet, "/ping"},
		{"POST request", http.MethodPost, "/ping"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()

			handler.HandlePing(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
			}

			if rec.Header().Get("Content-Type") != "application/json" {
				t.Errorf("Expected Content-Type application/json, got %s", rec.Header().Get("Content-Type"))
			}

			var response map[string]interface{}
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatalf("Failed to parse response: %v", err)
			}

			if response["status"] != "ok" {
				t.Errorf("Expected status 'ok', got %v", response["status"])
			}

			if response["peer_id"] == "" {
				t.Error("Expected peer_id to be set")
			}

			if response["method"] != tt.method {
				t.Errorf("Expected method %s, got %v", tt.method, response["method"])
			}

			if response["path"] != tt.path {
				t.Errorf("Expected path %s, got %v", tt.path, response["path"])
			}
		})
	}
}

func TestHandleGreetings(t *testing.T) {
	handler, _ := newTestHandler(t)

	tests := []struct {
		name   string
		method string
	}{
		{"GET request", http.MethodGet},
		{"POST request", http.MethodPost},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/greetings", nil)
			rec := httptest.NewRecorder()

			handler.HandleGreetings(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
			}

			if rec.Header().Get("Content-Type") != "application/json" {
				t.Errorf("Expected Content-Type application/json, got %s", rec.Header().Get("Content-Type"))
			}

			var response types.GreetingResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatalf("Failed to parse response: %v", err)
			}

			if response.PeerID == "" {
				t.Error("Expected peer_id to be set")
			}

			if response.Timestamp.IsZero() {
				t.Error("Expected timestamp to be set")
			}

			if response.Data == nil {
				t.Error("Expected data to be set")
			}

			data, ok := response.Data.(map[string]interface{})
			if !ok {
				t.Fatal("Expected data to be a map")
			}

			if data["http_capable"] != true {
				t.Error("Expected http_capable to be true")
			}

			if data["has_service"] != false {
				t.Error("Expected has_service to be false without service key")
			}

			addresses, ok := data["addresses"].([]interface{})
			if !ok {
				t.Error("Expected addresses to be an array")
			}
			if len(addresses) == 0 {
				t.Error("Expected at least one address")
			}
		})
	}
}

func TestHandleGreetingsWithServiceKey(t *testing.T) {
	priv, _, err := test.RandTestKeyPair(crypto.Ed25519, 256)
	if err != nil {
		t.Fatalf("Failed to generate test keys: %v", err)
	}

	host, err := libp2p.New(libp2p.Identity(priv))
	if err != nil {
		t.Fatalf("Failed to create libp2p host: %v", err)
	}
	t.Cleanup(func() { host.Close() })

	servicePriv, _, err := test.RandTestKeyPair(crypto.Ed25519, 256)
	if err != nil {
		t.Fatalf("Failed to generate service keys: %v", err)
	}

	connectionManager := &mockPeerManager{connections: make(map[peer.ID]*types.ConnectionItem)}
	serviceManager := &mockServiceManager{beacons: make([]interfaces.ServiceBeacon, 0)}
	cryptoManager := &mockCryptoManager{serviceKey: servicePriv, hasServiceKey: true}
	eventBroadcaster := &mockEventBroadcaster{}
	routeTable := types.NewRouteTable()

	handler := NewHandler(
		host,
		context.Background(),
		nil,
		connectionManager,
		serviceManager,
		cryptoManager,
		eventBroadcaster,
		routeTable,
		false,
		nil,
	).(*Handler)

	req := httptest.NewRequest(http.MethodGet, "/greetings", nil)
	rec := httptest.NewRecorder()

	handler.HandleGreetings(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var response types.GreetingResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if len(response.ServiceKey) == 0 {
		t.Error("Expected service_key to be set")
	}

	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Expected data to be a map")
	}

	if data["has_service"] != true {
		t.Error("Expected has_service to be true with service key")
	}

	if data["service_active"] != true {
		t.Error("Expected service_active to be true")
	}
}

func TestHandleGreetingsWithAddonDisclosure(t *testing.T) {
	handler, _ := newTestHandler(t)

	handler.SetAddonDisclosureProvider(func() []types.AddonDisclosure {
		return []types.AddonDisclosure{
			{Name: "test-addon", Version: "^1.0.0", Info: map[string]interface{}{"description": "Test addon"}},
			{Name: "", Version: "0.0.1"},
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/greetings", nil)
	rec := httptest.NewRecorder()

	handler.HandleGreetings(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var response types.GreetingResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Expected data to be a map")
	}

	addons, ok := data["addons"].([]interface{})
	if !ok {
		t.Fatal("Expected addons to be an array")
	}

	if len(addons) != 1 {
		t.Errorf("Expected 1 addon (empty name should be filtered), got %d", len(addons))
	}
}

func TestHandleGreetingsMethodNotAllowed(t *testing.T) {
	handler, _ := newTestHandler(t)

	req := httptest.NewRequest(http.MethodPut, "/greetings", nil)
	rec := httptest.NewRecorder()

	handler.HandleGreetings(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}
}

func TestHandleServiceFigs(t *testing.T) {
	handler, _ := newTestHandler(t)

	tests := []struct {
		name   string
		method string
	}{
		{"GET request", http.MethodGet},
		{"POST request", http.MethodPost},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/services/figs", nil)
			rec := httptest.NewRecorder()

			handler.HandleServiceFigs(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
			}

			if rec.Header().Get("Content-Type") != "application/json" {
				t.Errorf("Expected Content-Type application/json, got %s", rec.Header().Get("Content-Type"))
			}

			var response map[string]interface{}
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatalf("Failed to parse response: %v", err)
			}

			if response["peerId"] == "" {
				t.Error("Expected peerId to be set")
			}

			if response["time"] == nil {
				t.Error("Expected time to be set")
			}

			services := response["services"]

			if services != nil {
				if servicesArr, ok := services.([]interface{}); ok {
					if len(servicesArr) != 0 {
						t.Errorf("Expected empty services array for handler without beacons, got %d", len(servicesArr))
					}
				}
			}
		})
	}
}

func TestHandleServiceFigsWithBeacon(t *testing.T) {
	priv, _, err := test.RandTestKeyPair(crypto.Ed25519, 256)
	if err != nil {
		t.Fatalf("Failed to generate test keys: %v", err)
	}

	host, err := libp2p.New(libp2p.Identity(priv))
	if err != nil {
		t.Fatalf("Failed to create libp2p host: %v", err)
	}
	t.Cleanup(func() { host.Close() })

	servicePriv, _, err := test.RandTestKeyPair(crypto.Ed25519, 256)
	if err != nil {
		t.Fatalf("Failed to generate service keys: %v", err)
	}

	beacon := &mockServiceBeacon{
		serviceKey: servicePriv,
		alias:      "test-service",
		templates:  nil,
	}

	connectionManager := &mockPeerManager{connections: make(map[peer.ID]*types.ConnectionItem)}
	serviceManager := &mockServiceManager{beacons: []interfaces.ServiceBeacon{beacon}}
	cryptoManager := &mockCryptoManager{}
	eventBroadcaster := &mockEventBroadcaster{}
	routeTable := types.NewRouteTable()

	handler := NewHandler(
		host,
		context.Background(),
		nil,
		connectionManager,
		serviceManager,
		cryptoManager,
		eventBroadcaster,
		routeTable,
		false,
		nil,
	).(*Handler)

	body := bytes.NewBufferString(`{"nonce":"test123","requesterPeerId":"12D3KooWTest"}`)
	req := httptest.NewRequest(http.MethodPost, "/services/figs", body)
	rec := httptest.NewRecorder()

	handler.HandleServiceFigs(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	services, ok := response["services"].([]interface{})
	if !ok {
		t.Fatal("Expected services to be an array")
	}

	if len(services) != 1 {
		t.Fatalf("Expected 1 service, got %d", len(services))
	}

	service, ok := services[0].(map[string]interface{})
	if !ok {
		t.Fatal("Expected service to be a map")
	}

	if service["alias"] != "test-service" {
		t.Errorf("Expected alias 'test-service', got %v", service["alias"])
	}

	keys, ok := service["keys"].([]interface{})
	if !ok {
		t.Fatal("Expected keys to be an array")
	}
	if len(keys) == 0 {
		t.Error("Expected at least one key")
	}

	fig, ok := service["fig"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected fig to be a map")
	}
	if fig["serviceAlias"] != "test-service" {
		t.Errorf("Expected fig serviceAlias 'test-service', got %v", fig["serviceAlias"])
	}
}

func TestHandleServiceFigsMethodNotAllowed(t *testing.T) {
	handler, _ := newTestHandler(t)

	req := httptest.NewRequest(http.MethodPut, "/services/figs", nil)
	rec := httptest.NewRecorder()

	handler.HandleServiceFigs(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}
}

func TestHandleStatus(t *testing.T) {
	handler, _ := newTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	rec := httptest.NewRecorder()

	handler.HandleStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}

	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", rec.Header().Get("Content-Type"))
	}

	var response map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["peer_id"] == "" {
		t.Error("Expected peer_id to be set")
	}

	if response["status"] != "running" {
		t.Errorf("Expected status 'running', got %v", response["status"])
	}

	if response["timestamp"] == nil {
		t.Error("Expected timestamp to be set")
	}

	addrs, ok := response["addrs"].([]interface{})
	if !ok {
		t.Error("Expected addrs to be an array")
	}
	if len(addrs) == 0 {
		t.Error("Expected at least one address")
	}
}

func TestHandleConnectionsStatus(t *testing.T) {
	priv, _, err := test.RandTestKeyPair(crypto.Ed25519, 256)
	if err != nil {
		t.Fatalf("Failed to generate test keys: %v", err)
	}

	host, err := libp2p.New(libp2p.Identity(priv))
	if err != nil {
		t.Fatalf("Failed to create libp2p host: %v", err)
	}
	t.Cleanup(func() { host.Close() })

	peerID1, err := peer.Decode("12D3KooWGshytQABPXJV5R6sXsWt5DqQSBvhMTrJhmm7DwPPCBfV")
	if err != nil {
		t.Fatalf("Failed to decode peer ID: %v", err)
	}

	now := time.Now()
	connectionManager := &mockPeerManager{
		connections: map[peer.ID]*types.ConnectionItem{
			peerID1: {
				PeerID:         peerID1,
				Alias:          "test1",
				Status:         types.StatusConnected,
				ConnectionType: types.ConnTypeManual,
				HTTPCapable:    true,
				HTTPTestResult: types.HTTPTestSuccess,
				Connected:      now,
				LastActivity:   now,
			},
		},
	}

	serviceManager := &mockServiceManager{beacons: make([]interfaces.ServiceBeacon, 0)}
	cryptoManager := &mockCryptoManager{}
	eventBroadcaster := &mockEventBroadcaster{}
	routeTable := types.NewRouteTable()

	handler := NewHandler(
		host,
		context.Background(),
		nil,
		connectionManager,
		serviceManager,
		cryptoManager,
		eventBroadcaster,
		routeTable,
		false,
		nil,
	).(*Handler)

	req := httptest.NewRequest(http.MethodGet, "/connections", nil)
	rec := httptest.NewRecorder()

	handler.HandleConnectionsStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}

	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", rec.Header().Get("Content-Type"))
	}

	var response map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["count"].(float64) != 1 {
		t.Errorf("Expected count 1, got %v", response["count"])
	}

	if response["timestamp"] == nil {
		t.Error("Expected timestamp to be set")
	}

	connections, ok := response["connections"].([]interface{})
	if !ok {
		t.Fatal("Expected connections to be an array")
	}

	if len(connections) != 1 {
		t.Fatalf("Expected 1 connection, got %d", len(connections))
	}

	conn, ok := connections[0].(map[string]interface{})
	if !ok {
		t.Fatal("Expected connection to be a map")
	}

	if conn["peer_id"] != "12D3KooWGshytQABPXJV5R6sXsWt5DqQSBvhMTrJhmm7DwPPCBfV" {
		t.Errorf("Unexpected peer_id: %v", conn["peer_id"])
	}

	if conn["alias"] != "test1" {
		t.Errorf("Expected alias 'test1', got %v", conn["alias"])
	}

	if conn["status"] != types.StatusConnected {
		t.Errorf("Expected status '%s', got %v", types.StatusConnected, conn["status"])
	}

	if conn["connection_type"] != types.ConnTypeManual {
		t.Errorf("Expected connection_type '%s', got %v", types.ConnTypeManual, conn["connection_type"])
	}

	if conn["http_capable"] != true {
		t.Errorf("Expected http_capable true, got %v", conn["http_capable"])
	}

	if conn["http_test"] != types.HTTPTestSuccess {
		t.Errorf("Expected http_test '%s', got %v", types.HTTPTestSuccess, conn["http_test"])
	}
}

func TestHandleConnectionsStatusEmpty(t *testing.T) {
	handler, _ := newTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/connections", nil)
	rec := httptest.NewRecorder()

	handler.HandleConnectionsStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["count"].(float64) != 0 {
		t.Errorf("Expected count 0, got %v", response["count"])
	}

	connections, ok := response["connections"].([]interface{})
	if !ok {
		t.Fatal("Expected connections to be an array")
	}

	if len(connections) != 0 {
		t.Errorf("Expected empty connections array, got %d", len(connections))
	}
}

type mockPeerManager struct {
	connections map[peer.ID]*types.ConnectionItem
}

func (m *mockPeerManager) AddTrackedPeer(peerID peer.ID, options types.PeerOptions) *types.ConnectionItem {
	conn := &types.ConnectionItem{
		PeerID:         peerID,
		ConnectionType: options.ConnectionType,
		HTTPCapable:    options.HTTPCapable,
		Status:         types.StatusConnecting,
		Connected:      time.Now(),
		LastActivity:   time.Now(),
	}
	m.connections[peerID] = conn
	return conn
}

func (m *mockPeerManager) MarkPeerHTTPCapable(peerID peer.ID, bidirectional bool) {
	if conn, ok := m.connections[peerID]; ok {
		conn.HTTPCapable = true
		conn.BidirectionalHTTP = bidirectional
	}
}

func (m *mockPeerManager) MarkHTTPTestResult(peerID peer.ID, result string) {
	if conn, ok := m.connections[peerID]; ok {
		conn.HTTPTestResult = result
		now := time.Now()
		conn.LastHTTPTest = &now
	}
}

func (m *mockPeerManager) UpdateConnectionStatus(peerID peer.ID, status string) {
	if conn, ok := m.connections[peerID]; ok {
		conn.Status = status
	}
}

func (m *mockPeerManager) GetConnectionInfo(peerID peer.ID) (*types.ConnectionItem, bool) {
	conn, ok := m.connections[peerID]
	return conn, ok
}

func (m *mockPeerManager) GetConnectionsCopy() map[peer.ID]*types.ConnectionItem {
	result := make(map[peer.ID]*types.ConnectionItem)
	for k, v := range m.connections {
		result[k] = v
	}
	return result
}

func (m *mockPeerManager) GetPeerByAlias(alias string) (peer.ID, bool) {
	for pid, conn := range m.connections {
		if conn.Alias == alias {
			return pid, true
		}
	}
	return "", false
}

func (m *mockPeerManager) CleanupOldConnections() {}

func (m *mockPeerManager) ConnectToPeer(peerID peer.ID) error {
	return nil
}

func (m *mockPeerManager) ConnectToRequesterDirectly(peerID peer.ID, addresses []string) {}

func (m *mockPeerManager) LookupPeer(targetPeerID string, includeFrom bool) error {
	return nil
}

func (m *mockPeerManager) LookupPeerViaGossipsub(peerID peer.ID) {}

func (m *mockPeerManager) HandleNewConnection(peerID peer.ID) {}

func (m *mockPeerManager) HandleDisconnection(peerID peer.ID) {}

func (m *mockPeerManager) CreateBidirectionalConnections() {}

func (m *mockPeerManager) CreateBidirectionalConnection(peerID peer.ID) {}

func (m *mockPeerManager) TestPeerHTTPCapability(peerID peer.ID) {}

type mockServiceManager struct {
	beacons []interfaces.ServiceBeacon
}

func (m *mockServiceManager) CreateServiceBeacon(servicePrivKey crypto.PrivKey) (interfaces.ServiceBeacon, error) {
	return nil, nil
}

func (m *mockServiceManager) CreateServiceBeaconWithMeta(servicePrivKey crypto.PrivKey, alias string, figTemplates [][]byte) (interfaces.ServiceBeacon, error) {
	return nil, nil
}

func (m *mockServiceManager) StartServiceLocator(servicePubKey crypto.PubKey, mode types.ServiceBeaconMode) error {
	return nil
}

func (m *mockServiceManager) ExtractPeerIDFromServiceRequest(req *types.ServiceLookupRequest) string {
	return ""
}

func (m *mockServiceManager) ConnectToServiceProvider(peerID peer.ID) {}

func (m *mockServiceManager) SendServiceLookupResponse(req *types.ServiceLookupRequest) {}

func (m *mockServiceManager) GetServiceBeacon() interfaces.ServiceBeacon {
	if len(m.beacons) > 0 {
		return m.beacons[0]
	}
	return nil
}

func (m *mockServiceManager) ListServiceBeacons() []interfaces.ServiceBeacon {
	return m.beacons
}

func (m *mockServiceManager) SendConnectToPeerResponse(peerIDStr string) {}

func (m *mockServiceManager) SendDirectServiceResponse(peerID peer.ID, response *types.ServiceResponse) {}

func (m *mockServiceManager) ProcessServiceResponse(resp *types.ServiceResponse) {}

func (m *mockServiceManager) TestServicePeerBidirectionality(peerID peer.ID, serviceKey []byte) {}

func (m *mockServiceManager) ConnectToServiceProviderDirectly(peerID peer.ID, addresses []string) {}

type mockServiceBeacon struct {
	serviceKey crypto.PrivKey
	alias      string
	templates  [][]byte
	mode       types.ServiceBeaconMode
	peers      []peer.ID
}

func (m *mockServiceBeacon) Start() error { return nil }

func (m *mockServiceBeacon) Stop() error { return nil }

func (m *mockServiceBeacon) GetServiceKey() crypto.PrivKey { return m.serviceKey }

func (m *mockServiceBeacon) GetMode() types.ServiceBeaconMode { return m.mode }

func (m *mockServiceBeacon) SetMode(mode types.ServiceBeaconMode) { m.mode = mode }

func (m *mockServiceBeacon) GetPeers() []peer.ID { return m.peers }

func (m *mockServiceBeacon) GetAlias() string { return m.alias }

func (m *mockServiceBeacon) GetFigTemplates() [][]byte { return m.templates }

type mockCryptoManager struct {
	serviceKey    crypto.PrivKey
	hasServiceKey bool
}

func (m *mockCryptoManager) EncryptWithPublicKey(data []byte, requesterPubKeyBytes []byte) ([]byte, []byte, error) {
	return nil, nil, nil
}

func (m *mockCryptoManager) DecryptWithPublicKey(encryptedData, nonce []byte, senderPubKeyBytes []byte) ([]byte, error) {
	return nil, nil
}

func (m *mockCryptoManager) EncryptWithServiceKey(data []byte, requesterPubKeyBytes []byte, servicePrivKey crypto.PrivKey) ([]byte, []byte, error) {
	return nil, nil, nil
}

func (m *mockCryptoManager) DecryptWithLocatorKey(encryptedData, nonce []byte, senderServicePubKeyBytes []byte, locatorPrivKey crypto.PrivKey) ([]byte, error) {
	return nil, nil
}

func (m *mockCryptoManager) EncryptPeerIDWithServiceKey(peerID string, servicePubKey crypto.PubKey) ([]byte, []byte, error) {
	return nil, nil, nil
}

func (m *mockCryptoManager) DecryptPeerIDWithSpecificServiceKey(encryptedPeerID, nonce []byte, requesterPubKey []byte, servicePrivKey crypto.PrivKey) (string, error) {
	return "", nil
}

func (m *mockCryptoManager) RegisterServiceKey(keyID string, serviceKey crypto.PrivKey) {}

func (m *mockCryptoManager) UnregisterServiceKey(keyID string) {}

func (m *mockCryptoManager) GetServiceKeyByID(keyID string) crypto.PrivKey { return nil }

func (m *mockCryptoManager) HasServiceKeyByID(keyID string) bool { return false }

func (m *mockCryptoManager) ListServiceKeys() []string { return nil }

func (m *mockCryptoManager) GetServiceKey() crypto.PrivKey { return m.serviceKey }

func (m *mockCryptoManager) HasServiceKey() bool { return m.hasServiceKey }

func (m *mockCryptoManager) DecryptPeerIDWithServiceKey(encryptedPeerID, nonce []byte, requesterPubKey []byte) (string, error) {
	return "", nil
}

type mockEventBroadcaster struct{}

func (m *mockEventBroadcaster) BroadcastEvent(event types.Event) {}

func (m *mockEventBroadcaster) SendEvent(eventType string, data interface{}) {}

func (m *mockEventBroadcaster) Subscribe(subscriber interfaces.WebSocketHub) {}

func (m *mockEventBroadcaster) Unsubscribe(subscriber interfaces.WebSocketHub) {}

func (m *mockEventBroadcaster) GetSubscriberCount() int { return 0 }
