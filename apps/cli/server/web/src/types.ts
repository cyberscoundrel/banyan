export interface NodeStatus {
  node_id: string;
  total_connected_peers: number;
  tracked_peers: number;
  http_capable_peers: number;
  bidirectional_http_peers: number;
  connection_types: Record<string, number>;
  listening_addrs: string[];
  dht_enabled: boolean;
  discovery_methods: string[];
  timestamp: string;
}

export interface PeerConnection {
  peer_id: string;
  alias: string;
  status: string;
  connection_type: string;
  http_capable: boolean;
  http_test: string;
  connected: boolean;
  last_activity: string;
}

export interface Event {
  type: string;
  timestamp: string;
  data: unknown;
  node_id: string;
}

export interface ServiceFig {
  serviceAlias: string;
  service_key: string;
}

export interface NodeInstance {
  index: number;
  address: string;
  connected: boolean;
  active: boolean;
  subprocess: boolean;
  name?: string;
}

export interface LoggedInstance {
  index: number;
  address: string;
  name?: string;
  lastSeen?: string;
  createdAt?: string;
}

export interface InstancesResponse {
  instances: NodeInstance[];
  active_index: number;
  total: number;
}

export type ViewType = 'status' | 'events' | 'peers' | 'services' | 'instances' | 'help';

