# Banyan Chat App - Implementation Plan

## Tech Stack

### Frontend
| Technology | Purpose |
|------------|---------|
| TypeScript | Type safety, better DX |
| React | UI components |
| Tailwind CSS | Utility-first styling |
| Vite | Fast builds, HMR |

### Backend
| Technology | Purpose |
|------------|---------|
| Go | Service implementation |
| SQLite | Ledger persistence |

### Ledger/CRDT Options
| Option | Complexity | Notes |
|--------|------------|-------|
| Custom LWW-Register | Low | Simple timestamp-based merge, sufficient for chat |
| Automerge (automerge-go) | Medium | Full CRDT library, more features |
| Yrs (Yjs Go bindings) | Medium | CRDT for collaborative apps |

**Recommendation:** Start with custom LWW-Register. It's simple (timestamp + peer ID tiebreaker) and sufficient for chat. Can migrate to Automerge later if needed.

## Key Concept: Single Backend, Single Ledger

The chat service is **ONE HTTP server** with **ONE unified ledger**. Different service keys route to different endpoints on the same server. A node with multiple keys runs one server - the router just maps multiple keys to it.

```
┌─────────────────────────────────────────────────────────────┐
│              Chat Service (Single HTTP Server)              │
│                                                             │
│  All endpoints in one server:                               │
│  ├── GET /, /posts, /channels, WS /events                  │
│  ├── POST /ledger/post, /ledger/delete, /ledger/sync       │
│  ├── POST /mod/ban, /mod/delete, /mod/promote              │
│  └── POST /admin/issue-key, /admin/revoke                  │
│                                                             │
│  Unified Ledger (all entry types):                         │
│  └── posts, deletes, bans, promotions, key issuances       │
└─────────────────────────────────────────────────────────────┘
```

**Router maps keys to endpoints of the SAME server:**
| Key | Routes To | Access |
|-----|-----------|--------|
| `chat-static-key` | `/` | Read-only, serve webapp |
| `chat-ledger-key` | `/ledger/*` | Post, delete, sync |
| `chat-mod-key` | `/mod/*` | Ban, moderate |
| `chat-admin-key` | `/admin/*` | Issue keys, revoke |

**Node with all 4 keys:** Runs ONE server, router sends all keys to it.

## Priority Order

| Phase | Priority | Estimated Effort | Dependencies |
|-------|----------|------------------|--------------|
| 1. Topology Generator | High | 2-3 days | None |
| 2. Ledger Package | High | 3-4 days | None |
| 3. Chat Service Backend | High | 3-4 days | Ledger Package |
| 4. Web Frontend | Medium | 2-3 days | None (parallel) |
| 5. Integration | High | 2 days | All above |
| 6. Documentation | Medium | 1-2 days | All above |

## Phase 1: Topology Generator Improvements

### 1.1 Add CLI Documentation and Help
**File:** `apps/topology-gen/generator.go`

- [ ] Add comprehensive usage examples in help text
- [ ] Add `--help` flag documentation
- [ ] Add `version` command
- [ ] Colorize output for better readability

### 1.2 Enhanced Configuration Format
**File:** `apps/topology-gen/config.go` (new)

- [ ] Define v2 config schema with metadata
- [ ] Add key derivation configuration
- [ ] Add fig template support
- [ ] Add node bootstrap configuration
- [ ] Implement config migration from v1

### 1.3 Deterministic Key Generation
**File:** `apps/topology-gen/keys.go` (new)

- [ ] Implement HD-style key derivation from master seed
- [ ] Support derivation paths (e.g., "m/44'/0'/0'/0/0")
- [ ] Generate reproducible keys from seed phrase
- [ ] Export/import seed functionality

### 1.4 Fig Template System
**File:** `apps/topology-gen/templates.go` (new)

- [ ] Define reusable fig templates
- [ ] Variable substitution (e.g., `{{.NodeName}}`)
- [ ] Template inheritance
- [ ] Conditional sections

### 1.5 Output Enhancements
**File:** `apps/topology-gen/generator.go`

- [ ] Generate README.md per node
- [ ] Generate network overview diagram (ASCII)
- [ ] Generate public key export file
- [ ] Add dry-run mode

