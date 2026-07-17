// Package peerstore provides durable, service-key-scoped persistence of
// observed peers ("the persistence tree"). It records, per service key, the
// peers a node has seen advertise/serve that key together with the multiaddrs
// needed to redial them. On restart the node warm-dials these entries instead
// of rediscovering everything from scratch through the DHT, and fig
// participants can exchange verified entries over libp2p (PEX).
//
// All node code depends on the PeerStore interface, never on a concrete
// database. The only backend shipped today is an embedded, cgo-free SQLite
// store (see sqlite.go). The Open factory reserves a DSN field so a future
// external/shared SQL backend (e.g. Postgres/MySQL for a trusted group that
// runs one curated, live-sanitized tree) can drop in without touching callers.
package peerstore

import (
	"context"
	"errors"
	"time"
)

// Source describes how an entry was learned. It maps onto the connection
// manager's ConnectionType provenance.
const (
	SourceManual   = "manual"
	SourceService  = "service"
	SourceGossip   = "gossip"
	SourceImported = "imported"
)

// Entry is one observation: "peer PeerID, seen by Observer, serves ServiceKey,
// reachable at Multiaddrs". IDs are stored as strings so the package carries no
// libp2p dependency, which keeps it trivially testable and portable to an
// external SQL backend.
type Entry struct {
	// ServiceKey is the hex of the marshalled service public key. This is the
	// same identifier used in routing (/router/<serviceKeyHex>/...).
	ServiceKey string
	// Observer is the peer ID (string) of the node that made this observation.
	// For the embedded single-node store this is always this node's own ID; the
	// column exists so a future shared table can hold many observers and rank a
	// subject by independent-confirmation count.
	Observer string
	// PeerID is the subject peer ID (string) being described.
	PeerID string
	// Multiaddrs are dial candidates for the subject, including /p2p-circuit
	// relay addresses (how a NAT-stuck peer advertises reachability).
	Multiaddrs []string
	// Source records provenance (manual | service | gossip | imported).
	Source string
	// LastSeen is when this entry was last observed/updated.
	LastSeen time.Time
	// LastSuccess is the last time this node successfully dialed the subject.
	// Nil means never successfully dialed (dial candidate only).
	LastSuccess *time.Time
	// Attestation is an optional signed claim binding {observer, subject,
	// serviceKey, addrs, observedAt}. Nil until signing/sharing is enabled.
	Attestation []byte
}

// PeerStore is the durable, service-key-scoped peer store. Implementations must
// be safe for concurrent use.
type PeerStore interface {
	// Upsert inserts or updates an entry. If e.Observer is empty it defaults to
	// this store's Observer(). Multiaddrs replace any previously stored set for
	// the (serviceKey, observer, peerID) tuple. A nil LastSuccess never clobbers
	// an existing non-nil value.
	Upsert(ctx context.Context, e Entry) error

	// ForServiceKey returns up to limit entries for a service key, best first
	// (successfully-dialed and most-recently-seen ranked highest). A limit <= 0
	// means no limit.
	ForServiceKey(ctx context.Context, serviceKeyHex string, limit int) ([]Entry, error)

	// All returns entries across all service keys, capped at perKeyLimit per
	// service key (perKeyLimit <= 0 means no per-key cap). Used for startup
	// warm-up.
	All(ctx context.Context, perKeyLimit int) ([]Entry, error)

	// MarkSuccess records a successful dial of peerID for serviceKeyHex at time
	// at, for this store's Observer().
	MarkSuccess(ctx context.Context, serviceKeyHex, peerID string, at time.Time) error

	// Evict deletes entries whose most recent activity (last_success, else
	// last_seen) is older than ttl. Returns the number of rows removed. A ttl
	// <= 0 is a no-op.
	Evict(ctx context.Context, ttl time.Duration) (int, error)

	// CapPerServiceKey enforces a hard upper bound of max entries per service
	// key, dropping the lowest-ranked (never/least-recently successful) beyond
	// the cap. Returns the number of rows removed. A max <= 0 is a no-op.
	CapPerServiceKey(ctx context.Context, max int) (int, error)

	// Observer returns this store's self observer ID (used as the default
	// Observer on Upsert/MarkSuccess).
	Observer() string

	// Close releases underlying resources.
	Close() error
}

// Config selects and configures a backend.
type Config struct {
	// DSN, when non-empty, selects an external SQL backend. Reserved for a
	// future shared-store implementation; currently returns ErrExternalUnsupported.
	DSN string
	// Path is the embedded SQLite file path (used when DSN is empty).
	Path string
	// Observer is this node's own peer ID, stored on entries it writes.
	Observer string
}

// ErrExternalUnsupported is returned when a DSN is provided but no external
// backend is compiled in yet. The seam exists so callers need not change when
// one is added.
var ErrExternalUnsupported = errors.New("peerstore: external SQL backend not implemented; leave -peerstore-dsn unset to use the embedded store")

// Open returns a PeerStore for the given config. With an empty DSN it opens the
// embedded SQLite store at Config.Path.
func Open(cfg Config) (PeerStore, error) {
	if cfg.DSN != "" {
		return nil, ErrExternalUnsupported
	}
	return openSQLite(cfg.Path, cfg.Observer)
}
