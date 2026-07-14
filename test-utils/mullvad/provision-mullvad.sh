#!/usr/bin/env bash
# Provision a set of Mullvad WireGuard identities — one per banyan node — and
# emit a .env file that docker compose / generate-user-node can consume.
#
# Idempotent: if the output env file already has a key for a given node name,
# the script reuses it (so re-running doesn't burn through the per-account
# 5-key limit). Pass --force to mint fresh keys for every node.
#
# Usage:
#   provision-mullvad.sh \
#       --account <16-digit account number> \
#       --names "static-1,static-2,ledger-1,ledger-2,mod-1,admin-1" \
#       [--env-file <path>] [--cities <comma-list>] [--force]
#
# Output env vars per node (with NAME upper-cased and dashes turned into _):
#   MULLVAD_PRIVKEY_<NAME>     - WireGuard private key (base64)
#   MULLVAD_ADDR_<NAME>        - tunnel IPv4 address in CIDR form (e.g. 10.x.y.z/32)
#   MULLVAD_HOSTNAME_<NAME>    - specific Mullvad relay hostname (e.g. us-nyc-wg-001)
#   MULLVAD_CITY_<NAME>        - human-readable city (informational)
#   MULLVAD_COUNTRY_<NAME>     - human-readable country (informational)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORKSPACE_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

ACCOUNT=""
NAMES=""
ENV_FILE="$WORKSPACE_ROOT/apps/chat-service/docker/.env"
CITIES_OVERRIDE=""
FORCE=0

# Curated set of geographically diverse cities. Round-robin assigned to nodes
# in order so each container gets a distinct egress IP from a different region.
DEFAULT_CITIES=(
    "us-nyc"
    "de-fra"
    "jp-tyo"
    "gb-lon"
    "se-sto"
    "au-syd"
    "sg-sin"
    "br-sao"
    "us-lax"
    "us-sea"
)

usage() {
    cat <<EOF
Usage: $(basename "$0") --account <num> --names <csv> [options]

Required:
  --account <num>      Mullvad account number (16 digits).
  --names <csv>        Comma-separated list of node names to provision keys
                       for, in the order they should be assigned cities.

Options:
  --env-file <path>    Output env file. Default: $ENV_FILE
  --cities <csv>       Override the default city pool with a custom comma-
                       separated list of Mullvad relay city codes
                       (e.g. "us-nyc,de-fra,jp-tyo"). Codes are recycled
                       round-robin if shorter than --names.
  --force              Overwrite existing entries in --env-file rather than
                       reusing them.
  -h | --help          Show this help.
EOF
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --account)   ACCOUNT="$2";        shift 2 ;;
        --names)     NAMES="$2";          shift 2 ;;
        --env-file)  ENV_FILE="$2";       shift 2 ;;
        --cities)    CITIES_OVERRIDE="$2"; shift 2 ;;
        --force)     FORCE=1;             shift ;;
        -h|--help)   usage; exit 0 ;;
        *)
            echo "error: unknown argument: $1" >&2
            usage >&2
            exit 2
            ;;
    esac
done

if [[ -z "$ACCOUNT" || -z "$NAMES" ]]; then
    echo "error: --account and --names are required" >&2
    usage >&2
    exit 2
fi

if ! [[ "$ACCOUNT" =~ ^[0-9]{16}$ ]]; then
    echo "error: --account must be a 16-digit Mullvad account number" >&2
    exit 2
fi

for cmd in wg curl jq; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
        echo "error: '$cmd' not found on PATH" >&2
        echo "       brew install wireguard-tools jq    # if missing" >&2
        exit 1
    fi
done

if [[ -n "$CITIES_OVERRIDE" ]]; then
    IFS=',' read -r -a CITIES <<< "$CITIES_OVERRIDE"
else
    CITIES=("${DEFAULT_CITIES[@]}")
fi

# Validate city codes against Mullvad's relay list.
RELAYS_JSON="$(curl -fsS https://api.mullvad.net/public/relays/wireguard/v2/)"
for city in "${CITIES[@]}"; do
    if ! echo "$RELAYS_JSON" | jq -e --arg c "$city" '.locations[$c]' >/dev/null; then
        echo "error: unknown Mullvad city code: $city" >&2
        echo "       run: curl -sS https://api.mullvad.net/public/relays/wireguard/v2/ | jq '.locations | keys'" >&2
        exit 1
    fi
done

mkdir -p "$(dirname "$ENV_FILE")"
touch "$ENV_FILE"

# Convert a node name like "chat-static-1" or "static-1" into an env-var
# friendly suffix: uppercase, dashes -> underscores.
to_var() {
    echo "$1" | tr '[:lower:]-' '[:upper:]_'
}

