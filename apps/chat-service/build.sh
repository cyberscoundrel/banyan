#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FRONTEND_DIR="$SCRIPT_DIR/../chat-frontend"
SERVICE_DIR="$SCRIPT_DIR/../chat-service"

echo "Building frontend..."
cd "$FRONTEND_DIR"

if [ ! -d "node_modules" ]; then
    echo "Installing dependencies..."
    npm install
fi

echo "Running Vite build..."
npm run build

echo "Copying dist to chat-service..."
rm -rf "$SERVICE_DIR/frontend/dist"
cp -r dist "$SERVICE_DIR/frontend/"

echo "Building Go service..."
cd "$SERVICE_DIR"
go build -o chat-service .

echo "Build complete!"
echo "Run: ./apps/chat-service/chat-service -help"
