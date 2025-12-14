import React from 'react';

const API: React.FC = () => {
  return (
    <div className="prose">
      <h1>API Reference</h1>
      <p>
        The node exposes a comprehensive HTTP API on a random localhost port. 
        All endpoints support JSON requests/responses.
      </p>

      <h2>Basic Endpoints</h2>

      <h3>GET <code>/health</code></h3>
      <p>Health check endpoint.</p>
      <pre><code>curl http://localhost:54321/health</code></pre>
      <p><strong>Response:</strong></p>
      <pre><code>{`{
  "status": "healthy",
  "timestamp": "2024-01-15T10:30:45Z"
}`}</code></pre>

      <h3>GET <code>/api/test</code></h3>
      <p>API test endpoint.</p>
      <pre><code>curl http://localhost:54321/api/test</code></pre>

      <h3>GET <code>/</code></h3>
      <p>API documentation and available endpoints.</p>

      <h2>Node Status & Information</h2>

      <h3>GET <code>/libp2p/status</code></h3>
      <p>Get comprehensive node status.</p>
      <pre><code>curl http://localhost:54321/libp2p/status</code></pre>
      <p><strong>Response:</strong></p>
      <pre><code>{`{
  "node_id": "12D3KooWExample...",
  "total_connected_peers": 8,
  "tracked_peers": 3,
  "http_capable_peers": 2,
  "bidirectional_http_peers": 1,
  "connection_types": {
    "manual": 1,
    "gossipsub-lookup": 1,
    "http-verified": 1,
    "background": 5
  },
  "listening_addrs": ["/ip4/127.0.0.1/tcp/12345"],
  "discovery_methods": ["DHT", "mDNS"],
  "services": {
    "beacon_active": true,
    "locators_count": 2
  },
  "timestamp": "2024-01-15T10:30:45Z"
}`}</code></pre>

      <h3>GET <code>/libp2p/connections</code></h3>
      <p>Get detailed connection information.</p>
      <pre><code>curl http://localhost:54321/libp2p/connections</code></pre>
      <p><strong>Response:</strong></p>
      <pre><code>{`{
  "total_tracked": 3,
  "connections": [
    {
      "peer_id": "12D3KooWManual...",
      "alias": "mx7k",
      "status": "connected",
      "connected": "2024-01-15T10:25:30Z",
      "last_activity": "2024-01-15T10:30:00Z",
      "connect_attempts": 1,
      "connection_type": "manual",
      "http_capable": true,
      "bidirectional_http": true,
      "http_test_result": "success",
      "last_http_test": "2024-01-15T10:26:00Z",
      "libp2p_connected": true
    }
  ],
  "timestamp": "2024-01-15T10:30:45Z"
}`}</code></pre>

      <h2>Peer Management</h2>

      <h3>POST <code>/libp2p/connect-to-peer/{"{peerID}"}</code></h3>
      <p>Connect to a specific peer by ID.</p>
      <pre><code>curl -X POST http://localhost:54321/libp2p/connect-to-peer/12D3KooWExample...</code></pre>
      <p><strong>Response:</strong></p>
      <pre><code>Attempting to connect to peer 12D3KooWExample... (DHT first, then gossipsub if needed)</code></pre>

      <h3>POST <code>/libp2p/peers/add</code></h3>
      <p>Manually add a tracked peer (localhost only).</p>
      <pre><code>{`curl -X POST http://localhost:54321/libp2p/peers/add \\
  -H "Content-Type: application/json" \\
  -d '{
    "peer_id": "12D3KooWExample...",
    "connection_type": "manual",
    "http_capable": true
  }'`}</code></pre>

      <h3>GET <code>/libp2p/peers/all</code></h3>
      <p>Get all peer information (localhost only).</p>

      <h2>HTTP Proxy</h2>

      <h3>ANY <code>/libp2p/proxy/{"{peerID}"}/{"{path}"}</code></h3>
      <p>Proxy HTTP requests through libp2p to a peer.</p>
      <pre><code>{`# GET request to peer's /status endpoint
curl http://localhost:54321/libp2p/proxy/12D3KooWExample.../status

# POST request with data
curl -X POST http://localhost:54321/libp2p/proxy/12D3KooWExample.../api/data \\
  -H "Content-Type: application/json" \\
  -d '{"key": "value"}'`}</code></pre>

      <h3>ANY <code>/out/{"{alias}"}/{"{path}"}</code></h3>
      <p>Proxy via peer alias (4-character shorthand).</p>
      <pre><code>{`# Same as above but using alias
curl http://localhost:54321/out/mx7k/status`}</code></pre>

      <h2>Service Management</h2>

      <h3>POST <code>/find</code></h3>
      <p>Find a service by its compressed public key.</p>
      <pre><code>{`curl -X POST http://localhost:54321/find \\
  -H "Content-Type: application/json" \\
  -d '{
    "service_key": "base64-encoded-public-key",
    "encrypted": false
  }'`}</code></pre>

      <h3>GET <code>/locators</code></h3>
      <p>List all active service locators.</p>
      <pre><code>curl http://localhost:54321/locators</code></pre>

      <h3>POST <code>/serve</code></h3>
      <p>Start a service beacon with a PEM private key.</p>
      <pre><code>{`curl -X POST http://localhost:54321/serve \\
  -H "Content-Type: application/json" \\
  -d '{
    "private_key_pem": "-----BEGIN PRIVATE KEY-----\\n...\\n-----END PRIVATE KEY-----",
    "mode": "announce"
  }'`}</code></pre>

      <h3>POST <code>/beacon/suspend</code></h3>
      <p>Suspend the active service beacon.</p>
      <pre><code>curl -X POST http://localhost:54321/beacon/suspend</code></pre>

      <h3>POST <code>/beacon/kill</code></h3>
      <p>Terminate the active service beacon.</p>
      <pre><code>curl -X POST http://localhost:54321/beacon/kill</code></pre>

      <h2>Router Management</h2>

      <h3>POST <code>/router/add</code></h3>
      <p>Add or update a URL route mapping.</p>
      <pre><code>{`curl -X POST http://localhost:54321/router/add \\
  -H "Content-Type: application/json" \\
  -d '{
    "identifier": "api-server",
    "url": "http://localhost:8080"
  }'`}</code></pre>

      <h3>GET <code>/router/list</code></h3>
      <p>List all configured routes.</p>
      <pre><code>curl http://localhost:54321/router/list</code></pre>
    </div>
  );
};

export default API;

