# Banyan Chat App Specification

## Overview

A decentralized chat application demonstrating Banyan's P2P capabilities with:
- Peer-to-peer authentication via libp2p peer IDs
- Distributed ledger for chat state consensus
- Tiered service-key architecture for organizational trust delegation
- Web frontend served directly from Banyan nodes

## Tech Stack

### Frontend
- **TypeScript** - Type safety
- **React** - UI framework
- **Tailwind CSS** - Styling
- **Vite** - Build tool

### Backend
- **Go** - Service implementation
- **SQLite** - Ledger persistence

### Ledger/CRDT
Instead of building from scratch, consider:
- **Automerge** (via `automerge-go` bindings) - Mature CRDT library
- **Custom LWW-Register** - Simple last-write-wins with timestamps (sufficient for chat)

The ledger is a hash-chained append-only log with CRDT merge semantics. For a chat app, simple LWW (Last-Writer-Wins) registers are typically sufficient and easy to implement.

## Architecture

### Core Principle: Single Backend, Single Ledger

The chat service is **one HTTP server** with a **single unified ledger**. Different service keys map to different endpoints on this same server. A node with multiple keys still runs one server - the router just routes multiple keys to it.

```
┌─────────────────────────────────────────────────────────────────┐
│                     Chat Service (ONE HTTP Server)              │
│                                                                 │
│  Endpoints:                                                     │
│  ├── GET /, GET /posts, GET /channels, WS /events             │
│  ├── POST /ledger/post, POST /ledger/delete, /ledger/sync     │
│  ├── POST /mod/ban, POST /mod/delete, POST /mod/promote       │
│  └── POST /admin/issue-key, POST /admin/revoke                │
│                                                                 │
│  Ledger (unified, all actions):                                │
│  └── posts, deletes, bans, promotions, key issuances, etc.    │
└─────────────────────────────────────────────────────────────────┘
                              ▲
                              │ routes keys to endpoints
┌─────────────────────────────┴───────────────────────────────────┐
│                     Banyan Router                               │
│                                                                 │
│  chat-static-key  → routes to / (GET only, serves bundle)      │
│  chat-ledger-key  → routes to /ledger/*                        │
│  chat-mod-key     → routes to /mod/*                           │
│  chat-admin-key   → routes to /admin/*                         │
└─────────────────────────────────────────────────────────────────┘
```

### Node Deployment Examples

**Static-only node (CDN):**
- Has: `chat-static-key`
- Runs: ONE HTTP server with all endpoints
- Router: Only routes `chat-static-key` to server's `/` path
- Result: Can only serve GET requests at `/`

**Ledger node:**
- Has: `chat-static-key`, `chat-ledger-key`
- Runs: ONE HTTP server with all endpoints
- Router: Routes both keys to same server
- Result: Can serve `/` AND `/ledger/*`

**Full node:**
- Has: All 4 keys
- Runs: ONE HTTP server with all endpoints
- Router: Routes all keys to same server
- Result: Can serve everything

### Request Flow

```
User Browser                    Static Node                Ledger Node
    │                               │                           │
    │  GET /                        │                           │
    │──────────────────────────────>│                           │
    │  (webapp bundle)              │                           │
    │<──────────────────────────────│                           │
    │                               │                           │
    │  POST /ledger/post            │                           │
    │──────────────────────────────>│                           │
    │                               │  (no ledger-key,          │
    │                               │   forwards request)       │
    │                               │──────────────────────────>│
    │                               │                           │
    │                               │  (creates ledger entry)   │
    │                               │<──────────────────────────│
    │<──────────────────────────────│  (response)               │
```

### Service Key Tiers (Organizational Trust)

Service keys are delegated to nodes based on organizational trust. The fig defines base paths requiring each key.

```
Trust Tier     Key Name            Path        Node Responsibility
─────────────  ──────────────      ────────    ─────────────────────────────────
Highest        chat-admin-key      /admin      Issue/revoke keys, system config
               chat-mod-key        /mod        Ban peers, delete posts, promote users
               chat-ledger-key     /ledger     Accept posts, sync ledger state
Lowest         chat-static-key     /           Serve webapp bundle, read-only data
```

