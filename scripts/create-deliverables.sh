#!/usr/bin/env bash
# Create compressed deliverables containing node executable + CLI for each platform
set -euo pipefail

OUTPUT_DIR="${1:-dist/deliverables}"
NODE_DIR="${2:-dist/apps/node}"
CLI_DIR="${3:-dist/apps/cli}"

echo ""
echo "📦 Creating Banyan deliverables..."
echo "   Output: $OUTPUT_DIR"
echo "   Node builds: $NODE_DIR"
echo "   CLI builds: $CLI_DIR"
echo ""

# Clean and create output directory
rm -rf "$OUTPUT_DIR"
mkdir -p "$OUTPUT_DIR"

# Get version info
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
COMMIT=$(git rev-parse HEAD 2>/dev/null || echo "unknown")

# Function to get file size
get_size() {
    local file="$1"
    if [ -f "$file" ]; then
        stat -c%s "$file" 2>/dev/null || stat -f%z "$file" 2>/dev/null || echo "0"
    else
        echo "0"
    fi
}

# Function to format size
format_size() {
    local size=$1
    if [ "$size" -gt 1048576 ]; then
        awk "BEGIN {printf \"%.1f MB\", $size / 1048576}"
    else
        awk "BEGIN {printf \"%.1f KB\", $size / 1024}"
    fi
}

# Function to get checksum
get_checksum() {
    local file="$1"
    if [ -f "$file" ]; then
        sha256sum "$file" 2>/dev/null | cut -d' ' -f1 || shasum -a 256 "$file" 2>/dev/null | cut -d' ' -f1 || echo "unknown"
    else
        echo "unknown"
    fi
}

# Create Windows deliverable (zip)
echo "📦 Creating Windows deliverable..."
TEMP_WIN=$(mktemp -d)
mkdir -p "$TEMP_WIN/banyan"
cp "$NODE_DIR/win/banyan.exe" "$TEMP_WIN/banyan/"
cp "$CLI_DIR/win/banyan-cli.exe" "$TEMP_WIN/banyan/"
# Create a simple README
cat > "$TEMP_WIN/banyan/README.txt" << EOF
Banyan - LibP2P HTTP Proxy
Version: $VERSION
Build: $BUILD_TIME

Contents:
  banyan.exe     - Banyan Node (P2P network daemon)
  banyan-cli.exe - Banyan CLI (Terminal UI for managing nodes)

Quick Start:
  1. Run banyan-cli.exe to start the terminal UI
  2. Or run banyan.exe directly for headless operation

For more information, visit: https://banyan.cyberscoundrel.com
EOF
# Create zip
(cd "$TEMP_WIN" && zip -r "$OLDPWD/$OUTPUT_DIR/banyan-windows-x64.zip" banyan)
rm -rf "$TEMP_WIN"
WIN_SIZE=$(get_size "$OUTPUT_DIR/banyan-windows-x64.zip")
WIN_CHECKSUM=$(get_checksum "$OUTPUT_DIR/banyan-windows-x64.zip")
echo "✅ Created: $OUTPUT_DIR/banyan-windows-x64.zip ($(format_size $WIN_SIZE))"

# Create Linux deliverable (tar.gz)
echo "📦 Creating Linux deliverable..."
TEMP_LINUX=$(mktemp -d)
mkdir -p "$TEMP_LINUX/banyan"
cp "$NODE_DIR/linux/banyan" "$TEMP_LINUX/banyan/"
cp "$CLI_DIR/linux/banyan-cli" "$TEMP_LINUX/banyan/"
chmod +x "$TEMP_LINUX/banyan/banyan" "$TEMP_LINUX/banyan/banyan-cli"
cat > "$TEMP_LINUX/banyan/README.md" << EOF
# Banyan - LibP2P HTTP Proxy
Version: $VERSION
Build: $BUILD_TIME

