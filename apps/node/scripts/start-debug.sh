#!/bin/bash
# Banyan Node Debug Startup Script (Linux/macOS)
# This script launches the banyan node with a debug/test configuration
# for local development and testing.

set -e

# Get the directory where this script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Detect platform and set executable name and debug directory
if [[ "$OSTYPE" == "darwin"* ]]; then
    # macOS - detect architecture
    ARCH=$(uname -m)
    if [[ "$ARCH" == "arm64" ]]; then
        BANYAN_EXEC="$SCRIPT_DIR/banyan-arm64"
    else
        BANYAN_EXEC="$SCRIPT_DIR/banyan-amd64"
    fi
    DEBUG_DIR="$(dirname "$SCRIPT_DIR")/debug/osx"
else
    # Linux
    BANYAN_EXEC="$SCRIPT_DIR/banyan"
    DEBUG_DIR="$(dirname "$SCRIPT_DIR")/debug/linux"
fi

# Default debug directories
DEFAULT_SERVICES_DIR="$DEBUG_DIR/services"
DEFAULT_ADDONS_DIR="$DEBUG_DIR/addons"
DEFAULT_FIGS_DIR="$DEBUG_DIR/figs"

# Default values
LISTEN="/ip4/127.0.0.1/tcp/0"
CONFIG_FILE=""
PRIV_KEY=""
SERVICES_DIR="$DEFAULT_SERVICES_DIR"
ADDONS_DIR="$DEFAULT_ADDONS_DIR"
FIGS_DIR="$DEFAULT_FIGS_DIR"
NO_CRYPTO=""
DISABLE_DHT=""
ALLOW_EXPIRED=""
ALLOW_INSECURE=""
BARE_MODE=false

show_help() {
    echo "Banyan Debug Startup Script"
    echo ""
    echo "Usage: ./start-debug.sh [options]"
    echo ""
    echo "Options:"
    echo "  -h, --help           Show this help message"
    echo "  -b, --bare           Run with no arguments (use banyan defaults)"
    echo "  -l, --listen ADDR    Listen address (default: /ip4/127.0.0.1/tcp/0)"
    echo "  -c, --config PATH    Config file to use"
    echo "  -k, --privkey PATH   Private key file"
    echo "  -s, --services PATH  Services directory (default: ../debug/<platform>/services)"
    echo "  -a, --addons PATH    Addons directory (default: ../debug/<platform>/addons)"
    echo "  -f, --figs PATH      Figs directory (default: ../debug/<platform>/figs)"
    echo "  --no-crypto          Disable encryption for testing"
    echo "  --disable-dht        Disable DHT discovery"
    echo "  --allow-expired      Allow expired figs for testing"
    echo "  --allow-insecure     Allow insecure figs for testing"
    echo ""
    echo "Examples:"
    echo "  ./start-debug.sh"
    echo "  ./start-debug.sh --no-crypto --disable-dht"
    echo "  ./start-debug.sh -c my-config.json"
    echo "  ./start-debug.sh -l /ip4/0.0.0.0/tcp/9999"
    echo "  ./start-debug.sh --bare"
    exit 0
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        -h|--help)
            show_help
            ;;
        -b|--bare)
            BARE_MODE=true
            shift
            ;;
        -l|--listen)
            LISTEN="$2"
            shift 2
            ;;
        -c|--config)
            CONFIG_FILE="$2"
            shift 2
            ;;
        -k|--privkey)
            PRIV_KEY="$2"
            shift 2
            ;;
        -s|--services)
            SERVICES_DIR="$2"
            shift 2
            ;;
        -a|--addons)
            ADDONS_DIR="$2"
            shift 2
            ;;
        -f|--figs)
            FIGS_DIR="$2"
            shift 2
            ;;
        --no-crypto)
            NO_CRYPTO="-no-crypto"
            shift
            ;;
        --disable-dht)
            DISABLE_DHT="-disable-dht"
            shift
            ;;
        --allow-expired)
            ALLOW_EXPIRED="-allow-expired-figs"
            shift
            ;;
        --allow-insecure)
            ALLOW_INSECURE="-allow-insecure-figs"
            shift
            ;;
        *)
            echo "Unknown option: $1"
            show_help
            ;;
    esac
done

# Check if banyan executable exists
if [ ! -f "$BANYAN_EXEC" ]; then
    echo "Error: banyan executable not found at $BANYAN_EXEC"
    echo "Make sure you're running this script from the build output directory."
    exit 1
fi

# Make sure it's executable
chmod +x "$BANYAN_EXEC"

# Handle bare mode
if [ "$BARE_MODE" = true ]; then
    echo "=== Banyan Node (Bare Mode) ==="
    echo "Executable: $BANYAN_EXEC"
    echo "==============================="
    echo ""
    echo "Starting Banyan node..."
    exec "$BANYAN_EXEC"
fi

# Build arguments
ARGS="-listen=\"$LISTEN\""
ARGS="$ARGS -services-dir=\"$SERVICES_DIR\""
ARGS="$ARGS -addons-dir=\"$ADDONS_DIR\""
ARGS="$ARGS -figs-dir=\"$FIGS_DIR\""

if [ -n "$CONFIG_FILE" ]; then
    ARGS="$ARGS -config=\"$CONFIG_FILE\""
fi

if [ -n "$PRIV_KEY" ]; then
    ARGS="$ARGS -privkey=\"$PRIV_KEY\""
fi

ARGS="$ARGS $NO_CRYPTO $DISABLE_DHT $ALLOW_EXPIRED $ALLOW_INSECURE"

# Display startup info
echo "=== Banyan Node Debug Mode ==="
echo "Executable:   $BANYAN_EXEC"
echo "Services Dir: $SERVICES_DIR"
echo "Addons Dir:   $ADDONS_DIR"
echo "Figs Dir:     $FIGS_DIR"
echo "Arguments:    $ARGS"
echo "=============================="
echo ""

# Start the node
echo "Starting Banyan node..."
eval "$BANYAN_EXEC" $ARGS

