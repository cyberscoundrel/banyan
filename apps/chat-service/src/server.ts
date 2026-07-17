import express from 'express';
import cors from 'cors';
import { Server } from 'http';
import { join, resolve } from 'path';
import { existsSync } from 'fs';

import { Config } from './config.js';
import { State, SQLiteStorage, MultisocketSync, WsSyncServer, Entry } from './ledger/index.js';
import { WebSocketHub } from './websocket.js';
import { createStaticHandlers } from './handlers/static.js';
import { createLedgerHandlers } from './handlers/ledger.js';
import { createModHandlers } from './handlers/mod.js';
import { createAdminHandlers, ServiceKey } from './handlers/admin.js';
import { createAuthMiddleware } from './auth.js';

export function createApp(config: Config): { app: express.Application; server: Server; hub: WebSocketHub; storage: SQLiteStorage } {
  const app = express();
  const server = new Server(app);

  app.use(cors());
  app.use(express.json({
    verify: (req, _buf, _encoding) => {
      (req as any).rawBody = _buf.toString();
    },
  }));

  app.use((req, _res, next) => {
    const peerId = req.headers['x-peer-id'] as string | undefined;
    if (peerId) {
      req.peerId = peerId;
    }
    next();
  });

  const auth = createAuthMiddleware();
  app.post('/auth/challenge', auth.requestChallenge);
  app.post('/auth/verify', auth.verifyChallenge);

  const frontendPath = resolve('./frontend/dist');
  const frontendExists = existsSync(join(frontendPath, 'index.html'));
  const staticHandlers = createStaticHandlers(frontendExists ? frontendPath : '');

  app.get('/', staticHandlers.serveIndex);
  if (frontendExists) {
    app.use(express.static(frontendPath, { index: false }));
  }

  const dbPath = join(config.dataDir, 'chat.db');
  const storage = new SQLiteStorage(dbPath);
  const hub = new WebSocketHub();

  if (config.hasLedgerKey) {
    const state: State = storage.loadState();
    const keys = new Map<string, ServiceKey>();

    let multisocketSync: MultisocketSync | null = null;

    if (config.syncEnabled && config.proxyUrl && config.figAlias) {
      console.log(`Multisocket sync via proxy: ${config.proxyUrl} (fig alias: ${config.figAlias})`);

      multisocketSync = new MultisocketSync(
        state, storage,
        config.proxyUrl, config.figAlias,
        config.nodeId,
        (entry: Entry, _fromPeerId: string) => {
          const d = entry.data as Record<string, any>;
          if (entry.type === 'post') {
            hub.broadcast('post:new', { entry });
          } else if (entry.type === 'delete') {
            hub.broadcast('post:delete', { postId: d.postId });
          } else if (entry.type === 'channelCreate') {
            hub.broadcast('channel:new', { channel: state.channels.get(d.name) });
          } else if (entry.type === 'channelDelete') {
            hub.broadcast('channel:delete', { name: d.name });
          } else if (entry.type === 'ban') {
            hub.broadcast('mod:ban', { ban: entry.data });
          } else if (entry.type === 'promote') {
            hub.broadcast('mod:promote', { user: entry.data });
          }
        },
      );
      multisocketSync.start();
    }

    const syncBroadcast = multisocketSync ? (entries: Entry[]) => {
      for (const entry of entries) {
        multisocketSync!.broadcastEntry(entry);
      }
    } : undefined;

    let wsSyncServer: WsSyncServer | null = null;
    if (multisocketSync) {
      wsSyncServer = new WsSyncServer(state, storage, (entry: Entry, _fromPeerId: string) => {
        const d = entry.data as Record<string, any>;
        if (entry.type === 'post') {
          hub.broadcast('post:new', { entry });
        } else if (entry.type === 'delete') {
          hub.broadcast('post:delete', { postId: d.postId });
        } else if (entry.type === 'channelCreate') {
          hub.broadcast('channel:new', { channel: state.channels.get(d.name) });
        } else if (entry.type === 'channelDelete') {
          hub.broadcast('channel:delete', { name: d.name });
        } else if (entry.type === 'ban') {
          hub.broadcast('mod:ban', { ban: entry.data });
        } else if (entry.type === 'promote') {
          hub.broadcast('mod:promote', { user: entry.data });
        }
      }, config.nodeId);
    }

    hub.attach(server);
    server.removeAllListeners('upgrade');
    server.on('upgrade', (req, socket, head) => {
      const pathname = req.url?.split('?')[0] || '';
      if (pathname === '/ledger/ws-sync' && wsSyncServer) {
        wsSyncServer.handleUpgrade(req, socket, head);
      } else if (pathname === '/ledger/events') {
        hub.wss.handleUpgrade(req, socket, head, (ws) => {
          hub.wss.emit('connection', ws, req);
        });
      } else {
        socket.destroy();
      }
    });

    const ledgerHandlers = createLedgerHandlers(storage, state, hub, syncBroadcast);

    app.get('/ledger/channels', ledgerHandlers.listChannels);
    app.post('/ledger/channels', ledgerHandlers.createChannel);
    app.get('/ledger/posts', ledgerHandlers.listPosts);
    app.post('/ledger/post', ledgerHandlers.createPost);
    app.post('/ledger/delete', ledgerHandlers.deletePost);
    app.get('/ledger/sync', ledgerHandlers.getSync);
    app.post('/ledger/sync', ledgerHandlers.postSync);

    if (config.hasModKey) {
      const modHandlers = createModHandlers(storage, state, hub, syncBroadcast);
      app.post('/mod/ban', modHandlers.banUser);
      app.post('/mod/delete', modHandlers.modDeletePost);
      app.post('/mod/promote', modHandlers.promoteUser);
    }

    if (config.hasAdminKey) {
      const adminHandlers = createAdminHandlers(storage, state, hub, keys, syncBroadcast);
      app.post('/admin/issue-key', adminHandlers.issueKey);
      app.post('/admin/revoke', adminHandlers.revokeKey);
      app.get('/admin/status', adminHandlers.getStatus);
    }
  }

  app.get('/health', (_req, res) => {
    res.json({ status: 'ok', nodeId: config.nodeId });
  });

  app.get('/peer-info', (req, res) => {
    res.json({
      peerId: req.peerId || 'anonymous',
    });
  });

  app.use((err: Error, _req: express.Request, res: express.Response, _next: express.NextFunction) => {
    console.error('Error:', err);
    res.status(500).json({ error: 'Internal server error' });
  });

  return { app, server, hub, storage };
}
