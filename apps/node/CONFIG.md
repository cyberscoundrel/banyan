# Banyan Node Configuration Guide

## Overview

Banyan nodes can be configured using either command-line flags or a JSON configuration file. Command-line flags always take precedence over configuration file values, allowing you to use a base configuration file and override specific settings as needed.

## Configuration File

### Basic Usage

To use a configuration file, specify it with the `-config` flag:

```bash
./banyan -config=./node-config.json
```

### Configuration File Format

The configuration file is a JSON file with the following optional fields:

```json
{
  "privkey": "./keys/node-identity.pem",
  "servicesDir": "./services",
  "figsDir": "./figs",
  "listen": "/ip4/0.0.0.0/tcp/9000",
  "disableDht": false,
  "noAnnounce": false,
  "anonLookups": false,
  "noCrypto": false,
  "natTraversal": true,
  "routerConfig": "./router-config.json",
  "addonsDir": "./addons",
  "beaconIncludePeerIdAnnouncements": true,
  "beaconPeerIdDirectOnly": false,
  "allowExpiredFigs": false,
  "allowInsecureFigs": false,
  "bootstraps": [
    "/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN",
    "/dnsaddr/bootstrap.libp2p.io/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
    "/ip4/104.131.131.82/tcp/4001/p2p/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ"
  ]
}
```

### Configuration Fields

#### Network Configuration

- **`listen`** (string): The multiaddr to listen on for incoming connections
  - Default: `/ip4/0.0.0.0/tcp/0` (random port)
  - Example: `/ip4/0.0.0.0/tcp/9000`

- **`natTraversal`** (boolean): Enable NAT traversal features (hole punching, auto-relay, UPnP)
  - Default: `false`
  - Set to `true` for nodes behind NAT/firewalls

- **`bootstraps`** (array of strings OR string): Custom bootstrap peer multiaddrs
  - Default: Uses libp2p default bootstrap peers
  - Can be specified in two ways:
    1. **Inline array**: `"bootstraps": ["/ip4/...", "/ip4/..."]`
    2. **File path**: `"bootstraps": "./bootstraps.json"` (file must contain a JSON array)
  - Use this to specify custom bootstrap nodes for private networks
  - Each entry must be a valid multiaddr with peer ID
  - Example inline: `"/ip4/192.168.1.100/tcp/4001/p2p/QmYourPeerID"`
  - Example file: `"./bootstraps.json"` containing `["/ip4/...", ...]`

#### Identity and Keys

- **`privkey`** (string): Path to PEM file containing the node's private key
  - Used for peer identity and encryption
  - If not specified, a new key is generated on each startup

#### Service Configuration

- **`servicesDir`** (string): Directory containing services.json and per-service subfolders
  - Default: `./services` (relative to executable)

- **`figsDir`** (string): Directory containing fig files
  - Default: `./figs` (relative to executable)

- **`addonsDir`** (string): Directory containing addons.json and addon executables
  - Default: Uses built-in default location

- **`routerConfig`** (string): Path to JSON file containing router configuration
  - Maps service identifiers to URLs

#### Discovery and Announcement

- **`disableDht`** (boolean): Disable DHT peer discovery
  - Default: `false`
  - Set to `true` for local testing with mDNS only

- **`noAnnounce`** (boolean): Do not announce this node's presence
  - Default: `false`
  - Set to `true` for passive/hidden nodes

- **`beaconIncludePeerIdAnnouncements`** (boolean): Include beacon's PeerID in public announcements
  - Default: `true`

- **`beaconPeerIdDirectOnly`** (boolean): Only include beacon's PeerID in direct replies
  - Default: `false`

#### Security and Privacy

- **`noCrypto`** (boolean): Disable encryption on pubsub messages
  - Default: `false`
  - **WARNING**: Only use for testing/debugging

- **`anonLookups`** (boolean): Allow anonymous lookups and responses
  - Default: `false`

- **`allowExpiredFigs`** (boolean): Allow loading expired fig files
  - Default: `false`
  - **WARNING**: Security risk - only use for testing

- **`allowInsecureFigs`** (boolean): Allow loading fig files with invalid/missing signatures
  - Default: `false`
  - **WARNING**: Security risk - only use for testing

## Command-Line Flags

All configuration file options can be overridden via command-line flags:

| Flag | Config Field | Description |
|------|--------------|-------------|
| `-config` | N/A | Path to JSON configuration file |
| `-privkey` | `privkey` | Path to PEM private key file |
| `-services-dir` | `servicesDir` | Services directory path |
| `-figs-dir` | `figsDir` | Figs directory path |
| `-listen` | `listen` | Listen multiaddr |
| `-disable-dht` | `disableDht` | Disable DHT discovery |
| `-no-announce` | `noAnnounce` | Don't announce presence |
| `-anon-lookups` | `anonLookups` | Allow anonymous lookups |
| `-no-crypto` | `noCrypto` | Disable pubsub encryption |
| `-nat-traversal` | `natTraversal` | Enable NAT traversal |
| `-router-config` | `routerConfig` | Router config file path |
| `-addons-dir` | `addonsDir` | Addons directory path |
| `-beacon-include-peerid-announcements` | `beaconIncludePeerIdAnnouncements` | Include PeerID in announcements |
| `-beacon-peerid-direct-only` | `beaconPeerIdDirectOnly` | PeerID in direct replies only |
| `-allow-expired-figs` | `allowExpiredFigs` | Allow expired figs |
| `-allow-insecure-figs` | `allowInsecureFigs` | Allow insecure figs |

