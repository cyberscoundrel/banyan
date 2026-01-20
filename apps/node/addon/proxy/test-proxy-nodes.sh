#!/bin/bash
# Integration test script for HTTP proxy addon between two Banyan nodes
# This script tests the proxy addon's ability to route traffic via .peer addresses

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration - paths relative to workspace root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORKSPACE_ROOT="$(cd "$SCRIPT_DIR/../../../.." && pwd)"
DIST_DIR="$WORKSPACE_ROOT/dist/apps/node"
NODE1_DIR="$WORKSPACE_ROOT/test-node1"
NODE2_DIR="$WORKSPACE_ROOT/test-node2"
TEST_SERVER_PORT=9999
CLEANUP_ON_EXIT=true

# Parse arguments
while [[ "$#" -gt 0 ]]; do
    case $1 in
        --no-cleanup) CLEANUP_ON_EXIT=false ;;
        *) echo "Unknown parameter: $1"; exit 1 ;;
    esac
    shift
done

# Cleanup function
cleanup() {
    echo -e "${YELLOW}Cleaning up...${NC}"

    # Kill background processes
    [ -n "$NODE1_PID" ] && kill $NODE1_PID 2>/dev/null || true
    [ -n "$NODE2_PID" ] && kill $NODE2_PID 2>/dev/null || true
    [ -n "$TEST_SERVER_PID" ] && kill $TEST_SERVER_PID 2>/dev/null || true

    # Remove test directories
    if [ "$CLEANUP_ON_EXIT" = true ]; then
        rm -rf "$NODE1_DIR" "$NODE2_DIR"
    fi

    echo -e "${GREEN}Cleanup complete${NC}"
}

trap cleanup EXIT

echo -e "${BLUE}=== Banyan HTTP Proxy Integration Test ===${NC}"

# Detect platform and set executable path
if [[ "$OSTYPE" == "darwin"* ]]; then
    ARCH=$(uname -m)
    if [[ "$ARCH" == "arm64" ]]; then
        BANYAN_EXEC="$DIST_DIR/osx/banyan-arm64"
    else
        BANYAN_EXEC="$DIST_DIR/osx/banyan-amd64"
    fi
    ADDON_EXT=""
else
    # Linux
    BANYAN_EXEC="$DIST_DIR/linux/banyan"
    ADDON_EXT=""
fi

# Check prerequisites
if [ ! -f "$BANYAN_EXEC" ]; then
    echo -e "${RED}Error: banyan executable not found at $BANYAN_EXEC${NC}"
    echo "Please build the node first: nx build node"
    exit 1
fi

# Create test directories
mkdir -p "$NODE1_DIR/addons" "$NODE2_DIR/addons"

# Build proxy addon for current platform
echo -e "${YELLOW}Building proxy addon...${NC}"
cd "$WORKSPACE_ROOT"
go build -o "$NODE1_DIR/addons/proxy-addon$ADDON_EXT" ./apps/node/addon/proxy
cp "$NODE1_DIR/addons/proxy-addon$ADDON_EXT" "$NODE2_DIR/addons/"

# Define proxy ports for each node
NODE1_PROXY_PORT=18080
NODE2_PROXY_PORT=18081

# Create addons.json for both nodes
cat > "$NODE1_DIR/addons/addons.json" <<EOF
{
  "addons": [
    { "name": "proxy-addon", "exec": "./proxy-addon$ADDON_EXT", "args": ["--port", "$NODE1_PROXY_PORT"] }
  ]
}
EOF

cat > "$NODE2_DIR/addons/addons.json" <<EOF
{
  "addons": [
    { "name": "proxy-addon", "exec": "./proxy-addon$ADDON_EXT", "args": ["--port", "$NODE2_PROXY_PORT"] }
  ]
}
EOF

# Start simple test HTTP server
echo -e "${YELLOW}Starting test HTTP server on port $TEST_SERVER_PORT...${NC}"
python3 -c "
import http.server
import socketserver