**Example Deployment:**
- 100 nodes with `chat-static-key` → serve webapp at `/` (read-only, CDN-like)
- 10 nodes with `chat-ledger-key` → handle `/ledger/post`, `/ledger/delete`, sync with each other
- 5 nodes with `chat-mod-key` → handle `/mod/ban`, `/mod/delete`
- 2 nodes with `chat-admin-key` → handle `/admin/issue-key`

A node can hold multiple keys. A node with `chat-ledger-key` typically also has `chat-static-key`.

### Fig File Structure

The fig defines base paths and their required keys. Operations under each path are handled by the webapp itself, not separate fig entries.

```json
{
  "serviceAlias": "banyan-chat",
  "root": {
    "path": "/",
    "keys": ["chat-static-key"],
    "children": [
      {
        "path": "/ledger",
        "keys": ["chat-ledger-key"]
      },
      {
        "path": "/mod",
        "keys": ["chat-mod-key"]
      },
      {
        "path": "/admin",
        "keys": ["chat-admin-key"]
      }
    ]
  },
  "requiredSigners": ["chat-authority"]
}
```

**Path responsibilities (webapp endpoints, not fig entries):**

| Fig Path | Required Key | Webapp Endpoints (internal) |
|----------|--------------|----------------------------|
| `/` | `chat-static-key` | `GET /`, `GET /channels`, `GET /posts`, `WS /events` |
| `/ledger` | `chat-ledger-key` | `POST /ledger/post`, `POST /ledger/delete`, `GET /ledger/sync`, `POST /ledger/sync` |
| `/mod` | `chat-mod-key` | `POST /mod/ban`, `POST /mod/delete`, `POST /mod/promote` |
| `/admin` | `chat-admin-key` | `POST /admin/issue-key`, `POST /admin/revoke-node` |

**How it works:**
- User requests `GET /posts` → static-key node responds
- User requests `POST /ledger/post` → forwarded to ledger-key node
- User requests `POST /mod/ban` → forwarded to mod-key node
- The fig doesn't list `/ledger/post` or `/mod/ban` - those are webapp implementation details

## Data Model

### Unified Ledger

All actions go into a **single ledger** - posts, deletions, bans, promotions, key issuances. Different operations require different service keys, but everything is one consistent data structure.

```go
type LedgerEntry struct {
    ID        string          `json:"id"`         // UUID
    Type      EntryType       `json:"type"`       // post, delete, ban, promote, issue_key, etc.
    Author    string          `json:"author"`     // PeerID of actor
    Timestamp int64           `json:"timestamp"`  // Unix nano
    Data      json.RawMessage `json:"data"`       // Type-specific payload
    Signature string          `json:"signature"`  // Author's signature
    Hash      string          `json:"hash"`       // SHA256 of canonical form
    PrevHash  string          `json:"prevHash"`   // Previous entry hash (chain)
}

type EntryType int
const (
    EntryTypePost EntryType = iota      // /ledger/post - requires chat-ledger-key
    EntryTypeDelete                      // /ledger/delete - requires chat-ledger-key (own) or chat-mod-key (any)
    EntryTypeBan                         // /mod/ban - requires chat-mod-key
    EntryTypePromote                     // /mod/promote - requires chat-mod-key
    EntryTypeIssueKey                    // /admin/issue-key - requires chat-admin-key
    EntryTypeRevoke                      // /admin/revoke - requires chat-admin-key
)
```

### Entry Payloads

```go
type PostData struct {
    ChannelID string `json:"channelId"`
    Content   string `json:"content"`
}

type DeleteData struct {
    TargetID  string `json:"targetId"`   // Entry ID being deleted
    Reason    string `json:"reason"`
}

type BanData struct {
    TargetPeerID string `json:"targetPeerId"`
    Reason       string `json:"reason"`
    Duration     int64  `json:"duration"`  // 0 = permanent
}

type PromoteData struct {
    TargetPeerID string `json:"targetPeerId"`
    Role         string `json:"role"`      // "moderator", etc.
}

type IssueKeyData struct {
    KeyType     string `json:"keyType"`    // "ledger", "mod", "admin"
    TargetNode  string `json:"targetNode"` // PeerID receiving key
    ExpiresAt   int64  `json:"expiresAt"`
}
```

### CRDT-Based Conflict Resolution

Using simple **LWW-Register (Last-Writer-Wins)** - sufficient for chat:

