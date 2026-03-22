// Package events provides a real-time event broadcasting system for distributing
// node events to WebSocket clients. It implements a publish-subscribe pattern where
// internal components can broadcast events that are then forwarded to all connected
// WebSocket hubs.
//
// The Broadcaster manages a list of WebSocket hub subscribers and provides thread-safe
// methods for subscribing, unsubscribing, and broadcasting events. Each event includes
// a type, timestamp, data payload, and the originating node's identifier.
package events

import (
	"sync"
	"time"

	"banyan/interfaces"
	"banyan/types"
)

// Broadcaster implements a centralized event broadcasting system that distributes
// events to multiple WebSocket hubs. It provides thread-safe subscriber management
// and non-blocking event delivery.
type Broadcaster struct {
	subscribers []interfaces.WebSocketHub
	mutex       sync.RWMutex
	nodeID      string
}

// NewBroadcaster creates a new event broadcaster with the specified node identifier.
// The nodeID is included in all broadcast events to identify their source.
func NewBroadcaster(nodeID string) interfaces.EventBroadcaster {
	return &Broadcaster{
		subscribers: make([]interfaces.WebSocketHub, 0),
		nodeID:      nodeID,
	}
}

// BroadcastEvent sends the given event to all registered WebSocket hub subscribers.
// Each subscriber receives the event in a separate goroutine to prevent blocking.
func (b *Broadcaster) BroadcastEvent(event types.Event) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	for _, subscriber := range b.subscribers {
		// Send to each subscriber in a separate goroutine to avoid blocking
		go subscriber.BroadcastEvent(event)
	}
}

// SendEvent creates a new event with the specified type and data, then broadcasts
// it to all subscribers. The event is automatically populated with a timestamp
// and the broadcaster's node ID.
func (b *Broadcaster) SendEvent(eventType string, data interface{}) {
	event := types.Event{
		Type:      eventType,
		Timestamp: time.Now(),
		Data:      data,
		NodeID:    b.nodeID,
	}
	b.BroadcastEvent(event)
}

// Subscribe registers a WebSocket hub to receive broadcast events.
func (b *Broadcaster) Subscribe(subscriber interfaces.WebSocketHub) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.subscribers = append(b.subscribers, subscriber)
}

// Unsubscribe removes a WebSocket hub from the subscriber list.
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

// GetSubscriberCount returns the number of currently registered subscribers.
func (b *Broadcaster) GetSubscriberCount() int {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return len(b.subscribers)
}

// SetNodeID updates the node identifier used in future broadcast events.
func (b *Broadcaster) SetNodeID(nodeID string) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.nodeID = nodeID
}