class TestHandler(http.server.SimpleHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header('Content-type', 'text/plain')
        self.end_headers()
        self.wfile.write(b'Hello from test server!')
    def log_message(self, format, *args):
        pass

with socketserver.TCPServer(('', $TEST_SERVER_PORT), TestHandler) as httpd:
    httpd.serve_forever()
" &
TEST_SERVER_PID=$!
sleep 2

# Start Node 1
echo -e "${YELLOW}Starting Node 1 (proxy on port $NODE1_PROXY_PORT)...${NC}"
"$BANYAN_EXEC" -listen="/ip4/127.0.0.1/tcp/0" -addons-dir="$NODE1_DIR/addons" > "$NODE1_DIR/node1.log" 2>&1 &
NODE1_PID=$!

# Start Node 2
echo -e "${YELLOW}Starting Node 2 (proxy on port $NODE2_PROXY_PORT)...${NC}"
"$BANYAN_EXEC" -listen="/ip4/127.0.0.1/tcp/0" -addons-dir="$NODE2_DIR/addons" > "$NODE2_DIR/node2.log" 2>&1 &
NODE2_PID=$!

# Wait for nodes to start
echo -e "${YELLOW}Waiting for nodes to initialize...${NC}"
sleep 10

# Extract management ports
NODE1_MGMT_PORT=$(grep -o "Management API server available at.*127.0.0.1:\([0-9]*\)" "$NODE1_DIR/node1.log" | grep -o "[0-9]*$" | head -1)
NODE2_MGMT_PORT=$(grep -o "Management API server available at.*127.0.0.1:\([0-9]*\)" "$NODE2_DIR/node2.log" | grep -o "[0-9]*$" | head -1)

if [ -z "$NODE1_MGMT_PORT" ] || [ -z "$NODE2_MGMT_PORT" ]; then
    echo -e "${RED}Failed to extract management ports${NC}"
    cat "$NODE1_DIR/node1.log"
    cat "$NODE2_DIR/node2.log"
    exit 1
fi

echo -e "${GREEN}Node 1 management port: $NODE1_MGMT_PORT${NC}"
echo -e "${GREEN}Node 2 management port: $NODE2_MGMT_PORT${NC}"

# Get node IDs
NODE1_ID=$(curl -s "http://127.0.0.1:$NODE1_MGMT_PORT/node/status" | grep -o '"node_id":"[^"]*"' | cut -d'"' -f4)
NODE2_ID=$(curl -s "http://127.0.0.1:$NODE2_MGMT_PORT/node/status" | grep -o '"node_id":"[^"]*"' | cut -d'"' -f4)

if [ -z "$NODE1_ID" ] || [ -z "$NODE2_ID" ]; then
    echo -e "${RED}Failed to get node IDs${NC}"
    exit 1
fi

echo -e "${GREEN}Node 1 ID: $NODE1_ID${NC}"
echo -e "${GREEN}Node 2 ID: $NODE2_ID${NC}"

# Connect nodes
echo -e "${YELLOW}Connecting Node 1 to Node 2...${NC}"
curl -s -X POST "http://127.0.0.1:$NODE1_MGMT_PORT/network/connect/$NODE2_ID" > /dev/null || true
sleep 3

# Verify connection
CONNECTIONS=$(curl -s "http://127.0.0.1:$NODE1_MGMT_PORT/network/connections")
if echo "$CONNECTIONS" | grep -q "$NODE2_ID"; then
    echo -e "${GREEN}Nodes connected successfully${NC}"
else
    echo -e "${YELLOW}Warning: Connection may not be established${NC}"
fi

TEST_PASS=0
TEST_FAIL=0

# Test 1: Proxy addon status endpoint
echo -e "${BLUE}Test 1 - Proxy addon status${NC}"
RESPONSE=$(curl -s "http://127.0.0.1:$NODE1_MGMT_PORT/addons/proxy/status" || echo "FAILED")
if echo "$RESPONSE" | grep -q '"status":"running"'; then
    echo -e "${GREEN}[PASS] Proxy addon is running${NC}"
    ((TEST_PASS++))
else
    echo -e "${RED}[FAIL] Proxy addon status check failed: $RESPONSE${NC}"
    ((TEST_FAIL++))
fi

# Test 2: Direct HTTP through proxy
echo -e "${BLUE}Test 2 - Direct HTTP through proxy${NC}"
RESPONSE=$(curl -s --proxy "http://127.0.0.1:$NODE1_PROXY_PORT" --connect-timeout 10 "http://127.0.0.1:$TEST_SERVER_PORT/" || echo "FAILED")
if [ "$RESPONSE" = "Hello from test server!" ]; then
    echo -e "${GREEN}[PASS] Direct HTTP proxy access successful${NC}"
    ((TEST_PASS++))
else
    echo -e "${RED}[FAIL] Direct HTTP proxy access failed: $RESPONSE${NC}"
    ((TEST_FAIL++))
fi

# Test 3: Proxy via management API to peer
echo -e "${BLUE}Test 3 - Proxy via management API to peer${NC}"
RESPONSE=$(curl -s "http://127.0.0.1:$NODE1_MGMT_PORT/proxy/peer/$NODE2_ID/ping" --connect-timeout 10 || echo "FAILED")
if echo "$RESPONSE" | grep -q '"status":"ok"'; then
    echo -e "${GREEN}[PASS] P2P proxy to peer successful${NC}"
    ((TEST_PASS++))
else
    echo -e "${RED}[FAIL] P2P proxy to peer failed: $RESPONSE${NC}"
    ((TEST_FAIL++))
fi

echo ""
echo -e "${BLUE}=== Test Summary ===${NC}"
echo -e "${GREEN}Passed: $TEST_PASS${NC}"
echo -e "${RED}Failed: $TEST_FAIL${NC}"

if [ "$TEST_FAIL" -gt 0 ]; then
    echo -e "${YELLOW}Check logs for details:${NC}"
    echo "  Node 1: $NODE1_DIR/node1.log"
    echo "  Node 2: $NODE2_DIR/node2.log"
    exit 1
fi

echo -e "${GREEN}All tests passed!${NC}"
exit 0

