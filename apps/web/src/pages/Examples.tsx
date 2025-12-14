import React from 'react';

const Examples: React.FC = () => {
  return (
    <div className="prose">
      <h1>Usage Examples</h1>

      <h2>Example 1: Basic Peer-to-Peer HTTP Proxy</h2>

      <h3>Terminal 1 - Start first node:</h3>
      <pre><code>{`./banyan -privkey=node1.pem
# Note the peer ID and management URL`}</code></pre>

      <h3>Terminal 2 - Start second node:</h3>
      <pre><code>{`./banyan -privkey=node2.pem
# Note the peer ID and management URL`}</code></pre>

      <h3>Terminal 3 - Connect nodes and test proxy:</h3>
      <pre><code>{`# Connect node2 to node1
curl -X POST http://localhost:54322/libp2p/connect-to-peer/12D3KooWNode1...

# Check connection status
curl http://localhost:54322/libp2p/status

# Proxy a request from node2 to node1
curl http://localhost:54322/libp2p/proxy/12D3KooWNode1.../health`}</code></pre>

      <h2>Example 2: Service Discovery</h2>

      <h3>Terminal 1 - Start service provider:</h3>
      <pre><code>{`./banyan -service-key=service.pem
# This node will provide a service`}</code></pre>

      <h3>Terminal 2 - Start service consumer:</h3>
      <pre><code>{`./banyan -privkey=client.pem

# Find the service
curl -X POST http://localhost:54321/find \\
  -H "Content-Type: application/json" \\
  -d '{"service_key": "base64-service-public-key"}'`}</code></pre>

      <h2>Example 3: Real-time Monitoring</h2>

      <h3>Create monitoring client (monitor.html):</h3>
      <pre><code>{`<!DOCTYPE html>
<html>
<head><title>LibP2P Monitor</title></head>
<body>
    <div id="events"></div>
    <script>
        const ws = new WebSocket('ws://localhost:54321/events/subscribe');
        const eventsDiv = document.getElementById('events');
        
        ws.onmessage = function(event) {
            const data = JSON.parse(event.data);
            const eventElement = document.createElement('div');
            eventElement.textContent = \`\${data.timestamp}: \${data.type} - \${JSON.stringify(data.data)}\`;
            eventsDiv.appendChild(eventElement);
        };
    </script>
</body>
</html>`}</code></pre>

      <h2>Example 4: NAT Traversal Setup</h2>

      <pre><code>{`# Start node with NAT traversal enabled
./banyan -nat-traversal -privkey=nat-node.pem

# The node will automatically:
# - Enable UPnP port mapping
# - Use hole punching for direct connections
# - Enable auto-relay for unreachable peers`}</code></pre>


    </div>
  );
};

export default Examples;

