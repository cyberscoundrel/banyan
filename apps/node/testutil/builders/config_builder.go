package builders

import (
	"encoding/json"
	"os"
)

type TestConfig struct {
	Listen         string   `json:"listen,omitempty"`
	DisableDHT     bool     `json:"disableDht,omitempty"`
	NoCrypto       bool     `json:"noCrypto,omitempty"`
	PrivKeyFile    string   `json:"privKeyFile,omitempty"`
	ServicesDir    string   `json:"servicesDir,omitempty"`
	Bootstraps     []string `json:"bootstraps,omitempty"`
	NoAnnounce     bool     `json:"noAnnounce,omitempty"`
	NATTraversal   bool     `json:"natTraversal,omitempty"`
	ManagementPort int      `json:"managementPort,omitempty"`
}

type ConfigBuilder struct {
	config *TestConfig
}

func NewConfigBuilder() *ConfigBuilder {
	return &ConfigBuilder{
		config: &TestConfig{
			Listen:     "/ip4/127.0.0.1/tcp/0",
			DisableDHT: true,
			NoCrypto:   true,
		},
	}
}

func (b *ConfigBuilder) WithListen(addr string) *ConfigBuilder {
	b.config.Listen = addr
	return b
}

func (b *ConfigBuilder) WithDHT(enabled bool) *ConfigBuilder {
	b.config.DisableDHT = !enabled
	return b
}

func (b *ConfigBuilder) WithCrypto(enabled bool) *ConfigBuilder {
	b.config.NoCrypto = !enabled
	return b
}

func (b *ConfigBuilder) WithPrivKeyFile(path string) *ConfigBuilder {
	b.config.PrivKeyFile = path
	return b
}

func (b *ConfigBuilder) WithServicesDir(dir string) *ConfigBuilder {
	b.config.ServicesDir = dir
	return b
}

func (b *ConfigBuilder) WithBootstraps(bootstraps []string) *ConfigBuilder {
	b.config.Bootstraps = bootstraps
	return b
}

func (b *ConfigBuilder) WithNoAnnounce(noAnnounce bool) *ConfigBuilder {
	b.config.NoAnnounce = noAnnounce
	return b
}

func (b *ConfigBuilder) WithNATTraversal(enabled bool) *ConfigBuilder {
	b.config.NATTraversal = enabled
	return b
}

func (b *ConfigBuilder) WithManagementPort(port int) *ConfigBuilder {
	b.config.ManagementPort = port
	return b
}

func (b *ConfigBuilder) Build() *TestConfig {
	return b.config
}

func (b *ConfigBuilder) BuildJSON() (string, error) {
	data, err := json.MarshalIndent(b.config, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (b *ConfigBuilder) WriteToFile(path string) error {
	data, err := json.MarshalIndent(b.config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (b *ConfigBuilder) Reset() *ConfigBuilder {
	b.config = &TestConfig{
		Listen:     "/ip4/127.0.0.1/tcp/0",
		DisableDHT: true,
		NoCrypto:   true,
	}
	return b
}

func MinimalConfig() *TestConfig {
	return NewConfigBuilder().Build()
}

func ConfigWithDHT() *TestConfig {
	return NewConfigBuilder().
		WithDHT(true).
		WithBootstraps([]string{"/ip4/127.0.0.1/tcp/4001/p2p/QmTestBootstrap"}).
		Build()
}

func ConfigWithCrypto(keyFile string) *TestConfig {
	return NewConfigBuilder().
		WithCrypto(true).
		WithPrivKeyFile(keyFile).
		Build()
}

func ConfigWithServices(servicesDir string) *TestConfig {
	return NewConfigBuilder().
		WithCrypto(true).
		WithServicesDir(servicesDir).
		Build()
}
