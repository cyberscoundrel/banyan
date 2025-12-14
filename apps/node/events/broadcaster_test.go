package events

import (
	"sync"
	"testing"
	"time"

	"banyan/interfaces"
	"banyan/types"
)

func TestEventBroadcaster(t *testing.T) {
	nodeID := "test-node-123"
	broadcaster := NewBroadcaster(nodeID)

	// Test sending event without subscribers
	broadcaster.SendEvent(types.EventInfo, map[string]interface{}{
		"message": "test event",
	})

	// Create mock subscribers
	subscriber1 := &MockWebSocketHub{events: make([]types.Event, 0)}
	subscriber2 := &MockWebSocketHub{events: make([]types.Event, 0)}

	// Subscribe both
	broadcaster.Subscribe(subscriber1)
	broadcaster.Subscribe(subscriber2)

	// Verify subscriber count
	if count := broadcaster.GetSubscriberCount(); count != 2 {
		t.Errorf("Expected 2 subscribers, got %d", count)
	}

	// Send an event
	testEvent := types.Event{
		Type:      types.EventConnection,
		Timestamp: time.Now(),
		Data:      map[string]interface{}{"peer_id": "test-peer"},
		NodeID:    nodeID,
	}

	broadcaster.BroadcastEvent(testEvent)

	// Give some time for async processing
	time.Sleep(10 * time.Millisecond)

	// Verify both subscribers received the event
	if len(subscriber1.events) != 1 {
		t.Errorf("Subscriber1 expected 1 event, got %d", len(subscriber1.events))
	}
	if len(subscriber2.events) != 1 {
		t.Errorf("Subscriber2 expected 1 event, got %d", len(subscriber2.events))
	}

	// Verify event content
	if subscriber1.events[0].Type != types.EventConnection {
		t.Errorf("Expected event type %s, got %s", types.EventConnection, subscriber1.events[0].Type)
	}

	// Test unsubscribing
	broadcaster.Unsubscribe(subscriber1)
	if count := broadcaster.GetSubscriberCount(); count != 1 {
		t.Errorf("Expected 1 subscriber after unsubscribe, got %d", count)
	}

	// Send another event
	broadcaster.SendEvent(types.EventError, map[string]interface{}{
		"message": "test error",
	})

	time.Sleep(10 * time.Millisecond)

	// subscriber1 should still have 1 event, subscriber2 should have 2
	if len(subscriber1.events) != 1 {
		t.Errorf("Subscriber1 expected 1 event after unsubscribe, got %d", len(subscriber1.events))
	}
	if len(subscriber2.events) != 2 {
		t.Errorf("Subscriber2 expected 2 events, got %d", len(subscriber2.events))
	}

	t.Log("EventBroadcaster test passed")
}

func TestEventBroadcasterConcurrency(t *testing.T) {
	broadcaster := NewBroadcaster("test-node")
	subscriber := &MockWebSocketHub{events: make([]types.Event, 0)}
	broadcaster.Subscribe(subscriber)

	// Test concurrent event sending
	var wg sync.WaitGroup
	eventCount := 100

	for i := 0; i < eventCount; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			broadcaster.SendEvent(types.EventInfo, map[string]interface{}{
				"message": "concurrent test",
				"index":   i,
			})
		}(i)
	}

	wg.Wait()
	time.Sleep(50 * time.Millisecond) // Give time for async processing

	if len(subscriber.events) != eventCount {
		t.Errorf("Expected %d events, got %d", eventCount, len(subscriber.events))
	}

	t.Log("EventBroadcaster concurrency test passed")
}

// MockWebSocketHub implements interfaces.WebSocketHub for testing
type MockWebSocketHub struct {
	events []types.Event
	mutex  sync.Mutex
}

func (m *MockWebSocketHub) BroadcastEvent(event types.Event) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.events = append(m.events, event)
}

func (m *MockWebSocketHub) NewClient(conn interface{}) interfaces.WebSocketClient {
	return &MockWebSocketClient{}
}

func (m *MockWebSocketHub) Register(client interfaces.WebSocketClient) {
	// Mock implementation
}

func (m *MockWebSocketHub) Unregister(client interfaces.WebSocketClient) {
	// Mock implementation
}

func (m *MockWebSocketHub) Run() {
	// Mock implementation
}

// MockWebSocketClient implements interfaces.WebSocketClient for testing
type MockWebSocketClient struct{}

func (m *MockWebSocketClient) GetID() string {
	return "mock-client"
}

func (m *MockWebSocketClient) GetSendChannel() chan types.Event {
	return make(chan types.Event, 1)
}

func (m *MockWebSocketClient) GetConnection() interface{} {
	return nil
}
