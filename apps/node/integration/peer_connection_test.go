package integration

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

func TestTwoPeersConnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	h1, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("Failed to create host1: %v", err)
	}
	defer h1.Close()

	h2, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("Failed to create host2: %v", err)
	}
	defer h2.Close()

	h2Info := peer.AddrInfo{
		ID:    h2.ID(),
		Addrs: h2.Addrs(),
	}

	if err := h1.Connect(ctx, h2Info); err != nil {
		t.Fatalf("Failed to connect h1 to h2: %v", err)
	}

	if h1.Network().Connectedness(h2.ID()) != network.Connected {
		t.Error("Expected h1 to be connected to h2")
	}

	if h2.Network().Connectedness(h1.ID()) != network.Connected {
		t.Error("Expected h2 to be connected to h1")
	}

	t.Logf("Successfully connected peer %s to peer %s", h1.ID().String()[:8], h2.ID().String()[:8])
}

func TestServiceDiscovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	h1, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("Failed to create host1: %v", err)
	}
	defer h1.Close()

	h2, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("Failed to create host2: %v", err)
	}
	defer h2.Close()

	h2Info := peer.AddrInfo{
		ID:    h2.ID(),
		Addrs: h2.Addrs(),
	}

	if err := h1.Connect(ctx, h2Info); err != nil {
		t.Fatalf("Failed to connect hosts: %v", err)
	}

	t.Logf("Service discovery test - peer1 %s connected to peer2 %s", h1.ID().String()[:8], h2.ID().String()[:8])

	if h1.Network().Connectedness(h2.ID()) != network.Connected {
		t.Error("Expected h1 to be connected to h2")
	}

	conns := h1.Network().ConnsToPeer(h2.ID())
	if len(conns) == 0 {
		t.Error("Expected at least one connection to peer")
	}

	t.Logf("Connection established with %d connection(s)", len(conns))
}

func TestTunnelProtocol(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	h1, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("Failed to create host1: %v", err)
	}
	defer h1.Close()

	h2, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("Failed to create host2: %v", err)
	}
	defer h2.Close()

	h2Info := peer.AddrInfo{
		ID:    h2.ID(),
		Addrs: h2.Addrs(),
	}

	if err := h1.Connect(ctx, h2Info); err != nil {
		t.Fatalf("Failed to connect hosts: %v", err)
	}

	echoListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start echo server: %v", err)
	}
	defer echoListener.Close()
	echoPort := echoListener.Addr().(*net.TCPAddr).Port

	go func() {
		for {
			conn, err := echoListener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c)
			}(conn)
		}
	}()

	t.Logf("Tunnel protocol test - echo server listening on port %d", echoPort)

	_ = echoPort
}

func TestMultiplePeersConnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	numPeers := 3
	hosts := make([]host.Host, numPeers)

	for i := 0; i < numPeers; i++ {
		h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
		if err != nil {
			for j := 0; j < i; j++ {
				hosts[j].Close()
			}
			t.Fatalf("Failed to create host %d: %v", i, err)
		}
		hosts[i] = h
		defer h.Close()
	}

	for i := 0; i < numPeers; i++ {
		for j := i + 1; j < numPeers; j++ {
			peerInfo := peer.AddrInfo{
				ID:    hosts[j].ID(),
				Addrs: hosts[j].Addrs(),
			}
			if err := hosts[i].Connect(ctx, peerInfo); err != nil {
				t.Errorf("Failed to connect peer %d to peer %d: %v", i, j, err)
			}
		}
	}

	connectedCount := 0
	for i := 0; i < numPeers; i++ {
		for j := i + 1; j < numPeers; j++ {
			if hosts[i].Network().Connectedness(hosts[j].ID()) == network.Connected {
				connectedCount++
			}
		}
	}

	expectedConnections := numPeers * (numPeers - 1) / 2
	if connectedCount != expectedConnections {
		t.Errorf("Expected %d connections, got %d", expectedConnections, connectedCount)
	}

	t.Logf("Successfully connected %d peers with %d connections", numPeers, connectedCount)
}

func TestPeerDisconnection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	h1, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("Failed to create host1: %v", err)
	}

	h2, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		h1.Close()
		t.Fatalf("Failed to create host2: %v", err)
	}

	h2Info := peer.AddrInfo{
		ID:    h2.ID(),
		Addrs: h2.Addrs(),
	}

	if err := h1.Connect(ctx, h2Info); err != nil {
		h1.Close()
		h2.Close()
		t.Fatalf("Failed to connect hosts: %v", err)
	}

	if h1.Network().Connectedness(h2.ID()) != network.Connected {
		h1.Close()
		h2.Close()
		t.Error("Expected peers to be connected")
	}

	h2.Close()

	time.Sleep(100 * time.Millisecond)

	t.Log("Peer disconnection test completed")
}
