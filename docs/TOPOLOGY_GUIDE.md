# Topology Generator Guide

## 1. Overview

The topology generator is a tool that automates the creation of Banyan network topologies from a JSON configuration file. It generates:

- **Ed25519 cryptographic keys** for authorities and services
- **Fig files** with embedded public keys, signed by designated authorities
- **Node directories** with service configurations and startup scripts
- **Key mappings** for tracking public key references

The generator eliminates manual key management and ensures consistent, properly-signed configurations across distributed nodes.

## 2. Installation

### Building from Source

```bash
cd apps/topology-gen
go build -o generator generator.go
```

### Running

```bash
./generator <config-file> [output-dir] [--banyan-path=<path>]
```

**Arguments:**
- `config-file` - Path to JSON configuration file (required)
- `output-dir` - Output directory (default: `./topology-output`)
- `--banyan-path` - Relative path to banyan binaries directory (contains `win/`, `linux/`, `osx/`)

**Example:**
```bash
./generator example-config.json ./my-topology --banyan-path=../../dist/apps/node
```

## 3. Configuration Format

The configuration file is a JSON document with four main sections:

### 3.1 authorityKeys

List of authority key names that will be generated and used to sign fig files.

```json
{
  "authorityKeys": ["main-authority", "backup-authority"]
}
```

Authority keys are stored in `keys/authority/` and should be kept secure. They represent trusted signers that validate fig file authenticity.

### 3.2 nodes

Array of node definitions, each containing:

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Unique node identifier (used for directory name) |
| `pk` | string | `"true"` to generate a libp2p node identity key |
| `services` | array | List of services this node hosts |

```json
{
  "nodes": [
    {
      "name": "node-alpha",
      "pk": "true",
      "services": [
        {
          "key": "service-key-a",
          "figs": ["fig-read", "fig-write"]
        }
      ]
    }
  ]
}
```

### 3.3 services (within nodes)

Each service entry defines:

| Field | Type | Description |
|-------|------|-------------|
| `key` | string | Service key name (used to generate/lookup the key) |
| `figs` | array | List of fig file names this service uses |

The same service key can be referenced across multiple nodes, enabling key sharing for replicated services.

### 3.4 figs

Fig (file) definitions specifying authorization structure:

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Fig identifier |
| `root` | FigNode | Root node of the path hierarchy |
| `requiredSigners` | array | Authority key names that must sign this fig |

```json
{
  "figs": [
    {
      "name": "my-fig",
      "root": {
        "path": "/",
        "keys": ["service-key-a"],
        "children": [
          {
            "path": "/admin",
            "keys": ["admin-key"]
          }
        ]
      },
      "requiredSigners": ["main-authority"]
    }
  ]
}
```

#### FigNode Structure

| Field | Type | Description |
|-------|------|-------------|
| `path` | string | Path this key set authorizes |
| `keys` | array | Service key names authorized for this path |
| `children` | array | Child FigNodes for nested paths |

## 4. Generated Output Structure

```
output-dir/
├── keys/
│   ├── authority/
│   │   ├── authority-name.pem
│   │   └── ...
│   ├── service-key-a.pem
│   ├── service-key-b.pem
│   └── key-mapping.json
└── nodes/
    └── node-name/
        ├── node-identity.pem      # (if pk: true)
        ├── services/
        │   ├── services.json
        │   └── service-key-a/
        │       ├── service-key-a.pem
        │       └── fig-name.json
        ├── start-node.sh
        ├── start-node.bat
        └── start-node.ps1
```

### 4.1 keys/ Directory

Contains all generated cryptographic keys:

- `authority/` - Authority private keys (for signing figs)
- `*.pem` - Service private keys in PKCS#8 format
- `key-mapping.json` - Maps key names to compressed public key hex strings

### 4.2 key-mapping.json

```json
{
  "service-key-a.pem": "08011220...",
  "authority-name.pem": "08011220..."
}
```

### 4.3 nodes/<name>/ Directory

Each node directory contains:

| File | Description |
|------|-------------|
| `node-identity.pem` | Libp2p identity key (if `pk: "true"`) |
| `services/` | Service configurations |
| `services.json` | Service registry for the node |
| `start-node.sh` | Unix/macOS startup script |
| `start-node.bat` | Windows batch startup script |
| `start-node.ps1` | Windows PowerShell startup script |

### 4.4 services.json Format

```json
{
  "service-key-a": {
    "directory": "service-key-a",
    "pem": "service-key-a.pem",
    "fig": "fig-name.json"
  },
  "service-key-b": {
    "directory": "service-key-b",
    "pem": "service-key-b.pem",
    "figs": ["fig-read.json", "fig-write.json"]
  }
}
```

### 4.5 Fig File Format

Generated fig files contain:

```json
{
  "serviceAlias": "fig-name",
  "root": {
    "path": "/",
    "keys": ["08011220..."],
    "children": [...]
  },
  "expiresAt": "2026-04-26T00:00:00Z",
  "requiredSigners": ["08011220..."],
  "authoritySignatures": ["a1b2c3d4..."]
}
```

Key names are replaced with their compressed public key hex representations.

## 5. Usage Examples

### 5.1 Basic Usage

Minimal configuration with one node and one service:

```json
{
  "nodes": [
    {
      "name": "simple-node",
      "pk": "true",
      "services": [
        {
          "key": "my-service",
          "figs": ["my-fig"]
        }
      ]
    }
  ],
  "figs": [
    {
      "name": "my-fig",
      "root": {
        "path": "/",
        "keys": ["my-service"]
      }
    }
  ]
}
```

### 5.2 Multiple Figs Per Service

A service can reference multiple figs for different authorization levels:

