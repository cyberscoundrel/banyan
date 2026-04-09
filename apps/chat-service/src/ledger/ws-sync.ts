import { Entry, State } from './types.js';
import { SQLiteStorage } from './storage.js';
import { mergeIntoState } from './crdt.js';
import net from 'net';
import crypto from 'crypto';

export class MultisocketSync {
  private proxyUrl: string;
  private figDomain: string;
  private state: State;
  private storage: SQLiteStorage;
  private localPeerId: string;
  private onEntry: (entry: Entry, fromPeerId: string) => void;
  private sock: net.Socket | null = null;
  private seenHashes: Map<string, number> = new Map();
  private closed = false;
  private reconnectTimer?: ReturnType<typeof setTimeout>;
  private pingTimer?: ReturnType<typeof setInterval>;
  private buffer = Buffer.alloc(0);

  constructor(
    state: State,
    storage: SQLiteStorage,
    proxyUrl: string,
    figAlias: string,
    localPeerId: string,
    onEntry: (entry: Entry, fromPeerId: string) => void,
  ) {
    this.state = state;
    this.storage = storage;
    this.proxyUrl = proxyUrl;
    this.figDomain = `${figAlias}.fig`;
    this.localPeerId = localPeerId;
    this.onEntry = onEntry;

    this.pruneSeenHashes();
  }

  start(): void {
    this.connect();
  }

  stop(): void {
    this.closed = true;
    if (this.reconnectTimer) clearTimeout(this.reconnectTimer);
    if (this.pingTimer) clearInterval(this.pingTimer);
    if (this.sock) {
      this.sock.destroy();
      this.sock = null;
    }
  }

  broadcastEntry(entry: Entry): void {
    if (!this.sock) return;
    const payload = JSON.stringify({ d: { type: 'sync:entries', entries: [entry], from: this.localPeerId } });
    this.sendWsFrame(0x1, Buffer.from(payload));
  }

  private connect(): void {
    if (this.closed) return;

    const proxy = new URL(this.proxyUrl);
    const key = crypto.randomBytes(16).toString('base64');
    const upgradeReq = [
      `GET http://${this.figDomain}/ledger/ws-sync?multisocket HTTP/1.1`,
      `Host: ${this.figDomain}`,
      `Upgrade: websocket`,
      `Connection: Upgrade`,
      `Sec-WebSocket-Key: ${key}`,
      `Sec-WebSocket-Version: 13`,
      ``,
      ``,
    ].join('\r\n');

    let connected = false;
    const sock = net.createConnection({ host: proxy.hostname, port: parseInt(proxy.port) || 80 }, () => {
      sock.write(upgradeReq);
    });

    let respData = '';

    const onData = (chunk: Buffer) => {
      respData += chunk.toString();
      if (respData.includes('\r\n\r\n')) {
        sock.removeListener('data', onData);
        const statusLine = respData.split('\r\n')[0];
        if (statusLine.includes('101')) {
          connected = true;
          this.sock = sock;
          this.buffer = Buffer.alloc(0);
          const headerEnd = respData.indexOf('\r\n\r\n') + 4;
          const remaining = respData.substring(headerEnd);
          if (remaining.length > 0) {
            this.buffer = Buffer.from(remaining);
            this.processBuffer();
          }
          sock.on('data', (chunk: Buffer) => {
            this.buffer = Buffer.concat([this.buffer, chunk]);
            this.processBuffer();
          });
          this.pingTimer = setInterval(() => {
            this.sendWsFrame(0x9, Buffer.alloc(0));
          }, 30_000);
          console.log('[ws-sync] Multisocket connected');
        } else {
          console.error(`[ws-sync] WebSocket upgrade failed: ${statusLine}`);
          sock.destroy();
          this.scheduleReconnect();
        }
      }
    };

    sock.on('data', onData);
    sock.on('error', () => {});

    sock.on('close', () => {
      if (connected) {
        if (this.sock === sock) {
          this.sock = null;
          if (this.pingTimer) clearInterval(this.pingTimer);
          console.log('[ws-sync] Disconnected');
          this.scheduleReconnect();
        }
      } else {
        this.scheduleReconnect();
      }
    });
  }

  private processBuffer(): void {
    while (this.buffer.length >= 2) {
      const result = this.tryParseFrame();
      if (result === null) break;
      const { frame, consumed } = result;
      this.buffer = this.buffer.subarray(consumed);
      if (frame !== null) {
        this.handleFrame(frame.opcode, frame.payload);
      }
    }
  }

