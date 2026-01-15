import { useState, useEffect, useRef } from 'react';
import { Server, Circle, Trash2, Play, Square, Link2Off, History, RefreshCw } from 'lucide-react';
import { NodeInstance, LoggedInstance } from '../types';
import { api, createSessionSocket, SessionState } from '../api';

interface InstancesViewProps {
  onRefresh: () => void;
  onMessage: (msg: string, type: 'info' | 'error') => void;
  onInstanceSelected?: (address: string) => void;
}

export default function InstancesView({ onRefresh, onMessage, onInstanceSelected }: InstancesViewProps) {
  const [instances, setInstances] = useState<NodeInstance[]>([]);
  const [loggedInstances, setLoggedInstances] = useState<LoggedInstance[]>([]);
  const [loading, setLoading] = useState(false);
  const [connectAddress, setConnectAddress] = useState('');
  const wsRef = useRef<WebSocket | null>(null);

  // Subscribe to CLI session state via WebSocket
  useEffect(() => {
    wsRef.current = createSessionSocket((state: SessionState) => {
      if (state.type === 'instances') {
        setInstances(state.instances);
        setLoggedInstances(state.loggedInstances || []);
      }
    });

    return () => {
      wsRef.current?.close();
    };
  }, []);

  const handleSelect = async (index: number) => {
    setLoading(true);
    try {
      const result = await api.selectInstance(index);
      if (result.removed) {
        onMessage(`Removed disconnected instance ${index}`, 'info');
      } else {
        onMessage(`Switched to instance ${index}: ${result.address}`, 'info');
        // Notify parent that we've connected to this instance
        if (result.address && onInstanceSelected) {
          onInstanceSelected(result.address);
        }
      }
      // State will be pushed via WebSocket
      onRefresh();
    } catch (err) {
      onMessage(`Failed to select instance: ${err}`, 'error');
    }
    setLoading(false);
  };

  const handleStop = async (index: number) => {
    setLoading(true);
    try {
      await api.stopInstance(index);
      onMessage(`Stopped instance ${index}`, 'info');
      // State will be pushed via WebSocket
    } catch (err) {
      onMessage(`Failed to stop instance: ${err}`, 'error');
    }
    setLoading(false);
  };

  const handleConnect = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!connectAddress) return;
    setLoading(true);
    try {
      let address = connectAddress;
      if (!address.startsWith('http')) {
        address = 'http://' + address;
      }
      await api.connect(address);
      onMessage(`Connected to ${address}`, 'info');
      setConnectAddress('');
      // State will be pushed via WebSocket
      onRefresh();
    } catch (err) {
      onMessage(`Failed to connect: ${err}`, 'error');
    }
    setLoading(false);
  };

  const handleReconnectLogged = async (index: number) => {
    setLoading(true);
    try {
      const result = await api.reconnectLogged(index);
      if (result.success) {
        onMessage(result.result || `Reconnecting to logged instance L${index}...`, 'info');
        onRefresh();
      } else {
        onMessage(result.error || `Failed to reconnect to L${index}`, 'error');
      }
    } catch (err) {
      onMessage(`Failed to reconnect: ${err}`, 'error');
    }
    setLoading(false);
  };

  const handleClearLogged = async () => {
    setLoading(true);
    try {
      const result = await api.clearLogged();
      if (result.success) {
        onMessage(result.result || 'Logged instances cleared', 'info');
      } else {
        onMessage(result.error || 'Failed to clear logged instances', 'error');
      }
    } catch (err) {
      onMessage(`Failed to clear logged instances: ${err}`, 'error');
    }
    setLoading(false);
  };

  const formatTimestamp = (ts?: string) => {
    if (!ts) return 'Unknown';
    try {
      const date = new Date(ts);
      return date.toLocaleString();
    } catch {
      return ts;
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-gold-300 flex items-center gap-2">
          <Server className="w-5 h-5" />
          Node Instances
        </h2>
        <span className="text-sm text-gray-400">
          {instances.length} instance{instances.length !== 1 ? 's' : ''}
        </span>
      </div>

      {/* Connect Form */}
      <form onSubmit={handleConnect} className="flex gap-2">
        <input
          type="text"
          value={connectAddress}
          onChange={(e) => setConnectAddress(e.target.value)}
          placeholder="Connect to node (e.g., localhost:8080)"
          className="flex-1 bg-gray-700 border border-banyan-600 rounded px-3 py-2 text-sm
                     focus:outline-none focus:border-gold-500"
          disabled={loading}
        />
        <button
          type="submit"
          disabled={loading || !connectAddress}
          className="px-4 py-2 bg-banyan-600 hover:bg-banyan-500 rounded text-sm font-medium
                     disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
        >
          <Play className="w-4 h-4" />
          Connect
        </button>
      </form>

      {/* Instance List */}
      <div className="bg-gray-800 rounded-lg border border-banyan-700 overflow-hidden">
        {instances.length === 0 ? (
          <div className="p-8 text-center text-gray-400">
            <Server className="w-12 h-12 mx-auto mb-3 opacity-50" />
            <p>No instances connected</p>
            <p className="text-sm mt-1">Use the form above to connect to a node</p>
          </div>
        ) : (
          <table className="w-full">
            <thead className="bg-gray-700">
              <tr>
                <th className="px-4 py-2 text-left text-xs font-medium text-gray-400 uppercase">#</th>
                <th className="px-4 py-2 text-left text-xs font-medium text-gray-400 uppercase">Address</th>
                <th className="px-4 py-2 text-left text-xs font-medium text-gray-400 uppercase">Status</th>
                <th className="px-4 py-2 text-left text-xs font-medium text-gray-400 uppercase">Type</th>
                <th className="px-4 py-2 text-right text-xs font-medium text-gray-400 uppercase">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-banyan-700">
              {instances.map((inst) => (
                <tr
                  key={inst.index}
                  className={`hover:bg-banyan-800 cursor-pointer ${inst.active ? 'bg-banyan-700/50' : ''}`}
                  onClick={() => handleSelect(inst.index)}
                >
                  <td className="px-4 py-3 text-sm">
                    {inst.active && <span className="text-gold-400 mr-1">►</span>}
                    {inst.index}
                  </td>
                  <td className="px-4 py-3 text-sm font-mono">{inst.address}</td>
                  <td className="px-4 py-3">
                    {inst.connected ? (
                      <span className="flex items-center gap-1 text-green-400 text-sm">
                        <Circle className="w-3 h-3 fill-current" /> Connected
                      </span>
                    ) : (
                      <span className="flex items-center gap-1 text-gray-500 text-sm">
                        <Link2Off className="w-4 h-4" /> Disconnected
                      </span>
                    )}
                  </td>
                  <td className="px-4 py-3 text-sm text-gray-400">
                    {inst.subprocess ? 'Subprocess' : 'Remote'}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <button
                      onClick={(e) => { e.stopPropagation(); handleStop(inst.index); }}
                      disabled={loading}
                      className="p-1 hover:bg-red-900/50 rounded text-red-400 hover:text-red-300"
                      title={inst.connected ? 'Stop' : 'Remove'}
                    >
                      {inst.connected ? <Square className="w-4 h-4" /> : <Trash2 className="w-4 h-4" />}
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <p className="text-xs text-gray-500">
        Click an instance to select it • Click a disconnected instance to remove it
      </p>

      {/* Logged Instances Section */}
      <div className="mt-8">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold text-gold-300 flex items-center gap-2">
            <History className="w-5 h-5" />
            Logged Instances
          </h2>
          <div className="flex items-center gap-2">
            <span className="text-sm text-gray-400">
              {loggedInstances.length} logged
            </span>
            {loggedInstances.length > 0 && (
              <button
                onClick={handleClearLogged}
                disabled={loading}
                className="px-2 py-1 text-xs bg-red-900/50 hover:bg-red-800/50 rounded text-red-400
                           hover:text-red-300 disabled:opacity-50 flex items-center gap-1"
              >
                <Trash2 className="w-3 h-3" />
                Clear All
              </button>
            )}
          </div>
        </div>

        <div className="bg-gray-800 rounded-lg border border-banyan-700 overflow-hidden">
          {loggedInstances.length === 0 ? (
            <div className="p-6 text-center text-gray-400">
              <History className="w-10 h-10 mx-auto mb-2 opacity-50" />
              <p className="text-sm">No logged instances</p>
              <p className="text-xs mt-1">Instances you connect to will appear here</p>
            </div>
          ) : (
            <table className="w-full">
              <thead className="bg-gray-700">
                <tr>
                  <th className="px-4 py-2 text-left text-xs font-medium text-gray-400 uppercase">#</th>
                  <th className="px-4 py-2 text-left text-xs font-medium text-gray-400 uppercase">Address</th>
                  <th className="px-4 py-2 text-left text-xs font-medium text-gray-400 uppercase">Name</th>
                  <th className="px-4 py-2 text-left text-xs font-medium text-gray-400 uppercase">Last Seen</th>
                  <th className="px-4 py-2 text-right text-xs font-medium text-gray-400 uppercase">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-banyan-700">
                {loggedInstances.map((logged) => (
                  <tr key={logged.index} className="hover:bg-banyan-800">
                    <td className="px-4 py-3 text-sm text-gray-400">L{logged.index}</td>
                    <td className="px-4 py-3 text-sm font-mono">{logged.address}</td>
                    <td className="px-4 py-3 text-sm text-gray-400">{logged.name || '-'}</td>
                    <td className="px-4 py-3 text-sm text-gray-400">{formatTimestamp(logged.lastSeen)}</td>
                    <td className="px-4 py-3 text-right">
                      <button
                        onClick={() => handleReconnectLogged(logged.index)}
                        disabled={loading}
                        className="p-1 hover:bg-banyan-600/50 rounded text-banyan-400 hover:text-banyan-300"
                        title="Reconnect"
                      >
                        <RefreshCw className="w-4 h-4" />
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>

        <p className="text-xs text-gray-500 mt-2">
          Click reconnect to try connecting to a logged instance
        </p>
      </div>
    </div>
  );
}

