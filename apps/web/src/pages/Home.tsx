import React from 'react';

const Home: React.FC = () => {
  return (
    <div className="prose">
      <h1>Banyan</h1>
      <p>
        A peer-to-peer networking multitool. Turn your traditional web applications and microservices into peer-to-peer applications without changing a single line of code. Written in Go.
      </p>

      <h2>What is This Tool?</h2>
      <p>
        Banyan is a feature-rich peer-to-peer networking tool that enables:
      </p>
      <ul>
        <li><strong>Peer-to-Peer Networking</strong>: Connect to other nodes in the network and communicate bidirectionally with restful HTTP requests</li>
        <li><strong>Resources and Microservices</strong>: Hit shared microservices and expose resources to any peer</li>
        <li><strong>Built-in User Auth</strong>: Locally hosted keys and built in authentication allows your service to store minimal user data</li>
        <li><strong>Service Discovery</strong>: Find and connect to services across the decentralized network using cryptographic keys and plaintext aliases</li>
        <li><strong>Peer Management</strong>: Discover, connect to, and manage connections with other nodes</li>
        <li><strong>Real-time Monitoring</strong>: Stream events and connection status in real-time via WebSocket</li>
        <li><strong>Secure Communication</strong>: AES-256-GCM encryption and ECC digital signatures for secure peer discovery</li>
        <li><strong>Flexible Routing</strong>: Dynamic URL routing and alias-based proxy endpoints</li>
      </ul>
      <p>
        This tool is ideal for building decentralized web services, creating secure tunnels through NATs, 
        implementing service meshes, or any scenario requiring peer-to-peer HTTP communication.
      </p>

      <h2>Current Capabilities</h2>

      <h3>Core Features</h3>

      <h4>1. HTTP Proxy Functionality</h4>
      <ul>
        <li>Proxy HTTP requests through libp2p to remote peers</li>
        <li>Support for all HTTP methods (GET, POST, PUT, DELETE, etc.)</li>
        <li>Bidirectional HTTP communication testing</li>
        <li>Connection status tracking and quality metrics</li>
        <li>Alias-based routing for easier peer access</li>
      </ul>

      <h4>2. Peer Discovery & Connection Management</h4>
      <ul>
        <li><strong>DHT (Distributed Hash Table)</strong>: For wide-area peer discovery</li>
        <li><strong>mDNS</strong>: For local network peer discovery</li>
        <li><strong>GossipSub</strong>: For peer lookup requests and responses</li>
        <li><strong>Manual Connection</strong>: Direct connection to specific peer IDs</li>
        <li><strong>Connection Tracking</strong>: Comprehensive status history and metrics</li>
      </ul>

      <h4>3. Service Management</h4>
      <ul>
        <li><strong>Service Beacons</strong>: Advertise services using cryptographic keys</li>
        <li><strong>Service Locators</strong>: Find services by their public keys</li>
        <li><strong>Service Announcements</strong>: Broadcast service availability</li>
        <li><strong>Encrypted Service Discovery</strong>: Optional encryption for service queries</li>
      </ul>

      <h4>4. Cryptographic Security</h4>
      <ul>
        <li><strong>AES-256-GCM Encryption</strong>: For secure message transmission</li>
        <li><strong>ECDH Key Exchange</strong>: For deriving shared encryption keys</li>
        <li><strong>ECC Digital Signatures</strong>: For message authentication</li>
        <li><strong>Service Key Management</strong>: Separate keys for service identification</li>
      </ul>

      <h4>5. Real-time Event System</h4>
      <ul>
        <li><strong>WebSocket Streaming</strong>: Live event broadcasting</li>
        <li><strong>Event Types</strong>: Connection, discovery, proxy, service, and error events</li>
        <li><strong>Node Status Monitoring</strong>: Real-time peer and service status</li>
        <li><strong>Debugging Support</strong>: Comprehensive event logging</li>
      </ul>

      <h4>6. Management API</h4>
      <ul>
        <li><strong>RESTful HTTP API</strong>: For node control and monitoring</li>
        <li><strong>Peer Management</strong>: Add, remove, and query peer connections</li>
        <li><strong>Service Control</strong>: Start/stop services and manage beacons</li>
        <li><strong>Router Management</strong>: Dynamic URL routing configuration</li>
        <li><strong>Health Monitoring</strong>: Node status and diagnostics</li>
      </ul>

      <h3>Supported Protocols</h3>
      <ul>
        <li><strong>LibP2P HTTP Transport</strong>: <code>/http-proxy/1.0.0</code></li>
        <li><strong>GossipSub Messaging</strong>: <code>/meshsub/1.1.0</code></li>
        <li><strong>Kademlia DHT</strong>: For peer routing</li>
        <li><strong>mDNS</strong>: For local discovery</li>
        <li><strong>WebSocket</strong>: For real-time events</li>
      </ul>

      <h2>How It Works</h2>
      <p>Banyan creates a peer-to-peer network where nodes can:</p>
      <ol>
        <li><strong>Discover Peers</strong>: Automatically find other nodes via DHT, mDNS, or gossipsub messaging</li>
        <li><strong>Proxy HTTP Requests</strong>: Forward HTTP requests through the libp2p network to remote peers</li>
        <li><strong>Manage Services</strong>: Advertise and discover services using cryptographic keys or aliases</li>
        <li><strong>Monitor Activity</strong>: Stream real-time events via WebSocket for network visibility</li>
      </ol>
      <p>
        The architecture consists of a LibP2P node core with discovery, connection management, 
        service management, and HTTP proxy capabilities, all controlled via a RESTful management API.
      </p>
    </div>
  );
};

export default Home;