```
sort_key = (timestamp, peer_id)  // peer_id as deterministic tiebreaker
```

For deletions: **tombstone approach** - entry stays in ledger but marked as deleted.

**Why not full CRDT?**
- Chat doesn't need concurrent edits on same message
- LWW handles 99% of cases cleanly
- Simpler to implement and debug
- Can upgrade to Automerge later if needed

## API Endpoints

Endpoints are organized by fig path. Each path requires its corresponding service key.

### Path: `/` (chat-static-key)
*Served by many nodes (CDN-like distribution)*

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/` | Serve webapp bundle (SPA) |
| GET | `/static/*` | Static assets |
| GET | `/posts` | List posts (paginated) |
| GET | `/channels` | List channels |
| WS | `/events` | Real-time event stream |

### Path: `/ledger` (chat-ledger-key)
*Served by ledger-syncing nodes*

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/ledger/post` | Create new post (user action) |
| POST | `/ledger/delete` | Delete own post (user action) |
| GET | `/ledger/sync` | Get entries since hash (node-to-node) |
| POST | `/ledger/sync` | Push entries (node-to-node) |

### Path: `/mod` (chat-mod-key)
*Served by moderator nodes*

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/mod/ban` | Ban a peer ID |
| POST | `/mod/delete` | Delete any user's post |
| POST | `/mod/promote` | Grant user privileges |

### Path: `/admin` (chat-admin-key)
*Served by admin nodes*

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/admin/issue-key` | Issue new service key |
| POST | `/admin/revoke` | Revoke a node's access |
| GET | `/admin/status` | System status |

## Ledger Synchronization (HTTP-based)

All node-to-node communication uses HTTP over Banyan's libp2p proxy with service key authorization.

### Node-to-Node Sync Endpoints (at `/ledger` path)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/ledger/sync?since=<hash>` | Get entries since hash |
| POST | `/ledger/sync` | Push entries to this node |

Only nodes with `chat-ledger-key` serve these endpoints and participate in sync.

### Sync Algorithm

Only nodes with `chat-ledger-key` maintain full ledger state. Static nodes read from ledger nodes.

```
Periodic sync (every 30s) - ledger nodes only:
1. For each known ledger node peer:
   a. GET /ledger/sync?since=<last_hash> via Banyan HTTP proxy
   b. Merge received entries using CRDT rules
   c. Persist merged state

On user post (via static node):
1. Static node receives POST /ledger/post from user
2. Static node forwards to ledger node (has chat-ledger-key)
3. Ledger node creates entry, applies locally
4. Ledger node syncs with other ledger nodes

Read flow (at static nodes):
1. User GET /posts via static node
2. Static node queries ledger node for current state
3. Return data to user
```

### Discovery of Peer Nodes

Nodes discover each other via Banyan's existing beacon/locator system:
- **Static nodes** discover **ledger nodes** to forward write operations
- **Ledger nodes** discover each other to maintain ledger consistency
- Service key determines the discovery scope

### Persistence

Each node maintains:
- `ledger.db` - SQLite database for entries
- `state.json` - Current merged state (posts, channels)
- `peers.json` - Known peers and their latest hash

## Authentication Flow

Users are authenticated by their libp2p peer ID (no accounts/passwords). The node they connect to must have the appropriate service key for the requested path.

```
User Authentication (peer ID):
1. User connects to any static node's webapp at /
2. Webapp generates a challenge nonce
3. User signs challenge with their node's private key
4. Webapp verifies signature, extracts peer ID
5. Session established with peer ID as identity

Request Routing (path-based):
1. User submits a post from webapp
2. Webapp sends POST /ledger/post
3. Static node (only has chat-static-key) cannot serve /ledger/*
4. Static node discovers a ledger node (has chat-ledger-key)
5. Request forwarded to ledger node via Banyan HTTP proxy
6. Ledger node creates entry, syncs with other ledger nodes
```

No passwords, no accounts - identity is cryptographic.

## Implementation Plan

### Phase 1: Topology Generator Improvements

**Files to modify:**
- `apps/topology-gen/generator.go`

**Tasks:**
1. Add CLI documentation and help text
2. Support custom expiration durations
3. Add key derivation from master seed (deterministic key generation)
4. Support for fig file templates with variable substitution
5. Generate README for each node output
6. Add validation for key references

### Phase 2: Ledger Package

**New files:**
- `pkg/ledger/ledger.go` - Core ledger types and operations
- `pkg/ledger/crdt.go` - CRDT merge implementation
- `pkg/ledger/storage.go` - SQLite persistence
- `pkg/ledger/sync.go` - Gossip-based synchronization

**Tasks:**
1. Define ledger entry types
2. Implement hash chain construction
3. Implement CRDT merge semantics
4. SQLite storage layer
5. Gossip protocol handler
6. Sync algorithm implementation

### Phase 3: Chat Service Backend

**New files:**
- `apps/chat-service/main.go` - Service entry point
- `apps/chat-service/handlers/` - HTTP handlers
- `apps/chat-service/websocket.go` - WebSocket handler
- `apps/chat-service/auth.go` - Peer ID authentication

**Tasks:**
1. HTTP handler structure
2. Post/Channel CRUD operations
3. WebSocket real-time updates
4. Authentication middleware
5. Integration with ledger package
6. Static file serving

### Phase 4: Web Frontend

**New files:**
- `apps/chat-frontend/` - React/Vue application
- `apps/chat-frontend/src/components/` - UI components
- `apps/chat-frontend/src/api/` - API client
- `apps/chat-frontend/src/ws/` - WebSocket client

**Tasks:**
1. Project setup (Vite + React)
2. Channel list component
3. Message list component
4. Post composer
5. WebSocket integration
6. Authentication flow UI
7. Build and bundle for embedding

### Phase 5: Integration

**Files to modify:**
- `apps/node/addon/` - Addon SDK updates
- `apps/node/main.go` - Service loading

**Tasks:**
1. Embed frontend bundle in chat service
2. Register chat service as Banyan addon
3. Configure service key routing
4. End-to-end testing
5. Documentation

### Phase 6: Documentation

**New files:**
- `docs/TOPOLOGY_GUIDE.md` - How to use topology generator
- `docs/CHAT_APP_DEPLOYMENT.md` - Deploying chat service
- `docs/DEVELOPER_GUIDE.md` - Building services on Banyan

## Topology Generator Improvements (Detailed)

### New Config Format

```json
{
  "version": "2",
  "metadata": {
    "name": "banyan-chat-network",
    "description": "Chat service deployment",
    "generated": "2024-01-15T10:00:00Z"
  },
  "keys": {
    "authority": {
      "chat-authority": { "description": "Root authority for chat service" }
    },
    "service": {
      "chat-admin-key": { 
        "description": "Admin path access",
        "derivedFrom": "chat-authority",
        "derivationPath": "admin"
      },
      "chat-mod-key": { 
        "description": "Moderator path access",
        "derivedFrom": "chat-authority",
        "derivationPath": "mod"
      },
      "chat-ledger-key": { 
        "description": "Ledger path access - post/delete/sync",
        "derivedFrom": "chat-authority",
        "derivationPath": "ledger"
      },
      "chat-static-key": { 
        "description": "Static path access - serve webapp",
        "derivedFrom": "chat-authority",
        "derivationPath": "static"
      }
    }
  },
  "figTemplates": {
    "chat-fig": {
      "description": "Chat service fig - base paths only",
      "root": {
        "path": "/",
        "keys": ["chat-static-key"],
        "children": [
          { "path": "/ledger", "keys": ["chat-ledger-key"] },
          { "path": "/mod", "keys": ["chat-mod-key"] },
          { "path": "/admin", "keys": ["chat-admin-key"] }
        ]
      },
      "requiredSigners": ["chat-authority"],
      "expiresIn": "720h"
    }
  },
  "nodes": [
    {
      "name": "chat-static-1",
      "identity": { "generate": true },
      "services": [
        {
          "key": "chat-static-key",
          "figs": ["chat-fig"]
        }
      ],
      "config": {
        "listen": "/ip4/0.0.0.0/tcp/9001"
      }
    },
    {
      "name": "chat-ledger-1",
      "identity": { "generate": true },
      "services": [
        {
          "key": "chat-static-key",
          "figs": ["chat-fig"]
        },
        {
          "key": "chat-ledger-key",
          "figs": ["chat-fig"]
        }
      ],
      "config": {
        "listen": "/ip4/0.0.0.0/tcp/9002"
      }
    },
    {
      "name": "chat-full-1",
      "identity": { "generate": true },
      "services": [
        {
          "key": "chat-static-key",
          "figs": ["chat-fig"]
        },
        {
          "key": "chat-ledger-key",
          "figs": ["chat-fig"]
        },
        {
          "key": "chat-mod-key",
          "figs": ["chat-fig"]
        },
        {
          "key": "chat-admin-key",
          "figs": ["chat-fig"]
        }
      ],
      "config": {
        "listen": "/ip4/0.0.0.0/tcp/9003"
      }
    }
  ]
}
```

### New CLI Commands

```bash
# Generate topology from config
topology-gen generate config.json -o ./output

# Generate keys only
topology-gen keys config.json -o ./keys

# Validate config
topology-gen validate config.json

# Add a node interactively
topology-gen add-node config.json --name new-node

# Export public keys for distribution
topology-gen export-pubs config.json -o ./public-keys

# Generate documentation
topology-gen docs config.json -o ./docs
```

## File Structure After Implementation

```
banyan/
├── pkg/
│   └── ledger/
│       ├── ledger.go           # Core ledger, entry types, unified interface
│       ├── crdt.go             # CRDT merge for all entry types
│       ├── storage.go          # SQLite persistence
│       └── sync.go             # HTTP sync client
├── apps/
│   ├── topology-gen/
│   │   ├── generator.go        # (enhanced)
│   │   ├── config.go           # (new) config parsing
│   │   ├── keys.go             # (new) key operations
│   │   ├── validate.go         # (new) validation
│   │   └── cli.go              # (new) CLI commands
│   ├── chat-service/
│   │   ├── main.go             # Single HTTP server entry point
│   │   ├── server.go           # All routes registered here
│   │   ├── handlers.go         # All endpoint handlers
│   │   │   # Contains handlers for:
│   │   │   # - GET /, /posts, /channels
│   │   │   # - WS /events
│   │   │   # - POST /ledger/post, /ledger/delete, /ledger/sync
│   │   │   # - POST /mod/ban, /mod/delete, /mod/promote
│   │   │   # - POST /admin/issue-key, /admin/revoke
│   │   ├── forward.go          # Forward to nodes with more keys
│   │   ├── websocket.go        # Real-time events
│   │   ├── auth.go             # Peer ID authentication
│   │   └── embed.go            # Embedded frontend
│   └── chat-frontend/
│       ├── src/
│       │   ├── components/      # React components
│       │   │   ├── Auth.tsx
│       │   │   ├── ChannelList.tsx
│       │   │   ├── MessageList.tsx
│       │   │   └── PostComposer.tsx
│       │   ├── hooks/           # Custom React hooks
│       │   ├── api/             # API client functions
│       │   ├── ws/              # WebSocket client
│       │   ├── App.tsx
│       │   ├── main.tsx
│       │   └── index.css        # Tailwind imports
│       ├── package.json
│       ├── tsconfig.json
│       ├── tailwind.config.js
│       ├── postcss.config.js
│       └── vite.config.ts
└── docs/
    ├── TOPOLOGY_GUIDE.md
    ├── CHAT_APP_DEPLOYMENT.md
    └── DEVELOPER_GUIDE.md
```

## Security Considerations

1. **Signature Verification**: Every ledger entry must be signed by author's peer ID
2. **Service Key Distribution**: Admin keys distributed only to trusted operators
3. **Rate Limiting**: Per-peer rate limits on posting
4. **Content Validation**: Sanitize HTML, limit message size
5. **Ledger Integrity**: Hash chain prevents tampering
6. **Tombstone Privacy**: Deleted content not retrievable from ledger

## Testing Strategy

1. **Unit Tests**: Ledger CRDT operations, signature verification
2. **Integration Tests**: Multi-node sync, conflict resolution
3. **E2E Tests**: Full chat flow with multiple browsers
4. **Load Tests**: Message throughput, concurrent users
5. **Chaos Tests**: Node failures, network partitions

## Success Criteria

1. Users can connect to any node and see same chat content
2. Posts sync across nodes within 5 seconds
3. Deletions propagate correctly (tombstone)
4. New nodes can bootstrap from existing nodes
5. Topology generator produces working deployments
6. Documentation enables third-party service development
