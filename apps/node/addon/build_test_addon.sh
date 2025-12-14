#!/usr/bin/env sh
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")"/.. && pwd)"
ADDON_DIR="$ROOT_DIR/bin/addons"
mkdir -p "$ADDON_DIR"

# Build test addon
GOOS=${GOOS:-} GOARCH=${GOARCH:-} go build -o "$ADDON_DIR/testaddon" ./addon/testaddon

# Generate addons.json
cat > "$ADDON_DIR/addons.json" <<EOF
{
  "addons": [
    { "name": "testaddon", "exec": "./testaddon" }
  ]
}
EOF

echo "Built test addon to $ADDON_DIR and wrote addons.json"
