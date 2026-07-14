# Banyan Chat App

A decentralized chat application demonstrating Banyan's P2P capabilities.

## Features

- Peer-to-peer authentication via libp2p peer IDs
- Distributed ledger for chat state consensus (CRDT-based)
- Tiered service-key architecture for organizational trust delegation
- Real-time updates via WebSocket
- React frontend with Tailwind CSS

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│              Chat Service (Single HTTP Server)               │
│                                                              │
│  Endpoints:                                                  │
│  ├── GET /, /posts, /channels, WS /events (static-key)     │
│  ├── POST /ledger/post, /ledger/delete, /ledger/sync       │
│  ├── POST /mod/ban, /mod/delete, /mod/promote              │
│  └── POST /admin/issue-key, /admin/revoke                  │
│                                                              │
│  Unified Ledger (all entry types):                          │
│  └── posts, deletes, bans, promotions, key issuances       │
└─────────────────────────────────────────────────────────────┘
```

## Quick Start

### Prerequisites

- Node.js 18+

### Build

```bash
# Build everything
make -C apps/chat-service build

# Or manually:
cd apps/chat-frontend && npm install && npm run build
cp -r dist ../chat-service/frontend/
cd ../chat-service && npm install && npm run build
```

### Run

```bash
# Run with ledger key (can accept posts)
node apps/chat-service/dist/index.js --ledger-key

# Run with all keys
node apps/chat-service/dist/index.js --ledger-key --mod-key --admin-key

# Run with config file
node apps/chat-service/dist/index.js --config config.example.json
```

### Development

```bash
# Start both frontend dev server and backend
make -C apps/chat-service dev
```

## Docker Test Cluster

The `apps/chat-service/docker/` directory contains a docker-compose file that
brings up a 6-node test cluster mirroring a realistic deployment: every service
node runs in its own isolated Docker bridge network, so peer discovery happens
over the public IPFS DHT with libp2p NAT traversal (hole-punching + circuit
relay v2) — the same path a real user on their own machine would take. No
pre-generated keys or figs are committed to the repo; they are regenerated
fresh on every `docker:up` from [`topology/chat-config.json`](topology/chat-config.json).

### One-shot bring-up from a fresh clone

```bash
npm install
nx run chat-frontend:build
nx run chat-service:docker:up
```

That last command transitively:

1. Builds `topology-gen` and the `banyan` node binary.
2. Regenerates `apps/chat-service/topology/output/` (keys, signed figs, per-node
   directories) via `nx run chat-service:topology`.
3. Builds the `banyan-node:latest` Docker image (includes the Node.js
   chat-service and the `banyan` + `proxy-addon` binaries).
4. Starts the 6 service nodes (`chat-static-1`, `chat-static-2`, `chat-ledger-1`,
   `chat-ledger-2`, `chat-mod-1`, `chat-admin-1`), each on its own bridge
   network with `-nat-traversal` enabled.

To stop the cluster: `nx run chat-service:docker:down`.

### Spinning up a user node for browser access

Service nodes do not publish the http-proxy addon — the proxy is a
*client-side* entry point used to access services from a browser. To create
one, use the test-utils script:

```bash
./test-utils/user-node/generate-user-node.sh --name me --proxy-port 9090
(cd test-utils/user-node/nodes/me && ./up.sh)
# browser -> http://localhost:9090
```

On Windows:

```powershell
.\test-utils\user-node\generate-user-node.ps1 -Name me -ProxyPort 9090
cd .\test-utils\user-node\nodes\me
.\up.ps1
```

Each generated user node gets:

- A freshly generated Ed25519 identity key (`node-identity.pem`) in PKCS8 PEM
  form, produced via `openssl genpkey -algorithm ed25519` — same format libp2p
  reads for the service nodes.
- A copy of the signed client fig (`chat-fig.json`) from the most recent
  topology regeneration, defaulted from `chat-static-1`'s figs directory.
- An `http-proxy` addon exposing the browser-facing HTTP proxy on container
  port `9090`.
- A per-node `Dockerfile` (`FROM banyan-node:latest` + `COPY`) and an `up.sh`
  that creates a dedicated `user-<name>` bridge network and runs the
  container. Each user node is isolated on its own bridge so discovery
  exercises the DHT path, matching the isolation the service nodes have.

Options:

| Flag (bash / PowerShell)       | Default | Description                            |
| ------------------------------ | ------- | -------------------------------------- |
| `--name` / `-Name`             | —       | Required; used for container/image/network/output dir. |
| `--proxy-port` / `-ProxyPort`  | `9090`  | Host port mapped to container `:9090`. |
| `--listen-port` / `-ListenPort`| `9100`  | Host port mapped to container `:9100` (libp2p listen). |
| `--fig` / `-Fig`               | `chat-static-1`'s fig | Signed client fig to bake in. |
| `--output-dir` / `-OutputDir`  | `test-utils/user-node/nodes/<name>` | Where to write the generated directory. |

Generated user-node directories live under
`test-utils/user-node/nodes/` (gitignored). To tear one down:
`./test-utils/user-node/nodes/<name>/down.sh` (or `.\down.ps1` on Windows).

### Caveats to expect when testing

- **First convergence is slow.** libp2p's DHT advertise + `FindPeers` ticker
  is 30 seconds (see `apps/node/discovery/manager.go`), so cluster formation
  after `docker:up` can take up to a minute before all nodes have found each
  other. Subsequent reconnects are faster.
- **Same-host DCUtR may fall back to relay.** When two Docker Desktop
  containers on the same host both observe the host's public IP via AutoNAT,
  libp2p's hole-punching protocol sees identical observed addresses and may
  refuse to punch. In that case connections ride over circuit relay v2 via
  public IPFS relays — still functional, but with added latency. If this
  becomes a problem, a follow-up would be a Go-side `libp2p.AddrFactory`
  tweak to additionally announce `host.docker.internal:<hostPort>`.
- **Global peer-discovery rendezvous.** All banyan nodes advertise under the
  string `peer-discovery` on the global DHT and may connect to unrelated
  banyan nodes on the internet. This is intentional — it strengthens the
  network and provides additional gossip relays. A scoped/hierarchical
  rendezvous is a separate future feature.

## Configuration

| Flag | Default | Description |
|------|---------|-------------|
| `-listen` | `:8080` | Address to listen on |
| `-data-dir` | `./data` | Directory for data storage |
| `-node-id` | `""` | This node's peer ID |
| `-static-key` | `true` | Has chat-static-key |
| `-ledger-key` | `false` | Has chat-ledger-key |
| `-mod-key` | `false` | Has chat-mod-key |
| `-admin-key` | `false` | Has chat-admin-key |
| `-sync-peers` | `""` | Comma-separated peer URLs for sync |
| `-sync` | `true` | Enable periodic ledger sync |

## Service Key Tiers

| Key | Path | Responsibility |
|-----|------|----------------|
| `chat-static-key` | `/` | Serve webapp, read-only data |
| `chat-ledger-key` | `/ledger` | Accept posts, sync ledger |
| `chat-mod-key` | `/mod` | Ban peers, delete posts, promote users |
| `chat-admin-key` | `/admin` | Issue/revoke keys, system config |

## API Endpoints

### Static (chat-static-key)
- `GET /` - Serve webapp bundle
- `GET /posts` - List posts (paginated)
- `GET /channels` - List channels
- `WS /events` - Real-time event stream

### Ledger (chat-ledger-key)
- `POST /ledger/post` - Create new post
- `POST /ledger/delete` - Delete own post
- `GET /ledger/sync?since=<hash>` - Get entries since hash
- `POST /ledger/sync` - Push entries

### Moderation (chat-mod-key)
- `POST /mod/ban` - Ban a peer ID
- `POST /mod/delete` - Delete any post
- `POST /mod/promote` - Grant user privileges

### Admin (chat-admin-key)
- `POST /admin/issue-key` - Issue new service key
- `POST /admin/revoke` - Revoke node access
- `GET /admin/status` - System status

## Project Structure

```
apps/
├── chat-service/          # Node.js backend service
│   ├── src/
│   │   ├── index.ts       # Entry point
│   │   ├── server.ts      # Express server setup
│   │   ├── config.ts      # Configuration
│   │   ├── websocket.ts   # WebSocket hub
│   │   ├── auth.ts        # Authentication
│   │   ├── handlers/      # Route handlers
│   │   └── ledger/        # Ledger/CRDT
│   ├── dist/             # Compiled JavaScript
│   ├── package.json
│   └── frontend/         # Embedded frontend dist
│
└── chat-frontend/         # React frontend
    ├── src/
    │   ├── components/   # React components
    │   ├── api/          # HTTP client
    │   └── ws/           # WebSocket client
    └── package.json
