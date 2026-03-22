package connection

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"

	"banyan/interfaces"
	"banyan/types"
)

type mockEventBroadcaster struct {
	events []struct {
		eventType string
		data      interface{}
	}
}

func (m *mockEventBroadcaster) BroadcastEvent(event types.Event) {}

func (m *mockEventBroadcaster) SendEvent(eventType string, data interface{}) {
	m.events = append(m.events, struct {
		eventType string
		data      interface{}
	}{eventType: eventType, data: data})
}

func (m *mockEventBroadcaster) Subscribe(subscriber interfaces.WebSocketHub)   {}
func (m *mockEventBroadcaster) Unsubscribe(subscriber interfaces.WebSocketHub) {}
func (m *mockEventBroadcaster) GetSubscriberCount() int                        { return 0 }

func setupTestManager(t *testing.T) (*Manager, context.Context) {
	t.Helper()
	ctx := context.Background()
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("Failed to create host: %v", err)
	}

	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		t.Fatalf("Failed to create pubsub: %v", err)
	}

	topic, err := ps.Join("test-topic")
	if err != nil {
		t.Fatalf("Failed to join topic: %v", err)
	}

	mockBroadcaster := &mockEventBroadcaster{events: make([]struct {
		eventType string
		data      interface{}
	}, 0)}

	manager := NewManager(h, ctx, nil, nil, topic, mockBroadcaster).(*Manager)
	return manager, ctx
}

func generateTestPeerID(t *testing.T) peer.ID {
	t.Helper()
	h, err := libp2p.New()
	if err != nil {
		t.Fatalf("Failed to create host for peer ID: %v", err)
	}
	defer h.Close()
	return h.ID()
}

