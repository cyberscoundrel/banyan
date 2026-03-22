package builders

import (
	"context"
	"fmt"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	libp2ptls "github.com/libp2p/go-libp2p/p2p/security/tls"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	"github.com/libp2p/go-libp2p/p2p/transport/websocket"
	"github.com/multiformats/go-multiaddr"
)

type HostBuilder struct {
	ctx         context.Context
	privKey     crypto.PrivKey
	listenAddrs []string
	transports  []libp2p.Option
	security    []libp2p.Option
	muxers      []libp2p.Option
	options     []libp2p.Option
}

func NewHostBuilder(ctx context.Context) *HostBuilder {
	return &HostBuilder{
		ctx:         ctx,
		listenAddrs: []string{"/ip4/127.0.0.1/tcp/0"},
		transports:  []libp2p.Option{libp2p.DefaultTransports},
		muxers:      []libp2p.Option{libp2p.DefaultMuxers},
		options:     []libp2p.Option{},
	}
}

func (b *HostBuilder) WithPrivateKey(key crypto.PrivKey) *HostBuilder {
	b.privKey = key
	return b
}

func (b *HostBuilder) WithListenAddr(addr string) *HostBuilder {
	b.listenAddrs = []string{addr}
	return b
}

func (b *HostBuilder) WithListenAddrs(addrs []string) *HostBuilder {
	b.listenAddrs = addrs
	return b
}

func (b *HostBuilder) WithRandomPort() *HostBuilder {
	b.listenAddrs = []string{"/ip4/127.0.0.1/tcp/0"}
	return b
}

func (b *HostBuilder) WithPort(port int) *HostBuilder {
	b.listenAddrs = []string{fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", port)}
	return b
}

func (b *HostBuilder) WithAllInterfaces() *HostBuilder {
	b.listenAddrs = []string{"/ip4/0.0.0.0/tcp/0"}
	return b
}

func (b *HostBuilder) NoSecurity() *HostBuilder {
	b.security = []libp2p.Option{libp2p.NoSecurity}
	return b
}

func (b *HostBuilder) WithNoiseSecurity() *HostBuilder {
	b.security = []libp2p.Option{libp2p.Security(noise.ID, noise.New)}
	return b
}

func (b *HostBuilder) WithTLSSecurity() *HostBuilder {
	b.security = []libp2p.Option{libp2p.Security(libp2ptls.ID, libp2ptls.New)}
	return b
}

func (b *HostBuilder) WithTCPTransport() *HostBuilder {
	b.transports = []libp2p.Option{libp2p.Transport(tcp.NewTCPTransport)}
	return b
}

func (b *HostBuilder) WithWebSocketTransport() *HostBuilder {
	b.transports = []libp2p.Option{libp2p.Transport(websocket.New)}
	return b
}

func (b *HostBuilder) WithAllTransports() *HostBuilder {
	b.transports = []libp2p.Option{libp2p.DefaultTransports}
	return b
}

func (b *HostBuilder) WithOption(opt libp2p.Option) *HostBuilder {
	b.options = append(b.options, opt)
	return b
}

func (b *HostBuilder) WithPeerstore(peers map[peer.ID]peer.AddrInfo) *HostBuilder {
	b.options = append(b.options, libp2p.Peerstore(nil))
	return b
}

func (b *HostBuilder) Build() (host.Host, error) {
	var opts []libp2p.Option

	if b.privKey != nil {
		opts = append(opts, libp2p.Identity(b.privKey))
	}

	if len(b.listenAddrs) > 0 {
		opts = append(opts, libp2p.ListenAddrStrings(b.listenAddrs...))
	}

	if len(b.transports) > 0 {
		opts = append(opts, b.transports...)
	}

	if len(b.security) > 0 {
		opts = append(opts, b.security...)
	}

	if len(b.muxers) > 0 {
		opts = append(opts, b.muxers...)
	}

	opts = append(opts, b.options...)

	return libp2p.New(opts...)
}

func (b *HostBuilder) Reset() *HostBuilder {
	b.privKey = nil
	b.listenAddrs = []string{"/ip4/127.0.0.1/tcp/0"}
	b.transports = []libp2p.Option{libp2p.DefaultTransports}
	b.security = []libp2p.Option{}
	b.muxers = []libp2p.Option{libp2p.DefaultMuxers}
	b.options = []libp2p.Option{}
	return b
}

func CreateTestHost(ctx context.Context) (host.Host, error) {
	return NewHostBuilder(ctx).
		NoSecurity().
		WithRandomPort().
		Build()
}

func CreateTestHostWithKey(ctx context.Context, key crypto.PrivKey) (host.Host, error) {
	return NewHostBuilder(ctx).
		WithPrivateKey(key).
		NoSecurity().
		WithRandomPort().
		Build()
}

func CreateSecureTestHost(ctx context.Context) (host.Host, error) {
	return NewHostBuilder(ctx).
		WithNoiseSecurity().
		WithRandomPort().
		Build()
}

func CreateTestHosts(ctx context.Context, count int) ([]host.Host, error) {
	hosts := make([]host.Host, count)
	for i := 0; i < count; i++ {
		h, err := CreateTestHost(ctx)
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

func GetMultiaddr(h host.Host) (multiaddr.Multiaddr, error) {
	addrs := h.Addrs()
	if len(addrs) == 0 {
		return nil, fmt.Errorf("host has no addresses")
	}

	addr := addrs[0]
	ma, err := multiaddr.NewMultiaddr(fmt.Sprintf("%s/p2p/%s", addr.String(), h.ID().String()))
	if err != nil {
		return nil, fmt.Errorf("failed to create multiaddr: %w", err)
	}

	return ma, nil
}

func GetMultiaddrs(h host.Host) ([]multiaddr.Multiaddr, error) {
	addrs := h.Addrs()
	if len(addrs) == 0 {
		return nil, fmt.Errorf("host has no addresses")
	}

	result := make([]multiaddr.Multiaddr, len(addrs))
	for i, addr := range addrs {
		ma, err := multiaddr.NewMultiaddr(fmt.Sprintf("%s/p2p/%s", addr.String(), h.ID().String()))
		if err != nil {
			return nil, fmt.Errorf("failed to create multiaddr: %w", err)
		}
		result[i] = ma
	}

	return result, nil
}
