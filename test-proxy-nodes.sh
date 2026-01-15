#!/bin/bash
# Test script for HTTP proxy access between two Banyan nodes

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
DIST_DIR="./dist/apps/node"
NODE1_DIR="./test-node1"
NODE2_DIR="./test-node2"
TEST_SERVER_PORT=8080
CLEANUP_ON_EXIT=true

# Cleanup function
cleanup() {
    echo -e "${YELLOW}Cleaning up...${NC}"
    
    # Kill background processes
    if [ ! -z "$NODE1_PID" ]; then
        kill $NODE1_PID 2>/dev/null || true
    fi
    if [ ! -z "$NODE2_PID" ]; then
        kill $NODE2_PID 2>/dev/null || true
    fi
    if [ ! -z "$TEST_SERVER_PID" ]; then
        kill $TEST_SERVER_PID 2>/dev/null || true
    fi
    
    # Remove test directories
    if [ "$CLEANUP_ON_EXIT" = true ]; then
        rm -rf "$NODE1_DIR" "$NODE2_DIR"
    fi
    
    echo -e "${GREEN}Cleanup complete${NC}"
}

# Set trap for cleanup
trap cleanup EXIT

echo -e "${BLUE}=== Banyan HTTP Proxy Test ===${NC}"

# Check if dist directory exists
if [ ! -d "$DIST_DIR" ]; then
    echo -e "${RED}Error: $DIST_DIR not found${NC}"
    echo "Please build the node first: nx build node"
    exit 1
fi

# Detect platform and set executable path
if [[ "$OSTYPE" == "darwin"* ]]; then
    # macOS - detect architecture
    ARCH=$(uname -m)
    if [[ "$ARCH" == "arm64" ]]; then
        BANYAN_EXEC="$DIST_DIR/banyan-arm64"
    else
        BANYAN_EXEC="$DIST_DIR/banyan-amd64"
    fi
elif [[ "$OSTYPE" == "msys" || "$OSTYPE" == "cygwin" ]]; then
    # Windows
    BANYAN_EXEC="$DIST_DIR/banyan.exe"
else
    # Linux
    BANYAN_EXEC="$DIST_DIR/banyan"
fi

# Check if banyan executable exists
if [ ! -f "$BANYAN_EXEC" ]; then
    echo -e "${RED}Error: banyan executable not found at $BANYAN_EXEC${NC}"
    echo "Please build banyan first: nx build node"
    exit 1
fi

# Create test directories
mkdir -p "$NODE1_DIR/addons" "$NODE2_DIR/addons"

# Build proxy addon for current platform
echo -e "${YELLOW}Building proxy addon...${NC}"
if [[ "$OSTYPE" == "msys" || "$OSTYPE" == "cygwin" ]]; then
    go build -o "$NODE1_DIR/addons/proxy-addon.exe" ./apps/node/addon/proxy
    go build -o "$NODE2_DIR/addons/proxy-addon.exe" ./apps/node/addon/proxy
    ADDON_EXEC="./proxy-addon.exe"
else
    go build -o "$NODE1_DIR/addons/proxy-addon" ./apps/node/addon/proxy
    go build -o "$NODE2_DIR/addons/proxy-addon" ./apps/node/addon/proxy
    ADDON_EXEC="./proxy-addon"
fi

# Create addons.json for both nodes
cat > "$NODE1_DIR/addons/addons.json" <<EOF
{
  "addons": [
    { "name": "proxy-addon", "exec": "$ADDON_EXEC" }
  ]
}
EOF

cp "$NODE1_DIR/addons/addons.json" "$NODE2_DIR/addons/addons.json"

# Start simple test HTTP server
echo -e "${YELLOW}Starting test HTTP server on port $TEST_SERVER_PORT...${NC}"
python3 -c "
import http.server
import socketserver
import threading

