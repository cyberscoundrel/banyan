import { Request, Response } from 'express';
import { existsSync, createReadStream } from 'fs';
import { resolve, join } from 'path';
import { State, getActivePosts, getActiveChannels } from '../ledger/index.js';

export function createStaticHandlers(state: State, frontendPath: string) {
  const indexHtml = existsSync(join(frontendPath, 'index.html'));

  function serveIndex(_req: Request, res: Response): void {
    if (!indexHtml) {
      res.status(404).json({ error: 'Frontend not built' });
      return;
    }
    res.sendFile(join(frontendPath, 'index.html'));
  }

  function listPosts(req: Request, res: Response): void {
    const posts = getActivePosts(state);
    const limit = Math.min(parseInt(req.query.limit as string) || 50, 100);
    const offset = parseInt(req.query.offset as string) || 0;
    const channel = req.query.channel as string;

    let filtered = posts;
    if (channel) {
      filtered = posts.filter(p => p.channel === channel);
    }

    res.json({
      posts: filtered.slice(offset, offset + limit),
      total: filtered.length,
      hasMore: offset + limit < filtered.length,
    });
  }

  function listChannels(_req: Request, res: Response): void {
    const channels = getActiveChannels(state);
    res.json({ channels });
  }

  return {
    serveIndex,
    listPosts,
    listChannels,
    serveStatic: (req: Request, res: Response, next: Function) => {
      const filePath = join(frontendPath, req.path);
      if (existsSync(filePath) && !req.path.includes('..')) {
        res.sendFile(filePath);
      } else {
        next();
      }
    },
  };
}
