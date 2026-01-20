#!/bin/bash

# Banyan Cross-Platform Build Script
# Builds Banyan for Windows, Linux, and macOS

set -e  # Exit on any error

# Parse command line arguments
VERSION=""
WHATS_NEW=""
METADATA_ONLY=false
OUTPUT_DIR="bin"
CLEAN=false

while [[ $# -gt 0 ]]; do
  case $1 in
    --version)
      VERSION="$2"
      shift 2
      ;;
    --whats-new)
      WHATS_NEW="$2"
      shift 2
      ;;
    --metadata-only)
      METADATA_ONLY=true
      shift
      ;;
    --output)
      OUTPUT_DIR="$2"
      shift 2
      ;;
    --clean)
      CLEAN=true
      shift
      ;;
    -h|--help)
      echo "Usage: $0 [--version VERSION] [--whats-new DESCRIPTION] [--output DIR] [--clean] [--metadata-only]"
      echo "  --version       Version string in semver format (default: pre-release)"
      echo "  --whats-new     Comma-separated list of new features (default: smiley emoji)"
      echo "  --output        Output directory for builds (default: bin)"
      echo "  --clean         Clean previous builds before building"
      echo "  --metadata-only Only regenerate release metadata (no compilation)"
      exit 0
      ;;
    *)
      echo "Unknown option $1"
      exit 1
      ;;
  esac
done

if [ "$METADATA_ONLY" = true ]; then
    echo "🔄 Regenerating release metadata only..."
else
    echo "🌳 Building Banyan for all platforms..."
    echo "📁 Output directory: $OUTPUT_DIR"
fi
echo ""

# Get the current version/commit info
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

# Build flags
LDFLAGS="-s -w -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME}"

# Clean if requested
if [ "$CLEAN" = true ] && [ "$METADATA_ONLY" != true ]; then
    echo "🧹 Cleaning previous builds..."
    rm -rf "$OUTPUT_DIR/win" "$OUTPUT_DIR/linux" "$OUTPUT_DIR/osx" 2>/dev/null || true
fi

# Skip building if metadata only
if [ "$METADATA_ONLY" != true ]; then
    # Create output directories if they don't exist
    mkdir -p "$OUTPUT_DIR/win" "$OUTPUT_DIR/linux" "$OUTPUT_DIR/osx"

    # Create debug directories for each platform
    echo "📁 Creating debug folder structure..."
    for platform in win linux osx; do
        for subdir in addons figs services; do
            mkdir -p "$OUTPUT_DIR/debug/$platform/$subdir"
        done
    done
    echo "   Created debug/{win,linux,osx}/{addons,figs,services}"
else
    # For metadata only, just ensure output directory exists
    mkdir -p "$OUTPUT_DIR"
fi

if [ "$METADATA_ONLY" != true ]; then
    echo "📦 Building for Windows (amd64)..."
    GOOS=windows GOARCH=amd64 go build -ldflags="${LDFLAGS}" -o "$OUTPUT_DIR/win/banyan.exe" .
    echo "✅ Windows build complete: $OUTPUT_DIR/win/banyan.exe"

    echo ""
    echo "🐧 Building for Linux (amd64)..."
    GOOS=linux GOARCH=amd64 go build -ldflags="${LDFLAGS}" -o "$OUTPUT_DIR/linux/banyan" .
    echo "✅ Linux build complete: $OUTPUT_DIR/linux/banyan"

    echo ""
    echo "🍎 Building for macOS (amd64)..."
    GOOS=darwin GOARCH=amd64 go build -ldflags="${LDFLAGS}" -o "$OUTPUT_DIR/osx/banyan-amd64" .
    echo "✅ macOS AMD64 build complete: $OUTPUT_DIR/osx/banyan-amd64"

    echo ""
    echo "🍎 Building for macOS (arm64 - Apple Silicon)..."
    GOOS=darwin GOARCH=arm64 go build -ldflags="${LDFLAGS}" -o "$OUTPUT_DIR/osx/banyan-arm64" .
    echo "✅ macOS ARM64 build complete: $OUTPUT_DIR/osx/banyan-arm64"

    # Copy debug startup scripts to each platform directory
    echo ""
    echo "📜 Copying debug startup scripts..."
    SCRIPTS_DIR="$(dirname "$0")/scripts"

    # Windows - copy .bat and .ps1 scripts
    if [ -f "$SCRIPTS_DIR/start-debug.ps1" ]; then
        cp "$SCRIPTS_DIR/start-debug.ps1" "$OUTPUT_DIR/win/start-debug.ps1"
        cp "$SCRIPTS_DIR/start-debug.bat" "$OUTPUT_DIR/win/start-debug.bat"
        echo "   Copied Windows debug scripts"
    fi

    # Linux - copy .sh script
    if [ -f "$SCRIPTS_DIR/start-debug.sh" ]; then
        cp "$SCRIPTS_DIR/start-debug.sh" "$OUTPUT_DIR/linux/start-debug.sh"
        chmod +x "$OUTPUT_DIR/linux/start-debug.sh"
        echo "   Copied Linux debug script"
    fi

    # macOS - copy .sh script
    if [ -f "$SCRIPTS_DIR/start-debug.sh" ]; then
        cp "$SCRIPTS_DIR/start-debug.sh" "$OUTPUT_DIR/osx/start-debug.sh"
        chmod +x "$OUTPUT_DIR/osx/start-debug.sh"
        echo "   Copied macOS debug script"
    fi
