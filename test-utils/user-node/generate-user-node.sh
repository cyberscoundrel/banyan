#!/usr/bin/env bash
# Generate a standalone "user node" directory that runs the banyan proxy addon
# in its own isolated Docker context. Two modes:
#
#   --no-vpn    (default): node runs on a fresh Docker bridge network, with
#               http-proxy/libp2p ports published on the host. Same as before.
#
#   --vpn-city <code>:  node is paired with a Gluetun WireGuard sidecar that
#               tunnels all egress through a Mullvad relay in that city. The
#               sidecar holds the network namespace, so its bridge and host
#               port mappings live on the sidecar; the banyan node attaches
#               via `network_mode: container:<sidecar>`. This requires a
#               Mullvad subscription and a free key slot — see
#               test-utils/mullvad/provision-mullvad.sh which can also be
#               invoked with --names <user-name> to add this user node's
#               identity to the shared .env.
#
# After running this script, `cd` into the generated directory and run ./up.sh
# to build and start; ./down.sh to stop and clean up.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORKSPACE_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

NAME=""
PROXY_PORT="9090"
LISTEN_PORT="9100"
FIG_PATH=""
OUTPUT_DIR=""
VPN_CITY=""
MULLVAD_ACCOUNT=""

usage() {
    cat <<EOF
Usage: $(basename "$0") --name <node-name> [options]

Required:
  --name <node-name>        Identifier used for container, image tag, bridge network,
                            and output subdirectory.

Options:
  --proxy-port <hostPort>   Host port mapped to http-proxy addon (container :9090). Default: 9090
  --listen-port <hostPort>  Host port mapped to libp2p listen (container :9100). Default: 9100
  --fig <path>              Path to a signed client fig (.fig or .json). Always written
                            out as <stem>.fig in the generated figs/ directory because
                            the node management API only loads .fig files. Default:
                            <workspace>/apps/chat-service/topology/output/nodes/chat-static-1/services/chat-static-key/chat-fig.json
  --output-dir <dir>        Where to write the generated node directory.
                            Default: <workspace>/test-utils/user-node/nodes/<name>
  --vpn-city <code>         Wrap the node in a Gluetun WireGuard sidecar that egresses
                            through this Mullvad relay city code (e.g. "us-nyc",
                            "de-fra"). Requires --mullvad-account on first run; the
                            generated key is then stored in
                            apps/chat-service/docker/.env under MULLVAD_*_<NAME> so
                            re-running is idempotent.
  --mullvad-account <num>   Mullvad account number (16 digits). Only required the
                            first time you provision a key for this user node.
  -h | --help               Show this help.
EOF
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --name)             NAME="$2";             shift 2 ;;
        --proxy-port)       PROXY_PORT="$2";       shift 2 ;;
        --listen-port)      LISTEN_PORT="$2";      shift 2 ;;
        --fig)              FIG_PATH="$2";         shift 2 ;;
        --output-dir)       OUTPUT_DIR="$2";       shift 2 ;;
        --vpn-city)         VPN_CITY="$2";         shift 2 ;;
        --mullvad-account)  MULLVAD_ACCOUNT="$2";  shift 2 ;;
        -h|--help)          usage; exit 0 ;;
        *)
            echo "error: unknown argument: $1" >&2
            usage >&2
            exit 2
            ;;
    esac
done

if [[ -z "$NAME" ]]; then
    echo "error: --name is required" >&2
    usage >&2
    exit 2
fi

if [[ ! "$NAME" =~ ^[a-zA-Z0-9][a-zA-Z0-9_.-]*$ ]]; then
    echo "error: --name must match [a-zA-Z0-9][a-zA-Z0-9_.-]* (used as a docker identifier)" >&2
    exit 2
fi

if [[ -z "$FIG_PATH" ]]; then
    FIG_PATH="$WORKSPACE_ROOT/apps/chat-service/topology/output/nodes/chat-static-1/services/chat-static-key/chat-fig.json"
fi
if [[ -z "$OUTPUT_DIR" ]]; then
    OUTPUT_DIR="$WORKSPACE_ROOT/test-utils/user-node/nodes/$NAME"
fi

if [[ ! -f "$FIG_PATH" ]]; then
    echo "error: fig file not found: $FIG_PATH" >&2
    echo "       run 'nx run chat-service:topology' first (or pass --fig <path>)" >&2
    exit 1
fi
if [[ "$FIG_PATH" != *.fig && "$FIG_PATH" != *.json ]]; then
    echo "error: fig path must have a .fig or .json extension: $FIG_PATH" >&2
    exit 1
fi

if ! command -v openssl >/dev/null 2>&1; then
    echo "error: openssl is not installed or not on PATH" >&2
    exit 1
fi

CONTAINER_NAME="user-$NAME"
VPN_CONTAINER_NAME="vpn-user-$NAME"
NETWORK_NAME="$CONTAINER_NAME"

