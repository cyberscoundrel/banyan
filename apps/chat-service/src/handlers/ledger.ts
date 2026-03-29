import { Request, Response } from 'express';
import { v4 as uuidv4 } from 'uuid';
import { createHash } from 'crypto';
import { Entry, EntryType, State, PostData, DeleteData } from '../ledger/index.js';
import { SQLiteStorage } from '../ledger/storage.js';
import { SyncManager } from '../ledger/sync.js';
import { WebSocketHub } from '../websocket.js';
import { isBanned } from '../ledger/crdt.js';

function hashEntry(entry: Omit<Entry, 'hash'>): string {
  const data = `${entry.id}:${entry.type}:${entry.author}:${entry.timestamp}:${JSON.stringify(entry.data)}:${entry.signature}:${entry.prevHash}`;
  return createHash('sha256').update(data).digest('hex');
}

export function createLedgerHandlers(
  storage: SQLiteStorage,
  state: State,
  syncManager: SyncManager | null,
  hub: WebSocketHub
) {
  function createPost(req: Request, res: Response): void {
    if (!req.peerId) {
      res.status(401).json({ error: 'Unauthorized' });
      return;
    }

    if (isBanned(state, req.peerId)) {
      res.status(403).json({ error: 'Banned' });
      return;
    }

    const { content, channel, replyTo } = req.body;
    if (!content || !channel) {
      res.status(400).json({ error: 'Content and channel required' });
      return;
    }

    const data: PostData = { content, channel, replyTo };
    const prevHash = storage.getLatestHash();
    
    const entry: Omit<Entry, 'hash'> = {
      id: uuidv4(),
      type: EntryType.Post,
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
    state.posts.set(fullEntry.id, {
      id: fullEntry.id,
      content,
      channel,
      author: req.peerId,
      timestamp: fullEntry.timestamp,
      replyTo,
      deleted: false,
    });

    hub.broadcast('post:new', { post: fullEntry });
    syncManager?.pushToPeers([fullEntry]);

    res.status(201).json({ entry: fullEntry });
  }

  function deletePost(req: Request, res: Response): void {
    if (!req.peerId) {
      res.status(401).json({ error: 'Unauthorized' });
      return;
    }

    const { postId } = req.body;
    if (!postId) {
      res.status(400).json({ error: 'Post ID required' });
      return;
    }

    const post = state.posts.get(postId);
    if (!post) {
      res.status(404).json({ error: 'Post not found' });
      return;
    }

    if (post.author !== req.peerId) {
      res.status(403).json({ error: 'Not your post' });
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

    hub.broadcast('post:delete', { postId });
    syncManager?.pushToPeers([fullEntry]);

    res.json({ success: true });
  }

  function getSync(req: Request, res: Response): void {
    const since = req.query.since as string;
    const entries = storage.loadSince(since);
    res.json(entries);
  }

  function postSync(req: Request, res: Response): void {
    const entries = req.body as Entry[];
    if (!Array.isArray(entries)) {
      res.status(400).json({ error: 'Expected array of entries' });
      return;
    }

    storage.saveBatch(entries);
    
    for (const entry of entries) {
      state.entries.set(entry.id, entry);
      hub.broadcast('sync:entry', { entry });
    }

    res.json({ received: entries.length });
  }

  return { createPost, deletePost, getSync, postSync };
}
