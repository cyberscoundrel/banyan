#!/usr/bin/env bash
set -euo pipefail

OUTPUT_DIR="bin"
CLEAN=false
PLATFORM="${NX_BUILD_PLATFORM:-all}"

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
        --platform|-p)
            PLATFORM="$2"
            shift 2
            ;;
        --help|-h)
            echo "Usage: ./build.sh [--clean] [--output DIR] [--platform PLATFORM] [--help]"
            echo "  --clean, -c       Clean previous builds before building"
            echo "  --output, -o      Output directory for builds (default: bin)"
            echo "  --platform, -p    Platform to build: all, current, linux, windows, macos-amd64, macos-arm64"
            echo "                    (default: all, or \$NX_BUILD_PLATFORM if set)"
            echo "  --help, -h        Show this help message"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Resolve 'current' to actual platform
if [ "$PLATFORM" = "current" ]; then
    case "$(uname -s)" in
        Linux*)   PLATFORM="linux";;
        Darwin*)  PLATFORM="macos-$(uname -m)";;
        MINGW*|MSYS*|CYGWIN*) PLATFORM="windows";;
        *)        PLATFORM="linux";;
    esac
    echo "🔍 Detected current platform: $PLATFORM"
fi

# Determine which platforms to build
BUILD_WINDOWS=false
BUILD_LINUX=false
BUILD_MACOS_AMD64=false
BUILD_MACOS_ARM64=false

case "$PLATFORM" in
    all)
        BUILD_WINDOWS=true
        BUILD_LINUX=true
        BUILD_MACOS_AMD64=true
        BUILD_MACOS_ARM64=true
        ;;
    linux)
        BUILD_LINUX=true
        ;;
    windows)
        BUILD_WINDOWS=true
        ;;
    macos-amd64|macos-x86_64)
        BUILD_MACOS_AMD64=true
        ;;
    macos-arm64|macos-aarch64)
        BUILD_MACOS_ARM64=true
        ;;
    macos)
        BUILD_MACOS_AMD64=true
        BUILD_MACOS_ARM64=true
        ;;
    *)
        echo "❌ Unknown platform: $PLATFORM"
        echo "   Valid options: all, current, linux, windows, macos, macos-amd64, macos-arm64"
        exit 1
        ;;
esac

echo ""
echo "🌳 Building Banyan CLI for platform: $PLATFORM"
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

# Create output directories as needed
[ "$BUILD_WINDOWS" = true ] && mkdir -p "$OUTPUT_DIR/win"
[ "$BUILD_LINUX" = true ] && mkdir -p "$OUTPUT_DIR/linux"
[ "$BUILD_MACOS_AMD64" = true ] || [ "$BUILD_MACOS_ARM64" = true ] && mkdir -p "$OUTPUT_DIR/osx"

# Build Windows
if [ "$BUILD_WINDOWS" = true ]; then
    echo "📦 Building for Windows (amd64)..."
    GOOS=windows GOARCH=amd64 go build -ldflags="${LDFLAGS}" -o "$OUTPUT_DIR/win/banyan-cli.exe" .
    cat > "$OUTPUT_DIR/win/banyan-cli.json" << EOF
{
  "nodeExecutable": "../../node/win/banyan.exe",
  "webPort": 8080
}
EOF
    echo "✅ Windows build complete: $OUTPUT_DIR/win/banyan-cli.exe"
fi

# Build Linux
if [ "$BUILD_LINUX" = true ]; then
    echo "🐧 Building for Linux (amd64)..."
    GOOS=linux GOARCH=amd64 go build -ldflags="${LDFLAGS}" -o "$OUTPUT_DIR/linux/banyan-cli" .
    cat > "$OUTPUT_DIR/linux/banyan-cli.json" << EOF
{
  "nodeExecutable": "../../node/linux/banyan",
  "webPort": 8080
}
EOF
    echo "✅ Linux build complete: $OUTPUT_DIR/linux/banyan-cli"
fi

# Build macOS Intel
if [ "$BUILD_MACOS_AMD64" = true ]; then
    echo "🍎 Building for macOS (Intel)..."
    GOOS=darwin GOARCH=amd64 go build -ldflags="${LDFLAGS}" -o "$OUTPUT_DIR/osx/banyan-cli-amd64" .
    cat > "$OUTPUT_DIR/osx/banyan-cli-amd64.json" << EOF
{
  "nodeExecutable": "../../node/osx/banyan-amd64",
  "webPort": 8080
}
EOF
    echo "✅ macOS Intel build complete: $OUTPUT_DIR/osx/banyan-cli-amd64"
fi

# Build macOS ARM
if [ "$BUILD_MACOS_ARM64" = true ]; then
    echo "🍎 Building for macOS (Apple Silicon)..."
    GOOS=darwin GOARCH=arm64 go build -ldflags="${LDFLAGS}" -o "$OUTPUT_DIR/osx/banyan-cli-arm64" .
    cat > "$OUTPUT_DIR/osx/banyan-cli-arm64.json" << EOF
{
  "nodeExecutable": "../../node/osx/banyan-arm64",
  "webPort": 8080
}
EOF
    echo "✅ macOS ARM build complete: $OUTPUT_DIR/osx/banyan-cli-arm64"
fi

echo ""
echo "🎉 Build complete!"
echo ""
echo "Build outputs:"
[ "$BUILD_WINDOWS" = true ] && echo "  Windows:      $OUTPUT_DIR/win/banyan-cli.exe"
[ "$BUILD_LINUX" = true ] && echo "  Linux:        $OUTPUT_DIR/linux/banyan-cli"
[ "$BUILD_MACOS_AMD64" = true ] && echo "  macOS Intel:  $OUTPUT_DIR/osx/banyan-cli-amd64"
[ "$BUILD_MACOS_ARM64" = true ] && echo "  macOS ARM:    $OUTPUT_DIR/osx/banyan-cli-arm64"
echo ""
echo "📄 Config files created with node executable paths."

