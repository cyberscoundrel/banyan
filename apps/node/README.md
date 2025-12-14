# Banyan

A comprehensive libp2p node implementation that provides HTTP proxying capabilities, bidirectional connections, and gossipsub-based peer discovery with cryptographic features.

## Features

- **HTTP Proxy over LibP2P**: Acts as an HTTP proxy that forwards requests to a regular HTTP server using libp2p-http
- **Bidirectional Connections**: Automatically creates bidirectional connections when peers connect
- **Gossipsub Peer Discovery**: Uses gossipsub topic for peer lookup requests and responses
- **DHT + MDNS Discovery**: Combines DHT and MDNS for robust peer discovery
- **AES Encrypted Communication**: Uses AES-256-GCM with ECDH-derived keys for secure messaging
- **Digital Signatures**: Provides message authentication using ECC signatures
- **Direct Dial Capability**: Allows targeted peers to dial directly for responses
- **Regular HTTP Management API**: Standard HTTP server for testing and node management
- **Connection Status Tracking**: Enhanced connection management with status history
- **Real-Time Event Streaming**: WebSocket-based event streaming for live monitoring and debugging

## Features

### Core Components

1. **LibP2P HTTP Server**: Listens for HTTP requests over libp2p protocol
2. **Regular HTTP Server**: Standard HTTP API for testing and management (accessible via curl/Postman)
3. **HTTP Proxy**: Forwards incoming libp2p HTTP requests to a target HTTP server
4. **Peer Discovery**: Uses both DHT and MDNS for bootstrap peer discovery
5. **Gossipsub Messaging**: Handles peer lookup requests/responses over gossipsub
6. **Connection Management**: Tracks bidirectional connections with status history
7. **Cryptographic Layer**: AES encryption and ECC signing for secure communications
8. **Real-Time Event Streaming**: WebSocket-based event system for live monitoring

### Event Streaming Architecture

The system includes a real-time event streaming mechanism:

- **LibP2P Node → Test Server**: WebSocket connection (`/events/stream`) forwards all node events
- **Test Server → Clients**: WebSocket endpoint (`/events/subscribe`) broadcasts events to subscribers
- **Event Hub**: Central hub that manages client connections and event distribution
- **Event Types**: Peer connections, HTTP tests, gossipsub messages, DHT lookups, etc.

#### Event Types
- `peer_connected`: When a peer establishes a connection
- `peer_disconnected`: When a peer disconnects
- `peer_tracked`: When a peer is added to HTTP capability tracking
- `http_test`: Results of HTTP bidirectional connectivity tests
- `gossip_message`: Gossipsub message events (future)
- `dht_lookup`: DHT lookup events (future)
- `proxy_request`: HTTP proxy request events (future)

### Message Types

#### Lookup Request
```json
{
  "type": "lookup",
  "target": "peer_id_to_find",
  "from": "requester_peer_id",
  "publicKey": [binary_ecc_key],
  "timestamp": "2023-..."
}
```

#### Lookup Response
```json
{
  "type": "response",
  "target": "original_target_peer_id",
  "from": "responder_peer_id", 
  "addresses": ["multiaddr1", "multiaddr2"],
  "publicKey": [binary_ecc_key],
  "signature": [binary_signature],
  "encrypted": false,
  "encryptedData": [binary_aes_encrypted_data],
  "nonce": [aes_gcm_nonce],
  "timestamp": "2023-..."
}
```

## Usage

### Configuration

Banyan supports two ways to configure the node:
1. **Command-line flags** - for quick testing and overrides
2. **JSON configuration file** - for persistent configuration

Command-line flags always take precedence over configuration file values.

### Configuration File

Use the `-config` flag to specify a JSON configuration file:

```bash
./banyan -config=./node-config.json
```

#### Configuration File Format

See `example-node-config.json` for a complete example. All fields are optional:

```json
{
  "privkey": "./keys/node-identity.pem",
  "servicesDir": "./services",
  "figsDir": "./figs",
  "listen": "/ip4/0.0.0.0/tcp/9000",
  "disableDht": false,
  "noAnnounce": false,
  "anonLookups": false,
  "noCrypto": false,
  "natTraversal": true,
  "routerConfig": "./router-config.json",
  "addonsDir": "./addons",
  "beaconIncludePeerIdAnnouncements": true,
  "beaconPeerIdDirectOnly": false,
  "allowExpiredFigs": false,
  "allowInsecureFigs": false,
  "bootstraps": [
    "/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN",
    "/ip4/104.131.131.82/tcp/4001/p2p/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ"
  ]
}
```

