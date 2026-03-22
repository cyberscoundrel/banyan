package types

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

func TestCollectAllServiceKeys(t *testing.T) {
	tests := []struct {
		name     string
		figFile  FigFile
		expected []string
	}{
		{
			name: "Single root key",
			figFile: FigFile{
				Root: FigNode{
					Path: "/",
					Keys: []string{"root-key-1"},
				},
			},
			expected: []string{"root-key-1"},
		},
		{
			name: "Multiple root keys",
			figFile: FigFile{
				Root: FigNode{
					Path: "/",
					Keys: []string{"root-key-1", "root-key-2"},
				},
			},
			expected: []string{"root-key-1", "root-key-2"},
		},
		{
			name: "Root and child keys",
			figFile: FigFile{
				Root: FigNode{
					Path: "/",
					Keys: []string{"root-key-1"},
					Children: []FigNode{
						{
							Path: "/api",
							Keys: []string{"api-key-1"},
						},
					},
				},
			},
			expected: []string{"root-key-1", "api-key-1"},
		},
		{
			name: "Deeply nested keys",
			figFile: FigFile{
				Root: FigNode{
					Path: "/",
					Keys: []string{"root-key-1"},
					Children: []FigNode{
						{
							Path: "/api",
							Keys: []string{"api-key-1"},
							Children: []FigNode{
								{
									Path: "/api/v1",
									Keys: []string{"v1-key-1"},
									Children: []FigNode{
										{
											Path: "/api/v1/users",
											Keys: []string{"users-key-1"},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: []string{"root-key-1", "api-key-1", "v1-key-1", "users-key-1"},
		},
		{
			name: "Multiple children at same level",
			figFile: FigFile{
				Root: FigNode{
					Path: "/",
					Keys: []string{"root-key-1"},
					Children: []FigNode{
						{
							Path: "/api",
							Keys: []string{"api-key-1"},
						},
						{
							Path: "/admin",
							Keys: []string{"admin-key-1"},
						},
					},
				},
			},
			expected: []string{"root-key-1", "api-key-1", "admin-key-1"},
		},
		{
			name: "Node with no keys but children have keys",
			figFile: FigFile{
				Root: FigNode{
					Path: "/",
					Keys: []string{},
					Children: []FigNode{
						{
							Path: "/api",
							Keys: []string{"api-key-1"},
						},
					},
				},
			},
			expected: []string{"api-key-1"},
		},
		{
			name: "Empty fig tree",
			figFile: FigFile{
				Root: FigNode{
					Path: "",
					Keys: []string{},
				},
			},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.figFile.CollectAllServiceKeys()
			
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d keys, got %d keys", len(tt.expected), len(result))
				t.Errorf("Expected: %v", tt.expected)
				t.Errorf("Got: %v", result)
				return
			}

			// Check that all expected keys are present
			for i, expectedKey := range tt.expected {
				if i >= len(result) || result[i] != expectedKey {
					t.Errorf("Expected key at index %d to be %s, got %s", i, expectedKey, result[i])
				}
			}
		})
	}
}

func TestCollectKeysRecursive(t *testing.T) {
	tests := []struct {
		name     string
		node     FigNode
		expected []string
	}{
		{
			name: "Single node with keys",
			node: FigNode{
				Path: "/",
				Keys: []string{"key-1", "key-2"},
			},
			expected: []string{"key-1", "key-2"},
		},
		{
			name: "Node with children",
			node: FigNode{
				Path: "/",
				Keys: []string{"root-key"},
				Children: []FigNode{
					{
						Path: "/child1",
						Keys: []string{"child1-key"},
					},
					{
						Path: "/child2",
						Keys: []string{"child2-key"},
					},
				},
			},
			expected: []string{"root-key", "child1-key", "child2-key"},
		},
		{
			name: "Node with no keys",
			node: FigNode{
				Path: "/",
				Keys: []string{},
			},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.node.collectKeysRecursive()
			
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d keys, got %d keys", len(tt.expected), len(result))
				return
			}

			for i, expectedKey := range tt.expected {
				if result[i] != expectedKey {
					t.Errorf("Expected key at index %d to be %s, got %s", i, expectedKey, result[i])
				}
			}
		})
	}
}

func TestFindKeysForPath(t *testing.T) {
	tests := []struct {
		name        string
		figFile     FigFile
		requestPath string
		expected    []string
	}{
		{
			name: "Root path returns root keys",
			figFile: FigFile{
				Root: FigNode{
					Path: "/",
					Keys: []string{"root-key"},
				},
			},
			requestPath: "/",
			expected:    []string{"root-key"},
		},
		{
			name: "Exact match returns exact keys",
			figFile: FigFile{
				Root: FigNode{
					Path: "/",
					Keys: []string{"root-key"},
					Children: []FigNode{
						{
							Path: "/api",
							Keys: []string{"api-key"},
						},
					},
				},
			},
			requestPath: "/api",
			expected:    []string{"api-key"},
		},
		{
			name: "Nested path returns most specific keys",
			figFile: FigFile{
				Root: FigNode{
					Path: "/",
					Keys: []string{"root-key"},
					Children: []FigNode{
						{
							Path: "/api",
							Keys: []string{"api-key"},
							Children: []FigNode{
								{
									Path: "/api/v1",
									Keys: []string{"api-v1-key"},
								},
							},
						},
					},
				},
			},
			requestPath: "/api/v1",
			expected:    []string{"api-v1-key"},
		},
		{
			name: "Partial path returns parent keys",
			figFile: FigFile{
				Root: FigNode{
					Path: "/",
					Keys: []string{"root-key"},
					Children: []FigNode{
						{
							Path: "/api",
							Keys: []string{"api-key"},
						},
					},
				},
			},
			requestPath: "/api/users",
			expected:    []string{"api-key"},
		},
		{
			name: "Unknown path returns root keys",
			figFile: FigFile{
				Root: FigNode{
					Path: "/",
					Keys: []string{"root-key"},
				},
			},
			requestPath: "/unknown/path",
			expected:    []string{"root-key"},
		},
		{
			name: "Empty request path returns root keys",
			figFile: FigFile{
				Root: FigNode{
					Path: "/",
					Keys: []string{"root-key"},
				},
			},
			requestPath: "",
			expected:    []string{"root-key"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.figFile.FindKeysForPath(tt.requestPath)

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d keys, got %d keys", len(tt.expected), len(result))
				t.Errorf("Expected: %v", tt.expected)
				t.Errorf("Got: %v", result)
				return
			}

			for i, expectedKey := range tt.expected {
				if result[i] != expectedKey {
					t.Errorf("Expected key at index %d to be %s, got %s", i, expectedKey, result[i])
				}
			}
		})
	}
}

func TestFindKeysAndMatchedPath(t *testing.T) {
	tests := []struct {
		name            string
		figFile         FigFile
		requestPath     string
		expectedKeys    []string
		expectedPath    string
	}{
		{
			name: "Root path returns root keys and path",
			figFile: FigFile{
				Root: FigNode{
					Path: "/",
					Keys: []string{"root-key"},
				},
			},
			requestPath:  "/",
			expectedKeys: []string{"root-key"},
			expectedPath: "/",
		},
		{
			name: "Exact match returns matched path",
			figFile: FigFile{
				Root: FigNode{
					Path: "/",
					Keys: []string{"root-key"},
					Children: []FigNode{
						{
							Path: "/api/v1",
							Keys: []string{"api-v1-key"},
						},
					},
				},
			},
			requestPath:  "/api/v1",
			expectedKeys: []string{"api-v1-key"},
			expectedPath: "/api/v1",
		},
		{
			name: "Partial match returns parent path",
			figFile: FigFile{
				Root: FigNode{
					Path: "/",
					Keys: []string{"root-key"},
					Children: []FigNode{
						{
							Path: "/api",
							Keys: []string{"api-key"},
						},
					},
				},
			},
			requestPath:  "/api/users",
			expectedKeys: []string{"api-key"},
			expectedPath: "/api",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys, matchedPath := tt.figFile.FindKeysAndMatchedPath(tt.requestPath)

			if len(keys) != len(tt.expectedKeys) {
				t.Errorf("Expected %d keys, got %d keys", len(tt.expectedKeys), len(keys))
				return
			}

			for i, expectedKey := range tt.expectedKeys {
				if keys[i] != expectedKey {
					t.Errorf("Expected key at index %d to be %s, got %s", i, expectedKey, keys[i])
				}
			}

			if matchedPath != tt.expectedPath {
				t.Errorf("Expected matched path %s, got %s", tt.expectedPath, matchedPath)
			}
		})
	}
}

func TestBuildCanonicalPayload(t *testing.T) {
	tests := []struct {
		name     string
		figFile  FigFile
		contains []string
	}{
		{
			name: "Contains service alias",
			figFile: FigFile{
				ServiceAlias: "test-service",
				Root: FigNode{
					Path: "/",
					Keys: []string{"key1"},
				},
			},
			contains: []string{"test-service"},
		},
		{
			name: "Contains expiration time",
			figFile: FigFile{
				ServiceAlias: "test-service",
				ExpiresAt:    time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC),
				Root: FigNode{
					Path: "/",
					Keys: []string{"key1"},
				},
			},
			contains: []string{"test-service", "2025-12-31T23:59:59Z"},
		},
		{
			name: "Contains requester peer ID",
			figFile: FigFile{
				ServiceAlias:    "test-service",
				RequesterPeerID: "QmTestPeer123",
				Root: FigNode{
					Path: "/",
					Keys: []string{"key1"},
				},
			},
			contains: []string{"test-service", "QmTestPeer123"},
		},
		{
			name: "Contains nonce",
			figFile: FigFile{
				ServiceAlias: "test-service",
				Nonce:        "abc123",
				Root: FigNode{
					Path: "/",
					Keys: []string{"key1"},
				},
			},
			contains: []string{"test-service", "abc123"},
		},
		{
			name: "Deterministic with same data",
			figFile: FigFile{
				ServiceAlias: "test-service",
				Root: FigNode{
					Path: "/",
					Keys: []string{"key2", "key1"},
				},
			},
			contains: []string{"test-service"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := tt.figFile.BuildCanonicalPayload()
			payloadStr := string(payload)

			for _, expected := range tt.contains {
				if !contains(payloadStr, expected) {
					t.Errorf("Expected payload to contain %s, got %s", expected, payloadStr)
				}
			}
		})
	}

	t.Run("Deterministic payload", func(t *testing.T) {
		fig1 := FigFile{
			ServiceAlias: "test-service",
			Root: FigNode{
				Path: "/",
				Keys: []string{"key2", "key1", "key3"},
			},
		}
		fig2 := FigFile{
			ServiceAlias: "test-service",
			Root: FigNode{
				Path: "/",
				Keys: []string{"key3", "key1", "key2"},
			},
		}

		payload1 := fig1.BuildCanonicalPayload()
		payload2 := fig2.BuildCanonicalPayload()

		if string(payload1) != string(payload2) {
			t.Errorf("Payloads should be deterministic regardless of key order")
			t.Errorf("Payload1: %s", payload1)
			t.Errorf("Payload2: %s", payload2)
		}
	})
}

func TestBuildAuthorityPayload(t *testing.T) {
	tests := []struct {
		name           string
		figFile        FigFile
		shouldExist    []string
		shouldNotExist []string
	}{
		{
			name: "Excludes nonce and requester peer ID",
			figFile: FigFile{
				ServiceAlias:    "test-service",
				RequesterPeerID: "QmTestPeer123",
				Nonce:           "abc123",
				Root: FigNode{
					Path: "/",
					Keys: []string{"key1"},
				},
			},
			shouldExist:    []string{"test-service", "key1"},
			shouldNotExist: []string{"QmTestPeer123", "abc123"},
		},
		{
			name: "Includes required signers",
			figFile: FigFile{
				ServiceAlias:     "test-service",
				RequiredSigners:  []string{"authority1", "authority2"},
				Root: FigNode{
					Path: "/",
					Keys: []string{"key1"},
				},
			},
			shouldExist: []string{"test-service", "authority1", "authority2"},
		},
		{
			name: "Includes expiration",
			figFile: FigFile{
				ServiceAlias: "test-service",
				ExpiresAt:    time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC),
				Root: FigNode{
					Path: "/",
					Keys: []string{"key1"},
				},
			},
			shouldExist: []string{"test-service", "2025-12-31T23:59:59Z"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := tt.figFile.BuildAuthorityPayload()
			payloadStr := string(payload)

			for _, expected := range tt.shouldExist {
				if !contains(payloadStr, expected) {
					t.Errorf("Expected payload to contain %s, got %s", expected, payloadStr)
				}
			}

			for _, notExpected := range tt.shouldNotExist {
				if contains(payloadStr, notExpected) {
					t.Errorf("Expected payload NOT to contain %s, got %s", notExpected, payloadStr)
				}
			}
		})
	}
}