func TestAddTrackedPeer(t *testing.T) {
	manager, _ := setupTestManager(t)
	defer manager.host.Close()

	peerID := generateTestPeerID(t)

	t.Run("adds new peer with correct properties", func(t *testing.T) {
		options := types.NewManualPeerOptions(true)
		connItem := manager.AddTrackedPeer(peerID, options)

		if connItem == nil {
			t.Fatal("Expected non-nil connection item")
		}
		if connItem.PeerID != peerID {
			t.Errorf("Expected peer ID %s, got %s", peerID, connItem.PeerID)
		}
		if connItem.ConnectionType != types.ConnTypeManual {
			t.Errorf("Expected connection type %s, got %s", types.ConnTypeManual, connItem.ConnectionType)
		}
		if !connItem.HTTPCapable {
			t.Error("Expected HTTPCapable to be true")
		}
		if connItem.Status != types.StatusConnected {
			t.Errorf("Expected status %s, got %s", types.StatusConnected, connItem.Status)
		}
		if len(connItem.Alias) != 4 {
			t.Errorf("Expected alias length 4, got %d", len(connItem.Alias))
		}
	})

	t.Run("adds peer with service key", func(t *testing.T) {
		peerID2 := generateTestPeerID(t)
		serviceKey := []byte("test-service-key-12345")
		options := types.NewServicePeerOptions(serviceKey, true)
		connItem := manager.AddTrackedPeer(peerID2, options)

		if len(connItem.ServiceKeys) != 1 {
			t.Errorf("Expected 1 service key, got %d", len(connItem.ServiceKeys))
		}
		if string(connItem.ServiceKeys[0]) != string(serviceKey) {
			t.Errorf("Service key mismatch")
		}
	})

	t.Run("updates existing peer with higher priority connection type", func(t *testing.T) {
		peerID3 := generateTestPeerID(t)
		options := types.NewBackgroundPeerOptions()
		manager.AddTrackedPeer(peerID3, options)

		connItem, _ := manager.GetConnectionInfo(peerID3)
		if connItem.ConnectionType != types.ConnTypeBackground {
			t.Errorf("Expected background connection type, got %s", connItem.ConnectionType)
		}

		options2 := types.NewManualPeerOptions(true)
		manager.AddTrackedPeer(peerID3, options2)

		connItem, _ = manager.GetConnectionInfo(peerID3)
		if connItem.ConnectionType != types.ConnTypeManual {
			t.Errorf("Expected manual connection type after upgrade, got %s", connItem.ConnectionType)
		}
		if !connItem.HTTPCapable {
			t.Error("Expected HTTPCapable to be true after upgrade")
		}
	})

	t.Run("appends new service key to existing peer", func(t *testing.T) {
		peerID4 := generateTestPeerID(t)
		key1 := []byte("service-key-1")
		key2 := []byte("service-key-2")

		manager.AddTrackedPeer(peerID4, types.NewServicePeerOptions(key1, false))
		manager.AddTrackedPeer(peerID4, types.NewServicePeerOptions(key2, false))

		connItem, _ := manager.GetConnectionInfo(peerID4)
		if len(connItem.ServiceKeys) != 2 {
			t.Errorf("Expected 2 service keys, got %d", len(connItem.ServiceKeys))
		}
	})

	t.Run("does not duplicate service keys", func(t *testing.T) {
		peerID5 := generateTestPeerID(t)
		key := []byte("duplicate-key-test")

		manager.AddTrackedPeer(peerID5, types.NewServicePeerOptions(key, false))
		manager.AddTrackedPeer(peerID5, types.NewServicePeerOptions(key, false))

		connItem, _ := manager.GetConnectionInfo(peerID5)
		if len(connItem.ServiceKeys) != 1 {
			t.Errorf("Expected 1 service key (no duplicates), got %d", len(connItem.ServiceKeys))
		}
	})

	t.Run("defaults invalid connection type to background", func(t *testing.T) {
		peerID6 := generateTestPeerID(t)
		options := types.PeerOptions{
			ConnectionType: "invalid-type",
			HTTPCapable:    false,
		}
		connItem := manager.AddTrackedPeer(peerID6, options)

		if connItem.ConnectionType != types.ConnTypeBackground {
			t.Errorf("Expected connection type to default to %s, got %s", types.ConnTypeBackground, connItem.ConnectionType)
		}
	})

	t.Run("resets disconnect status on reconnect", func(t *testing.T) {
		peerID7 := generateTestPeerID(t)
		manager.AddTrackedPeer(peerID7, types.NewManualPeerOptions(true))
		manager.UpdateConnectionStatus(peerID7, types.StatusDisconnected)

		connItem, _ := manager.GetConnectionInfo(peerID7)
		if connItem.LastDisconnect == nil {
			t.Error("Expected LastDisconnect to be set")
		}

		manager.AddTrackedPeer(peerID7, types.NewManualPeerOptions(true))
		connItem, _ = manager.GetConnectionInfo(peerID7)
		if connItem.LastDisconnect != nil {
			t.Error("Expected LastDisconnect to be nil after re-add")
		}
		if connItem.Status != types.StatusConnected {
			t.Errorf("Expected status %s, got %s", types.StatusConnected, connItem.Status)
		}
	})
}

