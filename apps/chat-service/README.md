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

## License

MIT
