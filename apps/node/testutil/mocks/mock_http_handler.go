package mocks

import (
	"net/http"
	"sync"

	"banyan/types"
)

type MockHTTPHandler struct {
	mu                    sync.RWMutex
	pingCalled            bool
	greetingsCalled       bool
	serviceFigsCalled     bool
	statusCalled          bool
	connectionsCalled     bool
	proxyCalled           bool
	routerCalled          bool
	started               bool
	mountedPaths          map[string]func(http.ResponseWriter, *http.Request)
	addonDisclosureProv   func() []types.AddonDisclosure
	responseData          interface{}
	responseStatus        int
}

func NewMockHTTPHandler() *MockHTTPHandler {
	return &MockHTTPHandler{
		mountedPaths:   make(map[string]func(http.ResponseWriter, *http.Request)),
		responseStatus: http.StatusOK,
	}
}

func (m *MockHTTPHandler) StartP2PProtocolServer() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = true
}

func (m *MockHTTPHandler) HandlePing(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pingCalled = true
	w.WriteHeader(m.responseStatus)
	if m.responseData != nil {
		w.Write([]byte("pong"))
	}
}

func (m *MockHTTPHandler) HandleGreetings(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.greetingsCalled = true
	w.WriteHeader(m.responseStatus)
}

func (m *MockHTTPHandler) HandleServiceFigs(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.serviceFigsCalled = true
	w.WriteHeader(m.responseStatus)
}

func (m *MockHTTPHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statusCalled = true
	w.WriteHeader(m.responseStatus)
}

func (m *MockHTTPHandler) HandleConnectionsStatus(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connectionsCalled = true
	w.WriteHeader(m.responseStatus)
}

func (m *MockHTTPHandler) HandleLibp2pHTTPProxy(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.proxyCalled = true
	w.WriteHeader(m.responseStatus)
}

func (m *MockHTTPHandler) HandleRouter(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.routerCalled = true
	w.WriteHeader(m.responseStatus)
}

func (m *MockHTTPHandler) MountP2P(path string, h func(http.ResponseWriter, *http.Request)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mountedPaths[path] = h
}

func (m *MockHTTPHandler) SetAddonDisclosureProvider(provider func() []types.AddonDisclosure) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.addonDisclosureProv = provider
}

func (m *MockHTTPHandler) WasPingCalled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.pingCalled
}

func (m *MockHTTPHandler) WasGreetingsCalled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.greetingsCalled
}

func (m *MockHTTPHandler) WasServiceFigsCalled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.serviceFigsCalled
}

func (m *MockHTTPHandler) WasStatusCalled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.statusCalled
}

func (m *MockHTTPHandler) WasConnectionsCalled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connectionsCalled
}

func (m *MockHTTPHandler) WasProxyCalled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.proxyCalled
}

func (m *MockHTTPHandler) WasRouterCalled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.routerCalled
}

func (m *MockHTTPHandler) IsStarted() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.started
}

func (m *MockHTTPHandler) GetMountedPaths() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	paths := make([]string, 0, len(m.mountedPaths))
	for p := range m.mountedPaths {
		paths = append(paths, p)
	}
	return paths
}

func (m *MockHTTPHandler) SetResponseStatus(status int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responseStatus = status
}

func (m *MockHTTPHandler) SetResponseData(data interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responseData = data
}

func (m *MockHTTPHandler) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pingCalled = false
	m.greetingsCalled = false
	m.serviceFigsCalled = false
	m.statusCalled = false
	m.connectionsCalled = false
	m.proxyCalled = false
	m.routerCalled = false
}