```

## Key Management

Keys are **NOT** baked into Docker images. They are mounted at runtime from `topology/output/keys/` via volumes.

### Key Management Script

```bash
cd apps/chat-service/topology

# Initialize all keys and figs
./keys.sh init

# List all keys
./keys.sh list

# Rotate a specific key (e.g., if compromised)
./keys.sh rotate chat-ledger-key

# Rotate all service keys (keeps authority)
./keys.sh rotate-all

# Regenerate fig files with current keys
./keys.sh regenerate-figs

# Export public keys for distribution
./keys.sh export ./public-keys
```

### Key Rotation Workflow

When a key is compromised:

1. **Identify the compromised key** (e.g., `chat-ledger-key`)
2. **Rotate the key**: `./keys.sh rotate chat-ledger-key`
   - Generates new private key
   - Updates key mapping
   - Regenerates all fig files
   - Re-signs with authority key
3. **Redistribute keys**:
   - Export public keys: `./keys.sh export ./new-keys`
   - Copy new private keys to affected nodes
   - Restart affected containers
4. **Revoke old access**:
   - Old key is replaced in all fig files
   - Nodes with old key can no longer authenticate

### Key Security

| Key Type | Location | Distribution |
|----------|----------|--------------|
| `chat-authority` | `keys/authority/` | Keep offline, highly secured |
| `chat-admin-key` | `keys/` | Admin nodes only |
| `chat-mod-key` | `keys/` | Moderator nodes only |
| `chat-ledger-key` | `keys/` | Ledger nodes only |
| `chat-static-key` | `keys/` | All nodes |

**Best Practices:**
- Store authority key offline (air-gapped or HSM)
- Use different keys for different trust levels
- Rotate keys immediately if compromised
- Limit distribution of admin/mod keys
- Monitor for unauthorized key usage

## License

MIT