# When --vpn-city is provided we need to (a) ensure a Mullvad WG identity
# exists in apps/chat-service/docker/.env for this user and (b) generate an
# alternate compose-style up.sh that runs gluetun + banyan as a pair.
ENV_FILE="$WORKSPACE_ROOT/apps/chat-service/docker/.env"
USE_VPN=0
if [[ -n "$VPN_CITY" ]]; then
    USE_VPN=1
    if ! command -v wg >/dev/null 2>&1; then
        echo "error: --vpn-city requires wireguard-tools (brew install wireguard-tools)" >&2
        exit 1
    fi
    var="$(echo "$NAME" | tr '[:lower:]-' '[:upper:]_')"
    privkey_var="MULLVAD_PRIVKEY_${var}"
    if [[ ! -f "$ENV_FILE" ]] || ! grep -q "^${privkey_var}=" "$ENV_FILE"; then
        if [[ -z "$MULLVAD_ACCOUNT" ]]; then
            echo "error: no Mullvad key for ${var} in ${ENV_FILE}." >&2
            echo "       pass --mullvad-account <num> on first run, or pre-provision via:" >&2
            echo "       test-utils/mullvad/provision-mullvad.sh --account <num> --names $NAME --cities $VPN_CITY" >&2
            exit 1
        fi
        echo "Provisioning Mullvad WG key for $NAME -> $VPN_CITY"
        "$WORKSPACE_ROOT/test-utils/mullvad/provision-mullvad.sh" \
            --account "$MULLVAD_ACCOUNT" --names "$NAME" --cities "$VPN_CITY"
    fi
fi

mkdir -p "$OUTPUT_DIR/addons" "$OUTPUT_DIR/figs"

echo "Generating user node '$NAME' at $OUTPUT_DIR (vpn=$([[ $USE_VPN -eq 1 ]] && echo yes\ \($VPN_CITY\) || echo no))"

openssl genpkey -algorithm ed25519 -out "$OUTPUT_DIR/node-identity.pem" 2>/dev/null
chmod 600 "$OUTPUT_DIR/node-identity.pem"

FIG_STEM="$(basename "$FIG_PATH")"
FIG_STEM="${FIG_STEM%.fig}"
FIG_STEM="${FIG_STEM%.json}"
cp "$FIG_PATH" "$OUTPUT_DIR/figs/${FIG_STEM}.fig"

cat > "$OUTPUT_DIR/addons/addons.json" <<'EOF'
{
  "addons": [
    {
      "name": "http-proxy",
      "exec": "/usr/local/bin/proxy-addon",
      "args": ["-addr", ":9090"]
    }
  ]
}
EOF

cat > "$OUTPUT_DIR/Dockerfile" <<'EOF'
FROM banyan-node:latest

COPY node-identity.pem /app/node-identity.pem
COPY addons /app/addons
COPY figs /app/figs
EOF

if [[ $USE_VPN -eq 1 ]]; then
    cat > "$OUTPUT_DIR/up.sh" <<EOF
#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="\$(cd "\$(dirname "\${BASH_SOURCE[0]}")" && pwd)"
cd "\$SCRIPT_DIR"

CONTAINER=$CONTAINER_NAME
VPN_CONTAINER=$VPN_CONTAINER_NAME
IMAGE=$CONTAINER_NAME:latest
NETWORK=$NETWORK_NAME
PROXY_PORT=${PROXY_PORT}
LISTEN_PORT=${LISTEN_PORT}
ENV_FILE="$ENV_FILE"
NAME_VAR="$(echo "$NAME" | tr '[:lower:]-' '[:upper:]_')"

if ! docker image inspect banyan-node:latest >/dev/null 2>&1; then
    echo "error: banyan-node:latest image not found. Build it with:" >&2
    echo "       nx run chat-service:docker:build" >&2
    exit 1
fi

if [[ ! -f "\$ENV_FILE" ]]; then
    echo "error: env file with Mullvad creds not found: \$ENV_FILE" >&2
    exit 1
fi
set -a; source "\$ENV_FILE"; set +a
PRIV_VAR="MULLVAD_PRIVKEY_\${NAME_VAR}"
ADDR_VAR="MULLVAD_ADDR_\${NAME_VAR}"
HOST_VAR="MULLVAD_HOSTNAME_\${NAME_VAR}"
WG_PRIV="\${!PRIV_VAR:-}"
WG_ADDR="\${!ADDR_VAR:-}"
WG_HOST="\${!HOST_VAR:-}"
if [[ -z "\$WG_PRIV" || -z "\$WG_ADDR" || -z "\$WG_HOST" ]]; then
    echo "error: missing Mullvad creds for \$NAME_VAR in \$ENV_FILE" >&2
    exit 1
fi

docker network inspect "\$NETWORK" >/dev/null 2>&1 || docker network create --driver bridge "\$NETWORK"
docker build -t "\$IMAGE" .
docker rm -f "\$CONTAINER" "\$VPN_CONTAINER" >/dev/null 2>&1 || true

