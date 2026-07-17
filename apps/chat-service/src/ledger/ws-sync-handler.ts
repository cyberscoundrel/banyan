import { WebSocket, WebSocketServer } from 'ws';
import { Entry, State } from './types.js';
import { SQLiteStorage } from './storage.js';
import { mergeIntoState } from './crdt.js';

export class WsSyncServer {
  private wss: WebSocketServer;
  private state: State;
  private storage: SQLiteStorage;
  private onEntry: (entry: Entry, fromPeerId: string) => void;
  private localPeerId: string;

  constructor(
    state: State,
    storage: SQLiteStorage,
    onEntry: (entry: Entry, fromPeerId: string) => void,
    localPeerId = '',
  ) {
    this.state = state;
    this.storage = storage;
    this.onEntry = onEntry;
    this.localPeerId = localPeerId;

    this.wss = new WebSocketServer({ noServer: true });

    this.wss.on('connection', (ws: WebSocket) => {
      console.log('[ws-sync-server] New connection');

      const allEntries = Array.from(this.state.entries.values());
      if (allEntries.length > 0) {
        const fullSync = JSON.stringify({ type: 'sync:full', entries: allEntries });
        ws.send(fullSync);
        console.log(`[ws-sync-server] Sent full sync: ${allEntries.length} entries`);
      }

      ws.on('message', (data: Buffer) => {
        try {
          const raw = data.toString();
          const msg = JSON.parse(raw);
          if (msg.type === 'sync:entries' && Array.isArray(msg.entries)) {
            const fromPeerId = msg.from || 'unknown';
            this.handleSyncEntries(ws, msg.entries, fromPeerId);
          } else if (msg.type === 'sync:full' && Array.isArray(msg.entries)) {
            const fromPeerId = msg.from || 'unknown';
            this.handleSyncEntries(ws, msg.entries, fromPeerId);
          } else if (msg.type === 'sync:digest' && Array.isArray(msg.hashes)) {
            this.handleDigest(ws, msg.hashes);
          }
        } catch (e) {
          console.error('[ws-sync-server] Failed to parse message:', e);
        }
      });

      ws.on('error', (err: Error) => {
        console.log('[ws-sync-server] WebSocket error:', err.message);
      });

      ws.on('close', () => {
        console.log('[ws-sync-server] Connection closed');
      });
    });
  }

  handleUpgrade(req: any, socket: any, head: Buffer): void {
    this.wss.handleUpgrade(req, socket, head, (ws) => {
      this.wss.emit('connection', ws, req);
    });
  }

  private relayToOtherClients(sender: WebSocket, entries: Entry[], fromPeerId: string): void {
    const msg = JSON.stringify({ type: 'sync:entries', entries, from: fromPeerId });
    for (const client of this.wss.clients) {
      if (client !== sender && client.readyState === WebSocket.OPEN) {
        client.send(msg);
      }
    }
  }

  // handleDigest answers a peer's anti-entropy digest: the peer sent the set of
  // entry hashes it holds, so we reply with every entry it's missing. Hash-based,
  // so it needs no clocks and is idempotent (the peer's CRDT merge dedupes).
  private handleDigest(ws: WebSocket, hashes: string[]): void {
    const have = new Set(hashes);
    const missing: Entry[] = [];
    for (const entry of this.state.entries.values()) {
      if (entry.hash && !have.has(entry.hash)) {
        missing.push(entry);
      }
    }
    if (missing.length === 0) return;
    ws.send(JSON.stringify({ type: 'sync:entries', entries: missing, from: this.localPeerId }));
    console.log(`[ws-sync-server] Digest reconcile: sent ${missing.length} missing entries`);
  }

  private handleSyncEntries(ws: WebSocket, entries: Entry[], fromPeerId: string): void {
    const newEntries: Entry[] = [];

    for (const entry of entries) {
      if (!entry.hash || !entry.id) continue;
      if (this.state.entries.has(entry.id) && this.state.entries.get(entry.id)!.hash === entry.hash) {
        continue;
      }
      newEntries.push(entry);
    }

    if (newEntries.length === 0) return;

    this.storage.saveBatch(newEntries);
    const newState = mergeIntoState(this.state, newEntries);
    this.state.entries = newState.entries;
    this.state.posts = newState.posts;
    this.state.channels = newState.channels;
    this.state.bans = newState.bans;
    this.state.users = newState.users;
    this.state.latestHash = newState.latestHash;

    console.log(`[ws-sync-server] Received ${newEntries.length} entries from ${fromPeerId || 'unknown'}`);

    for (const entry of newEntries) {
      this.onEntry(entry, fromPeerId);
    }

    this.relayToOtherClients(ws, newEntries, fromPeerId);
  }

  close(): void {
    this.wss.close();
  }
}
