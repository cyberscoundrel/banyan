package ledger

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

type SQLiteStorage struct {
	db   *sql.DB
	path string
	mu   sync.RWMutex
}

func NewSQLiteStorage(dbPath string) (*SQLiteStorage, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	storage := &SQLiteStorage{
		db:   db,
		path: dbPath,
	}

	if err := storage.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return storage, nil
}

func (s *SQLiteStorage) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS entries (
		id TEXT PRIMARY KEY,
		type TEXT NOT NULL,
		author TEXT NOT NULL,
		timestamp INTEGER NOT NULL,
		data TEXT NOT NULL,
		signature BLOB,
		hash TEXT NOT NULL UNIQUE,
		prev_hash TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_entries_hash ON entries(hash);
	CREATE INDEX IF NOT EXISTS idx_entries_prev_hash ON entries(prev_hash);
	CREATE INDEX IF NOT EXISTS idx_entries_timestamp ON entries(timestamp);
	CREATE INDEX IF NOT EXISTS idx_entries_type ON entries(type);
	CREATE INDEX IF NOT EXISTS idx_entries_author ON entries(author);
	`
	_, err := s.db.Exec(schema)
	return err
}

func (s *SQLiteStorage) Save(entry *Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO entries (id, type, author, timestamp, data, signature, hash, prev_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.Type.String(), entry.Author, entry.Timestamp,
		string(entry.Data), entry.Signature, entry.Hash, entry.PrevHash,
	)
	return err
}

func (s *SQLiteStorage) Load(id string) (*Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var entry Entry
	var entryTypeStr string
	var dataStr string
	var prevHash sql.NullString
	var signature []byte

	err := s.db.QueryRow(`
		SELECT id, type, author, timestamp, data, signature, hash, prev_hash
		FROM entries WHERE id = ?`, id,
	).Scan(&entry.ID, &entryTypeStr, &entry.Author, &entry.Timestamp,
		&dataStr, &signature, &entry.Hash, &prevHash)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	entry.Type, err = EntryTypeFromString(entryTypeStr)
	if err != nil {
		return nil, err
	}
	entry.Data = []byte(dataStr)
	entry.Signature = signature
	if prevHash.Valid {
		entry.PrevHash = prevHash.String
	}

	return &entry, nil
}

func (s *SQLiteStorage) LoadSince(hash string) ([]*Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if hash == "" {
		return s.LoadAll()
	}

	rows, err := s.db.Query(`
		SELECT id, type, author, timestamp, data, signature, hash, prev_hash
		FROM entries WHERE prev_hash = ?`, hash)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanEntries(rows)
}

func (s *SQLiteStorage) LoadAll() ([]*Entry, error) {
	rows, err := s.db.Query(`
		SELECT id, type, author, timestamp, data, signature, hash, prev_hash
		FROM entries ORDER BY timestamp ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanEntries(rows)
}

func (s *SQLiteStorage) scanEntries(rows *sql.Rows) ([]*Entry, error) {
	var entries []*Entry
	for rows.Next() {
		var entry Entry
		var entryTypeStr string
		var dataStr string
		var prevHash sql.NullString
		var signature []byte

		err := rows.Scan(&entry.ID, &entryTypeStr, &entry.Author, &entry.Timestamp,
			&dataStr, &signature, &entry.Hash, &prevHash)
		if err != nil {
			return nil, err
		}

		entry.Type, err = EntryTypeFromString(entryTypeStr)
		if err != nil {
			return nil, err
		}
		entry.Data = []byte(dataStr)
		entry.Signature = signature
		if prevHash.Valid {
			entry.PrevHash = prevHash.String
		}
		entries = append(entries, &entry)
	}
	return entries, rows.Err()
}

func (s *SQLiteStorage) GetLatestHash() (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var hash string
	err := s.db.QueryRow(`
		SELECT hash FROM entries ORDER BY timestamp DESC LIMIT 1`).Scan(&hash)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	return hash, nil
}

func (s *SQLiteStorage) LoadByHash(hash string) (*Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var entry Entry
	var entryTypeStr string
	var dataStr string
	var prevHash sql.NullString
	var signature []byte

	err := s.db.QueryRow(`
		SELECT id, type, author, timestamp, data, signature, hash, prev_hash
		FROM entries WHERE hash = ?`, hash,
	).Scan(&entry.ID, &entryTypeStr, &entry.Author, &entry.Timestamp,
		&dataStr, &signature, &entry.Hash, &prevHash)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	entry.Type, err = EntryTypeFromString(entryTypeStr)
	if err != nil {
		return nil, err
	}
	entry.Data = []byte(dataStr)
	entry.Signature = signature
	if prevHash.Valid {
		entry.PrevHash = prevHash.String
	}

	return &entry, nil
}

func (s *SQLiteStorage) Close() error {
	return s.db.Close()
}

type LedgerImpl struct {
	storage *SQLiteStorage
	merger  *CRDTMerger
	mu      sync.RWMutex
}

func NewLedger(dbPath string) (Ledger, error) {
	storage, err := NewSQLiteStorage(dbPath)
	if err != nil {
		return nil, err
	}

	ledger := &LedgerImpl{
		storage: storage,
		merger:  NewCRDTMerger(),
	}

	entries, err := storage.LoadAll()
	if err != nil {
		storage.Close()
		return nil, err
	}

	for _, entry := range entries {
		if err := ledger.merger.AddEntry(entry); err != nil {
			storage.Close()
			return nil, err
		}
	}

	return ledger, nil
}

func (l *LedgerImpl) Append(entry *Entry) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if err := l.storage.Save(entry); err != nil {
		return err
	}

	return l.merger.AddEntry(entry)
}

func (l *LedgerImpl) Get(id string) (*Entry, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.storage.Load(id)
}

func (l *LedgerImpl) GetSince(hash string) ([]*Entry, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if hash == "" {
		return l.merger.GetAllEntries(), nil
	}

	return l.storage.LoadSince(hash)
}

func (l *LedgerImpl) GetAll() ([]*Entry, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.merger.GetAllEntries(), nil
}

func (l *LedgerImpl) GetLatestHash() (string, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.merger.GetLatestHash(), nil
}

func (l *LedgerImpl) Merge(entries []*Entry) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, entry := range entries {
		if err := l.storage.Save(entry); err != nil {
			return err
		}
		if err := l.merger.AddEntry(entry); err != nil {
			return err
		}
	}
	return nil
}

func (l *LedgerImpl) Close() error {
	return l.storage.Close()
}

func (l *LedgerImpl) GetState() (*State, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.merger.ComputeState(), nil
}
