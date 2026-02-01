import React from 'react';

const Home: React.FC = () => {
  return (
    <div className="prose">
      <h1>Banyan</h1>
      <p className="text-lg">
        An open-source peer-to-peer networking multitool that transforms traditional web applications
        and microservices into decentralized systems—without changing a single line of your application code.
      </p>

      <h2>Why Banyan?</h2>
      <p>
        Modern web services face challenges that centralized infrastructure struggles to solve:
        single points of failure, vendor lock-in, geographic latency, and privacy concerns.
        Banyan offers a different approach by leveraging peer-to-peer networking to create
        resilient, distributed systems that maintain the familiar HTTP interface developers already know.
      </p>

      <h2>What Makes It Different?</h2>
      <p>
        Unlike traditional networking tools, Banyan sits between your existing applications and the network,
        acting as a transparent proxy. Your services continue to speak HTTP, while Banyan handles the
        complexity of peer-to-peer communication, discovery, and routing. This means:
      </p>
      <ul>
        <li><strong>Zero Code Changes</strong>: Your existing HTTP services work as-is</li>
        <li><strong>NAT Traversal</strong>: Connect services across networks without port forwarding</li>
        <li><strong>Built-in Discovery</strong>: Find and connect to services without centralized registries</li>
        <li><strong>Cryptographic Identity</strong>: Services are identified by public keys, not IP addresses</li>
        <li><strong>Flexible Deployment</strong>: Run anywhere—cloud, edge, IoT, or desktop</li>
      </ul>

      <h2>Use Cases</h2>

      <h3>For Backend Developers</h3>
      <p>
        Build microservice meshes without Kubernetes complexity. Connect services across cloud providers
        or hybrid environments. Create development environments that mirror production topology without VPNs.
      </p>

      <h3>For DevOps Engineers</h3>
      <p>
        Simplify service discovery and load balancing. Enable secure communication between services
        without managing certificates or VPNs. Deploy services that can migrate between infrastructure
        without reconfiguration.
      </p>

      <h3>For IoT & Edge Computing</h3>
      <p>
        Connect devices behind NATs and firewalls. Build resilient networks that function during
        internet outages. Reduce cloud dependency by enabling direct device-to-device communication.
      </p>

      <h3>For Privacy-Conscious Applications</h3>
      <p>
        Minimize data centralization by enabling direct peer communication. Reduce reliance on
        third-party infrastructure. Build applications where users control their own data and connections.
      </p>

      <h2>Core Capabilities</h2>

      <h3>HTTP Proxy Over LibP2P</h3>
      <ul>
        <li>Proxy any HTTP request through the peer-to-peer network</li>
        <li>Support for all HTTP methods (GET, POST, PUT, DELETE, PATCH, etc.)</li>
        <li>Bidirectional communication—any peer can be client or server</li>
        <li>Connection quality metrics and health monitoring</li>
        <li>Human-readable aliases for easier peer addressing</li>
      </ul>

      <h3>Peer Discovery & Management</h3>
      <ul>
        <li><strong>DHT (Distributed Hash Table)</strong>: Global peer discovery across the internet</li>
        <li><strong>mDNS</strong>: Automatic discovery on local networks</li>
        <li><strong>GossipSub</strong>: Publish-subscribe messaging for service announcements</li>
        <li><strong>Manual Connections</strong>: Direct peer-to-peer connections when needed</li>
        <li><strong>Connection Tracking</strong>: Detailed status, history, and performance metrics</li>
      </ul>

      <h3>Service Discovery & Management</h3>
      <ul>
        <li><strong>Service Beacons</strong>: Announce service availability using cryptographic identities</li>
        <li><strong>Service Locators</strong>: Find services by their public key fingerprint</li>
        <li><strong>Encrypted Lookups</strong>: Optional encryption for service discovery messages</li>
        <li><strong>Flexible Privacy</strong>: Configurable anonymous or authenticated discovery</li>
      </ul>

      <h3>Real-time Event Monitoring</h3>
      <ul>
        <li><strong>WebSocket Event Stream</strong>: Subscribe to live network events</li>
        <li><strong>Comprehensive Events</strong>: Track peer connections, HTTP activity, service announcements, and errors</li>
        <li><strong>Structured Data</strong>: JSON-formatted events with timestamps and detailed metadata</li>
      </ul>

      <h3>Management API</h3>
      <ul>
        <li><strong>RESTful Interface</strong>: Full HTTP API for programmatic control</li>
        <li><strong>Status & Metrics</strong>: Query node health, connections, and performance data</li>
        <li><strong>Dynamic Configuration</strong>: Add peers, manage services, and configure routing at runtime</li>
        <li><strong>Security-First</strong>: Sensitive operations restricted to localhost by default</li>
      </ul>

      <h3>Security & Privacy</h3>
      <ul>
        <li><strong>Cryptographic Identities</strong>: Peers and services identified by public keys, not IP addresses</li>
        <li><strong>End-to-End Encryption</strong>: AES-256-GCM encryption for gossipsub messaging</li>
        <li><strong>Digital Signatures</strong>: ECC signatures verify service announcements</li>
        <li><strong>Configurable Trust</strong>: Choose your security/performance tradeoff</li>
      </ul>

      <h2>How It Works</h2>
      <p>
        Banyan acts as a bridge between traditional HTTP services and the LibP2P peer-to-peer network.
        When you start a Banyan node, it:
      </p>
      <ol>
        <li><strong>Establishes Identity</strong>: Generates or loads a cryptographic keypair for the node</li>
        <li><strong>Joins the Network</strong>: Connects to the DHT and begins discovering peers</li>
        <li><strong>Exposes Management API</strong>: Starts a local HTTP server for control and monitoring</li>
        <li><strong>Routes Traffic</strong>: Proxies HTTP requests between your services and remote peers</li>
      </ol>
      <p>
        Your applications interact with Banyan using standard HTTP requests to localhost. Banyan handles
        all the complexity of peer discovery, NAT traversal, and secure communication. The result is a
        decentralized network that feels like working with traditional web services.
      </p>

      <h2>Open Source</h2>
      <p>
        Banyan is free and open-source software, built on the robust LibP2P networking stack.
        The project welcomes contributions from developers, security researchers, and anyone interested
        in decentralized systems. Whether you're fixing bugs, adding features, or improving documentation,
        your contributions help make peer-to-peer networking more accessible to everyone.
      </p>
    </div>
  );
};

export default Home;

