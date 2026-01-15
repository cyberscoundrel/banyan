import { useState, useEffect, useCallback, useRef } from 'react';
import { Activity, Users, Server, HelpCircle, Terminal, Circle, Link2Off, Layers } from 'lucide-react';
import { NodeStatus, PeerConnection, Event, ViewType } from './types';
import { api, nodeApi, createNodeEventSocket } from './api';
import StatusView from './components/StatusView';
import EventsView from './components/EventsView';
import PeersView from './components/PeersView';
import ServicesView from './components/ServicesView';
import InstancesView from './components/InstancesView';
import HelpView from './components/HelpView';
import CommandInput from './components/CommandInput';

function App() {
  const [connected, setConnected] = useState(false);
  const [nodeAddress, setNodeAddress] = useState('');
  const [status, setStatus] = useState<NodeStatus | null>(null);
  const [peers, setPeers] = useState<PeerConnection[]>([]);
  const [events, setEvents] = useState<Event[]>([]);
  const [currentView, setCurrentView] = useState<ViewType>('status');
  const [message, setMessage] = useState('');
  const [messageType, setMessageType] = useState<'info' | 'error'>('info');
  const wsRef = useRef<WebSocket | null>(null);

  const showMessage = useCallback((msg: string, type: 'info' | 'error' = 'info') => {
    setMessage(msg);
    setMessageType(type);
    setTimeout(() => setMessage(''), 5000);
  }, []);

  const fetchData = useCallback(async () => {
    if (!connected || !nodeAddress) return;
    try {
      // Fetch directly from the node
      const [statusData, peersData] = await Promise.all([
        nodeApi.getStatus(nodeAddress),
        nodeApi.getPeers(nodeAddress),
      ]);
      setStatus(statusData);
      setPeers(peersData);
    } catch (err) {
      console.error('Failed to fetch data:', err);
    }
  }, [connected, nodeAddress]);

  useEffect(() => {
    if (connected && nodeAddress) {
      fetchData();
      const interval = setInterval(fetchData, 5000);

      // Set up WebSocket to the node's events endpoint
      wsRef.current = createNodeEventSocket(nodeAddress, (event) => {
        setEvents(prev => [...prev.slice(-99), event]);
      });

      return () => {
        clearInterval(interval);
        wsRef.current?.close();
      };
    }
  }, [connected, nodeAddress, fetchData]);

  const handleConnect = async (address: string) => {
    try {
      await api.connect(address);
      setConnected(true);
      setNodeAddress(address);
      showMessage(`Connected to ${address}`);
    } catch (err) {
      showMessage(`Failed to connect: ${err}`, 'error');
    }
  };

  const handleInstanceSelected = (address: string) => {
    setConnected(true);
    setNodeAddress(address);
    // Fetch data for the newly selected instance
    fetchData();
  };

  const handleCommand = async (command: string) => {
    const parts = command.trim().split(/\s+/);
    const cmd = parts[0]?.toLowerCase();
    const args = parts.slice(1);

    try {
      switch (cmd) {
        case 'connect':
          if (args[0]) await handleConnect(args[0]);
          else showMessage('Usage: connect <address>', 'error');
          break;
        case 'status':
          setCurrentView('status');
          fetchData();
          break;
        case 'peers':
          setCurrentView('peers');
          fetchData();
          break;
        case 'help':
          setCurrentView('help');
          break;
        case 'clear':
          setEvents([]);
          showMessage('Events cleared');
          break;
        default:
          const result = await api.executeCommand(command);
          if (result.error) showMessage(result.error, 'error');
          else showMessage(result.result || 'Command executed');
      }
    } catch (err) {
      showMessage(`Error: ${err}`, 'error');
    }
  };

  const tabs = [
    { id: 'status' as ViewType, label: 'Status', icon: Activity, key: '1' },
    { id: 'events' as ViewType, label: 'Events', icon: Terminal, key: '2' },
    { id: 'peers' as ViewType, label: 'Peers', icon: Users, key: '3' },
    { id: 'services' as ViewType, label: 'Services', icon: Server, key: '4' },
    { id: 'instances' as ViewType, label: 'Instances', icon: Layers, key: '5' },
    { id: 'help' as ViewType, label: 'Help', icon: HelpCircle, key: '6' },
  ];

  return (
    <div className="min-h-screen bg-gray-900 text-gray-100">
      {/* Header */}
      <header className="bg-gray-800 border-b border-banyan-700 px-4 py-3">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <span className="text-2xl">🌳</span>
            <h1 className="text-xl font-bold text-accent-500">Banyan CLI</h1>
          </div>
          <div className="flex items-center gap-2">
            {connected ? (
              <>
                <Circle className="w-3 h-3 text-green-400 fill-current" />
                <span className="text-sm text-green-400">Connected to {nodeAddress}</span>
              </>
            ) : (
              <>
                <Link2Off className="w-4 h-4 text-gray-500" />
                <span className="text-sm text-gray-500">Disconnected</span>
              </>
            )}
          </div>
        </div>
      </header>

      {/* Tab Bar */}
      <nav className="bg-gray-800 border-b border-banyan-700 px-4">
        <div className="flex gap-1">
          {tabs.map((tab) => (
            <button
              key={tab.id}
              onClick={() => setCurrentView(tab.id)}
              className={`flex items-center gap-2 px-4 py-2 text-sm font-medium transition-colors
                ${currentView === tab.id
                  ? 'bg-banyan-600 text-gold-300'
                  : 'text-gray-400 hover:text-gold-300 hover:bg-banyan-700'
                }`}
            >
              <tab.icon className="w-4 h-4" />
              <span>[{tab.key}] {tab.label}</span>
            </button>
          ))}
        </div>
      </nav>

      {/* Content Area */}
      <main className="p-4 h-[calc(100vh-200px)] overflow-auto">
        <div className="max-w-4xl mx-auto">
          {currentView === 'status' && <StatusView status={status} connected={connected} onConnect={handleConnect} />}
          {currentView === 'events' && <EventsView events={events} />}
          {currentView === 'peers' && <PeersView peers={peers} onRefresh={fetchData} />}
          {currentView === 'services' && <ServicesView connected={connected} />}
          {currentView === 'instances' && <InstancesView onRefresh={fetchData} onMessage={showMessage} onInstanceSelected={handleInstanceSelected} />}
          {currentView === 'help' && <HelpView />}
        </div>
      </main>

      {/* Message Bar */}
      {message && (
        <div className={`px-4 py-2 text-sm ${messageType === 'error' ? 'bg-red-900 text-red-200' : 'bg-banyan-700 text-gold-300'}`}>
          {messageType === 'error' ? '❌ ' : 'ℹ️ '}{message}
        </div>
      )}

      {/* Command Input */}
      <footer className="bg-gray-800 border-t border-banyan-700 p-4">
        <CommandInput onSubmit={handleCommand} />
        <p className="mt-2 text-xs text-gray-500">
          Press Tab to switch views • Type 'help' for commands
        </p>
      </footer>
    </div>
  );
}

export default App;

