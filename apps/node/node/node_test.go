package node_test

import (
	"context"
	"testing"
	"time"

	"banyan/interfaces"
	"banyan/testutil/builders"
)

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

func TestNodeMultipleClose(t *testing.T) {
	ctx := context.Background()

	node, err := builders.NewNodeBuilder(ctx).Build()
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	node.Close()
	node.Close()
}
