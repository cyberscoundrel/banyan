#!/usr/bin/env bash
set -euo pipefail

# Banyan Node - Remote Testing Launcher (Repo)
#
# Uses the node binary from the repo build output and the config from
# deploy/configs/. Designed for developers who have cloned and built the repo.

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

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
        echo "Use start-repo.ps1 on Windows."
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
    echo ""
    echo "Or build all platforms:"
    echo "  make all"
    exit 1
fi

# --- Locate config ---
CONFIG="$REPO_ROOT/deploy/configs/remote-testing.json"
if [ ! -f "$CONFIG" ]; then
    echo "ERROR: Config not found at $CONFIG"
    echo "The deploy/configs/ directory may be missing or incomplete."
    exit 1
fi

# --- Display startup info ---
echo "========================================"
echo "  Banyan Node - Remote Testing (Repo)"
echo "========================================"
echo ""
echo "  Binary:     $BINARY"
echo "  Config:     $CONFIG"
echo "  Repo Root:  $REPO_ROOT"
echo ""
echo "  NAT Traversal:  enabled"
echo "  Listen Address:  /ip4/0.0.0.0/tcp/9000"
echo ""
echo "========================================"
echo ""

chmod +x "$BINARY"
exec "$BINARY" -config="$CONFIG"