docker run -d --name "\$VPN_CONTAINER" --network "\$NETWORK" \\
    --restart unless-stopped \\
    --cap-add NET_ADMIN --device /dev/net/tun:/dev/net/tun \\
    -p "\$PROXY_PORT:9090" \\
    -p "\$LISTEN_PORT:9100" \\
    -e VPN_SERVICE_PROVIDER=mullvad \\
    -e VPN_TYPE=wireguard \\
    -e WIREGUARD_PRIVATE_KEY="\$WG_PRIV" \\
    -e WIREGUARD_ADDRESSES="\$WG_ADDR" \\
    -e SERVER_HOSTNAMES="\$WG_HOST" \\
    -e DOT=off -e DNS_ADDRESS=10.64.0.1 \\
    -e BLOCK_MALICIOUS=off -e BLOCK_SURVEILLANCE=off -e BLOCK_ADS=off \\
    -e HEALTH_VPN_DURATION_INITIAL=30s \\
    qmcgaw/gluetun:latest

echo -n "Waiting for VPN handshake "
for i in \$(seq 1 30); do
    if docker exec "\$VPN_CONTAINER" wget -qO- --timeout=3 http://ifconfig.me/ip >/dev/null 2>&1; then echo " ok"; break; fi
    echo -n "."; sleep 2
done

docker run -d --name "\$CONTAINER" \\
    --restart unless-stopped \\
    --network "container:\$VPN_CONTAINER" \\
    "\$IMAGE" \\
    -privkey=/app/node-identity.pem \\
    -addons-dir=/app/addons \\
    -figs-dir=/app/figs \\
    -listen=/ip4/0.0.0.0/tcp/9100 \\
    -no-crypto \\
    -allow-expired-figs \\
    -allow-insecure-figs \\
    -tunnel-enabled \\
    -override-transport-restrictions \\
    -nat-traversal

EXIT_IP=\$(docker exec "\$VPN_CONTAINER" wget -qO- --timeout=10 http://ifconfig.me/ip 2>/dev/null || echo "?")
echo "User node '\$CONTAINER' running via VPN sidecar '\$VPN_CONTAINER' on '\$NETWORK'"
echo "  egress IP:     \$EXIT_IP (\$WG_HOST)"
echo "  browser proxy: http://localhost:\$PROXY_PORT"
echo "  libp2p listen: host port \$LISTEN_PORT -> container :9100"
EOF
else
    cat > "$OUTPUT_DIR/up.sh" <<EOF
#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="\$(cd "\$(dirname "\${BASH_SOURCE[0]}")" && pwd)"
cd "\$SCRIPT_DIR"

CONTAINER=$CONTAINER_NAME
IMAGE=$CONTAINER_NAME:latest
NETWORK=$NETWORK_NAME
PROXY_PORT=${PROXY_PORT}
LISTEN_PORT=${LISTEN_PORT}

if ! docker image inspect banyan-node:latest >/dev/null 2>&1; then
    echo "error: banyan-node:latest image not found. Build it with:" >&2
    echo "       nx run chat-service:docker:build" >&2
    exit 1
fi

docker network inspect "\$NETWORK" >/dev/null 2>&1 || docker network create --driver bridge "\$NETWORK"
docker build -t "\$IMAGE" .
docker rm -f "\$CONTAINER" >/dev/null 2>&1 || true

docker run -d --name "\$CONTAINER" --network "\$NETWORK" \\
    --restart unless-stopped \\
    -p "\$PROXY_PORT:9090" \\
    -p "\$LISTEN_PORT:9100" \\
    "\$IMAGE" \\
    -privkey=/app/node-identity.pem \\
    -addons-dir=/app/addons \\
    -figs-dir=/app/figs \\
    -listen=/ip4/0.0.0.0/tcp/9100 \\
    -no-crypto \\
    -allow-expired-figs \\
    -allow-insecure-figs \\
    -tunnel-enabled \\
    -override-transport-restrictions \\
    -nat-traversal

echo "User node '\$CONTAINER' running on network '\$NETWORK' (no VPN)"
echo "  browser proxy: http://localhost:\$PROXY_PORT"
echo "  libp2p listen: host port \$LISTEN_PORT -> container :9100"
EOF
fi
chmod +x "$OUTPUT_DIR/up.sh"

cat > "$OUTPUT_DIR/down.sh" <<EOF
#!/usr/bin/env bash
set -euo pipefail

CONTAINER=$CONTAINER_NAME
VPN_CONTAINER=$VPN_CONTAINER_NAME
NETWORK=$NETWORK_NAME

docker rm -f "\$CONTAINER" "\$VPN_CONTAINER" >/dev/null 2>&1 || true
docker network rm "\$NETWORK" >/dev/null 2>&1 || true

echo "User node '\$CONTAINER' stopped and network removed"
EOF
chmod +x "$OUTPUT_DIR/down.sh"

echo ""
echo "Done. Next steps:"
echo "  cd $OUTPUT_DIR"
echo "  ./up.sh      # build image and start container"
echo "  ./down.sh    # stop container and remove its bridge network"
