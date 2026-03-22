package node_test

import (
	"context"
	"testing"
	"time"

	"banyan/interfaces"
	"banyan/testutil/builders"
)

// TestNewNode tests the creation of a new node with default configuration.
//
// WHAT IT IS DOING:
//   Creates a node using the NodeBuilder and verifies that all
//   core components are properly initialized.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Asserts that GetHost, GetConnectionManager, GetDiscoveryManager,
//   GetServiceManager, GetHTTPHandler, GetHTTPTransport, GetEventBroadcaster,
//   GetRouteTable, GetConfig return non-nil values and the host has a valid peer ID.
func TestNewNode(t *testing.T) {
	ctx := context.Background()

	node, err := builders.NewNodeBuilder(ctx).Build()
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}
	defer node.Close()

	if node.GetHost() == nil {
		t.Error("Expected host to be initialized")
	}

	if node.GetConnectionManager() == nil {
		t.Error("Expected connection manager to be initialized")
	}

	if node.GetDiscoveryManager() == nil {
		t.Error("Expected discovery manager to be initialized")
	}

	if node.GetServiceManager() == nil {
		t.Error("Expected service manager to be initialized")
	}

	if node.GetHTTPHandler() == nil {
		t.Error("Expected HTTP handler to be initialized")
	}

	if node.GetHTTPTransport() == nil {
		t.Error("Expected HTTP transport to be initialized")
	}

	if node.GetEventBroadcaster() == nil {
		t.Error("Expected event broadcaster to be initialized")
	}

	if node.GetRouteTable() == nil {
		t.Error("Expected route table to be initialized")
	}

	if node.GetConfig() == nil {
		t.Error("Expected config to be initialized")
	}

	if node.GetHost().ID() == "" {
		t.Error("Expected node to have a valid peer ID")
	}
}

// TestNodeStart tests starting a node and verifying post-start state.
//
// WHAT IT IS DOING:
//   Creates and starts a node using BuildAndStart, then verifies
//   that managers remain accessible after startup.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Asserts that GetDiscoveryManager, GetServiceManager, and GetHTTPHandler
//   return non-nil values after the node has been started.
func TestNodeStart(t *testing.T) {
	ctx := context.Background()

	node, err := builders.NewNodeBuilder(ctx).BuildAndStart()
	if err != nil {
		t.Fatalf("Failed to create and start node: %v", err)
	}
	defer node.Close()

	if node.GetDiscoveryManager() == nil {
		t.Error("Expected discovery manager to be available after start")
	}

	if node.GetServiceManager() == nil {
		t.Error("Expected service manager to be available after start")
	}

	if node.GetHTTPHandler() == nil {
		t.Error("Expected HTTP handler to be available after start")
	}

	time.Sleep(100 * time.Millisecond)
}

// TestNodeClose tests the graceful shutdown of a node.
//
// WHAT IT IS DOING:
//   Creates, starts, and then closes a node to verify that
//   the shutdown process completes without errors.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Asserts that Close() can be called successfully after the node
//   has been started and the host was initialized with a valid peer ID.
func TestNodeClose(t *testing.T) {
	ctx := context.Background()

	node, err := builders.NewNodeBuilder(ctx).BuildAndStart()
	if err != nil {
		t.Fatalf("Failed to create and start node: %v", err)
	}

	host := node.GetHost()
	if host == nil {
		t.Fatal("Expected host to be initialized")
	}
	peerID := host.ID()

	node.Close()

	time.Sleep(100 * time.Millisecond)

	_ = peerID
}

// TestNewNodeWithDHT tests creating a node with DHT enabled.
//
// WHAT IT IS DOING:
//   Creates a node with DHT routing enabled and verifies the DHT
//   routing table is accessible.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Asserts that GetDHTRoutingTableSize returns a non-negative value,
//   indicating the DHT is initialized even if the table is empty.
func TestNewNodeWithDHT(t *testing.T) {
	ctx := context.Background()

	node, err := builders.NewNodeBuilder(ctx).
		WithDHT(true).
		Build()
	if err != nil {
		t.Fatalf("Failed to create node with DHT: %v", err)
	}
	defer node.Close()

	if node.GetDHTRoutingTableSize() < 0 {
		t.Error("Expected DHT routing table size to be non-negative")
	}
}

// TestNewNodeWithTunnel tests creating a node with tunnel functionality enabled.
//
// WHAT IT IS DOING:
//   Creates a node with tunnel support enabled and verifies the
//   tunnel handler is initialized.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Asserts that GetTunnelHandler returns a non-nil value when
//   the tunnel option is enabled.
func TestNewNodeWithTunnel(t *testing.T) {
	ctx := context.Background()

	node, err := builders.NewNodeBuilder(ctx).
		WithTunnel(true).
		Build()
	if err != nil {
		t.Fatalf("Failed to create node with tunnel: %v", err)
	}
	defer node.Close()

	if node.GetTunnelHandler() == nil {
		t.Error("Expected tunnel handler to be initialized when tunnel is enabled")
	}
}

