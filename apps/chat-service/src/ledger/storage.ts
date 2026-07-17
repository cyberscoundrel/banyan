import Database from 'better-sqlite3';
import { Entry, State, EntryType } from './types.js';
import { mergeIntoState } from './crdt.js';
import { mkdirSync, existsSync } from 'fs';
import { dirname, resolve } from 'path';

export class SQLiteStorage {
  private db: Database.Database;

  constructor(dbPath: string) {
    const dir = dirname(dbPath);
    if (!existsSync(dir)) {
      mkdirSync(dir, { recursive: true });
    }

    this.db = new Database(resolve(dbPath));
    this.db.pragma('journal_mode = WAL');
    this.initSchema();
  }

  private initSchema(): void {
    this.db.exec(`
      CREATE TABLE IF NOT EXISTS entries (
        id TEXT PRIMARY KEY,
        type TEXT NOT NULL,
        author TEXT NOT NULL,
        timestamp INTEGER NOT NULL,
        data TEXT NOT NULL,
        signature TEXT NOT NULL,
        hash TEXT NOT NULL,
        prev_hash TEXT NOT NULL,
        deleted INTEGER DEFAULT 0
      );
      CREATE INDEX IF NOT EXISTS idx_entries_timestamp ON entries(timestamp);
      CREATE INDEX IF NOT EXISTS idx_entries_author ON entries(author);
      CREATE INDEX IF NOT EXISTS idx_entries_type ON entries(type);
    `);
  }

  save(entry: Entry): void {
    const stmt = this.db.prepare(`
      INSERT OR REPLACE INTO entries (id, type, author, timestamp, data, signature, hash, prev_hash, deleted)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
    `);
    stmt.run(
      entry.id,
      entry.type,
      entry.author,
      entry.timestamp,
      JSON.stringify(entry.data),
      entry.signature,
      entry.hash,
      entry.prevHash,
      entry.deleted ? 1 : 0
    );
  }

  saveBatch(entries: Entry[]): void {
    const stmt = this.db.prepare(`
      INSERT OR REPLACE INTO entries (id, type, author, timestamp, data, signature, hash, prev_hash, deleted)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
    `);
    const insertMany = this.db.transaction((items: Entry[]) => {
      for (const entry of items) {
        stmt.run(
          entry.id,
          entry.type,
          entry.author,
          entry.timestamp,
          JSON.stringify(entry.data),
          entry.signature,
          entry.hash,
          entry.prevHash,
          entry.deleted ? 1 : 0
        );
      }
    });
    insertMany(entries);
  }

  load(id: string): Entry | null {
    const stmt = this.db.prepare('SELECT * FROM entries WHERE id = ?');
    const row = stmt.get(id) as any;
    if (!row) return null;
    return this.rowToEntry(row);
  }

  loadAll(): Entry[] {
    const stmt = this.db.prepare('SELECT * FROM entries ORDER BY timestamp ASC');
    const rows = stmt.all() as any[];
    return rows.map(r => this.rowToEntry(r));
  }

  loadSince(sinceHash: string): Entry[] {
    if (!sinceHash) {
      return this.loadAll();
    }
    const hashRow = this.db.prepare('SELECT timestamp FROM entries WHERE hash = ?').get(sinceHash) as any;
    if (!hashRow) {
      return this.loadAll();
    }
    const stmt = this.db.prepare('SELECT * FROM entries WHERE timestamp > ? ORDER BY timestamp ASC');
    const rows = stmt.all(hashRow.timestamp) as any[];
    return rows.map(r => this.rowToEntry(r));
  }

  getLatestHash(): string {
    const stmt = this.db.prepare('SELECT hash FROM entries ORDER BY timestamp DESC LIMIT 1');
    const row = stmt.get() as any;
    return row?.hash || '';
  }

  loadState(): State {
    const entries = this.loadAll();
    const state: State = {
      entries: new Map(),
      posts: new Map(),
      channels: new Map(),
      bans: new Map(),
      users: new Map(),
      latestHash: '',
    };
    return mergeIntoState(state, entries);
  }

  private rowToEntry(row: any): Entry {
    return {
      id: row.id,
      type: row.type as EntryType,
      author: row.author,
      timestamp: row.timestamp,
      data: JSON.parse(row.data),
      signature: row.signature,
      hash: row.hash,
      prevHash: row.prev_hash,
      deleted: row.deleted === 1,
    };
  }

  close(): void {
    this.db.close();
  }
}
