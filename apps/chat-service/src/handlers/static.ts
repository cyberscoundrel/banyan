import { Request, Response } from 'express';
import { existsSync } from 'fs';
import { join } from 'path';

export function createStaticHandlers(frontendPath: string) {
  const indexHtml = frontendPath && existsSync(join(frontendPath, 'index.html'));

  function serveIndex(_req: Request, res: Response): void {
    if (!indexHtml) {
      res.status(404).json({ error: 'Frontend not built' });
      return;
    }
    res.sendFile(join(frontendPath, 'index.html'));
  }

  return { serveIndex };
}
