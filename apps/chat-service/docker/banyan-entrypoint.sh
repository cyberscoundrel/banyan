#!/bin/sh
set -e

if [ -d "/app/services" ]; then
    NODE_ID=$(hostname)
    HAS_STATIC="false"
    HAS_LEDGER="false"
    HAS_MOD="false"
    HAS_ADMIN="false"
    SERVICE_KEY_PEM=""
    PROXY_URL=""

    if [ -f /app/services/chat-admin-key/chat-admin-key.pem ]; then
        HAS_ADMIN="true"
    fi
    if [ -f /app/services/chat-mod-key/chat-mod-key.pem ]; then
        HAS_MOD="true"
    fi
    if [ -f /app/services/chat-ledger-key/chat-ledger-key.pem ]; then
        HAS_LEDGER="true"
        SERVICE_KEY_PEM="/app/services/chat-ledger-key/chat-ledger-key.pem"
    fi
    if [ -f /app/services/chat-static-key/chat-static-key.pem ]; then
        HAS_STATIC="true"
        if [ -z "$SERVICE_KEY_PEM" ]; then
            SERVICE_KEY_PEM="/app/services/chat-static-key/chat-static-key.pem"
        fi
    fi

    echo "Starting chat-service (node=$NODE_ID static=$HAS_STATIC ledger=$HAS_LEDGER mod=$HAS_MOD admin=$HAS_ADMIN)"

    LISTEN_ADDR="127.0.0.1:8080" \
    DATA_DIR="/app/data" \
    NODE_ID="$NODE_ID" \
    HAS_STATIC_KEY="$HAS_STATIC" \
    HAS_LEDGER_KEY="$HAS_LEDGER" \
    HAS_MOD_KEY="$HAS_MOD" \
    HAS_ADMIN_KEY="$HAS_ADMIN" \
    PROXY_URL="http://127.0.0.1:9090" \
    SYNC_ENABLED="true" \
    SERVICE_KEY_PEM="$SERVICE_KEY_PEM" \
    FIG_ALIAS="chat-fig" \
    node /app/chat-service/dist/index.js &

    CHAT_PID=$!
    echo "Started chat-service (PID $CHAT_PID) on 127.0.0.1:8080"

    trap "kill $CHAT_PID 2>/dev/null; wait $CHAT_PID 2>/dev/null" EXIT INT TERM
else
    echo "No services directory - skipping chat-service (proxy node)"
fi

exec banyan "$@"
