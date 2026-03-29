import express from 'express';
import cors from 'cors';
import { Server } from 'http';
import { join, resolve } from 'path';
import { existsSync } from 'fs';

import { Config } from './config.js';
import { State, SQLiteStorage, SyncManager } from './ledger/index.js';
import { WebSocketHub } from './websocket.js';
import { createAuthMiddleware } from './auth.js';
import { createStaticHandlers } from './handlers/static.js';
import { createLedgerHandlers } from './handlers/ledger.js';
import { createModHandlers } from './handlers/mod.js';
import { createAdminHandlers, ServiceKey } from './handlers/admin.js';

export function createApp(config: Config): { app: express.Application; server: Server; hub: WebSocketHub; storage: SQLiteStorage } {
  const app = express();
  const server = new Server(app);

  app.use(cors());
  app.use(express.json());

  const dbPath = join(config.dataDir, 'chat.db');
  const storage = new SQLiteStorage(dbPath);
  const state: State = storage.loadState();
  const keys = new Map<string, ServiceKey>();

  const hub = new WebSocketHub(server);
  const auth = createAuthMiddleware();

  let syncManager: SyncManager | null = null;
  if (config.syncEnabled && config.syncPeers.length > 0) {
    syncManager = new SyncManager(storage, config.syncPeers, config.syncInterval);
  }

  const frontendPath = resolve('./frontend/dist');
  const frontendExists = existsSync(join(frontendPath, 'index.html'));
  const staticHandlers = createStaticHandlers(state, frontendExists ? frontendPath : '');

  app.post('/auth/challenge', auth.requestChallenge);
  app.post('/auth/verify', auth.verifyChallenge);

  app.get('/', staticHandlers.serveIndex);
  app.get('/posts', staticHandlers.listPosts);
  app.get('/channels', staticHandlers.listChannels);

  if (frontendExists) {
    app.use(express.static(frontendPath, { index: false }));
  }

  if (config.hasLedgerKey) {
    const ledgerHandlers = createLedgerHandlers(storage, state, syncManager, hub);
    app.post('/ledger/post', auth.authMiddleware, auth.requireAuth, ledgerHandlers.createPost);
    app.post('/ledger/delete', auth.authMiddleware, auth.requireAuth, ledgerHandlers.deletePost);
    app.get('/ledger/sync', ledgerHandlers.getSync);
    app.post('/ledger/sync', ledgerHandlers.postSync);
  }

  if (config.hasModKey) {
    const modHandlers = createModHandlers(storage, state, syncManager, hub);
    app.post('/mod/ban', auth.authMiddleware, auth.requireAuth, modHandlers.banUser);
    app.post('/mod/delete', auth.authMiddleware, auth.requireAuth, modHandlers.modDeletePost);
    app.post('/mod/promote', auth.authMiddleware, auth.requireAuth, modHandlers.promoteUser);
  }

  if (config.hasAdminKey) {
    const adminHandlers = createAdminHandlers(storage, state, syncManager, hub, keys);
    app.post('/admin/issue-key', auth.authMiddleware, auth.requireAuth, adminHandlers.issueKey);
    app.post('/admin/revoke', auth.authMiddleware, auth.requireAuth, adminHandlers.revokeKey);
    app.get('/admin/status', auth.authMiddleware, auth.requireAuth, adminHandlers.getStatus);
  }

  app.get('/health', (_req, res) => {
    res.json({ status: 'ok', nodeId: config.nodeId });
  });

  app.use((err: Error, _req: express.Request, res: express.Response, _next: express.NextFunction) => {
    console.error('Error:', err);
    res.status(500).json({ error: 'Internal server error' });
  });

  return { app, server, hub, storage };
}
