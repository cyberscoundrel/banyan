#!/usr/bin/env bash
set -euo pipefail

# Banyan Node - Remote Testing Launcher (Portable)
#
# Expects the banyan binary in the same directory as this script.
# Config resolution:
#   1. remote-testing.json next to this script (flash drive / flat dir)
#   2. ../../configs/remote-testing.json (full deploy/ folder structure)

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# --- Locate binary ---
BINARY="$SCRIPT_DIR/banyan"
if [ ! -f "$BINARY" ]; then
    echo "ERROR: banyan binary not found at $BINARY"
    echo "Place the Linux banyan binary in the same directory as this script."
    exit 1
fi

# --- Locate config ---
CONFIG=""
if [ -f "$SCRIPT_DIR/remote-testing.json" ]; then
    CONFIG="$SCRIPT_DIR/remote-testing.json"
elif [ -f "$SCRIPT_DIR/../../configs/remote-testing.json" ]; then
    CONFIG="$(cd "$SCRIPT_DIR/../../configs" && pwd)/remote-testing.json"
else
    echo "ERROR: remote-testing.json not found."
    echo "Looked in:"
    echo "  $SCRIPT_DIR/remote-testing.json"
    echo "  $SCRIPT_DIR/../../configs/remote-testing.json"
    echo ""
    echo "Place remote-testing.json next to this script or ensure the"
    echo "deploy/configs/ directory is intact."
    exit 1
fi

# --- Display startup info ---
echo "========================================"
echo "  Banyan Node - Remote Testing"
echo "========================================"
echo ""
echo "  Binary:  $BINARY"
echo "  Config:  $CONFIG"
echo ""
echo "  NAT Traversal:  enabled"
echo "  Listen Address:  /ip4/0.0.0.0/tcp/9000"
echo ""
echo "========================================"
echo ""

chmod +x "$BINARY"
exec "$BINARY" -config="$CONFIG"
