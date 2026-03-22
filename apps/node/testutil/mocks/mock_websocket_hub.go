package mocks

import (
	"sync"

	"banyan/types"
)

type MockWebSocketClient struct {
	mu        sync.RWMutex
	id        string
	send      chan types.Event
	conn      interface{}
	closed    bool
}

func NewMockWebSocketClient(id string) *MockWebSocketClient {
	return &MockWebSocketClient{
		id:   id,
		send: make(chan types.Event, 100),
	}
}

func (c *MockWebSocketClient) GetID() string {
	return c.id
}

func (c *MockWebSocketClient) GetSendChannel() chan types.Event {
	return c.send
}

func (c *MockWebSocketClient) GetConnection() interface{} {
	return c.conn
}

func (c *MockWebSocketClient) SetConnection(conn interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.conn = conn
}

func (c *MockWebSocketClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		close(c.send)
		c.closed = true
	}
}

func (c *MockWebSocketClient) IsClosed() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.closed
}

func (c *MockWebSocketClient) GetEvents() []types.Event {
	events := make([]types.Event, 0)
	for {
		select {
		case e := <-c.send:
			events = append(events, e)
		default:
			return events
		}
	}
}

func (c *MockWebSocketClient) WaitForEvent() types.Event {
	return <-c.send
}

type MockWebSocketHub struct {
	mu             sync.RWMutex
	clients        map[string]*MockWebSocketClient
	events         []types.Event
	running        bool
}

func NewMockWebSocketHub() *MockWebSocketHub {
	return &MockWebSocketHub{
		clients: make(map[string]*MockWebSocketClient),
		events:  make([]types.Event, 0),
	}
}

func (h *MockWebSocketHub) BroadcastEvent(event types.Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, event)
	for _, client := range h.clients {
		select {
		case client.send <- event:
		default:
		}
	}
}

func (h *MockWebSocketHub) NewClient(conn interface{}) interface{} {
	h.mu.Lock()
	defer h.mu.Unlock()

	id := generateClientID(len(h.clients) + 1)
	client := NewMockWebSocketClient(id)
	client.SetConnection(conn)
	h.clients[id] = client
	return client
}

func (h *MockWebSocketHub) Register(client interface{}) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if c, ok := client.(*MockWebSocketClient); ok {
		h.clients[c.GetID()] = c
	}
}

func (h *MockWebSocketHub) Unregister(client interface{}) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if c, ok := client.(*MockWebSocketClient); ok {
		delete(h.clients, c.GetID())
		c.Close()
	}
}

func (h *MockWebSocketHub) Run() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.running = true
}

func (h *MockWebSocketHub) GetEvents() []types.Event {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return append([]types.Event{}, h.events...)
}

func (h *MockWebSocketHub) GetClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

func (h *MockWebSocketHub) GetClient(id string) *MockWebSocketClient {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.clients[id]
}

func (h *MockWebSocketHub) IsRunning() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.running
}

func (h *MockWebSocketHub) ClearEvents() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = make([]types.Event, 0)
}

func generateClientID(num int) string {
	return "client-" + string(rune('0'+num%10))
}
