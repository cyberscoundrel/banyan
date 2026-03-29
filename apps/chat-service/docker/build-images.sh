#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

VERSION="${VERSION:-latest}"
BUILD_DATE="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"

build_image() {
    local dockerfile="$1"
    local image_name="$2"
    local tag="$3"
    
    echo "Building $image_name:$tag..."
    docker build \
        --build-arg BUILD_DATE="$BUILD_DATE" \
        --build-arg VERSION="$tag" \
        -t "$image_name:$tag" \
        -t "$image_name:latest" \
        -f "$dockerfile" \
        "$PROJECT_ROOT"
    
    echo "Successfully built $image_name:$tag"
    echo ""
}

echo "Building Banyan Chat Service Docker Images (Node.js)"
echo "====================================================="
echo "Project root: $PROJECT_ROOT"
echo "Version: $VERSION"
echo "Build date: $BUILD_DATE"
echo ""

build_image "$SCRIPT_DIR/Dockerfile.static" "banyan-chat-static" "$VERSION"
build_image "$SCRIPT_DIR/Dockerfile.ledger" "banyan-chat-ledger" "$VERSION"
build_image "$SCRIPT_DIR/Dockerfile.mod" "banyan-chat-mod" "$VERSION"
build_image "$SCRIPT_DIR/Dockerfile.admin" "banyan-chat-admin" "$VERSION"

echo "====================================================="
echo "All images built successfully!"
echo ""
echo "Images:"
docker images | grep banyan-chat | head -20
echo ""
echo "To run the development cluster:"
echo "  cd $SCRIPT_DIR && docker-compose up -d"
echo ""
echo "To view logs:"
echo "  docker-compose logs -f [service-name]"
echo ""
echo "To stop:"
echo "  docker-compose down"