fi

# Calculate hashes and sizes for built binaries
echo ""
echo "🔐 Calculating checksums and file sizes..."

# Function to get file info
get_file_info() {
    local file="$1"
    if [ -f "$file" ]; then
        local hash=$(sha256sum "$file" 2>/dev/null | cut -d' ' -f1 || shasum -a 256 "$file" 2>/dev/null | cut -d' ' -f1 || echo "unknown")
        local size=$(stat -c%s "$file" 2>/dev/null || stat -f%z "$file" 2>/dev/null || echo "0")
        local size_formatted
        if [ "$size" -gt 1048576 ]; then
            # Use awk for better compatibility instead of bc
            size_formatted=$(awk "BEGIN {printf \"%.1f MB\", $size / 1048576}")
        else
            size_formatted=$(awk "BEGIN {printf \"%.1f KB\", $size / 1024}")
        fi
        echo "sha256:${hash}|${size}|${size_formatted}"
    else
        echo "unknown|0|unknown"
    fi
}

# Get file information
WINDOWS_INFO=$(get_file_info "$OUTPUT_DIR/win/banyan.exe")
LINUX_INFO=$(get_file_info "$OUTPUT_DIR/linux/banyan")
MACOS_INTEL_INFO=$(get_file_info "$OUTPUT_DIR/osx/banyan-amd64")
MACOS_ARM_INFO=$(get_file_info "$OUTPUT_DIR/osx/banyan-arm64")

# Parse the info strings
WINDOWS_HASH=$(echo "$WINDOWS_INFO" | cut -d'|' -f1)
WINDOWS_SIZE=$(echo "$WINDOWS_INFO" | cut -d'|' -f2)
WINDOWS_SIZE_FORMATTED=$(echo "$WINDOWS_INFO" | cut -d'|' -f3)

LINUX_HASH=$(echo "$LINUX_INFO" | cut -d'|' -f1)
LINUX_SIZE=$(echo "$LINUX_INFO" | cut -d'|' -f2)
LINUX_SIZE_FORMATTED=$(echo "$LINUX_INFO" | cut -d'|' -f3)

MACOS_INTEL_HASH=$(echo "$MACOS_INTEL_INFO" | cut -d'|' -f1)
MACOS_INTEL_SIZE=$(echo "$MACOS_INTEL_INFO" | cut -d'|' -f2)
MACOS_INTEL_SIZE_FORMATTED=$(echo "$MACOS_INTEL_INFO" | cut -d'|' -f3)

MACOS_ARM_HASH=$(echo "$MACOS_ARM_INFO" | cut -d'|' -f1)
MACOS_ARM_SIZE=$(echo "$MACOS_ARM_INFO" | cut -d'|' -f2)
MACOS_ARM_SIZE_FORMATTED=$(echo "$MACOS_ARM_INFO" | cut -d'|' -f3)

