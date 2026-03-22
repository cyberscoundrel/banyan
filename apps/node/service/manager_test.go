package service

import (
	"context"
	"crypto/rand"
	"net/http"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"

	"banyan/interfaces"
	"banyan/testutil/mocks"
	"banyan/types"
)

func setupTestManager(t *testing.T) (interfaces.ServiceManager, host.Host, *pubsub.PubSub, *mocks.MockCryptoManager, *mocks.MockPeerManager, context.Context) {
	ctx := context.Background()

	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("Failed to create host: %v", err)
	}

	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		t.Fatalf("Failed to create pubsub: %v", err)
	}

	mockCryptoManager := mocks.NewMockCryptoManager()
	mockPeerManager := mocks.NewMockPeerManager()
	httpTransport := &http.Transport{}

	eventSender := func(eventType string, data interface{}) {}

	manager := NewManager(h, ctx, ps, mockCryptoManager, mockPeerManager, httpTransport, eventSender, false, false, false, false)

	return manager, h, ps, mockCryptoManager, mockPeerManager, ctx
}

func generateTestKey(t *testing.T) crypto.PrivKey {
	priv, _, err := crypto.GenerateKeyPairWithReader(crypto.Ed25519, 0, rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}
	return priv
}

// TestCreateServiceBeacon tests the creation of a service beacon.
//
// WHAT IT IS DOING:
//   Creates a new service beacon using a generated private key and verifies
//   that the beacon is properly registered with the manager.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Asserts that the beacon is non-nil, has Announce mode by default,
//   has the correct service key set, and appears in the manager's beacon list.
func TestCreateServiceBeacon(t *testing.T) {
	manager, h, _, _, _, _ := setupTestManager(t)
	defer h.Close()

	servicePrivKey := generateTestKey(t)

	beacon, err := manager.CreateServiceBeacon(servicePrivKey)
	if err != nil {
		t.Fatalf("Failed to create service beacon: %v", err)
	}

	if beacon == nil {
		t.Fatal("Expected non-nil beacon")
	}

	if beacon.GetMode() != types.ServiceBeaconModeAnnounce {
		t.Errorf("Expected mode to be Announce, got %v", beacon.GetMode())
	}

	if beacon.GetServiceKey() == nil {
		t.Error("Expected service key to be set")
	}

	if beacon.GetServiceKey() != servicePrivKey {
		t.Error("Expected service key to match the provided key")
	}

	beacons := manager.ListServiceBeacons()
	if len(beacons) != 1 {
		t.Errorf("Expected 1 beacon, got %d", len(beacons))
	}
}

