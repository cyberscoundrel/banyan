package discovery

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"

	"banyan/interfaces"
	"banyan/testutil/mocks"
	"banyan/types"
)

// TestNewManager tests the creation of a new discovery Manager.
//
// WHAT IT IS DOING:
//   Creates a libp2p host with a local listener, initializes a GossipSub pubsub system,
//   and constructs a new Manager instance with a mock event broadcaster.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Verifies that the NewManager function returns a non-nil manager instance,
//   confirming successful initialization of all discovery components.
func TestNewManager(t *testing.T) {
	ctx := context.Background()
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("Failed to create host: %v", err)
	}
	defer h.Close()

	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		t.Fatalf("Failed to create pubsub: %v", err)
	}

	mockBroadcaster := &mockEventBroadcaster{}

	manager := NewManager(h, ctx, nil, ps, mockBroadcaster, true, true, false, nil)

	if manager == nil {
		t.Error("Expected non-nil manager")
	}
}

// TestIsDHTEnabled tests the DHT enabled status reporting of the Manager.
//
// WHAT IT IS DOING:
//   Creates a Manager with DHT disabled (noDHT=true) and another with DHT enabled
//   (noDHT=false), then queries each manager's DHT status via IsDHTEnabled.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Asserts that IsDHTEnabled returns false when DHT is disabled and true when enabled,
//   confirming the Manager correctly tracks and reports its DHT configuration.
func TestIsDHTEnabled(t *testing.T) {
	ctx := context.Background()
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("Failed to create host: %v", err)
	}
	defer h.Close()

	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		t.Fatalf("Failed to create pubsub: %v", err)
	}

	t.Run("DHT disabled", func(t *testing.T) {
		mockBroadcaster := &mockEventBroadcaster{}
		manager := NewManager(h, ctx, nil, ps, mockBroadcaster, true, true, false, nil)
		if manager.IsDHTEnabled() {
			t.Error("Expected DHT to be disabled")
		}
	})

	t.Run("DHT enabled", func(t *testing.T) {
		mockBroadcaster := &mockEventBroadcaster{}
		manager := NewManager(h, ctx, nil, ps, mockBroadcaster, false, true, false, nil)
		if !manager.IsDHTEnabled() {
			t.Error("Expected DHT to be enabled")
		}
	})
}

// TestGetDiscoveryMethods tests the retrieval of configured discovery methods.
//
// WHAT IT IS DOING:
//   Creates a Manager with DHT disabled but other discovery methods enabled,
//   then calls GetDiscoveryMethods to retrieve the list of active discovery mechanisms.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Verifies that at least one discovery method is returned, confirming the Manager
//   properly exposes its configured discovery mechanisms (e.g., mDNS, pubsub, etc.).
func TestGetDiscoveryMethods(t *testing.T) {
	ctx := context.Background()
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("Failed to create host: %v", err)
	}
	defer h.Close()

	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		t.Fatalf("Failed to create pubsub: %v", err)
	}

	mockBroadcaster := &mockEventBroadcaster{}
	manager := NewManager(h, ctx, nil, ps, mockBroadcaster, true, true, false, nil)

	methods := manager.GetDiscoveryMethods()
	if len(methods) == 0 {
		t.Error("Expected at least one discovery method")
	}
}

// TestHandleLookupRequest tests the handling of incoming peer lookup requests.
//
// WHAT IT IS DOING:
//   Creates a Manager with mock dependencies, constructs a LookupRequest with test
//   peer ID and public key data, then invokes HandleLookupRequest with a mock
//   crypto manager and peer manager.
//
// HOW IT DEMONSTRATES IT WORKS:
//   The test completes without panicking or erroring, demonstrating that
//   HandleLookupRequest can process incoming lookup requests and interact
//   with the crypto and peer management subsystems.
func TestHandleLookupRequest(t *testing.T) {
	ctx := context.Background()
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("Failed to create host: %v", err)
	}
	defer h.Close()

	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		t.Fatalf("Failed to create pubsub: %v", err)
	}

	mockBroadcaster := &mockEventBroadcaster{}
	mockPeerManager := mocks.NewMockPeerManager()
	mockCryptoManager := mocks.NewMockCryptoManager()

	manager := NewManager(h, ctx, nil, ps, mockBroadcaster, true, true, false, nil)

	req := &types.LookupRequest{
		Type:      "lookup",
		From:      "QmTestPeer",
		PublicKey: []byte("test-pubkey"),
		Timestamp: time.Now(),
	}

	fromPeer, _ := peer.Decode("QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN")

	manager.HandleLookupRequest(req, fromPeer, mockCryptoManager, mockPeerManager)
}

// TestProcessLookupResponse tests the processing of received lookup responses.
//
// WHAT IT IS DOING:
//   Creates a Manager with a mock peer manager, constructs a LookupResponse
//   containing peer addresses and public key information, then calls
//   ProcessLookupResponse to handle the response data.
//
// HOW IT DEMONSTRATES IT WORKS:
//   The test completes successfully, confirming that ProcessLookupResponse
//   can parse and process lookup response data including peer addresses
//   and public keys without errors.
func TestProcessLookupResponse(t *testing.T) {
	ctx := context.Background()
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("Failed to create host: %v", err)
	}
	defer h.Close()

	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		t.Fatalf("Failed to create pubsub: %v", err)
	}

	mockBroadcaster := &mockEventBroadcaster{}
	mockPeerManager := mocks.NewMockPeerManager()

	manager := NewManager(h, ctx, nil, ps, mockBroadcaster, true, true, false, nil)

	resp := &types.LookupResponse{
		Type:      "response",
		From:      "QmTestPeer",
		Addresses: []string{"/ip4/127.0.0.1/tcp/1234/p2p/QmTestPeer"},
		PublicKey: []byte("test-pubkey"),
		Timestamp: time.Now(),
	}

	manager.ProcessLookupResponse(resp, mockPeerManager)
}

// TestVerifyResponseSignature tests the signature verification for lookup responses.
//
// WHAT IT IS DOING:
//   Creates a Manager and tests VerifyResponseSignature with a LookupResponse
//   that has an empty signature slice, which should fail verification.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Asserts that VerifyResponseSignature returns false for responses with
//   empty signatures, confirming the method properly validates that
//   signatures must be present and non-empty to pass verification.
func TestVerifyResponseSignature(t *testing.T) {
	ctx := context.Background()
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("Failed to create host: %v", err)
	}
	defer h.Close()

	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		t.Fatalf("Failed to create pubsub: %v", err)
	}

	mockBroadcaster := &mockEventBroadcaster{}

	manager := NewManager(h, ctx, nil, ps, mockBroadcaster, true, true, false, nil)

	t.Run("Empty signature returns false", func(t *testing.T) {
		resp := &types.LookupResponse{
			Type:      "response",
			From:      "QmTestPeer",
			Signature: []byte{},
			Timestamp: time.Now(),
		}
		if manager.VerifyResponseSignature(resp) {
			t.Error("Expected false for empty signature")
		}
	})
}

type mockEventBroadcaster struct{}

func (m *mockEventBroadcaster) BroadcastEvent(event types.Event)          {}
func (m *mockEventBroadcaster) SendEvent(eventType string, data interface{}) {}
func (m *mockEventBroadcaster) Subscribe(subscriber interfaces.WebSocketHub)     {}
func (m *mockEventBroadcaster) Unsubscribe(subscriber interfaces.WebSocketHub)   {}
func (m *mockEventBroadcaster) GetSubscriberCount() int              { return 0 }
