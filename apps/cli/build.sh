#!/usr/bin/env bash
set -euo pipefail

OUTPUT_DIR="bin"
CLEAN=false

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --clean|-c)
            CLEAN=true
            shift
            ;;
        --output|-o)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        --help|-h)
            echo "Usage: ./build.sh [--clean] [--output DIR] [--help]"
            echo "  --clean, -c    Clean previous builds before building"
            echo "  --output, -o   Output directory for builds (default: bin)"
            echo "  --help, -h     Show this help message"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

echo ""
echo "🌳 Building Banyan CLI for all platforms..."
echo ""

# Copy web dist files for embedding
WEB_DIST_SOURCE="../web/dist"
WEB_DIST_DEST="server/web/dist"

if [ -d "$WEB_DIST_SOURCE" ]; then
    echo "📦 Copying web dist files for embedding..."
    rm -rf "$WEB_DIST_DEST"
    mkdir -p "server/web"
    cp -r "$WEB_DIST_SOURCE" "$WEB_DIST_DEST"
    echo "✅ Web dist files copied!"
else
    echo "⚠️  Warning: Web dist not found at $WEB_DIST_SOURCE"
    echo "   Run 'nx build web' first to build the web UI"
    exit 1
fi

# Get version info
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS="-s -w -X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME}"

# Clean if requested
if [ "$CLEAN" = true ] && [ -d "$OUTPUT_DIR" ]; then
    echo "🧹 Cleaning previous builds..."
    rm -rf "${OUTPUT_DIR:?}"/*
fi

# Create output directories
mkdir -p "$OUTPUT_DIR/win" "$OUTPUT_DIR/linux" "$OUTPUT_DIR/osx"

# Build Windows
echo "📦 Building for Windows (amd64)..."
GOOS=windows GOARCH=amd64 go build -ldflags="${LDFLAGS}" -o "$OUTPUT_DIR/win/banyan-cli.exe" .
cat > "$OUTPUT_DIR/win/banyan-cli.json" << EOF
{
  "nodeExecutable": "../../node/win/banyan.exe",
  "webPort": 8080
}
EOF
echo "✅ Windows build complete: $OUTPUT_DIR/win/banyan-cli.exe"

# Build Linux
echo "🐧 Building for Linux (amd64)..."
GOOS=linux GOARCH=amd64 go build -ldflags="${LDFLAGS}" -o "$OUTPUT_DIR/linux/banyan-cli" .
cat > "$OUTPUT_DIR/linux/banyan-cli.json" << EOF
{
  "nodeExecutable": "../../node/linux/banyan",
  "webPort": 8080
}
EOF
echo "✅ Linux build complete: $OUTPUT_DIR/linux/banyan-cli"

# Build macOS Intel
echo "🍎 Building for macOS (Intel)..."
GOOS=darwin GOARCH=amd64 go build -ldflags="${LDFLAGS}" -o "$OUTPUT_DIR/osx/banyan-cli-amd64" .
cat > "$OUTPUT_DIR/osx/banyan-cli-amd64.json" << EOF
{
  "nodeExecutable": "../../node/osx/banyan-amd64",
  "webPort": 8080
}
EOF
echo "✅ macOS Intel build complete: $OUTPUT_DIR/osx/banyan-cli-amd64"

# Build macOS ARM
echo "🍎 Building for macOS (Apple Silicon)..."
GOOS=darwin GOARCH=arm64 go build -ldflags="${LDFLAGS}" -o "$OUTPUT_DIR/osx/banyan-cli-arm64" .
cat > "$OUTPUT_DIR/osx/banyan-cli-arm64.json" << EOF
{
  "nodeExecutable": "../../node/osx/banyan-arm64",
  "webPort": 8080
}
EOF
echo "✅ macOS ARM build complete: $OUTPUT_DIR/osx/banyan-cli-arm64"

echo ""
echo "🎉 All builds complete!"
echo ""
echo "Build outputs:"
echo "  Windows:      $OUTPUT_DIR/win/banyan-cli.exe"
echo "  Linux:        $OUTPUT_DIR/linux/banyan-cli"
echo "  macOS Intel:  $OUTPUT_DIR/osx/banyan-cli-amd64"
echo "  macOS ARM:    $OUTPUT_DIR/osx/banyan-cli-arm64"
echo ""
echo "📄 Config files created with node executable paths."

