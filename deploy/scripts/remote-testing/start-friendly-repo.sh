#!/usr/bin/env bash
set -euo pipefail

# Banyan Node - Remote Testing (User-Friendly, Repo)
#
# Hides raw node output and displays only key status information.
# Uses the node binary from the repo build output and the config from
# deploy/configs/. Designed for developers who have cloned and built the repo.

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
LOG_FILE="$SCRIPT_DIR/banyan-node.log"
NODE_PID=""

cleanup() {
    if [ -n "$NODE_PID" ] && kill -0 "$NODE_PID" 2>/dev/null; then
        kill "$NODE_PID" 2>/dev/null || true
        wait "$NODE_PID" 2>/dev/null || true
    fi
    echo ""
    echo "Node stopped."
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
        echo "ERROR: Unsupported OS: $OS"
        echo "Use start-friendly-repo.ps1 on Windows."
        exit 1
        ;;
esac

# --- Validate binary exists ---
if [ ! -f "$BINARY" ]; then
    echo "ERROR: Node binary not found at $BINARY"
    echo ""
    echo "Build it first from the repo root:"
    echo "  cd $REPO_ROOT/apps/node"
    echo "  make $(echo "$OS" | tr '[:upper:]' '[:lower:]')"
    exit 1
fi

# --- Locate config ---
CONFIG="$REPO_ROOT/deploy/configs/remote-testing.json"
if [ ! -f "$CONFIG" ]; then
    echo "ERROR: Config not found at $CONFIG"
    exit 1
fi

chmod +x "$BINARY"

echo "========================================"
echo "  Banyan Node - Starting..."
echo "========================================"
echo ""

# Launch node in background, capture output to log
"$BINARY" -config="$CONFIG" > "$LOG_FILE" 2>&1 &
NODE_PID=$!

# Wait for management API URL and Peer ID from log output
API_URL=""
PEER_ID=""
WAIT_TIMEOUT=60
WAITED=0

while [ -z "$API_URL" ] || [ -z "$PEER_ID" ]; do
    if ! kill -0 "$NODE_PID" 2>/dev/null; then
        echo "ERROR: Node process exited unexpectedly."
        echo "Check the log file for details: $LOG_FILE"
        tail -20 "$LOG_FILE" 2>/dev/null || true
        exit 1
    fi

    if [ -f "$LOG_FILE" ]; then
        if [ -z "$PEER_ID" ]; then
            PEER_ID=$(grep -oP 'Libp2p node started with Peer ID: \K\S+' "$LOG_FILE" 2>/dev/null || true)
        fi
        if [ -z "$API_URL" ]; then
            API_URL=$(grep -oP 'Management API server available at: \K\S+' "$LOG_FILE" 2>/dev/null || true)
        fi
    fi

    WAITED=$((WAITED + 1))
    if [ "$WAITED" -ge "$WAIT_TIMEOUT" ]; then
        echo "ERROR: Timed out waiting for node to start."
        echo "Check the log file: $LOG_FILE"
        exit 1
    fi
    sleep 1
done

# JSON field extraction without jq dependency
json_val() {
    local json="$1" key="$2"
    echo "$json" | grep -oP "\"$key\"\s*:\s*\K[^\",}]+" 2>/dev/null | head -1 || echo ""
}

json_str() {
    local json="$1" key="$2"
    echo "$json" | grep -oP "\"$key\"\s*:\s*\"\K[^\"]*" 2>/dev/null | head -1 || echo ""
}

# Display and poll loop
echo ""
while true; do
    STATUS_JSON=$(curl -s "$API_URL/node/status" 2>/dev/null || echo "")

    DHT_READY="..."
    DHT_SIZE="0"
    NAT_REACHABILITY="..."
    RELAY_ADDR="false"
    CONNECTED_PEERS="0"

    if [ -n "$STATUS_JSON" ]; then
        CONNECTED_PEERS=$(json_val "$STATUS_JSON" "total_connected_peers")
        DHT_READY=$(json_val "$STATUS_JSON" "ready")
        DHT_SIZE=$(json_val "$STATUS_JSON" "routing_size")
        NAT_REACHABILITY=$(json_str "$STATUS_JSON" "reachability")
        RELAY_ADDR=$(json_val "$STATUS_JSON" "relay_addr")
    fi

    # Format DHT status
    if [ "$DHT_READY" = "true" ]; then
        DHT_DISPLAY="ready ($DHT_SIZE peers in routing table)"
    else
        DHT_DISPLAY="bootstrapping..."
    fi

    # Format NAT status
    case "$NAT_REACHABILITY" in
        Public)  NAT_DISPLAY="public (directly reachable)" ;;
        Private) NAT_DISPLAY="private (behind NAT)" ;;
        *)       NAT_DISPLAY="detecting..." ;;
    esac

    # Format relay status
    if [ "$RELAY_ADDR" = "true" ]; then
        RELAY_DISPLAY="connected (relay address acquired)"
    else
        RELAY_DISPLAY="searching for relays..."
    fi

    # Clear screen and display status
    clear 2>/dev/null || printf '\033[2J\033[H'
    echo "========================================"
    echo "  Banyan Node - Remote Testing"
    echo "========================================"
    echo ""
    echo "  Peer ID:  $PEER_ID"
    echo ""
    echo "  Peers:    $CONNECTED_PEERS connected"
    echo "  DHT:      $DHT_DISPLAY"
    echo "  NAT:      $NAT_DISPLAY"
    echo "  Relay:    $RELAY_DISPLAY"
    echo ""
    echo "  Press Ctrl+C to stop."
    echo "========================================"

    sleep 3
done
