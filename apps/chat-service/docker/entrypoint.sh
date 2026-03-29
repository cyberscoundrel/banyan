#!/bin/sh
set -e

IDENTITY_FILE="/app/data/node-identity.pem"
KEYS_DIR="/app/keys"

generate_identity() {
    if [ ! -f "$IDENTITY_FILE" ]; then
        echo "Generating node identity key..."
        mkdir -p "$(dirname "$IDENTITY_FILE")"
        
        node -e "
const crypto = require('crypto');
const { privateKey } = crypto.generateKeyPairSync('ed25519');
const pem = privateKey.export({ type: 'pkcs8', format: 'pem' });
require('fs').writeFileSync('$IDENTITY_FILE', pem, { mode: 0o600 });
"
        echo "Node identity generated at $IDENTITY_FILE"
    else
        echo "Using existing node identity at $IDENTITY_FILE"
    fi
}

echo "Starting Banyan Chat Service (${NODE_TYPE:-unknown} node)"
echo "Keys directory: $KEYS_DIR"
echo "Data directory: /app/data"

generate_identity

if [ -n "$SYNC_PEERS" ]; then
    export EXTRA_ARGS="$EXTRA_ARGS -sync-peers=$SYNC_PEERS"
fi

exec node /app/dist/index.js \
    --listen="${LISTEN_ADDR:-:8080}" \
    --data-dir=/app/data \
    ${EXTRA_ARGS:+$EXTRA_ARGS} \
    "$@"
