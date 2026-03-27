#!/bin/sh
set -e

IDENTITY_FILE="/app/data/node-identity.pem"
KEYS_DIR="/app/keys"

generate_identity() {
    if [ ! -f "$IDENTITY_FILE" ]; then
        echo "Generating node identity key..."
        mkdir -p "$(dirname "$IDENTITY_FILE")"
        
        openssl genpkey -algorithm ED25519 -out "$IDENTITY_FILE" 2>/dev/null || {
            echo "OpenSSL not available, using random bytes..."
            dd if=/dev/urandom bs=32 count=1 2>/dev/null | base64 > "$IDENTITY_FILE"
        }
        chmod 600 "$IDENTITY_FILE"
        echo "Node identity generated at $IDENTITY_FILE"
    else
        echo "Using existing node identity at $IDENTITY_FILE"
    fi
}

wait_for_peers() {
    if [ -n "$WAIT_FOR_PEERS" ]; then
        echo "Waiting for peer services: $WAIT_FOR_PEERS"
        for service in $(echo "$WAIT_FOR_PEERS" | tr ',' ' '); do
            host=$(echo "$service" | cut -d: -f1)
            port=$(echo "$service" | cut -d: -f2)
            if [ -n "$port" ]; then
                echo "Waiting for $host:$port..."
                max_attempts=30
                attempt=0
                while ! nc -z "$host" "$port" 2>/dev/null; do
                    attempt=$((attempt + 1))
                    if [ $attempt -ge $max_attempts ]; then
                        echo "Warning: $host:$port not available after $max_attempts attempts"
                        break
                    fi
                    sleep 1
                done
            fi
        done
    fi
}

echo "Starting Banyan Chat Service (${NODE_TYPE:-unknown} node)"
echo "Keys directory: $KEYS_DIR"
echo "Data directory: /app/data"

generate_identity

if [ -n "$SYNC_PEERS" ]; then
    export EXTRA_ARGS="$EXTRA_ARGS -sync-peers=$SYNC_PEERS"
fi

exec /app/chat-service \
    -listen="${LISTEN_ADDR:-:8080}" \
    -data-dir=/app/data \
    ${EXTRA_ARGS:+$EXTRA_ARGS} \
    "$@"