# Pull the value of KEY=... from the env file (last occurrence wins).
read_env_var() {
    local key="$1"
    grep -E "^${key}=" "$ENV_FILE" | tail -n1 | cut -d= -f2- || true
}

# Append (or replace) KEY=VALUE in the env file.
write_env_var() {
    local key="$1"
    local value="$2"
    if grep -q "^${key}=" "$ENV_FILE"; then
        # macOS sed needs `-i ''` for in-place; use a temp file for portability.
        local tmp
        tmp="$(mktemp)"
        awk -v k="$key" -v v="$value" 'BEGIN{set=0} \
            { if ($0 ~ "^"k"=") { print k"="v; set=1 } else { print } } \
            END{ if (!set) print k"="v }' "$ENV_FILE" > "$tmp"
        mv "$tmp" "$ENV_FILE"
    else
        echo "${key}=${value}" >> "$ENV_FILE"
    fi
}

IFS=',' read -r -a NAME_ARR <<< "$NAMES"

echo "Provisioning ${#NAME_ARR[@]} Mullvad WireGuard identities -> $ENV_FILE"

i=0
for name in "${NAME_ARR[@]}"; do
    var="$(to_var "$name")"
    city="${CITIES[$((i % ${#CITIES[@]}))]}"
    i=$((i + 1))

    privkey_var="MULLVAD_PRIVKEY_${var}"
    addr_var="MULLVAD_ADDR_${var}"
    hostname_var="MULLVAD_HOSTNAME_${var}"
    city_var="MULLVAD_CITY_${var}"
    country_var="MULLVAD_COUNTRY_${var}"

    # Pick the highest-weighted active WG relay in this city as the pinned
    # hostname. Gluetun's mullvad provider pulls from the same API, so the
    # hostname format matches.
    relay_hostname="$(echo "$RELAYS_JSON" \
        | jq -r --arg c "$city" '
            [.wireguard.relays[] | select(.location == $c and .active == true)]
            | sort_by(-.weight)
            | .[0].hostname // empty
          ')"
    if [[ -z "$relay_hostname" ]]; then
        echo "error: no active WireGuard relay found in city '$city'" >&2
        exit 1
    fi
    city_label="$(echo "$RELAYS_JSON" | jq -r --arg c "$city" '.locations[$c].city')"
    country="$(echo "$RELAYS_JSON" | jq -r --arg c "$city" '.locations[$c].country')"

    existing_priv="$(read_env_var "$privkey_var" || true)"
    existing_addr="$(read_env_var "$addr_var" || true)"

    if [[ "$FORCE" -eq 0 && -n "$existing_priv" && -n "$existing_addr" ]]; then
        echo "  $name: reusing existing key (already in $ENV_FILE) -> $relay_hostname"
        write_env_var "$hostname_var" "$relay_hostname"
        write_env_var "$city_var" "$city_label"
        write_env_var "$country_var" "$country"
        continue
    fi

    privkey="$(wg genkey)"
    pubkey="$(echo "$privkey" | wg pubkey)"

    # Submit pubkey to Mullvad. 201 -> success with body "ipv4/32,ipv6/128".
    response="$(curl -fsS -w '\n%{http_code}' -X POST https://api.mullvad.net/wg/ \
        -d "account=$ACCOUNT" --data-urlencode "pubkey=$pubkey" || true)"
    http_code="$(echo "$response" | tail -n1)"
    body="$(echo "$response" | sed '$d')"

    if [[ "$http_code" != "201" ]]; then
        echo "error: Mullvad rejected pubkey for $name (HTTP $http_code): $body" >&2
        if [[ "$http_code" == "400" || "$http_code" == "403" ]]; then
            echo "       common cause: 5-key per-account limit reached." >&2
            echo "       remove unused keys via the Mullvad app/web UI and retry," >&2
            echo "       or reduce --names to 5 entries." >&2
        fi
        exit 1
    fi

    addr="$(echo "$body" | tr -d '[:space:]' | cut -d, -f1)"
    if [[ -z "$addr" ]]; then
        echo "error: empty address from Mullvad for $name: $body" >&2
        exit 1
    fi

    write_env_var "$privkey_var" "$privkey"
    write_env_var "$addr_var" "$addr"
    write_env_var "$hostname_var" "$relay_hostname"
    write_env_var "$city_var" "$city_label"
    write_env_var "$country_var" "$country"

    echo "  $name -> $relay_hostname ($city_label, $country) addr=$addr"
done

chmod 600 "$ENV_FILE"
echo ""
echo "Wrote $ENV_FILE (mode 600)."
echo "docker compose up will pick this up automatically when launched from $(dirname "$ENV_FILE")."
