import { Request, Response, NextFunction } from 'express';
import { createHash, randomBytes } from 'crypto';

interface Session {
  peerId: string;
  challenge: string;
  createdAt: number;
  verified: boolean;
}

const sessions = new Map<string, Session>();
const CHALLENGE_EXPIRY = 5 * 60 * 1000;

export function createAuthMiddleware() {
  function requestChallenge(req: Request, res: Response): void {
    const peerId = req.body.peerId || req.query.peerId;
    if (!peerId) {
      res.status(400).json({ error: 'Peer ID required' });
      return;
    }

    const challenge = randomBytes(32).toString('hex');
    const sessionId = createHash('sha256')
      .update(peerId + challenge + Date.now())
      .digest('hex');

    sessions.set(sessionId, {
      peerId,
      challenge,
      createdAt: Date.now(),
      verified: false,
    });

    setTimeout(() => sessions.delete(sessionId), CHALLENGE_EXPIRY);

    res.json({ sessionId, challenge });
  }

  function verifyChallenge(req: Request, res: Response): void {
    const { sessionId, signature } = req.body;
    if (!sessionId || !signature) {
      res.status(400).json({ error: 'Session ID and signature required' });
      return;
    }

    const session = sessions.get(sessionId);
    if (!session) {
      res.status(404).json({ error: 'Session not found or expired' });
      return;
    }

    const expectedSignature = createHash('sha256')
      .update(session.challenge + session.peerId)
      .digest('hex');

    if (signature !== expectedSignature) {
      res.status(401).json({ error: 'Invalid signature' });
      return;
    }

    session.verified = true;
    res.json({ 
      success: true, 
      token: sessionId,
      peerId: session.peerId,
    });
  }

  function authMiddleware(req: Request, res: Response, next: NextFunction): void {
    const token = req.headers.authorization?.replace('Bearer ', '') || 
                  req.query.token as string;

    if (!token) {
      req.peerId = undefined;
      next();
      return;
    }

    const session = sessions.get(token);
    if (!session || !session.verified) {
      req.peerId = undefined;
      next();
      return;
    }

    req.peerId = session.peerId;
    next();
  }

  function requireAuth(req: Request, res: Response, next: NextFunction): void {
    if (!req.peerId) {
      res.status(401).json({ error: 'Authentication required' });
      return;
    }
    next();
  }

  return {
    requestChallenge,
    verifyChallenge,
    authMiddleware,
    requireAuth,
  };
}

declare global {
  namespace Express {
    interface Request {
      peerId?: string;
    }
  }
}
