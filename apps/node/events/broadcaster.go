package events

import (
	"sync"
	"time"

	"banyan/interfaces"
	"banyan/types"
)

// Broadcaster implements a centralized event broadcasting system
type Broadcaster struct {
	subscribers []interfaces.WebSocketHub
	mutex       sync.RWMutex
	nodeID      string
}

// NewBroadcaster creates a new event broadcaster
func NewBroadcaster(nodeID string) interfaces.EventBroadcaster {
	return &Broadcaster{
		subscribers: make([]interfaces.WebSocketHub, 0),
		nodeID:      nodeID,
	}
}

// BroadcastEvent sends an event to all registered subscribers
func (b *Broadcaster) BroadcastEvent(event types.Event) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	for _, subscriber := range b.subscribers {
		// Send to each subscriber in a separate goroutine to avoid blocking
		go subscriber.BroadcastEvent(event)
	}
}

// SendEvent creates and broadcasts an event with the specified type and data
func (b *Broadcaster) SendEvent(eventType string, data interface{}) {
	event := types.Event{
		Type:      eventType,
		Timestamp: time.Now(),
		Data:      data,
		NodeID:    b.nodeID,
	}
	b.BroadcastEvent(event)
}

// Subscribe adds a new subscriber to the broadcaster
func (b *Broadcaster) Subscribe(subscriber interfaces.WebSocketHub) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.subscribers = append(b.subscribers, subscriber)
}

// Unsubscribe removes a subscriber from the broadcaster
func (b *Broadcaster) Unsubscribe(subscriber interfaces.WebSocketHub) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	for i, s := range b.subscribers {
		if s == subscriber {
			// Remove subscriber by slicing
			b.subscribers = append(b.subscribers[:i], b.subscribers[i+1:]...)
			break
		}
	}
}

// GetSubscriberCount returns the number of active subscribers
func (b *Broadcaster) GetSubscriberCount() int {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return len(b.subscribers)
}

// SetNodeID updates the node ID for future events
func (b *Broadcaster) SetNodeID(nodeID string) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.nodeID = nodeID
}
