import React from 'react';

const Usage: React.FC = () => {
  return (
    <div className="prose">
      <h1>Basic Usage</h1>

      <h2>1. Start a Node</h2>
      <pre><code>{`# Start with default settings
./banyan

# Start with custom private key
./banyan -privkey=./keys/node1.pem

# Start with service key (to provide services)
./banyan -service-key=./keys/service1.pem

# Start without DHT (local testing)
./banyan -disable-dht

# Start with custom listen address
./banyan -listen="/ip4/0.0.0.0/tcp/8080"`}</code></pre>

      <h2>2. Node Output</h2>
      <p>When started, the node will display:</p>
      <pre><code>{`Libp2p node started with Peer ID: 12D3KooWExample...
Management API server available at: http://localhost:54321
Node started at: 2024-01-15 10:30:45
Listening addresses:
  /ip4/127.0.0.1/tcp/12345/p2p/12D3KooWExample...
  /ip4/192.168.1.100/tcp/12345/p2p/12D3KooWExample...
Node is running. Press Ctrl+C to stop.`}</code></pre>

      <h2>Command Line Options</h2>
      <table>
        <thead>
          <tr>
            <th>Flag</th>
            <th>Description</th>
            <th>Default</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td><code>-privkey</code></td>
            <td>Path to PEM private key file for peer identity</td>
            <td>Generated</td>
          </tr>
          <tr>
            <td><code>-service-key</code></td>
            <td>Path to PEM service key file for service identity</td>
            <td>None</td>
          </tr>
          <tr>
            <td><code>-disable-dht</code></td>
            <td>Disable DHT peer discovery (mDNS only)</td>
            <td>false</td>
          </tr>
          <tr>
            <td><code>-no-announce</code></td>
            <td>Don't announce service presence</td>
            <td>false</td>
          </tr>
          <tr>
            <td><code>-anon-lookups</code></td>
            <td>Allow anonymous service lookups</td>
            <td>false</td>
          </tr>
          <tr>
            <td><code>-listen</code></td>
            <td>LibP2P listen multiaddress</td>
            <td><code>/ip4/0.0.0.0/tcp/0</code></td>
          </tr>
          <tr>
            <td><code>-no-crypto</code></td>
            <td>Disable pubsub message encryption</td>
            <td>false</td>
          </tr>
          <tr>
            <td><code>-nat-traversal</code></td>
            <td>Enable NAT traversal (UPnP, hole punching)</td>
            <td>false</td>
          </tr>
          <tr>
            <td><code>-kubo-url</code></td>
            <td>Kubo (IPFS) node URL to connect to</td>
            <td>None</td>
          </tr>
          <tr>
            <td><code>-router-config</code></td>
            <td>Path to JSON router configuration file</td>
            <td>None</td>
          </tr>
        </tbody>
      </table>

      <h2>Configuration Files</h2>

      <h3>Router Configuration (JSON)</h3>
      <pre><code>{`{
  "api-server": "http://localhost:8080",
  "web-server": "http://localhost:3000",
  "database": "http://localhost:5432"
}`}</code></pre>
      <p>Load with: <code>-router-config=./config/routes.json</code></p>

      <h3>Private Key Files (PEM Format)</h3>
      <pre><code>{`-----BEGIN PRIVATE KEY-----
MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC...
-----END PRIVATE KEY-----`}</code></pre>

      <h2>Real-time Event Monitoring</h2>

      <h3>WebSocket Connection</h3>
      <pre><code>{`// Connect to event stream
const ws = new WebSocket('ws://localhost:54321/events/subscribe');

ws.onmessage = function(event) {
    const data = JSON.parse(event.data);
    console.log('Event:', data.type, data.data);
};

ws.onopen = function() {
    console.log('Connected to event stream');
};`}</code></pre>

      <h3>Event Types</h3>
      <table>
        <thead>
          <tr>
            <th>Event Type</th>
            <th>Description</th>
            <th>Example Data</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td><code>peer_connected</code></td>
            <td>Peer establishes connection</td>
            <td><code>{`{"peer_id": "12D3...", "time": "..."}`}</code></td>
          </tr>
          <tr>
            <td><code>peer_disconnected</code></td>
            <td>Peer disconnects</td>
            <td><code>{`{"peer_id": "12D3...", "time": "..."}`}</code></td>
          </tr>
          <tr>
            <td><code>peer_tracked</code></td>
            <td>Peer added to tracking</td>
            <td><code>{`{"peer_id": "12D3...", "connection_type": "manual"}`}</code></td>
          </tr>
          <tr>
            <td><code>http_test</code></td>
            <td>HTTP capability test result</td>
            <td><code>{`{"peer_id": "12D3...", "test_result": "success"}`}</code></td>
          </tr>
          <tr>
            <td><code>discovery</code></td>
            <td>Peer discovery events</td>
            <td><code>{`{"method": "DHT", "peer_id": "12D3..."}`}</code></td>
          </tr>
          <tr>
            <td><code>proxy_request</code></td>
            <td>HTTP proxy activity</td>
            <td><code>{`{"target_peer": "12D3...", "path": "/api"}`}</code></td>
          </tr>
          <tr>
            <td><code>service_announcement</code></td>
            <td>Service availability</td>
            <td><code>{`{"service_key": "abc123...", "peer_id": "12D3..."}`}</code></td>
          </tr>
          <tr>
            <td><code>error</code></td>
            <td>Error conditions</td>
            <td><code>{`{"message": "Connection failed", "error": "..."}`}</code></td>
          </tr>
          <tr>
            <td><code>info</code></td>
            <td>General information</td>
            <td><code>{`{"message": "DHT bootstrap completed"}`}</code></td>
          </tr>
        </tbody>
      </table>


    </div>
  );
};

export default Usage;