### 1.6 Validation
**File:** `apps/topology-gen/validate.go` (new)

- [ ] Validate key references exist
- [ ] Validate fig template references
- [ ] Validate bootstrap peer references
- [ ] Check for circular dependencies

---

## Phase 2: Ledger Package

### 2.1 Core Types
**File:** `pkg/ledger/ledger.go` (new)

```go
type Ledger interface {
    Append(entry Entry) error
    Get(id string) (*Entry, error)
    GetSince(hash string) ([]Entry, error)
    GetState() (*State, error)
    Merge(entries []Entry) error
}

type Entry struct {
    ID        string
    Type      EntryType
    Author    string  // PeerID
    Timestamp int64
    Data      []byte
    Signature []byte
    Hash      string
    PrevHash  string
}

type EntryType int
const (
    EntryTypePost EntryType = iota
    EntryTypeDelete
    EntryTypeChannelCreate
    EntryTypeChannelDelete
)
```

**Tasks:**
- [ ] Define Entry and EntryType types
- [ ] Implement Entry.Hash() calculation
- [ ] Implement Entry.Sign() and Entry.Verify()
- [ ] Define State type (merged view)
- [ ] Define Ledger interface

### 2.2 CRDT Implementation
**File:** `pkg/ledger/crdt.go` (new)

Start simple with LWW-Register. For chat, we don't need complex CRDTs - just last-write-wins with deterministic tiebreaking.

```go
// Simple LWW-Register merge
// Sort key: (timestamp, peer_id) - peer_id breaks ties
func CompareEntries(a, b *Entry) int {
    if a.Timestamp != b.Timestamp {
        return int(a.Timestamp - b.Timestamp)
    }
    return strings.Compare(a.Author, b.Author)
}

// Tombstone handling - deleted entries stay in ledger but marked
func (m *CRDTMerger) IsDeleted(entryID string) bool
func (m *CRDTMerger) ApplyTombstone(deleteEntry *Entry) error
```

