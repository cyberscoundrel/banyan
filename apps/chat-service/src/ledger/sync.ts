import { Entry } from './types.js';
import { SQLiteStorage } from './storage.js';

export class SyncClient {
  private peers: string[];

  constructor(peers: string[]) {
    this.peers = peers;
  }

  async fetchEntries(sinceHash: string): Promise<Entry[]> {
    const results = await Promise.allSettled(
      this.peers.map(peer => this.fetchFromPeer(peer, sinceHash))
    );

    const allEntries: Entry[] = [];
    for (const result of results) {
      if (result.status === 'fulfilled') {
        allEntries.push(...result.value);
      }
    }
    return allEntries;
  }

  private async fetchFromPeer(peer: string, sinceHash: string): Promise<Entry[]> {
    const url = sinceHash 
      ? `${peer}/ledger/sync?since=${encodeURIComponent(sinceHash)}`
      : `${peer}/ledger/sync`;
    
    const response = await fetch(url, {
      method: 'GET',
      headers: { 'Accept': 'application/json' },
    });

    if (!response.ok) {
      throw new Error(`Sync failed: ${response.status}`);
    }

    return response.json();
  }

  async pushEntries(peer: string, entries: Entry[]): Promise<void> {
    const response = await fetch(`${peer}/ledger/sync`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(entries),
    });

    if (!response.ok) {
      throw new Error(`Push failed: ${response.status}`);
    }
  }
}

export class SyncManager {
  private storage: SQLiteStorage;
  private client: SyncClient;
  private interval: number;
  private timer?: ReturnType<typeof setInterval>;
  private running = false;

  constructor(storage: SQLiteStorage, peers: string[], interval: number) {
    this.storage = storage;
    this.client = new SyncClient(peers);
    this.interval = interval;
  }

  start(): void {
    if (this.running) return;
    this.running = true;
    this.sync();
    this.timer = setInterval(() => this.sync(), this.interval);
  }

  stop(): void {
    this.running = false;
    if (this.timer) {
      clearInterval(this.timer);
      this.timer = undefined;
    }
  }

  private async sync(): Promise<void> {
    try {
      const latestHash = this.storage.getLatestHash();
      const remoteEntries = await this.client.fetchEntries(latestHash);
      
      if (remoteEntries.length > 0) {
        this.storage.saveBatch(remoteEntries);
      }
    } catch (err) {
      console.error('Sync error:', err);
    }
  }

  async pushToPeers(entries: Entry[]): Promise<void> {
    await Promise.allSettled(
      this.client['peers'].map(peer => this.client.pushEntries(peer, entries))
    );
  }
}