# Create release metadata
echo ""
echo "📝 Creating release metadata..."
COMMIT=$(git rev-parse HEAD 2>/dev/null || echo "unknown")

# Set version and whats new  
RELEASE_VERSION="${VERSION:-pre-release}"
# Handle emoji with better Unicode support
if [ -z "$WHATS_NEW" ]; then
    # Use Unicode escape sequence to avoid encoding issues
    RELEASE_WHATS_NEW=$(printf "\U1F60A")  # 😊 emoji
else
    RELEASE_WHATS_NEW="$WHATS_NEW"
fi

# Ensure UTF-8 encoding for JSON output
export LC_ALL=C.UTF-8 2>/dev/null || export LC_ALL=en_US.UTF-8 2>/dev/null || true

# Create JSON with proper UTF-8 handling
printf '{
  "version": "%s",
  "commit": "%s", 
  "buildDate": "%s",
  "buildTime": "%s",
  "whatsNew": "%s",
  "downloads": {
    "windows": {
      "platform": "Windows",
      "arch": "x64",
      "filename": "banyan.exe",
      "size": "%s",
      "sizeBytes": %s,
      "checksum": "%s"
    },
    "linux": {
      "platform": "Linux",
      "arch": "x64", 
      "filename": "banyan",
      "size": "%s",
      "sizeBytes": %s,
      "checksum": "%s"
    },
    "macos_intel": {
      "platform": "macOS",
      "arch": "x64 (Intel)",
      "filename": "banyan-amd64",
      "size": "%s",
      "sizeBytes": %s,
      "checksum": "%s"
    },
    "macos_arm": {
      "platform": "macOS",
      "arch": "ARM64 (Apple Silicon)",
      "filename": "banyan-arm64",
      "size": "%s",
      "sizeBytes": %s,
      "checksum": "%s"
    }
  }
}' \
"$RELEASE_VERSION" \
"$COMMIT" \
"$BUILD_TIME" \
"$BUILD_TIME" \
"$RELEASE_WHATS_NEW" \
"$WINDOWS_SIZE_FORMATTED" \
"$WINDOWS_SIZE" \
"$WINDOWS_HASH" \
"$LINUX_SIZE_FORMATTED" \
"$LINUX_SIZE" \
"$LINUX_HASH" \
"$MACOS_INTEL_SIZE_FORMATTED" \
"$MACOS_INTEL_SIZE" \
"$MACOS_INTEL_HASH" \
"$MACOS_ARM_SIZE_FORMATTED" \
"$MACOS_ARM_SIZE" \
"$MACOS_ARM_HASH" > "$OUTPUT_DIR/release-metadata.json"

echo ""
if [ "$METADATA_ONLY" = true ]; then
    echo "🎉 Metadata regeneration completed successfully!"
    echo ""
    echo "📝 Updated metadata: $OUTPUT_DIR/release-metadata.json"
    echo "   Version: ${RELEASE_VERSION}"
    echo "   What's New: ${RELEASE_WHATS_NEW}"
else
    echo "🎉 All builds completed successfully!"
    echo ""
    echo "📁 Build artifacts:"
    echo "   Windows: $OUTPUT_DIR/win/banyan.exe (${WINDOWS_SIZE_FORMATTED})"
    echo "   Linux:   $OUTPUT_DIR/linux/banyan (${LINUX_SIZE_FORMATTED})"
    echo "   macOS:   $OUTPUT_DIR/osx/banyan-amd64 (${MACOS_INTEL_SIZE_FORMATTED}, Intel)"
    echo "   macOS:   $OUTPUT_DIR/osx/banyan-arm64 (${MACOS_ARM_SIZE_FORMATTED}, Apple Silicon)"
    echo "   Metadata: $OUTPUT_DIR/release-metadata.json"
    echo ""
    echo "🔐 Checksums:"
    echo "   Windows: ${WINDOWS_HASH}"
    echo "   Linux:   ${LINUX_HASH}"
    echo "   macOS Intel: ${MACOS_INTEL_HASH}"
    echo "   macOS ARM:   ${MACOS_ARM_HASH}"
    echo ""
    echo "💡 Tip: You can run './banyan --help' on each platform to test the build"
fi