**Tasks:**
- [ ] Implement timestamp comparison with peer ID tiebreaker
- [ ] Implement tombstone handling (mark deleted, don't remove)
- [ ] Implement merge of entry lists
- [ ] Implement total order computation for display
- [ ] Add conflict detection/logging (for debugging)

### 2.3 Storage Layer
**File:** `pkg/ledger/storage.go` (new)

```go
type SQLiteStorage struct {
    db *sql.DB
}

func (s *SQLiteStorage) Save(entry *Entry) error
func (s *SQLiteStorage) Load(id string) (*Entry, error)
func (s *SQLiteStorage) LoadSince(timestamp int64) ([]Entry, error)
func (s *SQLiteStorage) GetLatestHash() (string, error)
```

**Tasks:**
- [ ] Design SQLite schema
- [ ] Implement CRUD operations
- [ ] Implement batch operations
- [ ] Add indexing for queries
- [ ] Implement state snapshots

### 2.4 HTTP-based Synchronization
**File:** `pkg/ledger/sync.go` (new)

Note: Only nodes with `chat-ledger-key` run full ledger sync. Sync endpoints are at `/ledger/sync`.

```go
type SyncClient struct {
    httpClient *http.Client  // Banyan proxy client
    selfID     peer.ID
    ledgerKey  string        // Service key for auth
}

func (s *SyncClient) FetchEntries(peerAddr string, since string) ([]Entry, error)
func (s *SyncClient) PushEntries(peerAddr string, entries []Entry) error
```

**Tasks:**
- [ ] Implement sync client for ledger node-to-node communication
- [ ] Integrate with `/ledger/sync` handlers
- [ ] Implement periodic sync scheduler (ledger-key nodes only)
- [ ] Handle peer discovery via Banyan beacon/locator (filter by ledger-key)

---

## Phase 3: Chat Service Backend

### 3.1 Service Entry Point
**File:** `apps/chat-service/main.go` (new)

**Important:** Single HTTP server with all endpoints. Router configuration determines which service keys map to which endpoints.

**Tasks:**
- [ ] Load service configuration and available keys
- [ ] Initialize single HTTP server with all route handlers
- [ ] Initialize unified ledger
- [ ] Register all endpoint handlers (/, /ledger/*, /mod/*, /admin/*)
- [ ] Start sync with other ledger nodes (if has ledger key)
- [ ] Graceful shutdown

### 3.2 HTTP Handlers (Single Server)
**File:** `apps/chat-service/handlers.go` (new)

All handlers in one file (or split logically), all part of single HTTP server.

**Endpoints (organized by path, all in same server):**

Path `/` (chat-static-key):
- `GET /` - Serve webapp bundle
- `GET /posts` - List posts
- `GET /channels` - List channels  
- `WS /events` - Real-time updates

Path `/ledger` (chat-ledger-key):
- `POST /ledger/post` - Create post entry
- `POST /ledger/delete` - Delete (tombstone) entry
- `GET /ledger/sync` - Get entries for sync
- `POST /ledger/sync` - Receive entries from peer

Path `/mod` (chat-mod-key):
- `POST /mod/ban` - Ban peer ID (creates ban entry in ledger)
- `POST /mod/delete` - Delete any post (creates delete entry)
- `POST /mod/promote` - Promote user (creates promote entry)

Path `/admin` (chat-admin-key):
- `POST /admin/issue-key` - Issue key (creates issue_key entry)
- `POST /admin/revoke` - Revoke access (creates revoke entry)
- `GET /admin/status` - System status

**Tasks:**
- [ ] Implement all handlers in single server
- [ ] Each handler writes to unified ledger
- [ ] Add rate limiting per endpoint
- [ ] Add content validation
- [ ] Embed and serve frontend bundle

### 3.3 WebSocket Handler
**File:** `apps/chat-service/websocket.go` (new)

**Events:**
- `post:new` - New post created
- `post:delete` - Post deleted
- `sync:state` - State hash update

**Tasks:**
- [ ] Implement WebSocket upgrade at `/events`
- [ ] Implement event broadcasting
- [ ] Implement per-channel subscriptions
- [ ] Add connection management

### 3.4 Authentication Middleware
**File:** `apps/chat-service/auth.go` (new)

**Tasks:**
- [ ] Implement challenge-response flow
- [ ] Verify peer ID signatures
- [ ] Create session tokens
- [ ] Middleware for protected routes

### 3.5 Request Forwarding
**File:** `apps/chat-service/forward.go` (new)

Nodes without a required key need to forward requests to nodes that have it.

**Tasks:**
- [ ] Detect if current node has key for requested path
- [ ] Discover appropriate node via beacon/locator
- [ ] Forward request with user's auth credentials
- [ ] Return response to user

---

## Phase 4: Web Frontend

### 4.1 Project Setup
**Files:** `apps/chat-frontend/` (new)

```bash
npm create vite@latest chat-frontend -- --template react-ts
cd chat-frontend
npm install -D tailwindcss postcss autoprefixer
npx tailwindcss init -p
```

**Tasks:**
- [ ] Initialize Vite + React + TypeScript
- [ ] Configure Tailwind CSS with PostCSS
- [ ] Set up ESLint and Prettier
- [ ] Configure build output for Go embedding (`dist/` folder)

### 4.2 Authentication Component
**File:** `apps/chat-frontend/src/components/Auth.tsx`

**Tasks:**
- [ ] Challenge request UI
- [ ] Signature input/verification
- [ ] Session persistence (localStorage)
- [ ] Logout functionality

### 4.3 Channel List Component
**File:** `apps/chat-frontend/src/components/ChannelList.tsx`

**Tasks:**
- [ ] Fetch and display channels
- [ ] Active channel selection
- [ ] Channel creation (moderator)
- [ ] Unread indicators

### 4.4 Message List Component
**File:** `apps/chat-frontend/src/components/MessageList.tsx`

**Tasks:**
- [ ] Paginated message loading
- [ ] Real-time message updates
- [ ] Message rendering (markdown support)
- [ ] Author identification
- [ ] Delete action (moderator)

### 4.5 Post Composer Component
**File:** `apps/chat-frontend/src/components/PostComposer.tsx`

**Tasks:**
- [ ] Text input with validation
- [ ] Character limit indicator
- [ ] Submit handling
- [ ] Error display

### 4.6 WebSocket Integration
**File:** `apps/chat-frontend/src/ws/client.ts`

**Tasks:**
- [ ] WebSocket connection management
- [ ] Auto-reconnect logic
- [ ] Event dispatching
- [ ] Heartbeat/ping-pong

### 4.7 API Client
**File:** `apps/chat-frontend/src/api/client.ts`

**Tasks:**
- [ ] HTTP client with auth headers
- [ ] Typed API responses
- [ ] Error handling
- [ ] Request retry logic

---

## Phase 5: Integration

### 5.1 Frontend Embedding
**File:** `apps/chat-service/embed.go` (new)

**Tasks:**
- [ ] Add go:embed directive for frontend bundle
- [ ] Build script to compile frontend before Go build
- [ ] Makefile targets for full build

### 5.2 Banyan Addon Registration
**File:** `apps/chat-service/addon.go` (new)

**Tasks:**
- [ ] Implement addon interface
- [ ] Register with service manager
- [ ] Configure routing
- [ ] Handle lifecycle events

### 5.3 Service Key Configuration
**File:** `apps/chat-service/config.go` (new)

**Tasks:**
- [ ] Load service keys from files
- [ ] Configure key permissions
- [ ] Set up route table

### 5.4 End-to-End Testing
**Files:** `tests/e2e/chat_test.go` (new)

**Tasks:**
- [ ] Multi-node test setup
- [ ] Post creation and sync test
- [ ] Deletion propagation test
- [ ] Conflict resolution test
- [ ] WebSocket event test

---

## Phase 6: Documentation

### 6.1 Topology Generator Guide
**File:** `docs/TOPOLOGY_GUIDE.md` (new)

**Contents:**
- [ ] Installation
- [ ] Configuration format reference
- [ ] CLI command reference
- [ ] Key derivation explained
- [ ] Fig template system
- [ ] Common patterns and examples
- [ ] Troubleshooting

### 6.2 Chat App Deployment Guide
**File:** `docs/CHAT_APP_DEPLOYMENT.md` (new)

**Contents:**
- [ ] Architecture overview
- [ ] Prerequisites
- [ ] Generating deployment artifacts
- [ ] Starting nodes
- [ ] Connecting users
- [ ] Key management
- [ ] Monitoring
- [ ] Scaling considerations

### 6.3 Developer Guide
**File:** `docs/DEVELOPER_GUIDE.md` (new)

**Contents:**
- [ ] Banyan service architecture
- [ ] Single HTTP server, single unified ledger
- [ ] Service key tiers and router mapping
- [ ] Request forwarding between nodes
- [ ] Creating a new service
- [ ] Fig file design (base paths only)
- [ ] Using the ledger package (all entry types)
- [ ] HTTP-based ledger sync
- [ ] WebSocket integration
- [ ] Testing services
- [ ] Best practices

---

## Milestones

### Milestone 1: Topology Generator v2
- All Phase 1 tasks complete
- Can generate chat service deployment
- Tests passing

### Milestone 2: Ledger MVP
- Phase 2 tasks complete
- Can create, store, sync entries
- CRDT merge working

### Milestone 3: Backend MVP
- Phase 3 tasks complete
- All endpoints working
- Authentication functional

### Milestone 4: Frontend MVP
- Phase 4 tasks complete
- Can view and post messages
- Real-time updates working

### Milestone 5: Integrated System
- Phase 5 tasks complete
- Multi-node sync working
- E2E tests passing

### Milestone 6: Production Ready
- Phase 6 tasks complete
- Documentation complete
- Ready for release

---

## Risk Mitigation

| Risk | Mitigation |
|------|------------|
| CRDT complexity | Start with simple LWW-Register, iterate |
| Sync performance | Batch operations, lazy loading |
| Frontend bundle size | Code splitting, tree shaking |
| Key management UX | Clear documentation, helper scripts |
| Spam/abuse | Rate limiting, reputation system |
| Request forwarding latency | Cache at edge, optimize discovery |
| Trust tier misconfiguration | Validate fig paths match node keys at startup |

---

## Next Steps

1. **Review this plan** with stakeholders
2. **Set up project tracking** (issues, milestones)
3. **Start Phase 1** (Topology Generator) - no dependencies
4. **Start Phase 2 in parallel** (Ledger Package) - no dependencies
5. **Begin Phase 4** once Phase 3 API contracts are defined