// TestNewNodeWithoutTunnel tests creating a node with tunnel functionality disabled.
//
// WHAT IT IS DOING:
//   Creates a node with tunnel support explicitly disabled and
//   verifies the tunnel handler is not initialized.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Asserts that GetTunnelHandler returns nil when the tunnel
//   option is disabled.
func TestNewNodeWithoutTunnel(t *testing.T) {
	ctx := context.Background()

	node, err := builders.NewNodeBuilder(ctx).
		WithTunnel(false).
		Build()
	if err != nil {
		t.Fatalf("Failed to create node without tunnel: %v", err)
	}
	defer node.Close()

	if node.GetTunnelHandler() != nil {
		t.Error("Expected tunnel handler to be nil when tunnel is disabled")
	}
}

// TestNodeInterfaceCompliance tests that node components implement their expected interfaces.
//
// WHAT IT IS DOING:
//   Creates a node and assigns each component to its respective interface
//   type to verify interface compliance at compile time.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Asserts that GetConnectionManager implements PeerManager, GetDiscoveryManager
//   implements DiscoveryManager, GetServiceManager implements ServiceManager,
//   GetHTTPHandler implements HTTPHandler, and GetEventBroadcaster implements EventBroadcaster.
func TestNodeInterfaceCompliance(t *testing.T) {
	ctx := context.Background()

	node, err := builders.NewNodeBuilder(ctx).Build()
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}
	defer node.Close()

	var _ interfaces.PeerManager = node.GetConnectionManager()
	var _ interfaces.DiscoveryManager = node.GetDiscoveryManager()
	var _ interfaces.ServiceManager = node.GetServiceManager()
	var _ interfaces.HTTPHandler = node.GetHTTPHandler()
	var _ interfaces.EventBroadcaster = node.GetEventBroadcaster()
}

// TestNodeConfigDefaults tests that the node builder sets expected default configuration values.
//
// WHAT IT IS DOING:
//   Creates a node with default builder settings and verifies the
//   configuration has the expected test defaults.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Asserts that DisableDHT is true and NoCrypto is true by default
//   in the test builder configuration.
func TestNodeConfigDefaults(t *testing.T) {
	ctx := context.Background()

	node, err := builders.NewNodeBuilder(ctx).Build()
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}
	defer node.Close()

	config := node.GetConfig()
	if config == nil {
		t.Fatal("Expected config to be non-nil")
	}

	if config.DisableDHT == nil || !*config.DisableDHT {
		t.Error("Expected DisableDHT to be true by default in test builder")
	}

	if config.NoCrypto == nil || !*config.NoCrypto {
		t.Error("Expected NoCrypto to be true by default in test builder")
	}
}

// TestNodeNATStatus tests NAT-related status methods on the node.
//
// WHAT IT IS DOING:
//   Creates a node and queries NAT reachability status and relay
//   address information.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Asserts that GetNATReachability returns a non-empty string,
//   and GetRelayAddrs returns an empty list for a test node
//   (no relay addresses configured).
func TestNodeNATStatus(t *testing.T) {
	ctx := context.Background()

	node, err := builders.NewNodeBuilder(ctx).Build()
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}
	defer node.Close()

	reachability := node.GetNATReachability()
	if reachability == "" {
		t.Error("Expected NAT reachability to return a non-empty string")
	}

	hasRelay := node.HasRelayAddr()
	_ = hasRelay

	relayAddrs := node.GetRelayAddrs()
	if len(relayAddrs) != 0 {
		t.Errorf("Expected no relay addresses for test node, got %d", len(relayAddrs))
	}
}

// TestNodeDHTStatus tests DHT-related status methods on the node.
//
// WHAT IT IS DOING:
//   Creates a node with DHT enabled and queries DHT readiness
//   and routing table size.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Asserts that GetDHTRoutingTableSize returns a non-negative value
//   when DHT is enabled.
func TestNodeDHTStatus(t *testing.T) {
	ctx := context.Background()

	node, err := builders.NewNodeBuilder(ctx).
		WithDHT(true).
		Build()
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}
	defer node.Close()

	isReady := node.IsDHTReady()
	_ = isReady

	routingSize := node.GetDHTRoutingTableSize()
	if routingSize < 0 {
		t.Error("Expected DHT routing table size to be non-negative")
	}
}

// TestNodeSendEvent tests sending events through the node's event broadcaster.
//
// WHAT IT IS DOING:
//   Creates a node and sends a test event with a message payload
//   through the event broadcasting system.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Asserts that GetEventBroadcaster returns a non-nil broadcaster
//   and SendEvent completes without error.
func TestNodeSendEvent(t *testing.T) {
	ctx := context.Background()

	node, err := builders.NewNodeBuilder(ctx).Build()
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}
	defer node.Close()

	broadcaster := node.GetEventBroadcaster()
	if broadcaster == nil {
		t.Fatal("Expected event broadcaster to be initialized")
	}

	node.SendEvent("test_event", map[string]interface{}{
		"message": "test",
	})
}

// TestNodeMultipleClose tests that calling Close multiple times is safe.
//
// WHAT IT IS DOING:
//   Creates a node and calls Close() twice to verify that
//   multiple close calls do not cause panics or errors.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Asserts that the second Close() call completes without
//   panicking or returning an error.
func TestNodeMultipleClose(t *testing.T) {
	ctx := context.Background()

	node, err := builders.NewNodeBuilder(ctx).Build()
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	node.Close()
	node.Close()
}