func TestRouteTable(t *testing.T) {
	t.Run("Add and get route", func(t *testing.T) {
		rt := NewRouteTable()
		rt.AddRoute("service1", "http://localhost:8080")

		url, exists := rt.GetRoute("service1")
		if !exists {
			t.Error("Expected route to exist")
		}
		if url != "http://localhost:8080" {
			t.Errorf("Expected URL http://localhost:8080, got %s", url)
		}
	})

	t.Run("Get non-existent route", func(t *testing.T) {
		rt := NewRouteTable()
		_, exists := rt.GetRoute("nonexistent")
		if exists {
			t.Error("Expected route not to exist")
		}
	})

	t.Run("Get all routes", func(t *testing.T) {
		rt := NewRouteTable()
		rt.AddRoute("service1", "http://localhost:8080")
		rt.AddRoute("service2", "http://localhost:8081")

		routes := rt.GetAllRoutes()
		if len(routes) != 2 {
			t.Errorf("Expected 2 routes, got %d", len(routes))
		}
	})

	t.Run("Load routes", func(t *testing.T) {
		rt := NewRouteTable()
		routes := map[string]string{
			"service1": "http://localhost:8080",
			"service2": "http://localhost:8081",
		}
		rt.LoadRoutes(routes)

		if len(rt.GetAllRoutes()) != 2 {
			t.Errorf("Expected 2 routes after loading")
		}
	})

	t.Run("Concurrent access", func(t *testing.T) {
		rt := NewRouteTable()
		var wg sync.WaitGroup

		for i := 0; i < 100; i++ {
			wg.Add(2)

			go func(i int) {
				defer wg.Done()
				rt.AddRoute("service"+string(rune(i)), "http://localhost:8080")
			}(i)

			go func(i int) {
				defer wg.Done()
				rt.GetRoute("service" + string(rune(i)))
			}(i)
		}

		wg.Wait()
	})

	t.Run("Add route with config", func(t *testing.T) {
		rt := NewRouteTable()
		config := ServiceRoutingConfig{
			RoutePrefix:  "/api/v1",
			KeepFullPath: false,
		}
		rt.AddRouteWithConfig("service1", "http://localhost:8080", config)

		url, exists := rt.GetRoute("service1")
		if !exists {
			t.Error("Expected route to exist")
		}
		if url != "http://localhost:8080" {
			t.Errorf("Expected URL http://localhost:8080, got %s", url)
		}

		retrievedConfig, exists := rt.GetRouteConfig("service1")
		if !exists {
			t.Error("Expected route config to exist")
		}
		if retrievedConfig.RoutePrefix != "/api/v1" {
			t.Errorf("Expected route prefix /api/v1, got %s", retrievedConfig.RoutePrefix)
		}
	})
}

