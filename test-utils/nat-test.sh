#!/usr/bin/env bash
set -euo pipefail

# Banyan - NAT Connectivity Test Utility
#
# Starts a local node, then repeatedly attempts to connect to a target peer
# and verify its identity via the management API proxy. Works for both
# NAT-traversed and port-forwarded/DHT-announced nodes.
#
# Usage: ./nat-test.sh <target-peer-id>

if [ $# -lt 1 ]; then
    echo "Usage: $0 <target-peer-id>"
    echo ""
    echo "Starts a local Banyan node and continuously attempts to connect"
    echo "to the given peer ID, verifying its identity via libp2p proxy."
    exit 1
fi

TARGET_PEER_ID="$1"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
LOG_FILE="$SCRIPT_DIR/nat-test-node.log"
NODE_PID=""

cleanup() {
    if [ -n "$NODE_PID" ] && kill -0 "$NODE_PID" 2>/dev/null; then
        kill "$NODE_PID" 2>/dev/null || true
        wait "$NODE_PID" 2>/dev/null || true
    fi
    echo ""
    echo "Test stopped. Node shut down."
}
trap cleanup EXIT INT TERM

# --- Detect platform and set binary path ---
OS="$(uname -s)"
case "$OS" in
    Linux*)  BINARY="$REPO_ROOT/dist/apps/node/linux/banyan" ;;
    Darwin*)
        ARCH="$(uname -m)"
        if [ "$ARCH" = "arm64" ]; then
            BINARY="$REPO_ROOT/dist/apps/node/osx/banyan-arm64"
        else
            BINARY="$REPO_ROOT/dist/apps/node/osx/banyan-amd64"
        fi
        ;;
    *)
        echo "ERROR: Unsupported OS: $OS. Use nat-test.ps1 on Windows."
        exit 1
        ;;
esac

if [ ! -f "$BINARY" ]; then
    echo "ERROR: Node binary not found at $BINARY"
    echo "Build it first: cd $REPO_ROOT/apps/node && make"
    exit 1
fi

CONFIG="$REPO_ROOT/deploy/configs/remote-testing.json"
if [ ! -f "$CONFIG" ]; then
    echo "ERROR: Config not found at $CONFIG"
    exit 1
fi

chmod +x "$BINARY"

echo "========================================"
echo "  Banyan NAT Connectivity Test"
echo "  Starting local node..."
echo "========================================"
echo ""

# Launch node in background
"$BINARY" -config="$CONFIG" > "$LOG_FILE" 2>&1 &
NODE_PID=$!

# Wait for management API URL and Peer ID
API_URL=""
LOCAL_PEER_ID=""
WAITED=0

while [ -z "$API_URL" ] || [ -z "$LOCAL_PEER_ID" ]; do
    if ! kill -0 "$NODE_PID" 2>/dev/null; then
        echo "ERROR: Node process exited unexpectedly."
        tail -20 "$LOG_FILE" 2>/dev/null || true
        exit 1
    fi
    if [ -f "$LOG_FILE" ]; then
        if [ -z "$LOCAL_PEER_ID" ]; then
            LOCAL_PEER_ID=$(grep -oP 'Libp2p node started with Peer ID: \K\S+' "$LOG_FILE" 2>/dev/null || true)
        fi
        if [ -z "$API_URL" ]; then
            API_URL=$(grep -oP 'Management API server available at: \K\S+' "$LOG_FILE" 2>/dev/null || true)
        fi
    fi
    WAITED=$((WAITED + 1))
    if [ "$WAITED" -ge 60 ]; then
        echo "ERROR: Timed out waiting for node to start."
        exit 1
    fi
    sleep 1
done

# JSON helpers (no jq dependency)
json_val() {
    echo "$1" | grep -oP "\"$2\"\s*:\s*\K[^\",}]+" 2>/dev/null | head -1 || echo ""
}

json_str() {
    echo "$1" | grep -oP "\"$2\"\s*:\s*\"\K[^\"]*" 2>/dev/null | head -1 || echo ""
}

