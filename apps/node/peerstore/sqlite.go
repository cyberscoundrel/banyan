package peerstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// sqliteStore is the embedded, cgo-free backend built on modernc.org/sqlite via
// database/sql. It uses database/sql (not the sqlite-specific API) so the SQL
// layer stays close to what a future Postgres/MySQL backend would need; only
// dialect quirks (upsert syntax) differ.
type sqliteStore struct {
	db       *sql.DB
	observer string
	mu       sync.Mutex // serializes writes; SQLite handles a single writer
}

const sqliteSchema = `
CREATE TABLE IF NOT EXISTS peer_observations (
    service_key  TEXT NOT NULL,
    observer     TEXT NOT NULL,
    peer_id      TEXT NOT NULL,
    multiaddrs   TEXT NOT NULL,          -- JSON array of dial multiaddrs
    source       TEXT NOT NULL,
    last_seen    INTEGER NOT NULL,       -- unix seconds
    last_success INTEGER,                -- unix seconds, NULL if never
    attestation  BLOB,
    PRIMARY KEY (service_key, observer, peer_id)
);
CREATE INDEX IF NOT EXISTS idx_peer_obs_key ON peer_observations(service_key, last_success);
`

func openSQLite(path, observer string) (PeerStore, error) {
	if path == "" {
		return nil, fmt.Errorf("peerstore: empty sqlite path")
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("peerstore: create data dir: %w", err)
		}
	}
	// WAL + a busy timeout keep concurrent reads (warm-up) from tripping over
	// the write path.
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("peerstore: open sqlite: %w", err)
	}
	// A single writer connection avoids "database is locked" churn; SQLite is
	// not a concurrent-writer store.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(sqliteSchema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("peerstore: init schema: %w", err)
	}
	return &sqliteStore{db: db, observer: observer}, nil
}

func (s *sqliteStore) Observer() string { return s.observer }

func (s *sqliteStore) Close() error { return s.db.Close() }