**Note**: Bootstrap peers can only be configured via the configuration file, not command-line flags.

## Usage Examples

### Example 1: Production Node with Custom Bootstraps

**config.json:**
```json
{
  "privkey": "./keys/prod-node.pem",
  "servicesDir": "./services",
  "listen": "/ip4/0.0.0.0/tcp/4001",
  "natTraversal": true,
  "bootstraps": [
    "/ip4/10.0.1.100/tcp/4001/p2p/QmBootstrap1",
    "/ip4/10.0.1.101/tcp/4001/p2p/QmBootstrap2"
  ]
}
```

**Command:**
```bash
./banyan -config=./config.json
```

### Example 2: Development Node with Overrides

**dev-config.json:**
```json
{
  "servicesDir": "./dev-services",
  "disableDht": true,
  "noCrypto": true
}
```

**Command (override listen port):**
```bash
./banyan -config=./dev-config.json -listen="/ip4/0.0.0.0/tcp/8080"
```

### Example 3: Private Network Node

**private-network.json:**
```json
{
  "privkey": "./keys/private-node.pem",
  "disableDht": false,
  "bootstraps": [
    "/ip4/192.168.1.10/tcp/4001/p2p/QmPrivateBootstrap1",
    "/ip4/192.168.1.11/tcp/4001/p2p/QmPrivateBootstrap2"
  ],
  "noAnnounce": false
}
```

**Command:**
```bash
./banyan -config=./private-network.json
```

### Example 4: Testing Node (No DHT, No Crypto)

**test-config.json:**
```json
{
  "disableDht": true,
  "noCrypto": true,
  "allowExpiredFigs": true,
  "allowInsecureFigs": true
}
```

**Command:**
```bash
./banyan -config=./test-config.json
```

## Bootstrap Peers

### What are Bootstrap Peers?

Bootstrap peers are the initial nodes that a new node connects to when joining the DHT network. They help the node discover other peers and integrate into the network.

### Default Bootstrap Peers

By default, Banyan uses the standard libp2p bootstrap peers:
- `/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN`
- `/dnsaddr/bootstrap.libp2p.io/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa`
- And others...

### Custom Bootstrap Peers

For private networks or custom deployments, you can specify your own bootstrap peers in two ways:

#### Option 1: Inline Array

```json
{
  "bootstraps": [
    "/ip4/10.0.1.100/tcp/4001/p2p/QmYourBootstrapPeer1",
    "/ip4/10.0.1.101/tcp/4001/p2p/QmYourBootstrapPeer2",
    "/dns4/bootstrap.example.com/tcp/4001/p2p/QmYourBootstrapPeer3"
  ]
}
```

#### Option 2: External File

**config.json:**
```json
{
  "bootstraps": "./bootstraps.json"
}
```

**bootstraps.json:**
```json
[
  "/ip4/10.0.1.100/tcp/4001/p2p/QmYourBootstrapPeer1",
  "/ip4/10.0.1.101/tcp/4001/p2p/QmYourBootstrapPeer2",
  "/dns4/bootstrap.example.com/tcp/4001/p2p/QmYourBootstrapPeer3"
]
```

Using a separate file is useful when:
- You have many bootstrap peers
- You want to share bootstrap lists across multiple configs
- You need to update bootstrap peers without modifying the main config

### Bootstrap Peer Format

Each bootstrap peer must be specified as a multiaddr with the peer ID:
- IP-based: `/ip4/<ip>/tcp/<port>/p2p/<peerID>`
- DNS-based: `/dns4/<domain>/tcp/<port>/p2p/<peerID>`
- DNS with address: `/dnsaddr/<domain>/p2p/<peerID>`

## Configuration Precedence

The configuration system follows this precedence order (highest to lowest):

1. **Command-line flags** - Always take precedence
2. **Configuration file** - Used if no command-line flag is set
3. **Default values** - Used if neither flag nor config file specifies a value

### Example:

**config.json:**
```json
{
  "listen": "/ip4/0.0.0.0/tcp/9000",
  "disableDht": false
}
```

**Command:**
```bash
./banyan -config=./config.json -listen="/ip4/0.0.0.0/tcp/8080"
```

**Result:**
- Listen address: `/ip4/0.0.0.0/tcp/8080` (from command-line flag)
- DHT disabled: `false` (from config file)

## Best Practices

1. **Use configuration files for persistent settings** - Store your standard configuration in a file
2. **Use command-line flags for temporary overrides** - Override specific settings without modifying the config file
3. **Keep sensitive keys secure** - Protect your private key files with appropriate file permissions
4. **Document custom bootstrap peers** - Maintain a list of your bootstrap peers for network management
5. **Use separate configs for different environments** - Create `dev-config.json`, `staging-config.json`, `prod-config.json`
6. **Version control your configs** - Track configuration changes (but exclude private keys!)
7. **Test configuration changes** - Verify new configurations in a test environment first

## Troubleshooting

### Configuration file not loading

- Check the file path is correct
- Verify the JSON syntax is valid
- Ensure the file has read permissions

### Bootstrap peers not connecting

- Verify the multiaddr format is correct
- Check network connectivity to bootstrap peers
- Ensure peer IDs are correct
- Review firewall rules

### Command-line flags not overriding config

- Ensure you're using the correct flag name
- Check flag syntax (use `=` or space: `-listen="/ip4/..."` or `-listen "/ip4/..."`)
- Verify the flag is supported for command-line override

## See Also

- `example-node-config.json` - Complete configuration example
- `README.md` - General Banyan documentation
- `Banyan_Documentation.md` - Detailed technical documentation

