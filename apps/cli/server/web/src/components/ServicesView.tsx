import { useState } from 'react';
import { Server, Search, Play, Square } from 'lucide-react';
import { api } from '../api';

interface ServicesViewProps {
  connected: boolean;
}

export default function ServicesView({ connected }: ServicesViewProps) {
  const [findInput, setFindInput] = useState('');
  const [findResult, setFindResult] = useState<string | null>(null);
  const [serveInput, setServeInput] = useState('');
  const [loading, setLoading] = useState(false);

  const handleFind = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!findInput.trim()) return;

    setLoading(true);
    try {
      // Use CLI command to find service
      const result = await api.executeCommand(`find ${findInput}`);
      setFindResult(result.result || result.error || 'No result');
    } catch (err) {
      setFindResult(`Error: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  const handleServeStart = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!serveInput.trim()) return;

    setLoading(true);
    try {
      await api.executeCommand(`serve ${serveInput}`);
      setServeInput('');
    } catch (err) {
      console.error('Failed to start beacon:', err);
    } finally {
      setLoading(false);
    }
  };

  const handleServeStop = async () => {
    setLoading(true);
    try {
      await api.executeCommand('serve stop');
    } catch (err) {
      console.error('Failed to stop beacon:', err);
    } finally {
      setLoading(false);
    }
  };

  if (!connected) {
    return (
      <div className="flex flex-col items-center justify-center h-64 text-gray-400">
        <Server className="w-16 h-16 mb-4 opacity-50" />
        <p>Not connected to a node</p>
        <p className="text-sm mt-2">Connect to manage services</p>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {/* Find Service */}
      <div className="bg-gray-800 rounded-lg p-4 border border-banyan-700">
        <h2 className="text-lg font-semibold mb-3 flex items-center gap-2">
          <Search className="w-5 h-5 text-accent-500" />
          Find Service
        </h2>
        <form onSubmit={handleFind} className="flex gap-2">
          <input
            type="text"
            value={findInput}
            onChange={(e) => setFindInput(e.target.value)}
            placeholder="Service key or alias"
            className="flex-1 bg-gray-700 rounded px-3 py-2 text-sm terminal-font border border-banyan-800 focus:border-accent-500 focus:outline-none"
          />
          <button
            type="submit"
            disabled={loading}
            className="bg-banyan-600 hover:bg-banyan-500 text-gold-300 px-4 py-2 rounded text-sm font-medium disabled:opacity-50"
          >
            Find
          </button>
        </form>
        {findResult && (
          <pre className="mt-3 bg-gray-700 rounded p-3 text-xs overflow-auto max-h-40">
            {JSON.stringify(findResult, null, 2)}
          </pre>
        )}
      </div>

      {/* Start Service Beacon */}
      <div className="bg-gray-800 rounded-lg p-4 border border-banyan-700">
        <h2 className="text-lg font-semibold mb-3 flex items-center gap-2">
          <Play className="w-5 h-5 text-accent-500" />
          Start Service Beacon
        </h2>
        <form onSubmit={handleServeStart} className="flex gap-2">
          <input
            type="text"
            value={serveInput}
            onChange={(e) => setServeInput(e.target.value)}
            placeholder="Fig file location"
            className="flex-1 bg-gray-700 rounded px-3 py-2 text-sm terminal-font border border-banyan-800 focus:border-accent-500 focus:outline-none"
          />
          <button
            type="submit"
            disabled={loading}
            className="bg-green-600 hover:bg-green-500 px-4 py-2 rounded text-sm font-medium disabled:opacity-50"
          >
            Start
          </button>
        </form>
      </div>

      {/* Stop Service */}
      <div className="bg-gray-800 rounded-lg p-4 border border-banyan-700">
        <h2 className="text-lg font-semibold mb-3 flex items-center gap-2">
          <Square className="w-5 h-5 text-red-500" />
          Stop Service
        </h2>
        <button
          onClick={handleServeStop}
          disabled={loading}
          className="bg-red-600 hover:bg-red-500 px-4 py-2 rounded text-sm font-medium disabled:opacity-50"
        >
          Stop Active Beacon
        </button>
        <p className="text-xs text-gray-500 mt-2">
          Use 'figs' command in the CLI to view active services
        </p>
      </div>
    </div>
  );
}

