import React, { useState } from 'react';
import { NodeStatus } from '../types';
import { Activity, Globe, Users, Radio, Play, Settings } from 'lucide-react';
import { api } from '../api';

interface StatusViewProps {
  status: NodeStatus | null;
  connected: boolean;
  onConnect?: (address: string) => void;
}

export default function StatusView({ status, connected, onConnect }: StatusViewProps) {
  const [showStartForm, setShowStartForm] = useState(false);
  const [nodePath, setNodePath] = useState('');
  const [configPath, setConfigPath] = useState('');
  const [extraArgs, setExtraArgs] = useState('');
  const [starting, setStarting] = useState(false);
  const [error, setError] = useState('');
  const [connectAddress, setConnectAddress] = useState('');

  const handleStartNode = async (e: React.FormEvent) => {
    e.preventDefault();
    setStarting(true);
    setError('');
    try {
      const args = extraArgs.trim() ? extraArgs.split(/\s+/) : undefined;
      const result = await api.startNode({
        nodePath: nodePath || undefined,
        configPath: configPath || undefined,
        extraArgs: args,
      });
      if (result.connected && result.url && onConnect) {
        onConnect(result.url);
      }
      setShowStartForm(false);
    } catch (err) {
      setError(String(err));
    } finally {
      setStarting(false);
    }
  };

  const handleConnect = async (e: React.FormEvent) => {
    e.preventDefault();
    if (connectAddress && onConnect) {
      onConnect(connectAddress);
    }
  };

  if (!connected) {
    return (
      <div className="space-y-6">
        <div className="flex flex-col items-center justify-center h-48 text-gray-400">
          <Globe className="w-16 h-16 mb-4 opacity-50" />
          <p>Not connected to a node</p>
        </div>

        {/* Connect Form */}
        <div className="bg-gray-800 rounded-lg p-4 border border-banyan-700">
          <h3 className="text-lg font-semibold mb-3">Connect to Node</h3>
          <form onSubmit={handleConnect} className="flex gap-2">
            <input
              type="text"
              value={connectAddress}
              onChange={(e) => setConnectAddress(e.target.value)}
              placeholder="http://localhost:8080"
              className="flex-1 bg-gray-700 border border-gray-600 rounded px-3 py-2 text-sm focus:outline-none focus:border-banyan-500"
            />
            <button
              type="submit"
              className="bg-banyan-600 hover:bg-banyan-500 px-4 py-2 rounded text-sm font-medium"
            >
              Connect
            </button>
          </form>
        </div>

        {/* Start Node Section */}
        <div className="bg-gray-800 rounded-lg p-4 border border-banyan-700">
          <div className="flex items-center justify-between mb-3">
            <h3 className="text-lg font-semibold flex items-center gap-2">
              <Play className="w-5 h-5 text-accent-500" />
              Start Node
            </h3>
            <button
              onClick={() => setShowStartForm(!showStartForm)}
              className="text-sm text-gray-400 hover:text-gold-300 flex items-center gap-1"
            >
              <Settings className="w-4 h-4" />
              {showStartForm ? 'Hide Options' : 'Show Options'}
            </button>
          </div>

          {error && (
            <div className="bg-red-900/50 border border-red-700 text-red-200 px-3 py-2 rounded mb-3 text-sm">
              {error}
            </div>
          )}

          <form onSubmit={handleStartNode} className="space-y-3">
            {showStartForm && (
              <>
                <div>
                  <label className="block text-sm text-gray-400 mb-1">Node Executable Path (optional)</label>
                  <input
                    type="text"
                    value={nodePath}
                    onChange={(e) => setNodePath(e.target.value)}
                    placeholder="Auto-detect in same directory"
                    className="w-full bg-gray-700 border border-gray-600 rounded px-3 py-2 text-sm focus:outline-none focus:border-banyan-500"
                  />
                </div>
                <div>
                  <label className="block text-sm text-gray-400 mb-1">Config File Path (optional)</label>
                  <input
                    type="text"
                    value={configPath}
                    onChange={(e) => setConfigPath(e.target.value)}
                    placeholder="Path to node config file"
                    className="w-full bg-gray-700 border border-gray-600 rounded px-3 py-2 text-sm focus:outline-none focus:border-banyan-500"
                  />
                </div>
                <div>
                  <label className="block text-sm text-gray-400 mb-1">Extra Arguments (optional)</label>
                  <input
                    type="text"
                    value={extraArgs}
                    onChange={(e) => setExtraArgs(e.target.value)}
                    placeholder="-port 9000 -debug"
                    className="w-full bg-gray-700 border border-gray-600 rounded px-3 py-2 text-sm focus:outline-none focus:border-banyan-500"
                  />
                </div>
              </>
            )}
            <button
              type="submit"
              disabled={starting}
              className="bg-accent-600 hover:bg-accent-500 disabled:bg-gray-600 px-6 py-2 rounded font-medium inline-flex items-center gap-2"
            >
              {starting ? (
                <>
                  <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white" />
                  Starting...
                </>
              ) : (
                <>
                  <Play className="w-4 h-4" />
                  Start Node
                </>
              )}
            </button>
          </form>
          <p className="text-xs text-gray-500 mt-2">
            Auto-detects banyan-* executable in the same directory as the CLI
          </p>
        </div>
      </div>
    );
  }

  if (!status) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-banyan-500" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Node Info */}
      <div className="bg-gray-800 rounded-lg p-4 border border-banyan-700">
        <h2 className="text-lg font-semibold mb-3 flex items-center gap-2">
          <Activity className="w-5 h-5 text-accent-500" />
          Node Information
        </h2>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <span className="text-gray-400 text-sm">Node ID:</span>
            <p className="terminal-font text-sm break-all">{status.node_id}</p>
          </div>
          <div>
            <span className="text-gray-400 text-sm">DHT Enabled:</span>
            <p className={status.dht_enabled ? 'text-green-400' : 'text-yellow-400'}>
              {status.dht_enabled ? 'Yes' : 'No'}
            </p>
          </div>
          <div>
            <span className="text-gray-400 text-sm">Discovery Methods:</span>
            <p>{status.discovery_methods?.join(', ') || 'None'}</p>
          </div>
          <div>
            <span className="text-gray-400 text-sm">Last Updated:</span>
            <p className="text-sm">{new Date(status.timestamp).toLocaleString()}</p>
          </div>
        </div>
      </div>

      {/* Peer Statistics */}
      <div className="bg-gray-800 rounded-lg p-4 border border-banyan-700">
        <h2 className="text-lg font-semibold mb-3 flex items-center gap-2">
          <Users className="w-5 h-5 text-accent-500" />
          Peer Statistics
        </h2>
        <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
          <StatCard label="Total Connected" value={status.total_connected_peers} />
          <StatCard label="Tracked" value={status.tracked_peers} />
          <StatCard label="HTTP Capable" value={status.http_capable_peers} />
          <StatCard label="Bidirectional" value={status.bidirectional_http_peers} />
        </div>
      </div>

      {/* Connection Types */}
      <div className="bg-gray-800 rounded-lg p-4">
        <h2 className="text-lg font-semibold mb-3">Connection Types</h2>
        <div className="grid grid-cols-2 md:grid-cols-4 gap-2">
          {Object.entries(status.connection_types || {}).map(([type, count]) => (
            <div key={type} className="bg-gray-700 rounded px-3 py-2">
              <span className="text-gray-400 text-sm">{type}:</span>
              <span className="ml-2 font-semibold">{count}</span>
            </div>
          ))}
        </div>
      </div>

      {/* Listening Addresses */}
      <div className="bg-gray-800 rounded-lg p-4 border border-banyan-700">
        <h2 className="text-lg font-semibold mb-3 flex items-center gap-2">
          <Radio className="w-5 h-5 text-accent-500" />
          Listening Addresses
        </h2>
        <ul className="space-y-1 terminal-font text-sm">
          {status.listening_addrs?.map((addr, i) => (
            <li key={i} className="text-gray-300 break-all">{String(addr)}</li>
          ))}
        </ul>
      </div>
    </div>
  );
}

function StatCard({ label, value }: { label: string; value: number }) {
  return (
    <div className="bg-gray-700 rounded-lg p-3 text-center border border-banyan-800">
      <p className="text-2xl font-bold text-accent-500">{value}</p>
      <p className="text-xs text-gray-400">{label}</p>
    </div>
  );
}

