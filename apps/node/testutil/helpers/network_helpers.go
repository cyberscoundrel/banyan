package helpers

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/net/swarm"
	"github.com/multiformats/go-multiaddr"
)

func GetFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()

	return l.Addr().(*net.TCPAddr).Port, nil
}

func CreateTestHost(opts ...libp2p.Option) (host.Host, error) {
	defaultOpts := []libp2p.Option{
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.NoSecurity,
		libp2p.DefaultTransports,
		libp2p.DefaultMuxers,
	}

	allOpts := append(defaultOpts, opts...)
	return libp2p.New(allOpts...)
}

func CreateTestHostWithKey(privKey interface{}) (host.Host, error) {
	return libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.NoSecurity,
		libp2p.DefaultTransports,
		libp2p.DefaultMuxers,
	)
}

func CreateTestHosts(count int) ([]host.Host, error) {
	hosts := make([]host.Host, count)
	for i := 0; i < count; i++ {
		h, err := CreateTestHost()
		if err != nil {
			for j := 0; j < i; j++ {
				hosts[j].Close()
			}
			return nil, fmt.Errorf("failed to create host %d: %w", i, err)
		}
		hosts[i] = h
	}
	return hosts, nil
}

func ConnectHosts(ctx context.Context, h1, h2 host.Host) error {
	h2Info := peer.AddrInfo{
		ID:    h2.ID(),
		Addrs: h2.Addrs(),
	}
	return h1.Connect(ctx, h2Info)
}

func ConnectAllHosts(ctx context.Context, hosts []host.Host) error {
	for i := 0; i < len(hosts); i++ {
		for j := i + 1; j < len(hosts); j++ {
			if err := ConnectHosts(ctx, hosts[i], hosts[j]); err != nil {
				return fmt.Errorf("failed to connect host %d to host %d: %w", i, j, err)
			}
		}
	}
	return nil
}

func CloseHosts(hosts []host.Host) {
	for _, h := range hosts {
		if h != nil {
			h.Close()
		}
	}
}

func MultiaddrFromHost(h host.Host) (multiaddr.Multiaddr, error) {
	addrs := h.Addrs()
	if len(addrs) == 0 {
		return nil, fmt.Errorf("host has no addresses")
	}

	addr := addrs[0]
	peerID := h.ID()

	ma, err := multiaddr.NewMultiaddr(fmt.Sprintf("%s/p2p/%s", addr.String(), peerID.String()))
	if err != nil {
		return nil, fmt.Errorf("failed to create multiaddr: %w", err)
	}

	return ma, nil
}

func IsConnected(h1, h2 host.Host) bool {
	return h1.Network().Connectedness(h2.ID()) == network.Connected
}

func GetHostAddresses(h host.Host) []string {
	addrs := h.Addrs()
	result := make([]string, len(addrs))
	for i, addr := range addrs {
		result[i] = fmt.Sprintf("%s/p2p/%s", addr.String(), h.ID().String())
	}
	return result
}

func WaitForConnection(ctx context.Context, h1, h2 host.Host, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for connection")
		case <-ticker.C:
			if IsConnected(h1, h2) {
				return nil
			}
		}
	}
}

func ClearBackends(h host.Host) {
	swrm, ok := h.Network().(*swarm.Swarm)
	if ok {
		swrm.Backoff().Clear(h.ID())
	}
}