# Wait for DHT readiness
echo "  Waiting for DHT to bootstrap..."
DHT_WAIT=0
while true; do
    STATUS=$(curl -s "$API_URL/node/status" 2>/dev/null || echo "")
    if [ -n "$STATUS" ]; then
        DHT_READY=$(json_val "$STATUS" "ready")
        if [ "$DHT_READY" = "true" ]; then
            break
        fi
    fi
    DHT_WAIT=$((DHT_WAIT + 1))
    if [ "$DHT_WAIT" -ge 120 ]; then
        echo "WARNING: DHT not ready after 120s, proceeding anyway."
        break
    fi
    sleep 1
done

# Main test loop
TRY_COUNT=0
CONNECTED="false"
VERIFIED="false"

while true; do
    TRY_COUNT=$((TRY_COUNT + 1))

    # Fetch local node status
    STATUS=$(curl -s "$API_URL/node/status" 2>/dev/null || echo "")
    DHT_SIZE=$(json_val "$STATUS" "routing_size")
    DHT_READY=$(json_val "$STATUS" "ready")
    CONNECTED_PEERS=$(json_val "$STATUS" "total_connected_peers")

    if [ "$DHT_READY" = "true" ]; then
        DHT_DISPLAY="ready ($DHT_SIZE peers)"
    else
        DHT_DISPLAY="bootstrapping..."
    fi

    # Step A: Attempt connection
    CONNECT_RESULT=$(curl -s -X POST "$API_URL/network/connect/$TARGET_PEER_ID" 2>/dev/null || echo "")
    CONNECT_STATUS=$(json_str "$CONNECT_RESULT" "status")

    # Step B: Check connection status
    CONN_DISPLAY="attempting... (try #$TRY_COUNT)"
    CONNS=$(curl -s "$API_URL/network/connections" 2>/dev/null || echo "")
    if echo "$CONNS" | grep -q "$TARGET_PEER_ID" 2>/dev/null; then
        TARGET_CONNECTED=$(echo "$CONNS" | grep -oP "\"libp2p_connected\"\s*:\s*\K(true|false)" 2>/dev/null | head -1 || echo "false")
        if [ "$TARGET_CONNECTED" = "true" ]; then
            CONNECTED="true"
            CONN_DISPLAY="connected"
        fi
    fi

    # Step C: If connected, verify identity via proxy
    VERIFY_DISPLAY="no"
    if [ "$CONNECTED" = "true" ]; then
        REMOTE_STATUS=$(curl -s "$API_URL/proxy/peer/$TARGET_PEER_ID/status" 2>/dev/null || echo "")
        if [ -n "$REMOTE_STATUS" ]; then
            REMOTE_PEER_ID=$(json_str "$REMOTE_STATUS" "peer_id")
            if [ "$REMOTE_PEER_ID" = "$TARGET_PEER_ID" ]; then
                VERIFIED="true"
                VERIFY_DISPLAY="YES - identity confirmed"
            else
                VERIFY_DISPLAY="MISMATCH - got $REMOTE_PEER_ID"
            fi
        else
            VERIFY_DISPLAY="connected, proxy pending..."
        fi
    fi

    # Display
    clear 2>/dev/null || printf '\033[2J\033[H'
    echo "========================================"
    echo "  Banyan NAT Connectivity Test"
    echo "========================================"
    echo ""
    echo "  Local Peer ID:   $LOCAL_PEER_ID"
    echo "  Target Peer ID:  $TARGET_PEER_ID"
    echo ""
    echo "  Local Peers:  $CONNECTED_PEERS connected"
    echo "  DHT:          $DHT_DISPLAY"
    echo "  Connection:   $CONN_DISPLAY"
    echo "  Verified:     $VERIFY_DISPLAY"
    echo ""
    echo "  Press Ctrl+C to stop."
    echo "========================================"

    # Reset connected flag if connection drops
    if [ "$CONNECTED" = "true" ] && [ "$TARGET_CONNECTED" != "true" ]; then
        CONNECTED="false"
        VERIFIED="false"
    fi

    sleep 5
done
