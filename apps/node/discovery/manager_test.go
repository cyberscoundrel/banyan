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
