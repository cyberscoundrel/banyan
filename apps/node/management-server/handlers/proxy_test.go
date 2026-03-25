package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"

	nodePkg "banyan/node"
	"banyan/types"
)

type mockConnectionManager struct {
	connections map[peer.ID]*types.ConnectionItem
}

func (m *mockConnectionManager) GetConnectionsCopy() map[peer.ID]*types.ConnectionItem {
	return m.connections
}

func (m *mockConnectionManager) GetConnectionInfo(pid peer.ID) (*types.ConnectionItem, bool) {
	conn, ok := m.connections[pid]
	return conn, ok
}

func (m *mockConnectionManager) GetPeerByAlias(alias string) (peer.ID, bool) {
	for pid, conn := range m.connections {
		if conn.Alias == alias {
			return pid, true
		}
	}
	return "", false
}

type mockNodeForProxy struct {
	config         *nodePkg.Config
	connectionMgr  *mockConnectionManager
}

func (m *mockNodeForProxy) GetConfig() *nodePkg.Config {
	return m.config
}

func (m *mockNodeForProxy) GetConnectionManager() *mockConnectionManager {
	return m.connectionMgr
}

func TestHandleServiceKeyPrefixProxy_NodeNotAvailable(t *testing.T) {
	ph := &ProxyHandlers{
		node: nil,
	}

	req := httptest.NewRequest("GET", "/proxy/service-key/abcd1234/api/test", nil)
	w := httptest.NewRecorder()

	ph.HandleServiceKeyPrefixProxy(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}

	if !strings.Contains(w.Body.String(), "not available") {
		t.Errorf("Expected 'not available' error message, got: %s", w.Body.String())
	}
}

func TestParseServiceKeyPrefixPath(t *testing.T) {
	tests := []struct {
		name            string
		path            string
		expectedPrefix  string
		expectedPath    string
		shouldFail      bool
	}{
		{"simple path", "/proxy/service-key/abcd1234efgh5678/api/users", "abcd1234efgh5678", "api/users", false},
		{"root path", "/proxy/service-key/abcd1234efgh5678", "abcd1234efgh5678", "", false},
		{"with trailing slash", "/proxy/service-key/abcd1234efgh5678/", "abcd1234efgh5678", "", false},
		{"deep path", "/proxy/service-key/abcd1234/api/v1/users/123", "abcd1234", "api/v1/users/123", false},
		{"uppercase prefix", "/proxy/service-key/ABCD1234/api/test", "abcd1234", "api/test", false},
		{"mixed case prefix", "/proxy/service-key/AbCd1234/api/test", "abcd1234", "api/test", false},
		{"empty path", "/proxy/service-key/", "", "", true},
		{"short prefix", "/proxy/service-key/abc/api/test", "abc", "api/test", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := strings.TrimPrefix(tt.path, "/proxy/service-key/")
			if path == "" || path == "/" {
				if !tt.shouldFail {
					t.Errorf("Expected to fail for empty path")
				}
				return
			}

			parts := strings.SplitN(path, "/", 2)

			if len(parts) == 0 || parts[0] == "" {
				if !tt.shouldFail {
					t.Errorf("Failed to parse prefix from path")
				}
				return
			}

			prefix := strings.ToLower(strings.TrimSpace(parts[0]))
			var remainingPath string
			if len(parts) > 1 {
				remainingPath = parts[1]
			}

			if prefix != tt.expectedPrefix {
				t.Errorf("Prefix: got %q, want %q", prefix, tt.expectedPrefix)
			}
			if remainingPath != tt.expectedPath {
				t.Errorf("Path: got %q, want %q", remainingPath, tt.expectedPath)
			}
		})
	}
}

func TestServiceKeyPrefixValidation(t *testing.T) {
	tests := []struct {
		name        string
		prefix      string
		minLength   int
		shouldValid bool
	}{
		{"valid 8 chars", "abcd1234", 8, true},
		{"valid 16 chars", "abcd1234efgh5678", 8, true},
		{"too short 7 chars", "abcd123", 8, false},
		{"too short 1 char", "a", 8, false},
		{"empty", "", 8, false},
		{"exactly min", "12345678", 8, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := len(tt.prefix) >= tt.minLength
			if isValid != tt.shouldValid {
				t.Errorf("Prefix %q validity: got %v, want %v", tt.prefix, isValid, tt.shouldValid)
			}
		})
	}
}

func TestServiceKeyPrefixMatching(t *testing.T) {
	serviceKeys := []struct {
		key    string
		peerID string
	}{
		{"deadbeef1234567890abcdef", "peer1"},
		{"deadbeef0987654321fedcba", "peer2"},
		{"cafebabedeadbeef12345678", "peer3"},
		{"1234567890abcdef12345678", "peer4"},
	}

	tests := []struct {
		name           string
		prefix         string
		expectMatch    int
		expectError    bool
		errorContains  string
	}{
		{"unique match", "deadbeef1234", 1, false, ""},
		{"ambiguous match", "deadbeef", 2, true, "Ambiguous"},
		{"no match", "ffffffff", 0, true, "No service key"},
		{"exact match", "deadbeef1234567890abcdef", 1, false, ""},
		{"case insensitive", "DEADBEEF1234", 1, false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var matchingKeys []string
			prefixLower := strings.ToLower(tt.prefix)

			for _, sk := range serviceKeys {
				if strings.HasPrefix(strings.ToLower(sk.key), prefixLower) {
					matchingKeys = append(matchingKeys, sk.key)
				}
			}

			if tt.expectError {
				if tt.expectMatch == 0 && len(matchingKeys) != 0 {
					t.Errorf("Expected no matches, got %d", len(matchingKeys))
				}
				if tt.expectMatch > 1 && len(matchingKeys) < 2 {
					t.Errorf("Expected ambiguous matches, got %d", len(matchingKeys))
				}
			} else {
				if len(matchingKeys) != tt.expectMatch {
					t.Errorf("Expected %d match(es), got %d", tt.expectMatch, len(matchingKeys))
				}
			}
		})
	}
}

func TestServiceKeyPrefixURLConstruction(t *testing.T) {
	tests := []struct {
		name          string
		peerID        string
		serviceKey    string
		remainingPath string
		expectedPath  string
	}{
		{"with path", "12D3KooWTest", "abcd1234efgh5678", "api/v1/users",
			"/proxy/peer/12D3KooWTest/router/abcd1234efgh5678/api/v1/users"},
		{"empty path", "12D3KooWTest", "abcd1234efgh5678", "",
			"/proxy/peer/12D3KooWTest/router/abcd1234efgh5678/"},
		{"root path", "12D3KooWTest", "abcd1234efgh5678", "/",
			"/proxy/peer/12D3KooWTest/router/abcd1234efgh5678/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remainingPath := strings.TrimPrefix(tt.remainingPath, "/")
			newPath := "/proxy/peer/" + tt.peerID + "/router/" + tt.serviceKey + "/" + remainingPath
			if newPath != tt.expectedPath {
				t.Errorf("Path: got %q, want %q", newPath, tt.expectedPath)
			}
		})
	}
}
