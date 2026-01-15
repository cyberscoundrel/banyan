import { NodeStatus, PeerConnection, ServiceFig, Event, InstancesResponse } from './types';

const CLI_API = '/api'; // CLI server endpoints

// Fetch from CLI server
async function fetchCli<T>(url: string, options?: RequestInit): Promise<T> {
  const response = await fetch(`${CLI_API}${url}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...options?.headers,
    },
  });
  if (!response.ok) {
    const error = await response.text();
    throw new Error(error || response.statusText);
  }
  return response.json();
}

// Fetch directly from a node (node API doesn't have /api prefix)
async function fetchNode<T>(nodeAddress: string, path: string, options?: RequestInit): Promise<T> {
  const base = nodeAddress.endsWith('/') ? nodeAddress.slice(0, -1) : nodeAddress;
  const response = await fetch(`${base}${path}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...options?.headers,
    },
  });
  if (!response.ok) {
    const error = await response.text();
    throw new Error(error || response.statusText);
  }
  return response.json();
}

// Response types from node API
interface ConnectionsResponse {
  connections: PeerConnection[];
  total_tracked: number;
  timestamp: string;
}

// Node API - calls directly to the node instance
// Uses the node's management server endpoints (not /api prefix)
export const nodeApi = {
  getStatus: (nodeAddress: string) => fetchNode<NodeStatus>(nodeAddress, '/node/status'),
  getPeers: async (nodeAddress: string): Promise<PeerConnection[]> => {
    const resp = await fetchNode<ConnectionsResponse>(nodeAddress, '/network/connections');
    return resp.connections || [];
  },
  getServices: (nodeAddress: string) => fetchNode<unknown>(nodeAddress, '/services/list'),
  getFigs: (nodeAddress: string) => fetchNode<ServiceFig[]>(nodeAddress, '/services/figs'),
};

// CLI API - calls to the CLI server for instance/command management
export const api = {
  // Instance management (via CLI server -> TUI)
  getInstances: () => fetchCli<InstancesResponse>('/instances'),
  selectInstance: (index: number) =>
    fetchCli<{ success: boolean; removed?: boolean; index?: number; address?: string }>('/instances/select', {
      method: 'POST',
      body: JSON.stringify({ index }),
    }),
  stopInstance: (index?: number) =>
    fetchCli<{ success: boolean; index: number }>('/instances/stop', {
      method: 'POST',
      body: JSON.stringify({ index: index || 0 }),
    }),

  // Node process management (via CLI server)
  startNode: (options?: { nodePath?: string; configPath?: string; extraArgs?: string[] }) =>
    fetchCli<{ success: boolean; url: string; connected: boolean }>('/node/start', {
      method: 'POST',
      body: JSON.stringify(options || {}),
    }),
  stopNode: () =>
    fetchCli<{ success: boolean }>('/node/stop', { method: 'POST' }),

  // Connect to a node (via CLI server -> TUI)
  connect: (address: string) =>
    fetchCli<{ connected: boolean; address: string }>('/connect', {
      method: 'POST',
      body: JSON.stringify({ address }),
    }),

  // Command execution (via CLI server -> TUI)
  executeCommand: (command: string) =>
    fetchCli<{ success: boolean; result?: string; error?: string }>('/execute', {
      method: 'POST',
      body: JSON.stringify({ command }),
    }),

  // Logged instances management
  reconnectLogged: (index: number) =>
    fetchCli<{ success: boolean; result?: string; error?: string }>('/execute', {
      method: 'POST',
      body: JSON.stringify({ command: `reconnect L${index}` }),
    }),
  clearLogged: () =>
    fetchCli<{ success: boolean; result?: string; error?: string }>('/execute', {
      method: 'POST',
      body: JSON.stringify({ command: 'clear logged' }),
    }),
};

// WebSocket connection for events directly from a node instance
export function createNodeEventSocket(nodeAddress: string, onEvent: (event: Event) => void): WebSocket {
  // Convert http://host:port to ws://host:port/events/subscribe
  const url = new URL(nodeAddress);
  const wsProtocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
  const wsUrl = `${wsProtocol}//${url.host}/events/subscribe`;

  const ws = new WebSocket(wsUrl);

  ws.onmessage = (e) => {
    try {
      const event = JSON.parse(e.data) as Event;
      onEvent(event);
    } catch (err) {
      console.error('Failed to parse event:', err);
    }
  };

  ws.onerror = (e) => {
    console.error('Node WebSocket error:', e);
  };

  ws.onopen = () => {
    console.log('Connected to node events:', wsUrl);
  };

  return ws;
}

// Session state message from CLI
export interface SessionState {
  type: 'instances';
  instances: {
    index: number;
    address: string;
    connected: boolean;
    active: boolean;
    subprocess: boolean;
    name?: string;
  }[];
  loggedInstances?: {
    index: number;
    address: string;
    name?: string;
    lastSeen?: string;
    createdAt?: string;
  }[];
}

// WebSocket connection for CLI session instance state sync
export function createSessionSocket(onState: (state: SessionState) => void): WebSocket {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const ws = new WebSocket(`${protocol}//${window.location.host}/api/session`);

  ws.onmessage = (e) => {
    try {
      const state = JSON.parse(e.data) as SessionState;
      onState(state);
    } catch (err) {
      console.error('Failed to parse session state:', err);
    }
  };

  ws.onerror = (e) => {
    console.error('Session WebSocket error:', e);
  };

  return ws;
}

