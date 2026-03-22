import React, { useState } from 'react';
import { Link } from 'react-router-dom';
import { FileText, ChevronRight, Package, Layers, Code2, Settings, Zap } from 'lucide-react';

interface PackageDoc {
  name: string;
  path: string;
  description: string;
  icon: React.ElementType;
}

const packages: PackageDoc[] = [
  {
    name: 'Types',
    path: '/docs/api/types',
    description: 'Core type definitions for FigFile, FigNode, Event, ConnectionItem, and more.',
    icon: Layers,
  },
  {
    name: 'Interfaces',
    path: '/docs/api/interfaces',
    description: 'Interface contracts for decoupling components - PeerManager, CryptoManager, DiscoveryManager, etc.',
    icon: Code2,
  },
  {
    name: 'Crypto',
    path: '/docs/api/crypto',
    description: 'Cryptographic operations for secure peer communication using AES-GCM encryption.',
    icon: Zap,
  },
  {
    name: 'Discovery',
    path: '/docs/api/discovery',
    description: 'Peer discovery mechanisms including DHT, mDNS, and GossipSub.',
    icon: Package,
  },
  {
    name: 'Connection',
    path: '/docs/api/connection',
    description: 'Connection tracking and management for peer state and HTTP capability testing.',
    icon: Settings,
  },
  {
    name: 'Service',
    path: '/docs/api/service',
    description: 'Service beacon and locator management for service mesh functionality.',
    icon: Package,
  },
  {
    name: 'Events',
    path: '/docs/api/events',
    description: 'Real-time event broadcasting system for WebSocket clients.',
    icon: Zap,
  },
  {
    name: 'Tunnel',
    path: '/docs/api/tunnel',
    description: 'TCP tunnel handling over libp2p for secure port forwarding.',
    icon: Settings,
  },
  {
    name: 'Libp2p-HTTP',
    path: '/docs/api/libp2p-http',
    description: 'P2P HTTP protocol handlers for ping, greetings, and service figs.',
    icon: Code2,
  },
  {
    name: 'Management Server',
    path: '/docs/api/management-server',
    description: 'HTTP management API server for node administration.',
    icon: Settings,
  },
  {
    name: 'Node',
    path: '/docs/api/node',
    description: 'Main libp2p node implementation with all managers and lifecycle.',
    icon: Package,
  },
];

const DocsAPI: React.FC = () => {
  const [searchTerm, setSearchTerm] = useState('');

  const filteredPackages = packages.filter(pkg =>
    pkg.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
    pkg.description.toLowerCase().includes(searchTerm.toLowerCase())
  );

  return (
    <div className="prose">
      <h1>API Reference</h1>
      <p className="text-lg text-gray-600 dark:text-gray-400">
        Auto-generated documentation for all Banyan packages. Click on a package to view its full API documentation.
      </p>

      <div className="my-6">
        <input
          type="text"
          placeholder="Search packages..."
          value={searchTerm}
          onChange={(e) => setSearchTerm(e.target.value)}
          className="w-full px-4 py-2 rounded-lg border border-gray-300 dark:border-gray-700 bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:ring-2 focus:ring-primary-500 focus:border-transparent"
        />
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        {filteredPackages.map((pkg) => {
          const Icon = pkg.icon;
          return (
            <Link
              key={pkg.path}
              to={pkg.path}
              className="block p-4 bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 hover:border-primary-500 dark:hover:border-primary-500 transition-colors group"
            >
              <div className="flex items-start justify-between">
                <div className="flex items-start space-x-3">
                  <div className="p-2 bg-primary-100 dark:bg-primary-900 rounded-lg">
                    <Icon className="w-5 h-5 text-primary-600 dark:text-primary-400" />
                  </div>
                  <div>
                    <h3 className="text-lg font-semibold text-gray-900 dark:text-white group-hover:text-primary-600 dark:group-hover:text-primary-400 m-0">
                      {pkg.name}
                    </h3>
                    <p className="text-sm text-gray-600 dark:text-gray-400 mt-1 m-0">
                      {pkg.description}
                    </p>
                  </div>
                </div>
                <ChevronRight className="w-5 h-5 text-gray-400 group-hover:text-primary-500 transition-colors" />
              </div>
            </Link>
          );
        })}
      </div>

      {filteredPackages.length === 0 && (
        <div className="text-center py-8 text-gray-500 dark:text-gray-400">
          No packages found matching "{searchTerm}"
        </div>
      )}

      <div className="mt-8 p-4 bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg">
        <h3 className="text-lg font-semibold text-blue-800 dark:text-blue-200 mt-0 mb-2">
          Generated Documentation
        </h3>
        <p className="text-blue-700 dark:text-blue-300 text-sm mb-0">
          This documentation is automatically generated from the Go source code using gomarkdoc.
          For the most up-to-date information, always refer to the source code or run{' '}
          <code className="px-1 py-0.5 bg-blue-100 dark:bg-blue-800 rounded">go doc</code> locally.
        </p>
      </div>
    </div>
  );
};

export default DocsAPI;
