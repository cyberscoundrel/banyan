import { Request, Response } from 'express';
import { v4 as uuidv4 } from 'uuid';
import { createHash } from 'crypto';
import { Entry, EntryType, State, IssueKeyData, RevokeData } from '../ledger/index.js';
import { SQLiteStorage } from '../ledger/storage.js';
import { WebSocketHub } from '../websocket.js';

function hashEntry(entry: Omit<Entry, 'hash'>): string {
  const data = `${entry.id}:${entry.type}:${entry.author}:${entry.timestamp}:${JSON.stringify(entry.data)}:${entry.signature}:${entry.prevHash}`;
  return createHash('sha256').update(data).digest('hex');
}

export interface ServiceKey {
  id: string;
  peerId: string;
  keyType: 'static' | 'ledger' | 'mod' | 'admin';
  issuedAt: number;
  issuedBy: string;
  revoked: boolean;
}

export function createAdminHandlers(
  storage: SQLiteStorage,
  state: State,
  hub: WebSocketHub,
  keys: Map<string, ServiceKey>,
  syncBroadcast?: (entries: Entry[]) => void,
) {
  function issueKey(req: Request, res: Response): void {
    if (!req.peerId) {
      res.status(401).json({ error: 'Unauthorized' });
      return;
    }

    const { peerId, keyType } = req.body;
    if (!peerId || !keyType) {
      res.status(400).json({ error: 'Peer ID and key type required' });
      return;
    }

    const validTypes = ['static', 'ledger', 'mod', 'admin'];
    if (!validTypes.includes(keyType)) {
      res.status(400).json({ error: 'Invalid key type' });
      return;
    }

    const keyId = uuidv4();
    const key: ServiceKey = {
      id: keyId,
      peerId,
      keyType,
      issuedAt: Date.now(),
      issuedBy: req.peerId,
      revoked: false,
    };

    const data: IssueKeyData = { peerId, keyType };
    const prevHash = storage.getLatestHash();

    const entry: Omit<Entry, 'hash'> = {
      id: keyId,
      type: EntryType.IssueKey,
      author: req.peerId,
      timestamp: Date.now(),
      data,
      signature: '',
      prevHash,
    };

    const fullEntry: Entry = {
      ...entry,
      hash: hashEntry(entry),
    };

    storage.save(fullEntry);
    state.entries.set(fullEntry.id, fullEntry);
    keys.set(keyId, key);

    hub.broadcast('admin:issue-key', { keyId, peerId, keyType });
    syncBroadcast?.([fullEntry]);

    res.status(201).json({ keyId, key });
  }

  function revokeKey(req: Request, res: Response): void {
    if (!req.peerId) {
      res.status(401).json({ error: 'Unauthorized' });
      return;
    }

    const { peerId, keyType } = req.body;
    if (!peerId) {
      res.status(400).json({ error: 'Peer ID required' });
      return;
    }

    const data: RevokeData = { peerId, keyType };
    const prevHash = storage.getLatestHash();

    const entry: Omit<Entry, 'hash'> = {
      id: uuidv4(),
      type: EntryType.Revoke,
      author: req.peerId,
      timestamp: Date.now(),
      data,
      signature: '',
      prevHash,
    };

    const fullEntry: Entry = {
      ...entry,
      hash: hashEntry(entry),
    };

    storage.save(fullEntry);
    state.entries.set(fullEntry.id, fullEntry);

    for (const [id, key] of keys) {
      if (key.peerId === peerId && (!keyType || key.keyType === keyType)) {
        key.revoked = true;
      }
    }

    hub.broadcast('admin:revoke', { peerId, keyType });
    syncBroadcast?.([fullEntry]);

    res.json({ success: true });
  }

  function getStatus(_req: Request, res: Response): void {
    const activeKeys = Array.from(keys.values()).filter(k => !k.revoked);
    const entries = state.entries.size;
    const posts = state.posts.size;
    const channels = state.channels.size;
    const bans = state.bans.size;

    res.json({
      keys: {
        total: keys.size,
        active: activeKeys.length,
        byType: {
          static: activeKeys.filter(k => k.keyType === 'static').length,
          ledger: activeKeys.filter(k => k.keyType === 'ledger').length,
          mod: activeKeys.filter(k => k.keyType === 'mod').length,
          admin: activeKeys.filter(k => k.keyType === 'admin').length,
        },
      },
      ledger: {
        entries,
        posts,
        channels,
        bans,
      },
    });
  }

  return { issueKey, revokeKey, getStatus };
}