**Bootstrap Peers**: The `bootstraps` field allows you to override the default libp2p bootstrap peers. It can be specified in two ways:
1. **Inline array**: `"bootstraps": ["/ip4/...", "/ip4/..."]`
2. **File path**: `"bootstraps": "./bootstraps.json"` (file must contain a JSON array)

This is useful for:
- Private networks with custom bootstrap nodes
- Testing with specific peer configurations
- Isolated network deployments
- Sharing bootstrap lists across multiple configurations

If not specified, the node will use the default libp2p bootstrap peers.

### Command Line Arguments

```bash
./banyan [options]
```

#### Options:
- `-config <path>`: Path to JSON configuration file
- `-privkey <path>`: Path to PEM file containing private key for peer identity and AES encryption
- `-services-dir <path>`: Directory containing services.json and per-service subfolders
- `-figs-dir <path>`: Directory containing fig files
- `-listen <multiaddr>`: Multiaddr to listen on (default: `/ip4/0.0.0.0/tcp/0`)
- `-disable-dht`: Disable DHT peer discovery and lookup (useful for local testing)
- `-no-announce`: Do not announce this node's presence via DHT or pubsub
- `-anon-lookups`: Allow anonymous lookups and responses
- `-no-crypto`: Disable encryption on pubsub messages
- `-nat-traversal`: Enable NAT traversal (hole punching, auto-relay, UPnP)
- `-router-config <path>`: Path to JSON file containing router configuration
- `-addons-dir <path>`: Directory containing addons.json and addon executables
- `-beacon-include-peerid-announcements`: Include beacon's PeerID in public announcements (default: true)
- `-beacon-peerid-direct-only`: Only include beacon's PeerID in direct replies to locators
- `-allow-expired-figs`: Allow loading expired fig files (SECURITY RISK)
- `-allow-insecure-figs`: Allow loading fig files with invalid/missing signatures (SECURITY RISK)

#### Examples:

**Using a configuration file:**
```bash
./banyan -config=./node-config.json
```

**Using configuration file with command-line overrides:**
```bash
# Config file sets most options, but override the listen address
./banyan -config=./node-config.json -listen="/ip4/0.0.0.0/tcp/8080"
```

**Using default generated keys:**
```bash
./banyan
```

**Using custom PEM private key:**
```bash
./banyan -privkey=./keys/node1.pem
```

**Local testing without DHT (MDNS only):**
```bash
./banyan -disable-dht
```

The same private key is used for both:
1. **LibP2P Peer Identity**: Your node's unique peer ID is derived from this key
2. **AES Encryption**: A 256-bit AES key is derived from the same private key for message encryption

This ensures that peers using the same private key will have consistent peer IDs and encryption keys across restarts.