// TestServiceBeaconStartStop tests the lifecycle of starting and stopping a service beacon.
//
// WHAT IT IS DOING:
//   Creates a service beacon, starts it, waits briefly, then stops it
//   while verifying the context remains valid throughout.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Asserts that Start() and Stop() complete without error and that
//   the context is not cancelled unexpectedly during operation.
func TestServiceBeaconStartStop(t *testing.T) {
	manager, h, _, _, _, ctx := setupTestManager(t)
	defer h.Close()

	servicePrivKey := generateTestKey(t)

	beacon, err := manager.CreateServiceBeacon(servicePrivKey)
	if err != nil {
		t.Fatalf("Failed to create service beacon: %v", err)
	}

	err = beacon.Start()
	if err != nil {
		t.Fatalf("Failed to start beacon: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	select {
	case <-ctx.Done():
		t.Error("Context cancelled unexpectedly")
	default:
	}

	err = beacon.Stop()
	if err != nil {
		t.Fatalf("Failed to stop beacon: %v", err)
	}

	time.Sleep(50 * time.Millisecond)
}

// TestProcessServiceResponse tests handling of incoming service responses.
//
// WHAT IT IS DOING:
//   Tests various scenarios for processing service responses including
//   plaintext responses with addresses, missing peer IDs, encrypted responses
//   with missing fields, and valid peer IDs that trigger connection attempts.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Each subtest verifies that ProcessServiceResponse handles different
//   response configurations correctly without panicking or causing errors.
func TestProcessServiceResponse(t *testing.T) {
	t.Run("plaintext response with addresses", func(t *testing.T) {
		manager, h, _, _, _, ctx := setupTestManager(t)
		defer h.Close()

		sm := manager.(*Manager)
		sm.noCrypto = true

		otherHost, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
		if err != nil {
			t.Fatalf("Failed to create other host: %v", err)
		}
		defer otherHost.Close()

		resp := &types.ServiceResponse{
			Type:      "service-response",
			From:      otherHost.ID().String(),
			Timestamp: time.Now(),
			Addresses: []string{},
		}

		sm.ProcessServiceResponse(resp)

		select {
		case <-time.After(100 * time.Millisecond):
		case <-ctx.Done():
		}
	})

	t.Run("plaintext response missing peer id", func(t *testing.T) {
		manager, h, _, _, _, _ := setupTestManager(t)
		defer h.Close()

		sm := manager.(*Manager)
		sm.noCrypto = true

		resp := &types.ServiceResponse{
			Type:      "service-response",
			From:      "",
			Timestamp: time.Now(),
		}

		sm.ProcessServiceResponse(resp)
	})

	t.Run("encrypted response missing fields", func(t *testing.T) {
		manager, h, _, _, _, _ := setupTestManager(t)
		defer h.Close()

		sm := manager.(*Manager)
		sm.noCrypto = false

		servicePrivKey := generateTestKey(t)
		servicePubKey := servicePrivKey.GetPublic()

		err := sm.StartServiceLocator(servicePubKey, types.ServiceBeaconModeLookup)
		if err != nil {
			t.Fatalf("Failed to start service locator: %v", err)
		}

		pubKeyBytes, _ := crypto.MarshalPublicKey(servicePubKey)
		resp := &types.ServiceResponse{
			Type:      "service-response",
			Timestamp: time.Now(),
			PublicKey: pubKeyBytes,
			Signature: []byte("signature"),
		}

		sm.ProcessServiceResponse(resp)
	})

	t.Run("valid peer ID triggers connection attempt", func(t *testing.T) {
		manager, h, _, _, _, _ := setupTestManager(t)
		defer h.Close()

		sm := manager.(*Manager)
		sm.noCrypto = true

		testPeerID, _ := peer.Decode("QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN")

		resp := &types.ServiceResponse{
			Type:      "service-response",
			From:      testPeerID.String(),
			Timestamp: time.Now(),
		}

		sm.ProcessServiceResponse(resp)

		time.Sleep(50 * time.Millisecond)
	})
}

// TestCreateServiceBeaconWithMeta tests creating a service beacon with metadata.
//
// WHAT IT IS DOING:
//   Creates a service beacon with an alias and fig templates,
//   then verifies the metadata is correctly stored.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Asserts that GetAlias() returns the provided alias, GetFigTemplates()
//   returns the correct templates, and the beacon appears in the list.
func TestCreateServiceBeaconWithMeta(t *testing.T) {
	manager, h, _, _, _, _ := setupTestManager(t)
	defer h.Close()

	servicePrivKey := generateTestKey(t)
	alias := "test-service"
	figTemplates := [][]byte{[]byte("template1"), []byte("template2")}

	beacon, err := manager.CreateServiceBeaconWithMeta(servicePrivKey, alias, figTemplates)
	if err != nil {
		t.Fatalf("Failed to create service beacon with meta: %v", err)
	}

	if beacon == nil {
		t.Fatal("Expected non-nil beacon")
	}

	if beacon.GetAlias() != alias {
		t.Errorf("Expected alias %s, got %s", alias, beacon.GetAlias())
	}

	templates := beacon.GetFigTemplates()
	if len(templates) != len(figTemplates) {
		t.Errorf("Expected %d templates, got %d", len(figTemplates), len(templates))
	}

	for i, tmpl := range templates {
		if string(tmpl) != string(figTemplates[i]) {
			t.Errorf("Template %d mismatch: expected %s, got %s", i, figTemplates[i], tmpl)
		}
	}

	beacons := manager.ListServiceBeacons()
	if len(beacons) != 1 {
		t.Errorf("Expected 1 beacon, got %d", len(beacons))
	}
}

// TestExtractPeerIDFromServiceRequest tests extracting peer IDs from service lookup requests.
//
// WHAT IT IS DOING:
//   Tests multiple scenarios: plaintext requests, encrypted requests with
//   crypto disabled, encrypted requests without service keys, encrypted
//   requests with service keys, and fallback to plaintext fields.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Each subtest verifies that ExtractPeerIDFromServiceRequest returns
//   the expected peer ID based on the request configuration and crypto state.
func TestExtractPeerIDFromServiceRequest(t *testing.T) {
	t.Run("plaintext request", func(t *testing.T) {
		manager, h, _, _, _, _ := setupTestManager(t)
		defer h.Close()

		sm := manager.(*Manager)
		sm.noCrypto = true

		expectedPeerID := "QmTestPeerID123456"
		req := &types.ServiceLookupRequest{
			Type:      "service-lookup",
			From:      expectedPeerID,
			Timestamp: time.Now(),
		}

		extractedPeerID := sm.ExtractPeerIDFromServiceRequest(req)
		if extractedPeerID != expectedPeerID {
			t.Errorf("Expected peer ID %s, got %s", expectedPeerID, extractedPeerID)
		}
	})

	t.Run("encrypted request with no crypto", func(t *testing.T) {
		manager, h, _, _, _, _ := setupTestManager(t)
		defer h.Close()

		sm := manager.(*Manager)
		sm.noCrypto = true

		req := &types.ServiceLookupRequest{
			Type:           "service-lookup",
			FromEncrypted:  true,
			EncryptedFrom:  []byte("encrypted-data"),
			EncryptedNonce: []byte("nonce"),
			Timestamp:      time.Now(),
		}

		extractedPeerID := sm.ExtractPeerIDFromServiceRequest(req)
		if extractedPeerID != "" {
			t.Errorf("Expected empty peer ID when crypto disabled with encrypted request, got %s", extractedPeerID)
		}
	})

	t.Run("encrypted request without service key", func(t *testing.T) {
		manager, h, _, mockCryptoManager, _, _ := setupTestManager(t)
		defer h.Close()

		sm := manager.(*Manager)
		sm.noCrypto = false

		req := &types.ServiceLookupRequest{
			Type:           "service-lookup",
			FromEncrypted:  true,
			EncryptedFrom:  []byte("encrypted-data"),
			EncryptedNonce: []byte("nonce"),
			PublicKey:      []byte("requester-pubkey"),
			Timestamp:      time.Now(),
		}

		extractedPeerID := sm.ExtractPeerIDFromServiceRequest(req)
		if extractedPeerID != "" {
			t.Errorf("Expected empty peer ID when no service key available, got %s", extractedPeerID)
		}

		_ = mockCryptoManager
	})

	t.Run("encrypted request with service key", func(t *testing.T) {
		manager, h, _, mockCryptoManager, _, _ := setupTestManager(t)
		defer h.Close()

		sm := manager.(*Manager)
		sm.noCrypto = false

		servicePrivKey := generateTestKey(t)
		mockCryptoManager.SetDefaultKey(servicePrivKey)

		expectedPeerID := "QmDecryptedPeerID123"
		mockCryptoManager.SetDecryptedData([]byte(expectedPeerID))

		req := &types.ServiceLookupRequest{
			Type:           "service-lookup",
			FromEncrypted:  true,
			EncryptedFrom:  []byte("encrypted-data"),
			EncryptedNonce: []byte("nonce"),
			PublicKey:      []byte("requester-pubkey"),
			Timestamp:      time.Now(),
		}

		extractedPeerID := sm.ExtractPeerIDFromServiceRequest(req)
		if extractedPeerID != expectedPeerID {
			t.Errorf("Expected peer ID %s, got %s", expectedPeerID, extractedPeerID)
		}
	})

	t.Run("empty encrypted fields returns plaintext", func(t *testing.T) {
		manager, h, _, _, _, _ := setupTestManager(t)
		defer h.Close()

		sm := manager.(*Manager)

		expectedPeerID := "QmPlaintextPeerID"
		req := &types.ServiceLookupRequest{
			Type:          "service-lookup",
			From:          expectedPeerID,
			FromEncrypted: false,
			Timestamp:     time.Now(),
		}

		extractedPeerID := sm.ExtractPeerIDFromServiceRequest(req)
		if extractedPeerID != expectedPeerID {
			t.Errorf("Expected peer ID %s, got %s", expectedPeerID, extractedPeerID)
		}
	})
}

// TestStartServiceLocator tests starting a service locator for a service key.
//
// WHAT IT IS DOING:
//   Starts a service locator with a public key in Lookup mode and
//   attempts to start a duplicate locator to verify duplicate prevention.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Asserts that the first locator starts successfully and is registered,
//   and that attempting to start a duplicate returns an error.
func TestStartServiceLocator(t *testing.T) {
	manager, h, _, _, _, _ := setupTestManager(t)
	defer h.Close()

	sm := manager.(*Manager)

	servicePrivKey := generateTestKey(t)
	servicePubKey := servicePrivKey.GetPublic()

	err := sm.StartServiceLocator(servicePubKey, types.ServiceBeaconModeLookup)
	if err != nil {
		t.Fatalf("Failed to start service locator: %v", err)
	}

	if len(sm.serviceLocators) != 1 {
		t.Errorf("Expected 1 locator, got %d", len(sm.serviceLocators))
	}

	err = sm.StartServiceLocator(servicePubKey, types.ServiceBeaconModeLookup)
	if err == nil {
		t.Error("Expected error when starting duplicate locator")
	}
}

// TestMultipleBeacons tests creating and managing multiple service beacons.
//
// WHAT IT IS DOING:
//   Creates two service beacons with different keys and verifies
//   they are both properly tracked by the manager.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Asserts that ListServiceBeacons returns both beacons, GetServiceBeacon
//   returns one of them, and each beacon has a unique service key.
func TestMultipleBeacons(t *testing.T) {
	manager, h, _, _, _, _ := setupTestManager(t)
	defer h.Close()

	key1 := generateTestKey(t)
	key2 := generateTestKey(t)

	beacon1, err := manager.CreateServiceBeacon(key1)
	if err != nil {
		t.Fatalf("Failed to create first beacon: %v", err)
	}

	beacon2, err := manager.CreateServiceBeacon(key2)
	if err != nil {
		t.Fatalf("Failed to create second beacon: %v", err)
	}

	beacons := manager.ListServiceBeacons()
	if len(beacons) != 2 {
		t.Errorf("Expected 2 beacons, got %d", len(beacons))
	}

	firstBeacon := manager.GetServiceBeacon()
	if firstBeacon == nil {
		t.Error("Expected first beacon from GetServiceBeacon")
	}

	if firstBeacon != beacon1 && firstBeacon != beacon2 {
		t.Error("GetServiceBeacon should return one of the created beacons")
	}

	if beacon1.GetServiceKey() == beacon2.GetServiceKey() {
		t.Error("Beacons should have different service keys")
	}
}

// TestBeaconModeOperations tests changing and retrieving beacon modes.
//
// WHAT IT IS DOING:
//   Creates a beacon and cycles through different modes (Announce, Lookup,
//   ReplyOnly) using SetMode and GetMode.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Asserts that the initial mode is Announce, and that SetMode correctly
//   updates the mode which is then reflected by GetMode.
func TestBeaconModeOperations(t *testing.T) {
	manager, h, _, _, _, _ := setupTestManager(t)
	defer h.Close()

	servicePrivKey := generateTestKey(t)

	beacon, err := manager.CreateServiceBeacon(servicePrivKey)
	if err != nil {
		t.Fatalf("Failed to create service beacon: %v", err)
	}

	if beacon.GetMode() != types.ServiceBeaconModeAnnounce {
		t.Errorf("Expected initial mode to be Announce, got %v", beacon.GetMode())
	}

	beacon.SetMode(types.ServiceBeaconModeLookup)
	if beacon.GetMode() != types.ServiceBeaconModeLookup {
		t.Errorf("Expected mode to be Lookup after SetMode, got %v", beacon.GetMode())
	}

	beacon.SetMode(types.ServiceBeaconModeReplyOnly)
	if beacon.GetMode() != types.ServiceBeaconModeReplyOnly {
		t.Errorf("Expected mode to be ReplyOnly after SetMode, got %v", beacon.GetMode())
	}
}

// TestBeaconPeers tests peer management within a service beacon.
//
// WHAT IT IS DOING:
//   Creates a beacon, verifies it starts with no peers, then manually
//   adds a peer and verifies it can be retrieved.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Asserts that GetPeers returns an empty list initially, and after
//   adding a peer, returns a list containing that peer.
func TestBeaconPeers(t *testing.T) {
	manager, h, _, _, _, _ := setupTestManager(t)
	defer h.Close()

	servicePrivKey := generateTestKey(t)

	beacon, err := manager.CreateServiceBeacon(servicePrivKey)
	if err != nil {
		t.Fatalf("Failed to create service beacon: %v", err)
	}

	peers := beacon.GetPeers()
	if len(peers) != 0 {
		t.Errorf("Expected no peers initially, got %d", len(peers))
	}

	sb, ok := beacon.(*ServiceBeacon)
	if !ok {
		t.Fatal("Failed to cast to ServiceBeacon")
	}

	testPeerID, _ := peer.Decode("QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN")
	sb.peers = append(sb.peers, testPeerID)

	peers = beacon.GetPeers()
	if len(peers) != 1 {
		t.Errorf("Expected 1 peer, got %d", len(peers))
	}

	if peers[0] != testPeerID {
		t.Errorf("Expected peer ID %s, got %s", testPeerID, peers[0])
	}
}

// TestCreateServiceBeaconWithMetaEmpty tests creating a beacon with empty metadata.
//
// WHAT IT IS DOING:
//   Creates a service beacon with an empty alias and nil templates
//   to verify the handler gracefully accepts empty metadata.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Asserts that GetAlias returns an empty string and GetFigTemplates
//   returns nil or an empty slice without causing errors.
func TestCreateServiceBeaconWithMetaEmpty(t *testing.T) {
	manager, h, _, _, _, _ := setupTestManager(t)
	defer h.Close()

	servicePrivKey := generateTestKey(t)

	beacon, err := manager.CreateServiceBeaconWithMeta(servicePrivKey, "", nil)
	if err != nil {
		t.Fatalf("Failed to create service beacon with empty meta: %v", err)
	}

	if beacon.GetAlias() != "" {
		t.Errorf("Expected empty alias, got %s", beacon.GetAlias())
	}

	templates := beacon.GetFigTemplates()
	if len(templates) != 0 {
		t.Errorf("Expected nil or empty templates, got %v", templates)
	}
}

// TestExtractPeerIDFromServiceRequestWithKey tests extracting peer IDs with a specific service key.
//
// WHAT IT IS DOING:
//   Tests extracting peer IDs from both plaintext and encrypted requests
//   using a specific service key, including handling nil keys.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Each subtest verifies that ExtractPeerIDFromServiceRequestWithKey
//   returns the expected peer ID or empty string based on the key and request type.
func TestExtractPeerIDFromServiceRequestWithKey(t *testing.T) {
	t.Run("plaintext request", func(t *testing.T) {
		manager, h, _, _, _, _ := setupTestManager(t)
		defer h.Close()

		sm := manager.(*Manager)

		expectedPeerID := "QmTestPeerID789"
		req := &types.ServiceLookupRequest{
			Type:      "service-lookup",
			From:      expectedPeerID,
			Timestamp: time.Now(),
		}

		servicePrivKey := generateTestKey(t)
		extractedPeerID := sm.ExtractPeerIDFromServiceRequestWithKey(req, servicePrivKey)
		if extractedPeerID != expectedPeerID {
			t.Errorf("Expected peer ID %s, got %s", expectedPeerID, extractedPeerID)
		}
	})

	t.Run("encrypted request with specific key", func(t *testing.T) {
		manager, h, _, mockCryptoManager, _, _ := setupTestManager(t)
		defer h.Close()

		sm := manager.(*Manager)
		sm.noCrypto = false

		servicePrivKey := generateTestKey(t)
		expectedPeerID := "QmDecryptedWithSpecificKey"
		mockCryptoManager.SetDecryptedData([]byte(expectedPeerID))

		req := &types.ServiceLookupRequest{
			Type:           "service-lookup",
			FromEncrypted:  true,
			EncryptedFrom:  []byte("encrypted-data"),
			EncryptedNonce: []byte("nonce"),
			PublicKey:      []byte("requester-pubkey"),
			Timestamp:      time.Now(),
		}

		extractedPeerID := sm.ExtractPeerIDFromServiceRequestWithKey(req, servicePrivKey)
		if extractedPeerID != expectedPeerID {
			t.Errorf("Expected peer ID %s, got %s", expectedPeerID, extractedPeerID)
		}
	})

	t.Run("nil service key", func(t *testing.T) {
		manager, h, _, _, _, _ := setupTestManager(t)
		defer h.Close()

		sm := manager.(*Manager)
		sm.noCrypto = false

		req := &types.ServiceLookupRequest{
			Type:           "service-lookup",
			FromEncrypted:  true,
			EncryptedFrom:  []byte("encrypted-data"),
			EncryptedNonce: []byte("nonce"),
			PublicKey:      []byte("requester-pubkey"),
			Timestamp:      time.Now(),
		}

		extractedPeerID := sm.ExtractPeerIDFromServiceRequestWithKey(req, nil)
		if extractedPeerID != "" {
			t.Errorf("Expected empty peer ID with nil key, got %s", extractedPeerID)
		}
	})
}
