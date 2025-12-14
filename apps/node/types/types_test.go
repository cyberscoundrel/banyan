package types

import (
	"testing"
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

