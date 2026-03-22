import React from 'react';
import { Routes, Route } from 'react-router-dom';
import DocsAPI from './DocsAPI';
import PackageDoc from './PackageDoc';

const DocsHome: React.FC = () => {
  return (
    <div className="prose">
      <h1>Documentation</h1>
      <p className="text-lg">
        Comprehensive documentation for Banyan - the peer-to-peer networking multitool.
      </p>

      <div className="bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg p-6 my-6">
        <h2 className="text-xl font-semibold text-blue-800 dark:text-blue-200 mt-0 mb-3">
          Documentation In Progress
        </h2>
        <p className="text-blue-700 dark:text-blue-300 mb-0">
          We're currently building out comprehensive documentation. Check back soon for detailed guides, 
          tutorials, and API references.
        </p>
      </div>

      <h2>What's Coming</h2>
      <p>The documentation will cover:</p>

      <h3>Getting Started</h3>
      <ul>
        <li>Installation and setup</li>
        <li>Quick start guide</li>
        <li>Basic concepts and terminology</li>
        <li>Your first peer-to-peer connection</li>
      </ul>

      <h3>Core Concepts</h3>
      <ul>
        <li>Understanding LibP2P and peer-to-peer networking</li>
        <li>Peer discovery mechanisms (DHT, mDNS, GossipSub)</li>
        <li>Service discovery and management</li>
        <li>Cryptographic identities and security</li>
        <li>NAT traversal and connectivity</li>
      </ul>

      <h3>Guides</h3>
      <ul>
        <li>Setting up a service mesh</li>
        <li>Connecting microservices across cloud providers</li>
        <li>Building IoT networks with Banyan</li>
        <li>Implementing secure peer communication</li>
        <li>Monitoring and debugging your network</li>
      </ul>

      <h3>API Reference</h3>
      <ul>
        <li>Complete REST API documentation</li>
        <li>WebSocket event reference</li>
        <li>Command-line interface</li>
        <li>Configuration options</li>
      </ul>

      <h3>Advanced Topics</h3>
      <ul>
        <li>Custom routing configurations</li>
        <li>Performance tuning and optimization</li>
        <li>Security best practices</li>
        <li>Troubleshooting common issues</li>
        <li>Contributing to Banyan</li>
      </ul>

      <h2>In the Meantime</h2>
      <p>
        While we build out the full documentation, you can explore the existing pages:
      </p>
      <ul>
        <li><strong>API Reference</strong>: <a href="/docs/api">Auto-generated package documentation</a></li>
        <li><strong>Usage</strong>: Basic usage and command-line options</li>
        <li><strong>API</strong>: REST API endpoints and examples</li>
        <li><strong>Examples</strong>: Common usage scenarios and code samples</li>
      </ul>
    </div>
  );
};

const Docs: React.FC = () => {
  return (
    <Routes>
      <Route index element={<DocsHome />} />
      <Route path="api" element={<DocsAPI />} />
      <Route path="api/*" element={<PackageDoc />} />
      <Route path="*" element={<DocsHome />} />
    </Routes>
  );
};

export default Docs;