## Contents
- \`banyan\` - Banyan Node (P2P network daemon)
- \`banyan-cli\` - Banyan CLI (Terminal UI for managing nodes)

## Quick Start
\`\`\`bash
chmod +x banyan banyan-cli
./banyan-cli
\`\`\`

Or run the node directly for headless operation:
\`\`\`bash
./banyan --help
\`\`\`

For more information, visit: https://banyan.cyberscoundrel.com
EOF
(cd "$TEMP_LINUX" && tar -czvf "$OLDPWD/$OUTPUT_DIR/banyan-linux-x64.tar.gz" banyan)
rm -rf "$TEMP_LINUX"
LINUX_SIZE=$(get_size "$OUTPUT_DIR/banyan-linux-x64.tar.gz")
LINUX_CHECKSUM=$(get_checksum "$OUTPUT_DIR/banyan-linux-x64.tar.gz")
echo "✅ Created: $OUTPUT_DIR/banyan-linux-x64.tar.gz ($(format_size $LINUX_SIZE))"

# Create macOS Intel deliverable (tar.gz)
echo "📦 Creating macOS Intel deliverable..."
TEMP_MACOS_INTEL=$(mktemp -d)
mkdir -p "$TEMP_MACOS_INTEL/banyan"
cp "$NODE_DIR/osx/banyan-amd64" "$TEMP_MACOS_INTEL/banyan/banyan"
cp "$CLI_DIR/osx/banyan-cli-amd64" "$TEMP_MACOS_INTEL/banyan/banyan-cli"
chmod +x "$TEMP_MACOS_INTEL/banyan/banyan" "$TEMP_MACOS_INTEL/banyan/banyan-cli"
cat > "$TEMP_MACOS_INTEL/banyan/README.md" << EOF
# Banyan - LibP2P HTTP Proxy (macOS Intel)
Version: $VERSION
Build: $BUILD_TIME

## Contents
- \`banyan\` - Banyan Node (P2P network daemon)
- \`banyan-cli\` - Banyan CLI (Terminal UI for managing nodes)

## Quick Start
\`\`\`bash
chmod +x banyan banyan-cli
./banyan-cli
\`\`\`

**Note:** You may need to allow the binaries in System Preferences → Security & Privacy
if macOS blocks them due to being unsigned.

For more information, visit: https://banyan.cyberscoundrel.com
EOF
(cd "$TEMP_MACOS_INTEL" && tar -czvf "$OLDPWD/$OUTPUT_DIR/banyan-macos-x64.tar.gz" banyan)
rm -rf "$TEMP_MACOS_INTEL"
MACOS_INTEL_SIZE=$(get_size "$OUTPUT_DIR/banyan-macos-x64.tar.gz")
MACOS_INTEL_CHECKSUM=$(get_checksum "$OUTPUT_DIR/banyan-macos-x64.tar.gz")
echo "✅ Created: $OUTPUT_DIR/banyan-macos-x64.tar.gz ($(format_size $MACOS_INTEL_SIZE))"

# Create macOS ARM deliverable (tar.gz)
echo "📦 Creating macOS ARM deliverable..."
TEMP_MACOS_ARM=$(mktemp -d)
mkdir -p "$TEMP_MACOS_ARM/banyan"
cp "$NODE_DIR/osx/banyan-arm64" "$TEMP_MACOS_ARM/banyan/banyan"
cp "$CLI_DIR/osx/banyan-cli-arm64" "$TEMP_MACOS_ARM/banyan/banyan-cli"
chmod +x "$TEMP_MACOS_ARM/banyan/banyan" "$TEMP_MACOS_ARM/banyan/banyan-cli"
cat > "$TEMP_MACOS_ARM/banyan/README.md" << EOF
# Banyan - LibP2P HTTP Proxy (macOS Apple Silicon)
Version: $VERSION
Build: $BUILD_TIME

## Contents
- \`banyan\` - Banyan Node (P2P network daemon)
- \`banyan-cli\` - Banyan CLI (Terminal UI for managing nodes)

## Quick Start
\`\`\`bash
chmod +x banyan banyan-cli
./banyan-cli
\`\`\`

**Note:** You may need to allow the binaries in System Preferences → Security & Privacy
if macOS blocks them due to being unsigned.

For more information, visit: https://banyan.cyberscoundrel.com
EOF
(cd "$TEMP_MACOS_ARM" && tar -czvf "$OLDPWD/$OUTPUT_DIR/banyan-macos-arm64.tar.gz" banyan)
rm -rf "$TEMP_MACOS_ARM"
MACOS_ARM_SIZE=$(get_size "$OUTPUT_DIR/banyan-macos-arm64.tar.gz")
MACOS_ARM_CHECKSUM=$(get_checksum "$OUTPUT_DIR/banyan-macos-arm64.tar.gz")
echo "✅ Created: $OUTPUT_DIR/banyan-macos-arm64.tar.gz ($(format_size $MACOS_ARM_SIZE))"

# Create release metadata
echo ""
echo "📝 Creating release metadata..."

cat > "$OUTPUT_DIR/release-metadata.json" << EOF
{
  "version": "$VERSION",
  "commit": "$COMMIT",
  "buildDate": "$BUILD_TIME",
  "buildTime": "$BUILD_TIME",
  "whatsNew": "Bundled node + CLI deliverables",
  "deliverables": {
    "windows_x64": {
      "id": "windows_x64",
      "platform": "Windows",
      "arch": "x64",
      "filename": "banyan-windows-x64.zip",
      "size": "$(format_size $WIN_SIZE)",
      "sizeBytes": $WIN_SIZE,
      "checksum": "sha256:$WIN_CHECKSUM",
      "contents": ["banyan.exe", "banyan-cli.exe", "README.txt"]
    },
    "linux_x64": {
      "id": "linux_x64",
      "platform": "Linux",
      "arch": "x64",
      "filename": "banyan-linux-x64.tar.gz",
      "size": "$(format_size $LINUX_SIZE)",
      "sizeBytes": $LINUX_SIZE,
      "checksum": "sha256:$LINUX_CHECKSUM",
      "contents": ["banyan", "banyan-cli", "README.md"]
    },
    "macos_x64": {
      "id": "macos_x64",
      "platform": "macOS",
      "arch": "x64 (Intel)",
      "filename": "banyan-macos-x64.tar.gz",
      "size": "$(format_size $MACOS_INTEL_SIZE)",
      "sizeBytes": $MACOS_INTEL_SIZE,
      "checksum": "sha256:$MACOS_INTEL_CHECKSUM",
      "contents": ["banyan", "banyan-cli", "README.md"]
    },
    "macos_arm64": {
      "id": "macos_arm64",
      "platform": "macOS",
      "arch": "ARM64 (Apple Silicon)",
      "filename": "banyan-macos-arm64.tar.gz",
      "size": "$(format_size $MACOS_ARM_SIZE)",
      "sizeBytes": $MACOS_ARM_SIZE,
      "checksum": "sha256:$MACOS_ARM_CHECKSUM",
      "contents": ["banyan", "banyan-cli", "README.md"]
    }
  }
}
EOF

echo "✅ Created: $OUTPUT_DIR/release-metadata.json"
echo ""
echo "🎉 All deliverables created successfully!"
echo ""
echo "📁 Deliverables:"
echo "   $OUTPUT_DIR/banyan-windows-x64.zip ($(format_size $WIN_SIZE))"
echo "   $OUTPUT_DIR/banyan-linux-x64.tar.gz ($(format_size $LINUX_SIZE))"
echo "   $OUTPUT_DIR/banyan-macos-x64.tar.gz ($(format_size $MACOS_INTEL_SIZE))"
echo "   $OUTPUT_DIR/banyan-macos-arm64.tar.gz ($(format_size $MACOS_ARM_SIZE))"
echo "   $OUTPUT_DIR/release-metadata.json"
