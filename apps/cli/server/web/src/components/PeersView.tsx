import React, { useState } from 'react';
import { PeerConnection } from '../types';
import { Users, RefreshCw, Link, Circle, Link2Off } from 'lucide-react';
import { api } from '../api';

interface PeersViewProps {
  peers: PeerConnection[];
  onRefresh: () => void;
}

export default function PeersView({ peers, onRefresh }: PeersViewProps) {
  const [connecting, setConnecting] = useState<string | null>(null);
  const [connectInput, setConnectInput] = useState('');

  const handleConnect = async (peerId: string) => {
    setConnecting(peerId);
    try {
      // Use CLI command to connect to peer
      await api.executeCommand(`peer connect ${peerId}`);
      onRefresh();
    } catch (err) {
      console.error('Failed to connect:', err);
    } finally {
      setConnecting(null);
    }
  };

  const handleAddPeer = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!connectInput.trim()) return;

    try {
      // Use CLI command - it handles both peer IDs and multiaddrs
      await api.executeCommand(`peer add ${connectInput}`);
      setConnectInput('');
      onRefresh();
    } catch (err) {
      console.error('Failed to add peer:', err);
    }
  };

  return (
    <div className="space-y-4">
      {/* Add Peer Form */}
      <div className="bg-gray-800 rounded-lg p-4 border border-banyan-700">
        <h2 className="text-lg font-semibold mb-3 flex items-center gap-2">
          <Link className="w-5 h-5 text-accent-500" />
          Connect to Peer
        </h2>
        <form onSubmit={handleAddPeer} className="flex gap-2">
          <input
            type="text"
            value={connectInput}
            onChange={(e) => setConnectInput(e.target.value)}
            placeholder="Peer ID or Multiaddr"
            className="flex-1 bg-gray-700 rounded px-3 py-2 text-sm terminal-font border border-banyan-800 focus:border-accent-500 focus:outline-none"
          />
          <button
            type="submit"
            className="bg-banyan-600 hover:bg-banyan-500 text-gold-300 px-4 py-2 rounded text-sm font-medium"
          >
            Connect
          </button>
        </form>
      </div>

      {/* Peers List */}
      <div className="bg-gray-800 rounded-lg p-4 border border-banyan-700">
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-lg font-semibold flex items-center gap-2">
            <Users className="w-5 h-5 text-accent-500" />
            Connected Peers ({peers.length})
          </h2>
          <button
            onClick={onRefresh}
            className="text-gray-400 hover:text-white p-1 rounded hover:bg-gray-700"
          >
            <RefreshCw className="w-4 h-4" />
          </button>
        </div>

        {peers.length === 0 ? (
          <p className="text-gray-400 text-center py-8">No peers connected</p>
        ) : (
          <div className="space-y-2">
            {peers.map((peer) => (
              <div
                key={peer.peer_id}
                className="bg-gray-700 rounded-lg p-3 flex items-center justify-between"
              >
                <div className="flex items-center gap-3">
                  {peer.connected ? (
                    <Circle className="w-3 h-3 text-green-400 fill-current" />
                  ) : (
                    <Link2Off className="w-4 h-4 text-gray-500" />
                  )}
                  <div>
                    <div className="flex items-center gap-2">
                      <span className="font-mono text-sm text-accent-500">[{peer.alias}]</span>
                      <span className="terminal-font text-xs text-gray-400">
                        {peer.peer_id.slice(0, 20)}...
                      </span>
                    </div>
                    <div className="flex gap-2 text-xs text-gray-500 mt-1">
                      <span className="px-2 py-0.5 bg-gray-600 rounded">{peer.connection_type}</span>
                      {peer.http_capable && (
                        <span className="px-2 py-0.5 bg-blue-900 text-blue-300 rounded">HTTP</span>
                      )}
                      <span className={peer.status === 'connected' ? 'text-green-400' : 'text-gray-400'}>
                        {peer.status}
                      </span>
                    </div>
                  </div>
                </div>
                <button
                  onClick={() => handleConnect(peer.peer_id)}
                  disabled={connecting === peer.peer_id}
                  className="text-sm px-3 py-1 bg-gray-600 hover:bg-gray-500 rounded disabled:opacity-50"
                >
                  {connecting === peer.peer_id ? 'Connecting...' : 'Reconnect'}
                </button>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