func TestFigNodeFindNodeForPath(t *testing.T) {
	tests := []struct {
		name        string
		figFile     FigFile
		requestPath string
		expectPath  string
		expectFound bool
	}{
		{
			name: "Find root node",
			figFile: FigFile{
				Root: FigNode{
					Path: "/",
					Keys: []string{"root-key"},
				},
			},
			requestPath: "/",
			expectPath:  "/",
			expectFound: true,
		},
		{
			name: "Find nested node",
			figFile: FigFile{
				Root: FigNode{
					Path: "/",
					Keys: []string{"root-key"},
					Children: []FigNode{
						{
							Path: "/api",
							Keys: []string{"api-key"},
							Children: []FigNode{
								{
									Path: "/api/v1",
									Keys: []string{"api-v1-key"},
								},
							},
						},
					},
				},
			},
			requestPath: "/api/v1",
			expectPath:  "/api/v1",
			expectFound: true,
		},
		{
			name: "Find parent for unknown child path",
			figFile: FigFile{
				Root: FigNode{
					Path: "/",
					Keys: []string{"root-key"},
					Children: []FigNode{
						{
							Path: "/api",
							Keys: []string{"api-key"},
						},
					},
				},
			},
			requestPath: "/api/users",
			expectPath:  "/api",
			expectFound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, found := tt.figFile.FindNodeForPath(tt.requestPath)

			if found != tt.expectFound {
				t.Errorf("Expected found to be %v, got %v", tt.expectFound, found)
				return
			}

			if tt.expectFound && node.Path != tt.expectPath {
				t.Errorf("Expected path %s, got %s", tt.expectPath, node.Path)
			}
		})
	}
}

func TestFigFileWithAllowedTransports(t *testing.T) {
	fig := FigFile{
		ServiceAlias: "restricted-service",
		Root: FigNode{
			Path: "/",
			Keys: []string{"root-key"},
			Children: []FigNode{
				{
					Path:              "/api",
					Keys:              []string{"api-key"},
					AllowedTransports: []string{"tcp", "ws"},
				},
			},
		},
	}

	node, found := fig.FindNodeForPath("/api")
	if !found {
		t.Error("Expected to find node")
		return
	}

	if len(node.AllowedTransports) != 2 {
		t.Errorf("Expected 2 allowed transports, got %d", len(node.AllowedTransports))
	}
}

func TestFigFileWithData(t *testing.T) {
	data := json.RawMessage(`{"custom": "value"}`)
	fig := FigFile{
		ServiceAlias: "data-service",
		Root: FigNode{
			Path: "/",
			Keys: []string{"key1"},
		},
		Data: data,
	}

	if string(fig.Data) != `{"custom": "value"}` {
		t.Errorf("Expected data to be preserved")
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