func (s *sqliteStore) Upsert(ctx context.Context, e Entry) error {
	if e.ServiceKey == "" || e.PeerID == "" {
		return fmt.Errorf("peerstore: upsert requires service key and peer id")
	}
	observer := e.Observer
	if observer == "" {
		observer = s.observer
	}
	if e.LastSeen.IsZero() {
		e.LastSeen = time.Now()
	}
	addrsJSON, err := json.Marshal(e.Multiaddrs)
	if err != nil {
		return fmt.Errorf("peerstore: marshal multiaddrs: %w", err)
	}
	source := e.Source
	if source == "" {
		source = SourceService
	}
	var lastSuccess sql.NullInt64
	if e.LastSuccess != nil {
		lastSuccess = sql.NullInt64{Int64: e.LastSuccess.Unix(), Valid: true}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.db.ExecContext(ctx, `
INSERT INTO peer_observations
    (service_key, observer, peer_id, multiaddrs, source, last_seen, last_success, attestation)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(service_key, observer, peer_id) DO UPDATE SET
    multiaddrs   = excluded.multiaddrs,
    source       = excluded.source,
    last_seen    = excluded.last_seen,
    -- never let a nil (candidate-only) observation clobber a real success
    last_success = COALESCE(excluded.last_success, peer_observations.last_success),
    attestation  = COALESCE(excluded.attestation, peer_observations.attestation)
`, e.ServiceKey, observer, e.PeerID, string(addrsJSON), source, e.LastSeen.Unix(), lastSuccess, e.Attestation)
	if err != nil {
		return fmt.Errorf("peerstore: upsert: %w", err)
	}
	return nil
}

func (s *sqliteStore) ForServiceKey(ctx context.Context, serviceKeyHex string, limit int) ([]Entry, error) {
	// NULL last_success sorts last under DESC in SQLite, so verified peers rank
	// above candidate-only ones.
	q := `
SELECT service_key, observer, peer_id, multiaddrs, source, last_seen, last_success, attestation
FROM peer_observations
WHERE service_key = ?
ORDER BY last_success DESC, last_seen DESC`
	args := []any{serviceKeyHex}
	if limit > 0 {
		q += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("peerstore: query service key: %w", err)
	}
	defer rows.Close()
	return scanEntries(rows)
}

func (s *sqliteStore) All(ctx context.Context, perKeyLimit int) ([]Entry, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT service_key, observer, peer_id, multiaddrs, source, last_seen, last_success, attestation
FROM peer_observations
ORDER BY service_key ASC, last_success DESC, last_seen DESC`)
	if err != nil {
		return nil, fmt.Errorf("peerstore: query all: %w", err)
	}
	defer rows.Close()
	all, err := scanEntries(rows)
	if err != nil {
		return nil, err
	}
	if perKeyLimit <= 0 {
		return all, nil
	}
	// Rows are already ordered best-first within each service key; cap per key.
	counts := make(map[string]int)
	out := all[:0]
	for _, e := range all {
		if counts[e.ServiceKey] >= perKeyLimit {
			continue
		}
		counts[e.ServiceKey]++
		out = append(out, e)
	}
	return out, nil
}

func (s *sqliteStore) MarkSuccess(ctx context.Context, serviceKeyHex, peerID string, at time.Time) error {
	if at.IsZero() {
		at = time.Now()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.ExecContext(ctx, `
UPDATE peer_observations
SET last_success = ?, last_seen = ?
WHERE service_key = ? AND observer = ? AND peer_id = ?`,
		at.Unix(), at.Unix(), serviceKeyHex, s.observer, peerID)
	if err != nil {
		return fmt.Errorf("peerstore: mark success: %w", err)
	}
	return nil
}

func (s *sqliteStore) Evict(ctx context.Context, ttl time.Duration) (int, error) {
	if ttl <= 0 {
		return 0, nil
	}
	cutoff := time.Now().Add(-ttl).Unix()
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.ExecContext(ctx, `
DELETE FROM peer_observations
WHERE COALESCE(last_success, last_seen) < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("peerstore: evict: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (s *sqliteStore) CapPerServiceKey(ctx context.Context, max int) (int, error) {
	if max <= 0 {
		return 0, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Keep the top `max` rows per service key (verified and most-recently-seen
	// ranked highest); drop the rest.
	res, err := s.db.ExecContext(ctx, `
DELETE FROM peer_observations
WHERE rowid IN (
  SELECT rowid FROM (
    SELECT rowid, ROW_NUMBER() OVER (
      PARTITION BY service_key
      ORDER BY last_success DESC, last_seen DESC
    ) AS rn
    FROM peer_observations
  ) WHERE rn > ?
)`, max)
	if err != nil {
		return 0, fmt.Errorf("peerstore: cap per key: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func scanEntries(rows *sql.Rows) ([]Entry, error) {
	var out []Entry
	for rows.Next() {
		var (
			e           Entry
			addrsJSON   string
			lastSeen    int64
			lastSuccess sql.NullInt64
			attestation []byte
		)
		if err := rows.Scan(&e.ServiceKey, &e.Observer, &e.PeerID, &addrsJSON, &e.Source, &lastSeen, &lastSuccess, &attestation); err != nil {
			return nil, fmt.Errorf("peerstore: scan: %w", err)
		}
		if addrsJSON != "" {
			if err := json.Unmarshal([]byte(addrsJSON), &e.Multiaddrs); err != nil {
				return nil, fmt.Errorf("peerstore: unmarshal multiaddrs: %w", err)
			}
		}
		e.LastSeen = time.Unix(lastSeen, 0)
		if lastSuccess.Valid {
			t := time.Unix(lastSuccess.Int64, 0)
			e.LastSuccess = &t
		}
		if len(attestation) > 0 {
			e.Attestation = attestation
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Stable ordering guard for callers that don't re-sort.
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].ServiceKey < out[j].ServiceKey
	})
	return out, nil
}