```json
{
  "nodes": [
    {
      "name": "multi-fig-node",
      "pk": "true",
      "services": [
        {
          "key": "data-service",
          "figs": ["read-fig", "write-fig"]
        }
      ]
    }
  ],
  "figs": [
    {
      "name": "read-fig",
      "root": {
        "path": "/read",
        "keys": ["data-service"]
      }
    },
    {
      "name": "write-fig",
      "root": {
        "path": "/write",
        "keys": ["data-service"]
      }
    }
  ]
}
```

### 5.3 Hierarchical Paths with Children

Nested authorization with parent/child path relationships:

```json
{
  "figs": [
    {
      "name": "hierarchical-fig",
      "root": {
        "path": "/",
        "keys": ["root-key"],
        "children": [
          {
            "path": "/users",
            "keys": ["user-admin-key"],
            "children": [
              {
                "path": "/users/profiles",
                "keys": ["profile-key"]
              }
            ]
          },
          {
            "path": "/system",
            "keys": ["system-key"]
          }
        ]
      }
    }
  ]
}
```

### 5.4 Required Signers

Authority-based signature requirements:

```json
{
  "authorityKeys": ["primary-auth", "secondary-auth"],
  "figs": [
    {
      "name": "secure-fig",
      "root": {
        "path": "/",
        "keys": ["secure-service"]
      },
      "requiredSigners": ["primary-auth", "secondary-auth"]
    }
  ]
}
```

Multiple required signers create multiple signatures in the fig file, enabling multi-authority validation.

## 6. Chat App Example

The chat application topology (`apps/chat-service/topology/chat-config.json`) demonstrates a role-based permission system:

```json
{
  "authorityKeys": ["chat-authority"],
  "nodes": [
    {
      "name": "chat-static-1",
      "pk": "true",
      "services": [
        {
          "key": "chat-static-key",
          "figs": ["chat-fig"]
        }
      ]
    },
    {
      "name": "chat-admin-1",
      "pk": "true",
      "services": [
        { "key": "chat-static-key", "figs": ["chat-fig"] },
        { "key": "chat-ledger-key", "figs": ["chat-fig"] },
        { "key": "chat-mod-key", "figs": ["chat-fig"] },
        { "key": "chat-admin-key", "figs": ["chat-fig"] }
      ]
    }
  ],
  "figs": [
    {
      "name": "chat-fig",
      "root": {
        "path": "/",
        "keys": ["chat-static-key"],
        "children": [
          { "path": "/ledger", "keys": ["chat-ledger-key"] },
          { "path": "/mod", "keys": ["chat-mod-key"] },
          { "path": "/admin", "keys": ["chat-admin-key"] }
        ]
      },
      "requiredSigners": ["chat-authority"]
    }
  ]
}
```

**Key Architecture:**
- `chat-static-key` - Base read access (all nodes)
- `chat-ledger-key` - Ledger write access
- `chat-mod-key` - Moderator privileges
- `chat-admin-key` - Administrator privileges

**Node Roles:**
- `chat-static-1/2` - Basic nodes (read-only)
- `chat-ledger-1/2` - Ledger nodes (read + ledger)
- `chat-mod-1` - Moderator node (read + ledger + mod)
- `chat-admin-1` - Admin node (all permissions)

## 7. Key Management

### 7.1 Key Storage

| Key Type | Location | Purpose |
|----------|----------|---------|
| Authority keys | `keys/authority/*.pem` | Sign fig files |
| Service keys | `keys/*.pem` | Service authentication |
| Node identity | `nodes/<name>/node-identity.pem` | Libp2p peer identity |

### 7.2 Security Considerations

1. **Protect authority keys** - Store `keys/authority/` in a secure location; these keys can sign any fig
2. **Distribute keys carefully** - Only copy service keys to nodes that need them
3. **Use file permissions** - Restrict read access to `.pem` files (`chmod 600`)
4. **Secure transmission** - Use encrypted channels when distributing keys to nodes
5. **Rotate keys periodically** - See key rotation below

### 7.3 Key Rotation

To rotate keys:

1. Generate new configuration with new key names
2. Deploy new keys to nodes
3. Update fig files with new public keys
4. Re-sign figs with authority keys
5. Restart nodes with new configuration

Example rotation strategy:
```json
{
  "services": [
    { "key": "service-key-v2", "figs": ["my-fig-v2"] }
  ]
}
```

## 8. Troubleshooting

### Common Issues

#### "undefined fig" Error

**Symptom:** `node X service Y references undefined fig Z`

**Solution:** Ensure all figs referenced in `services[].figs` are defined in the `figs` array.

#### "required signer not found" Error

**Symptom:** `required signer X not found in service keys or authority keys`

**Solution:** Ensure all keys in `requiredSigners` are defined in either `authorityKeys` or as service keys.

#### Startup Script Path Issues

**Symptom:** `banyan executable not found`

**Solution:** 
- Specify `--banyan-path` when running the generator
- Or manually edit the generated `start-node.*` script to set the correct path

#### Key Mismatch Between Nodes

**Symptom:** Services cannot communicate or authorize operations

**Solution:**
- Ensure nodes sharing a service use the same key name
- Check `key-mapping.json` to verify public key consistency
- Regenerate topology if keys were generated separately

#### Fig Signature Validation Fails

**Symptom:** Authority signatures rejected at runtime

**Solution:**
- Verify authority keys match between signer and validator
- Check that `requiredSigners` keys are in `authorityKeys`
- Ensure fig files weren't modified after generation

### Validation Checklist

- [ ] All fig names in `services[].figs` exist in `figs` array
- [ ] All keys in `fig.root.keys` exist as service keys
- [ ] All `requiredSigners` exist in `authorityKeys` or service keys
- [ ] Node names are unique
- [ ] Service key names are unique (per generation)
- [ ] Paths in fig hierarchy don't conflict