func TestMarkPeerHTTPCapable(t *testing.T) {
	manager, _ := setupTestManager(t)
	defer manager.host.Close()

	peerID := generateTestPeerID(t)
	manager.AddTrackedPeer(peerID, types.NewManualPeerOptions(false))

	t.Run("marks peer as HTTP capable", func(t *testing.T) {
		manager.MarkPeerHTTPCapable(peerID, true)

		connItem, exists := manager.GetConnectionInfo(peerID)
		if !exists {
			t.Fatal("Expected peer to exist")
		}
		if !connItem.HTTPCapable {
			t.Error("Expected HTTPCapable to be true")
		}
		if !connItem.BidirectionalHTTP {
			t.Error("Expected BidirectionalHTTP to be true")
		}
		if connItem.HTTPTestResult != types.HTTPTestSuccess {
			t.Errorf("Expected HTTPTestResult %s, got %s", types.HTTPTestSuccess, connItem.HTTPTestResult)
		}
		if connItem.LastHTTPTest == nil {
			t.Error("Expected LastHTTPTest to be set")
		}
	})

	t.Run("sends HTTP test event", func(t *testing.T) {
		broadcaster := manager.eventBroadcaster.(*mockEventBroadcaster)
		found := false
		for _, e := range broadcaster.events {
			if e.eventType == types.EventHTTPTest {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected HTTP test event to be sent")
		}
	})

	t.Run("does nothing for unknown peer", func(t *testing.T) {
		unknownPeerID := generateTestPeerID(t)
		manager.MarkPeerHTTPCapable(unknownPeerID, true)

		_, exists := manager.GetConnectionInfo(unknownPeerID)
		if exists {
			t.Error("Expected unknown peer to not exist")
		}
	})
}

func TestGetPeerByAlias(t *testing.T) {
	manager, _ := setupTestManager(t)
	defer manager.host.Close()

	t.Run("returns peer ID for valid alias", func(t *testing.T) {
		peerID := generateTestPeerID(t)
		connItem := manager.AddTrackedPeer(peerID, types.NewManualPeerOptions(true))

		resolvedPeerID, exists := manager.GetPeerByAlias(connItem.Alias)
		if !exists {
			t.Error("Expected alias to exist")
		}
		if resolvedPeerID != peerID {
			t.Errorf("Expected peer ID %s, got %s", peerID, resolvedPeerID)
		}
	})

	t.Run("returns false for invalid alias", func(t *testing.T) {
		_, exists := manager.GetPeerByAlias("zzzz")
		if exists {
			t.Error("Expected alias to not exist")
		}
	})

	t.Run("alias is 4 characters", func(t *testing.T) {
		peerID := generateTestPeerID(t)
		connItem := manager.AddTrackedPeer(peerID, types.NewManualPeerOptions(true))

		if len(connItem.Alias) != 4 {
			t.Errorf("Expected alias length 4, got %d", len(connItem.Alias))
		}
	})

	t.Run("generates unique aliases for different peers", func(t *testing.T) {
		aliases := make(map[string]bool)
		for i := 0; i < 10; i++ {
			peerID := generateTestPeerID(t)
			connItem := manager.AddTrackedPeer(peerID, types.NewManualPeerOptions(true))

			if aliases[connItem.Alias] {
				t.Errorf("Duplicate alias generated: %s", connItem.Alias)
			}
			aliases[connItem.Alias] = true
		}
	})
}

func TestCleanupOldConnections(t *testing.T) {
	manager, _ := setupTestManager(t)
	defer manager.host.Close()

	t.Run("removes old disconnected peers", func(t *testing.T) {
		peerID := generateTestPeerID(t)
		manager.AddTrackedPeer(peerID, types.NewManualPeerOptions(true))
		manager.UpdateConnectionStatus(peerID, types.StatusDisconnected)

		connItem, _ := manager.GetConnectionInfo(peerID)
		oldTime := time.Now().Add(-35 * time.Minute)
		connItem.LastDisconnect = &oldTime

		manager.CleanupOldConnections()

		_, exists := manager.GetConnectionInfo(peerID)
		if exists {
			t.Error("Expected old disconnected peer to be removed")
		}
	})

	t.Run("keeps recently disconnected peers", func(t *testing.T) {
		peerID := generateTestPeerID(t)
		manager.AddTrackedPeer(peerID, types.NewManualPeerOptions(true))
		manager.UpdateConnectionStatus(peerID, types.StatusDisconnected)

		connItem, _ := manager.GetConnectionInfo(peerID)
		recentTime := time.Now().Add(-5 * time.Minute)
		connItem.LastDisconnect = &recentTime

		manager.CleanupOldConnections()

		_, exists := manager.GetConnectionInfo(peerID)
		if !exists {
			t.Error("Expected recently disconnected peer to be kept")
		}
	})

	t.Run("keeps connected peers", func(t *testing.T) {
		peerID := generateTestPeerID(t)
		manager.AddTrackedPeer(peerID, types.NewManualPeerOptions(true))

		connItem, _ := manager.GetConnectionInfo(peerID)
		oldTime := time.Now().Add(-35 * time.Minute)
		connItem.LastDisconnect = &oldTime

		manager.CleanupOldConnections()

		_, exists := manager.GetConnectionInfo(peerID)
		if !exists {
			t.Error("Expected connected peer to be kept")
		}
	})

	t.Run("removes alias mapping when cleaning up", func(t *testing.T) {
		peerID := generateTestPeerID(t)
		connItem := manager.AddTrackedPeer(peerID, types.NewManualPeerOptions(true))
		alias := connItem.Alias

		manager.UpdateConnectionStatus(peerID, types.StatusDisconnected)
		connItem.LastDisconnect = ptrTime(time.Now().Add(-35 * time.Minute))

		manager.CleanupOldConnections()

		_, exists := manager.GetPeerByAlias(alias)
		if exists {
			t.Error("Expected alias to be removed")
		}
	})
}

func TestHandleNewConnection(t *testing.T) {
	manager, _ := setupTestManager(t)
	defer manager.host.Close()

	t.Run("updates status for tracked peer", func(t *testing.T) {
		peerID := generateTestPeerID(t)
		manager.AddTrackedPeer(peerID, types.NewManualPeerOptions(true))
		manager.UpdateConnectionStatus(peerID, types.StatusDisconnected)

		manager.HandleNewConnection(peerID)

		connItem, _ := manager.GetConnectionInfo(peerID)
		if connItem.Status != types.StatusConnected {
			t.Errorf("Expected status %s, got %s", types.StatusConnected, connItem.Status)
		}
	})

	t.Run("sends peer connected event", func(t *testing.T) {
		broadcaster := manager.eventBroadcaster.(*mockEventBroadcaster)
		broadcaster.events = nil

		peerID := generateTestPeerID(t)
		manager.AddTrackedPeer(peerID, types.NewManualPeerOptions(true))
		manager.HandleNewConnection(peerID)

		found := false
		for _, e := range broadcaster.events {
			if e.eventType == types.EventPeerConnected {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected peer connected event")
		}
	})

	t.Run("does not track untracked peers", func(t *testing.T) {
		peerID := generateTestPeerID(t)
		manager.HandleNewConnection(peerID)

		_, exists := manager.GetConnectionInfo(peerID)
		if exists {
			t.Error("Expected untracked peer to not be added")
		}
	})

	t.Run("sends info event for reconnected tracked peer", func(t *testing.T) {
		peerID := generateTestPeerID(t)
		manager.AddTrackedPeer(peerID, types.NewManualPeerOptions(true))
		manager.UpdateConnectionStatus(peerID, types.StatusDisconnected)

		broadcaster := manager.eventBroadcaster.(*mockEventBroadcaster)
		broadcaster.events = nil

		manager.HandleNewConnection(peerID)

		found := false
		for _, e := range broadcaster.events {
			if e.eventType == types.EventInfo {
				data, ok := e.data.(map[string]interface{})
				if ok && data["message"] == "Tracked peer reconnected" {
					found = true
					break
				}
			}
		}
		if !found {
			t.Error("Expected info event for reconnected tracked peer")
		}
	})
}

func TestHandleDisconnection(t *testing.T) {
	manager, _ := setupTestManager(t)
	defer manager.host.Close()

	t.Run("updates status to disconnected", func(t *testing.T) {
		peerID := generateTestPeerID(t)
		manager.AddTrackedPeer(peerID, types.NewManualPeerOptions(true))

		manager.HandleDisconnection(peerID)

		connItem, _ := manager.GetConnectionInfo(peerID)
		if connItem.Status != types.StatusDisconnected {
			t.Errorf("Expected status %s, got %s", types.StatusDisconnected, connItem.Status)
		}
		if connItem.LastDisconnect == nil {
			t.Error("Expected LastDisconnect to be set")
		}
	})

	t.Run("sends peer disconnected event", func(t *testing.T) {
		peerID := generateTestPeerID(t)
		manager.AddTrackedPeer(peerID, types.NewManualPeerOptions(true))

		broadcaster := manager.eventBroadcaster.(*mockEventBroadcaster)
		broadcaster.events = nil

		manager.HandleDisconnection(peerID)

		found := false
		for _, e := range broadcaster.events {
			if e.eventType == types.EventPeerDisconnected {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected peer disconnected event")
		}
	})

	t.Run("sets last disconnect time", func(t *testing.T) {
		peerID := generateTestPeerID(t)
		manager.AddTrackedPeer(peerID, types.NewManualPeerOptions(true))

		beforeDisconnect := time.Now()
		manager.HandleDisconnection(peerID)

		connItem, _ := manager.GetConnectionInfo(peerID)
		if connItem.LastDisconnect == nil {
			t.Fatal("Expected LastDisconnect to be set")
		}
		if connItem.LastDisconnect.Before(beforeDisconnect) {
			t.Error("LastDisconnect time should be after disconnect call")
		}
	})
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
