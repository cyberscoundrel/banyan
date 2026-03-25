package builders

import (
	"context"

	"github.com/libp2p/go-libp2p/core/crypto"

	nodePkg "banyan/node"
)

type NodeBuilder struct {
	ctx        context.Context
	config     *nodePkg.Config
	keyLoader  nodePkg.PrivateKeyLoader
	privKey    crypto.PrivKey
}

func NewNodeBuilder(ctx context.Context) *NodeBuilder {
	disableDHT := true
	noCrypto := true
	noAnnounce := true

	return &NodeBuilder{
		ctx: ctx,
		config: &nodePkg.Config{
			DisableDHT:      &disableDHT,
			NoCrypto:        &noCrypto,
			NoAnnounce:      &noAnnounce,
			ListenMultiaddr: strPtr("/ip4/127.0.0.1/tcp/0"),
		},
		keyLoader: defaultKeyLoader,
	}
}

func (b *NodeBuilder) WithDHT(enabled bool) *NodeBuilder {
	b.config.DisableDHT = boolPtr(!enabled)
	return b
}

func (b *NodeBuilder) WithCrypto(enabled bool) *NodeBuilder {
	b.config.NoCrypto = boolPtr(!enabled)
	return b
}

func (b *NodeBuilder) WithAnnounce(enabled bool) *NodeBuilder {
	b.config.NoAnnounce = boolPtr(!enabled)
	return b
}

func (b *NodeBuilder) WithNATTraversal(enabled bool) *NodeBuilder {
	b.config.NATTraversal = &enabled
	return b
}

func (b *NodeBuilder) WithListenAddr(addr string) *NodeBuilder {
	b.config.ListenMultiaddr = &addr
	return b
}

func (b *NodeBuilder) WithPrivKeyFile(path string) *NodeBuilder {
	b.config.PrivKeyFile = &path
	return b
}

func (b *NodeBuilder) WithServicesDir(dir string) *NodeBuilder {
	b.config.ServicesDir = &dir
	return b
}

func (b *NodeBuilder) WithFigsDir(dir string) *NodeBuilder {
	b.config.FigsDir = &dir
	return b
}

func (b *NodeBuilder) WithBootstraps(bootstraps []string) *NodeBuilder {
	b.config.Bootstraps = bootstraps
	return b
}

func (b *NodeBuilder) WithKeyLoader(loader nodePkg.PrivateKeyLoader) *NodeBuilder {
	b.keyLoader = loader
	return b
}

func (b *NodeBuilder) WithPrivateKey(key crypto.PrivKey) *NodeBuilder {
	b.privKey = key
	return b
}

func (b *NodeBuilder) WithTunnel(enabled bool) *NodeBuilder {
	b.config.TunnelEnabled = &enabled
	return b
}

func (b *NodeBuilder) WithTunnelAllowedTargets(targets []string) *NodeBuilder {
	b.config.TunnelAllowedTargets = targets
	return b
}

func (b *NodeBuilder) WithRouterConfig(path string) *NodeBuilder {
	b.config.RouterConfig = &path
	return b
}

func (b *NodeBuilder) WithAllowUnsafeServiceKeyInjection(enabled bool) *NodeBuilder {
	b.config.AllowUnsafeServiceKeyInjection = &enabled
	return b
}

func (b *NodeBuilder) WithBeaconIncludePeerIDAnnouncements(enabled bool) *NodeBuilder {
	b.config.BeaconIncludePeerIDAnnouncements = &enabled
	return b
}

func (b *NodeBuilder) Build() (*nodePkg.Node, error) {
	return nodePkg.NewNode(b.ctx, b.config, b.keyLoader)
}

func (b *NodeBuilder) BuildAndStart() (*nodePkg.Node, error) {
	node, err := nodePkg.NewNode(b.ctx, b.config, b.keyLoader)
	if err != nil {
		return nil, err
	}
	if err := node.Start(); err != nil {
		node.Close()
		return nil, err
	}
	return node, nil
}

func (b *NodeBuilder) GetConfig() *nodePkg.Config {
	return b.config
}

func (b *NodeBuilder) Reset() *NodeBuilder {
	disableDHT := true
	noCrypto := true
	noAnnounce := true

	b.config = &nodePkg.Config{
		DisableDHT:      &disableDHT,
		NoCrypto:        &noCrypto,
		NoAnnounce:      &noAnnounce,
		ListenMultiaddr: strPtr("/ip4/127.0.0.1/tcp/0"),
	}
	b.keyLoader = defaultKeyLoader
	b.privKey = nil
	return b
}

func defaultKeyLoader(filename string) (crypto.PrivKey, error) {
	return nil, nil
}

func strPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}

func CreateTestNode(ctx context.Context) (*nodePkg.Node, error) {
	return NewNodeBuilder(ctx).BuildAndStart()
}

func CreateTestNodeWithDHT(ctx context.Context) (*nodePkg.Node, error) {
	return NewNodeBuilder(ctx).
		WithDHT(true).
		WithAnnounce(true).
		BuildAndStart()
}

func CreateTestNodes(ctx context.Context, count int) ([]*nodePkg.Node, error) {
	nodes := make([]*nodePkg.Node, count)
	for i := 0; i < count; i++ {
		node, err := CreateTestNode(ctx)
		if err != nil {
			for j := 0; j < i; j++ {
				nodes[j].Close()
			}
			return nil, err
		}
		nodes[i] = node
	}
	return nodes, nil
}

func CloseNodes(nodes []*nodePkg.Node) {
	for _, n := range nodes {
		if n != nil {
			n.Close()
		}
	}
}
