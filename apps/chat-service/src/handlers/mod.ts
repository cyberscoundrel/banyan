import { Request, Response } from 'express';
import { v4 as uuidv4 } from 'uuid';
import { createHash } from 'crypto';
import { Entry, EntryType, State, BanData, DeleteData, PromoteData } from '../ledger/index.js';
import { SQLiteStorage } from '../ledger/storage.js';
import { SyncManager } from '../ledger/sync.js';
import { WebSocketHub } from '../websocket.js';
import { getUserRole } from '../ledger/crdt.js';

function hashEntry(entry: Omit<Entry, 'hash'>): string {
  const data = `${entry.id}:${entry.type}:${entry.author}:${entry.timestamp}:${JSON.stringify(entry.data)}:${entry.signature}:${entry.prevHash}`;
  return createHash('sha256').update(data).digest('hex');
}

export function createModHandlers(
  storage: SQLiteStorage,
  state: State,
  syncManager: SyncManager | null,
  hub: WebSocketHub
) {
  function banUser(req: Request, res: Response): void {
    if (!req.peerId) {
      res.status(401).json({ error: 'Unauthorized' });
      return;
    }

    const { peerId, reason } = req.body;
    if (!peerId) {
      res.status(400).json({ error: 'Peer ID required' });
      return;
    }

    const data: BanData = { peerId, reason };
    const prevHash = storage.getLatestHash();

    const entry: Omit<Entry, 'hash'> = {
      id: uuidv4(),
      type: EntryType.Ban,
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
    state.bans.set(peerId, {
      peerId,
      reason,
      bannedAt: fullEntry.timestamp,
      bannedBy: req.peerId,
    });

    hub.broadcast('mod:ban', { peerId, reason });
    syncManager?.pushToPeers([fullEntry]);

    res.json({ success: true });
  }

  function modDeletePost(req: Request, res: Response): void {
    if (!req.peerId) {
      res.status(401).json({ error: 'Unauthorized' });
      return;
    }

    const { postId, reason } = req.body;
    if (!postId) {
      res.status(400).json({ error: 'Post ID required' });
      return;
    }

    const post = state.posts.get(postId);
    if (!post) {
      res.status(404).json({ error: 'Post not found' });
      return;
    }

    const data: DeleteData = { postId };
    const prevHash = storage.getLatestHash();

    const entry: Omit<Entry, 'hash'> = {
      id: uuidv4(),
      type: EntryType.Delete,
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
    post.deleted = true;

    hub.broadcast('post:delete', { postId, reason, moderator: req.peerId });
    syncManager?.pushToPeers([fullEntry]);

    res.json({ success: true });
  }

  function promoteUser(req: Request, res: Response): void {
    if (!req.peerId) {
      res.status(401).json({ error: 'Unauthorized' });
      return;
    }

    const { peerId, role } = req.body;
    if (!peerId || !role) {
      res.status(400).json({ error: 'Peer ID and role required' });
      return;
    }

    if (role !== 'moderator' && role !== 'admin') {
      res.status(400).json({ error: 'Invalid role' });
      return;
    }

    const data: PromoteData = { peerId, role };
    const prevHash = storage.getLatestHash();

    const entry: Omit<Entry, 'hash'> = {
      id: uuidv4(),
      type: EntryType.Promote,
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
    state.users.set(peerId, {
      peerId,
      role,
      promotedAt: fullEntry.timestamp,
      promotedBy: req.peerId,
    });

    hub.broadcast('mod:promote', { peerId, role });
    syncManager?.pushToPeers([fullEntry]);

    res.json({ success: true });
  }

  return { banUser, modDeletePost, promoteUser };
}