  private tryParseFrame(): { frame: { opcode: number; payload: Buffer } | null; consumed: number } | null {
    if (this.buffer.length < 2) return null;

    const opcode = this.buffer[0] & 0x0F;
    const masked = (this.buffer[1] & 0x80) !== 0;
    let payloadLen = this.buffer[1] & 0x7F;
    let headerLen = 2;

    if (payloadLen === 126) {
      if (this.buffer.length < 4) return null;
      payloadLen = this.buffer.readUInt16BE(2);
      headerLen = 4;
    } else if (payloadLen === 127) {
      if (this.buffer.length < 10) return null;
      payloadLen = Number(this.buffer.readBigUInt64BE(2));
      headerLen = 10;
    }

    if (masked) headerLen += 4;

    if (this.buffer.length < headerLen + payloadLen) return null;

    let payload = this.buffer.subarray(headerLen, headerLen + payloadLen);
    if (masked) {
      const maskKey = this.buffer.subarray(headerLen - 4, headerLen);
      payload = Buffer.from(payload);
      for (let i = 0; i < payload.length; i++) {
        payload[i] ^= maskKey[i % 4];
      }
    }

    return { frame: { opcode, payload }, consumed: headerLen + payloadLen };
  }

  private handleFrame(opcode: number, payload: Buffer): void {
    switch (opcode) {
      case 0x1:
        this.handleMessage(payload.toString());
        break;
      case 0x8:
        if (this.sock) this.sock.destroy();
        break;
      case 0x9:
        this.sendWsFrame(0xA, payload);
        break;
    }
  }

  private sendWsFrame(opcode: number, payload: Buffer): void {
    if (!this.sock) return;
    const len = payload.length;
    const maskKey = crypto.randomBytes(4);
    const masked = Buffer.from(payload);
    for (let i = 0; i < masked.length; i++) {
      masked[i] ^= maskKey[i % 4];
    }

    let header: Buffer;
    if (len < 126) {
      header = Buffer.alloc(6);
      header[1] = len | 0x80;
      maskKey.copy(header, 2);
    } else if (len < 65536) {
      header = Buffer.alloc(8);
      header[1] = 126 | 0x80;
      header.writeUInt16BE(len, 2);
      maskKey.copy(header, 4);
    } else {
      header = Buffer.alloc(14);
      header[1] = 127 | 0x80;
      header.writeBigUInt64BE(BigInt(len), 2);
      maskKey.copy(header, 10);
    }

    header[0] = 0x80 | opcode;

    this.sock.write(Buffer.concat([header, masked]));
  }

  private handleMessage(raw: string): void {
    let peerId: string;
    let payload: any;

    try {
      const msg = JSON.parse(raw);
      peerId = msg.p;
      payload = typeof msg.d === 'string' ? JSON.parse(msg.d) : msg.d;
    } catch {
      return;
    }

    if (!payload || !payload.type) return;

    if (payload.type === 'sync:full' && Array.isArray(payload.entries)) {
      this.handleFullSync(payload.entries, peerId);
    } else if (payload.type === 'sync:entries' && Array.isArray(payload.entries)) {
      this.handleEntries(payload.entries, peerId);
    }
  }

  private handleFullSync(entries: Entry[], fromPeerId: string): void {
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

    console.log(`[ws-sync] Full sync: ${newEntries.length} new entries from ${fromPeerId}`);

    for (const entry of newEntries) {
      this.onEntry(entry, fromPeerId);
    }
  }

  private handleEntries(entries: Entry[], fromPeerId: string): void {
    const newEntries: Entry[] = [];

    for (const entry of entries) {
      if (!entry.hash || !entry.id) continue;
      if (this.seenHashes.has(entry.hash)) continue;

      if (this.state.entries.has(entry.id) && this.state.entries.get(entry.id)!.hash === entry.hash) {
        this.seenHashes.set(entry.hash, Date.now());
        continue;
      }

      this.seenHashes.set(entry.hash, Date.now());
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

    console.log(`[ws-sync] Received ${newEntries.length} entries from ${fromPeerId}`);

    for (const entry of newEntries) {
      this.onEntry(entry, fromPeerId);
    }

    this.broadcastToPeers(newEntries, fromPeerId);
  }

  private broadcastToPeers(entries: Entry[], fromPeerId: string): void {
    if (!this.sock) return;
    const payload = JSON.stringify({
      not: [fromPeerId],
      d: { type: 'sync:entries', entries, from: this.localPeerId },
    });
    this.sendWsFrame(0x1, Buffer.from(payload));
  }

  private scheduleReconnect(): void {
    if (this.closed) return;
    this.reconnectTimer = setTimeout(() => this.connect(), 5000);
  }

  private pruneSeenHashes(): void {
    setInterval(() => {
      const cutoff = Date.now() - 120_000;
      for (const [hash, ts] of this.seenHashes) {
        if (ts < cutoff) this.seenHashes.delete(hash);
      }
    }, 30_000);
  }
}
