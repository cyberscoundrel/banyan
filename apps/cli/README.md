# Banyan CLI

A terminal user interface (TUI) and web dashboard for managing Banyan nodes.

## Features

- **Terminal UI (TUI)**: Interactive terminal interface using Bubble Tea
- **Web Dashboard**: React-based web UI with feature parity to the CLI
- **Node Management**: Start, stop, and monitor Banyan nodes
- **Live Status**: Real-time view of node state and peer connections
- **WebSocket Events**: Live log of node events
- **Manual Operations**: Commands for peer connections, service keys, and proxying

## Installation

```bash
cd apps/cli
go build -o banyan-cli .
```

### Building the Web UI (Optional)

```bash
cd apps/cli/server/web
npm install
npm run build
```

## Configuration

Create a `banyan-cli.json` file in the same directory as the executable:

```json
{
  "nodeExecutable": "/path/to/banyan-node",
  "nodeConfigPath": "/path/to/node-config.json",
  "defaultNodeAddress": "http://localhost:8080",
  "webPort": 8080
}
```

- `nodeExecutable` (required): Path to the Banyan node executable
- `nodeConfigPath` (optional): Path to node configuration file
- `defaultNodeAddress` (optional): Default node address to connect to
- `webPort` (optional): Port for the web UI server (default: 8080)

## Usage

```bash
# Start with TUI (default)
./banyan-cli

# Connect to a specific node
./banyan-cli --node http://localhost:9000

# Run web UI only (no TUI)
./banyan-cli --web

# Run on different port
./banyan-cli --port 3000

# Disable web UI
./banyan-cli --no-web
```

## TUI Navigation

- **Tab / Shift+Tab**: Switch between views
- **1-5**: Jump to specific view
- **↑/↓**: Navigate command history
- **Ctrl+C or 'q'**: Quit

## Views

1. **Status**: Node information and peer statistics
2. **Events**: Live WebSocket event log
3. **Peers**: Connected peers list and management
4. **Services**: Service management (figs, beacons)
5. **Help**: Command reference

## Commands

### Connection
- `connect <address>` - Connect to a node
- `start` - Start a new node (requires config)
- `stop` - Stop the managed node
- `status` - Refresh node status

### Peers
- `peers` - List all connected peers
- `peer connect <id>` - Connect to peer by ID or alias
- `add <id> <addr>` - Add peer with multiaddr

### Services
- `services` - List configured services
- `figs` - List service figs
- `find <key|alias>` - Find a service
- `serve start <file>` - Start service beacon
- `serve stop [hash]` - Stop service beacon

### Proxy
- `proxy peer <id> <path> [method]` - Proxy through peer
- `proxy service <key> <path> [method]` - Proxy through service

### Routes
- `route list` - List configured routes
- `route add <path> <target>` - Add a route

### Other
- `clear` - Clear event log
- `help` - Show help

## Web API

The CLI also exposes a REST API for the web dashboard:

- `GET /api/status` - Node status
- `GET /api/peers` - Peer list
- `POST /api/connect` - Connect to node
- `POST /api/peer/connect` - Connect to peer
- `POST /api/peer/add` - Add peer
- `GET /api/services` - List services
- `GET /api/figs` - List figs
- `POST /api/find` - Find service
- `POST /api/serve/start` - Start beacon
- `POST /api/serve/stop` - Stop beacon
- `POST /api/proxy` - Proxy request
- `GET /api/routes` - List routes
- `POST /api/routes` - Add route
- `POST /api/node/start` - Start node
- `POST /api/node/stop` - Stop node
- `POST /api/execute` - Execute CLI command
- `WS /api/events` - WebSocket event stream

