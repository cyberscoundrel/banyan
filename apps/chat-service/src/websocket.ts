import { WebSocketServer, WebSocket } from 'ws';
import { Server } from 'http';

type EventHandler = (data: any) => void;

interface Client {
  ws: WebSocket;
  subscriptions: Set<string>;
}

export class WebSocketHub {
  private clients: Set<Client> = new Set();
  private wss: WebSocketServer;

  constructor(server: Server) {
    this.wss = new WebSocketServer({ server });
    this.wss.on('connection', (ws) => {
      const client: Client = { ws, subscriptions: new Set() };
      this.clients.add(client);

      ws.on('message', (data) => {
        try {
          const msg = JSON.parse(data.toString());
          if (msg.type === 'subscribe' && Array.isArray(msg.events)) {
            for (const event of msg.events) {
              client.subscriptions.add(event);
            }
          } else if (msg.type === 'unsubscribe' && Array.isArray(msg.events)) {
            for (const event of msg.events) {
              client.subscriptions.delete(event);
            }
          }
        } catch {
          ws.send(JSON.stringify({ error: 'Invalid message' }));
        }
      });

      ws.on('close', () => {
        this.clients.delete(client);
      });

      ws.send(JSON.stringify({ type: 'connected' }));
    });
  }

  broadcast(event: string, data: any): void {
    const message = JSON.stringify({ event, data, timestamp: Date.now() });
    
    for (const client of this.clients) {
      if (client.subscriptions.has(event) || client.subscriptions.has('*')) {
        if (client.ws.readyState === WebSocket.OPEN) {
          client.ws.send(message);
        }
      }
    }
  }

  broadcastAll(data: any): void {
    const message = JSON.stringify({ ...data, timestamp: Date.now() });
    
    for (const client of this.clients) {
      if (client.ws.readyState === WebSocket.OPEN) {
        client.ws.send(message);
      }
    }
  }

  getClientCount(): number {
    return this.clients.size;
  }

  close(): void {
    this.wss.close();
  }
}