class TestHandler(http.server.SimpleHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header('Content-type', 'text/plain')
        self.end_headers()
        self.wfile.write(b'Hello from test server!')

with socketserver.TCPServer(('', $TEST_SERVER_PORT), TestHandler) as httpd:
    httpd.serve_forever()
" &
TEST_SERVER_PID=$!

# Wait for test server to start
sleep 2

# Start Node 1
echo -e "${YELLOW}Starting Node 1...${NC}"
cd "$NODE1_DIR"
"../$BANYAN_EXEC" -listen="/ip4/127.0.0.1/tcp/0" -addons-dir="./addons" > node1.log 2>&1 &
NODE1_PID=$!
cd ..

# Start Node 2  
echo -e "${YELLOW}Starting Node 2...${NC}"
cd "$NODE2_DIR"
"../$BANYAN_EXEC" -listen="/ip4/127.0.0.1/tcp/0" -addons-dir="./addons" > node2.log 2>&1 &
NODE2_PID=$!
cd ..

# Wait for nodes to start
echo -e "${YELLOW}Waiting for nodes to initialize...${NC}"
sleep 5

# Extract node information
NODE1_MGMT_PORT=$(grep "Management API server available at" "$NODE1_DIR/node1.log" | grep -o "127.0.0.1:[0-9]*" | cut -d: -f2)
NODE2_MGMT_PORT=$(grep "Management API server available at" "$NODE2_DIR/node2.log" | grep -o "127.0.0.1:[0-9]*" | cut -d: -f2)

if [ -z "$NODE1_MGMT_PORT" ] || [ -z "$NODE2_MGMT_PORT" ]; then
    echo -e "${RED}Failed to extract management ports${NC}"
    echo "Node 1 log:"
    cat "$NODE1_DIR/node1.log"
    echo "Node 2 log:"
    cat "$NODE2_DIR/node2.log"
    exit 1
fi

echo -e "${GREEN}Node 1 management port: $NODE1_MGMT_PORT${NC}"
echo -e "${GREEN}Node 2 management port: $NODE2_MGMT_PORT${NC}"

# Get node IDs
NODE1_ID=$(curl -s "http://127.0.0.1:$NODE1_MGMT_PORT/status" | grep -o '"node_id":"[^"]*"' | cut -d'"' -f4)
NODE2_ID=$(curl -s "http://127.0.0.1:$NODE2_MGMT_PORT/status" | grep -o '"node_id":"[^"]*"' | cut -d'"' -f4)

if [ -z "$NODE1_ID" ] || [ -z "$NODE2_ID" ]; then
    echo -e "${RED}Failed to get node IDs${NC}"
    exit 1
fi

echo -e "${GREEN}Node 1 ID: $NODE1_ID${NC}"
echo -e "${GREEN}Node 2 ID: $NODE2_ID${NC}"

# Connect nodes
echo -e "${YELLOW}Connecting Node 1 to Node 2...${NC}"
curl -s -X POST "http://127.0.0.1:$NODE1_MGMT_PORT/connect-to-peer/$NODE2_ID" > /dev/null

# Wait for connection
sleep 3

# Verify connection
CONNECTIONS=$(curl -s "http://127.0.0.1:$NODE1_MGMT_PORT/connections")
if echo "$CONNECTIONS" | grep -q "$NODE2_ID"; then
    echo -e "${GREEN}Nodes connected successfully${NC}"
else
    echo -e "${RED}Failed to connect nodes${NC}"
    exit 1
fi

# Get proxy port from Node 1 (assuming it starts on 8080 by default)
PROXY_PORT=8080

# Test HTTP proxy access
echo -e "${YELLOW}Testing HTTP proxy access...${NC}"

# Test 1: Access test server through Node 1's proxy using Node 2's .peer address
echo -e "${BLUE}Test 1: HTTP CONNECT to .peer address${NC}"
RESPONSE=$(curl -s --proxy "http://127.0.0.1:$PROXY_PORT" \
    --connect-timeout 10 \
    "http://$NODE2_ID.peer:$TEST_SERVER_PORT/" || echo "FAILED")

if [ "$RESPONSE" = "Hello from test server!" ]; then
    echo -e "${GREEN}✓ HTTP proxy .peer access successful${NC}"
else
    echo -e "${RED}✗ HTTP proxy .peer access failed: $RESPONSE${NC}"
fi

# Test 2: Direct HTTP request through proxy
echo -e "${BLUE}Test 2: Direct HTTP through proxy${NC}"
RESPONSE=$(curl -s --proxy "http://127.0.0.1:$PROXY_PORT" \
    --connect-timeout 10 \
    "http://127.0.0.1:$TEST_SERVER_PORT/" || echo "FAILED")

if [ "$RESPONSE" = "Hello from test server!" ]; then
    echo -e "${GREEN}✓ Direct HTTP proxy access successful${NC}"
else
    echo -e "${RED}✗ Direct HTTP proxy access failed: $RESPONSE${NC}"
fi

echo -e "${BLUE}=== Test Summary ===${NC}"
echo -e "${GREEN}HTTP Proxy tests completed${NC}"
echo -e "${YELLOW}Check logs for detailed information:${NC}"
echo -e "  Node 1: $NODE1_DIR/node1.log"
echo -e "  Node 2: $NODE2_DIR/node2.log"