### PEM Key Format
The private key should be in PEM format. Supported key types:
- RSA keys (PKCS#1 or PKCS#8 format)
- Other key types supported by libp2p crypto

Example PEM file:
```
-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA1234567890abcdef...
...
-----END RSA PRIVATE KEY-----
```

### Build and Run

1. **Build the main proxy node:**
   ```bash
   go build -o banyan main.go server.go
   ```

2. **Start the node:**
   ```bash
   ./banyan
   ```
   This starts the node with a generated peer identity and opens a HTTP management server.

3. **View your peer ID:**
   The node will emit a `node_started` event through the WebSocket stream showing:
   ```json
   {
     "type": "node_started",
     "data": {
       "peer_id": "12D3KooWExample...",
       "addresses": ["/ip4/127.0.0.1/tcp/12345/p2p/12D3KooWExample..."]
     }
   }
   ```

### Real-Time Event Monitoring

The node provides comprehensive event streaming for monitoring:

1. **Connect to event stream:**
   ```javascript
   const ws = new WebSocket('ws://localhost:<port>/events/subscribe');
   ws.onmessage = (event) => console.log(JSON.parse(event.data));
   ```

2. **Monitor peer connections:**
   - `peer_connected`: When peers establish connections
   - `peer_disconnected`: When peers disconnect
   - `peer_tracked`: When peers are tracked for HTTP capabilities

3. **Watch discovery events:**
   - `discovery`: MDNS/DHT peer discovery
   - `dht_lookup`: DHT-based peer lookups
   - `connection`: Connection establishment/failures

4. **Monitor HTTP activity:**
   - `proxy_request`: HTTP requests proxied through libp2p
   - `http_test`: Bidirectional HTTP connectivity tests

### Connection Management

The system automatically categorizes and tracks peer connections:

**Connection Types:**
- `manual`: Manually connected via API calls
- `gossipsub-lookup`: Discovered via gossipsub peer lookup responses  
- `http-verified`: Verified to have HTTP server capability
- `background`: Background discovered peers (DHT/MDNS)

**HTTP Capability Tracking:**
- Only tracks peers likely to have HTTP servers (not all background peers)
- Tests bidirectional HTTP connectivity
- Maintains connection history and quality metrics

### API Endpoints

Once running, the node exposes several HTTP endpoints:

```bash
# Basic endpoints
GET /health                           # Health check
GET /api/test                         # API test endpoint

# LibP2P status and control
GET /libp2p/status                    # Node status and peer count
GET /libp2p/connections               # Detailed connection information
POST /libp2p/connect-to-peer/{peerID} # Manually connect to a peer
```

### Peer Discovery and Connection

1. **Automatic Discovery**: The node automatically discovers peers via DHT and MDNS
2. **Manual Connection**: Connect to specific peers using their peer ID
3. **Gossipsub Lookup**: Find peers by broadcasting lookup requests
4. **HTTP Testing**: Automatically tests HTTP connectivity with discovered peers

### Testing with Regular HTTP API

Each node exposes a **regular HTTP management server** on a random localhost port for testing:

```
Regular HTTP management server listening on: http://127.0.0.1:54321
Test endpoints:
  GET  http://127.0.0.1:54321/status
  GET  http://127.0.0.1:54321/connections  
  POST http://127.0.0.1:54321/connect-to-peer/{peerID}
  GET  http://127.0.0.1:54321/hello
```

### Testing with Postman/curl

#### 1. Get Node Status
```bash
curl http://127.0.0.1:54321/status
```
Response:
```json
{
  "node_id": "12D3KooWExample...",
  "total_connected_peers": 8,
  "tracked_peers": 3,
  "http_capable_peers": 2,
  "bidirectional_http_peers": 1,
  "connection_types": {
    "manual": 1,
    "gossipsub-lookup": 1,
    "http-verified": 1
  },
  "listening_addrs": ["/ip4/127.0.0.1/tcp/12345"],
  "proxy_target": "http://localhost:8080",
  "timestamp": "2023-..."
}
```

#### 2. View Connection History
```bash
curl http://127.0.0.1:54321/connections
```
Response:
```json
{
  "total_tracked": 3,
  "connections": [
    {
      "peer_id": "12D3KooWManual...",
      "status": "connected",
      "connected": "2023-...",
      "last_activity": "2023-...",
      "connect_attempts": 1,
      "connection_type": "manual",
      "http_capable": true,
      "bidirectional_http": true,
      "http_test_result": "success",
      "last_http_test": "2023-...",
      "libp2p_connected": true
    },
    {
      "peer_id": "12D3KooWGossip...",
      "status": "connected",
      "connected": "2023-...",
      "last_activity": "2023-...",
      "connect_attempts": 0,
      "connection_type": "gossipsub-lookup",
      "http_capable": true,
      "bidirectional_http": false,
      "http_test_result": "failed",
      "last_http_test": "2023-...",
      "libp2p_connected": true
    }
  ],
  "timestamp": "2023-..."
}
```

#### 3. Connect to Specific Peer
```bash
curl -X POST http://127.0.0.1:54321/connect-to-peer/12D3KooWExample...
```
Response:
```
Attempting to connect to peer 12D3KooWExample... (DHT first, then gossipsub if needed)
```

#### 4. API Discovery
```bash
curl http://127.0.0.1:54321/
```
Response:
```json
{
  "message": "LibP2P Node Management API",
  "node_id": "12D3KooWExample...",
  "endpoints": {
    "GET /status": "Node status information",
    "GET /connections": "Connection tracking information",
    "POST /connect-to-peer/{peerID}": "Connect to a specific peer",
    "GET /hello": "Simple hello endpoint"
  }
}
```

### Peer Discovery Flow

1. **Bootstrap**: Nodes connect to DHT bootstrap peers
2. **Advertisement**: Nodes advertise themselves on the gossipsub topic
3. **Discovery**: Nodes discover each other via DHT and MDNS
4. **Manual Connection**: Use `/connect-to-peer/{peerID}` endpoint to connect to specific peers
5. **DHT Lookup**: First attempts DHT lookup for fast direct connection
6. **Gossipsub Fallback**: Falls back to gossipsub lookup if DHT fails
7. **Connection**: Establishes connections using discovered addresses

### Security Features

#### AES Encryption
- **AES-256-GCM**: Industry standard authenticated encryption
- **ECDH Key Derivation**: Shared secrets derived from ECC keys
- **Perfect Forward Secrecy**: New nonce for each encrypted message

#### Digital Signatures
All lookup responses are signed:
- Response data is signed with responder's ECC private key
- Recipients verify signatures using the provided public key
- Uses libp2p's native ECC keys (Ed25519/secp256k1)

#### Connection Status Tracking
- **Smart Peer Classification**: Distinguishes between HTTP-capable and gossipsub-only peers
- **Connection Types**: 
  - `manual`: Manually connected via `/connect-to-peer/` endpoint
  - `gossipsub-lookup`: Peers that respond to gossipsub lookup requests
  - `http-verified`: Peers with verified HTTP server capability
  - `background`: Background discovered peers (DHT/MDNS) - not tracked for HTTP
- **HTTP Capability Detection**: Tests and tracks which peers support libp2p HTTP
- **Bidirectional HTTP Status**: Tracks whether HTTP communication works in both directions
- **HTTP Test Results**: Records success/failure of HTTP connectivity tests
- **Selective Tracking**: Only tracks peers likely to have HTTP servers (not all background peers)
- **Status History**: Tracks "connected", "disconnected", "connecting" states
- **Connection Attempts**: Counts retry attempts for reliability metrics
- **Automatic Cleanup**: Removes old disconnected peers after 24 hours
- **Last Activity**: Tracks recent communication for connection quality

## Configuration

### Environment Variables
- `LIBP2P_LISTEN_ADDR`: Listen address for libp2p (default: `/ip4/0.0.0.0/tcp/0`)

### Command Line Arguments
```bash
./banyan <target_http_server> [target_peer_id_to_lookup]
```

- `target_http_server`: HTTP server to proxy requests to (e.g., `http://localhost:8080`)
- `target_peer_id_to_lookup`: Optional peer ID to lookup after 10 seconds (for testing)

## API Endpoints

### LibP2P HTTP Endpoints (over libp2p)

Each node exposes these endpoints over libp2p HTTP:

- `GET /ping`: Health check endpoint (returns "pong")
- `POST /lookup-response`: Receives direct lookup responses
- `GET|POST /`: Main proxy endpoint (forwards to target server)
- `GET|POST /proxy/*`: Alternative proxy endpoint with prefix
- `GET /status`: Node status information
- `GET /connections`: Connection tracking details
- `POST /connect-to-peer/{peerID}`: Connect to specific peer

### Regular HTTP Endpoints (for testing)

Each node also exposes a regular HTTP server on localhost for testing:

- `GET /`: API documentation and available endpoints
- `GET /hello`: Simple hello endpoint
- `GET /status`: Node status information  
- `GET /connections`: Connection tracking details
- `POST /connect-to-peer/{peerID}`: Connect to specific peer (DHT first, gossipsub fallback)

### Test Server Endpoints

The included test server provides:

- `GET /health`: Health check
- `ANY /api/test`: API test endpoint
- `ANY /`: Echo endpoint (returns request details)
- `GET /libp2p/status`: Proxy to libp2p node status
- `GET /libp2p/connections`: Proxy to libp2p connections
- `POST /libp2p/connect-to-peer/{peerID}`: Proxy peer connection requests
- `WS /events/stream`: WebSocket endpoint for libp2p node to send events
- `WS /events/subscribe`: WebSocket endpoint for clients to receive events

## Network Protocols

### LibP2P Protocols Used
- `/libp2p-http/1.0.0`: LibP2P HTTP transport protocol
- `/meshsub/1.1.0`: GossipSub for peer discovery messages
- DHT protocols for peer routing
- MDNS for local peer discovery

### Connection Flow

1. **Regular HTTP Request**: Made to management API for testing
2. **LibP2P HTTP Request**: Received over libp2p HTTP for proxying
3. **Bidirectional Setup**: Node attempts to create connection back to client
4. **Proxy Forward**: Request is forwarded to target HTTP server
5. **Response Return**: HTTP response is returned over libp2p or regular HTTP

## Development

### Dependencies
- `github.com/libp2p/go-libp2p`: Core libp2p functionality
- `github.com/libp2p/go-libp2p-http`: HTTP over libp2p transport
- `github.com/libp2p/go-libp2p-gostream`: Stream-based networking
- `github.com/libp2p/go-libp2p-pubsub`: GossipSub implementation
- `github.com/libp2p/go-libp2p-kad-dht`: Kademlia DHT

### Key Files
- `main.go`: Main proxy node implementation
- `testserver/main.go`: Simple HTTP test server
- `peerdiscovery.go`: Additional peer discovery utilities (if needed)

## Service Fig Files

Banyan supports service fig files for service discovery and authentication. A fig file defines:
- Service aliases for easy identification
- Hierarchical key structures for path-based access control
- Expiration timestamps for security
- Authority signatures for governance
- Service signatures for authentication

### Getting Signed Fig Files

The `/services/fig/{serviceKey}` endpoint allows nodes to retrieve signed fig files for specific service keys:

**Local (Management Server):**
```bash
# GET request
curl http://localhost:8080/services/fig/08011220b1d18a38cf2464e9afc79c1f35498bb788f7c5734be94c2d3cdbcbaea681a4de

# POST request with nonce
curl -X POST http://localhost:8080/services/fig/08011220b1d18a38cf2464e9afc79c1f35498bb788f7c5734be94c2d3cdbcbaea681a4de \
  -H "Content-Type: application/json" \
  -d '{"nonce": "abc123def456"}'
```

**Remote (LibP2P HTTP):**
The same endpoint is available on the libp2p HTTP server for peer-to-peer fig file requests.

**Testing:**
```bash
# Run the test script
./test-service-fig-endpoint.sh

# Or with PowerShell
./test-service-fig-endpoint.ps1 -WithNonce -WithRequesterPeerId
```

See [SERVICE_FIG_ENDPOINT.md](SERVICE_FIG_ENDPOINT.md) for complete documentation.

## Example Use Cases

1. **Development Testing**: Use regular HTTP API to test node functionality
2. **Decentralized Web Proxy**: Route HTTP traffic through libp2p network
3. **Service Discovery**: Find and connect to specific services by peer ID or service key
4. **Fig File Distribution**: Request signed fig files from nodes serving specific service keys
5. **Load Balancing**: Distribute HTTP requests across multiple backend servers
6. **NAT Traversal**: Access HTTP services behind NATs using libp2p
7. **Encrypted Routing**: Securely route HTTP traffic with end-to-end encryption

## Testing Workflow

1. **Start Test Server**: `./testserver/test-server` (port 8080)
2. **Start Node 1**: `./banyan http://localhost:8080`
3. **Start Node 2**: `./banyan http://localhost:8080` 
4. **Test Management API**: Use curl/Postman on the regular HTTP ports
5. **Connect Peers**: Use `/connect-to-peer/{peerID}` endpoint
6. **Monitor Status**: Check `/status` and `/connections` endpoints
7. **Real-Time Monitoring**: Open `event-client.html` in browser for live events
8. **Test Proxy**: Make requests through libp2p HTTP (when connected)

## Real-Time Event Monitoring

### Using the HTML Client

1. **Open the client**: Open `event-client.html` in your browser
2. **Enter server port**: Use the test server port (e.g., 63580)
3. **Connect**: Click "Connect" to start receiving events
4. **Monitor**: Watch real-time events as peers connect, disconnect, and communicate

### Using WebSocket Directly

```javascript
// Connect to event stream
const ws = new WebSocket('ws://localhost:63580/events/subscribe');

ws.onmessage = function(event) {
    const data = JSON.parse(event.data);
    console.log('Event:', data.type, data.data);
};
```

### Event Examples

```json
{
  "type": "peer_connected",
  "timestamp": "2023-...",
  "data": {
    "peer_id": "12D3KooWExample...",
    "time": "2023-..."
  },
  "node_id": "12D3KooWLocal..."
}

{
  "type": "peer_tracked",
  "timestamp": "2023-...",
  "data": {
    "peer_id": "12D3KooWExample...",
    "connection_type": "manual",
    "http_capable": true,
    "time": "2023-..."
  },
  "node_id": "12D3KooWLocal..."
}

{
  "type": "http_test",
  "timestamp": "2023-...",
  "data": {
    "peer_id": "12D3KooWExample...",
    "test_result": "success",
    "bidirectional_http": true,
    "time": "2023-..."
  },
  "node_id": "12D3KooWLocal..."
}
```

## Limitations

- HTTP/1.1 only (no HTTP/2 support)
- AES encryption requires proper ECDH implementation in production
- Gossipsub messages are not guaranteed delivery
- DHT lookups may be slow for new peers

## Future Enhancements

- WebSocket support over libp2p
- HTTP/2 and HTTP/3 support
- Proper ECIES implementation
- Peer reputation system
- Load balancing algorithms
- Metrics and monitoring dashboard 