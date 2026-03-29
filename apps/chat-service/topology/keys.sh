#!/bin/bash
#
# Key Management Script for Banyan Chat
# 
# Usage:
#   ./keys.sh init                    # Initialize all keys and figs
#   ./keys.sh rotate <key-name>       # Rotate a specific key (e.g., chat-ledger-key)
#   ./keys.sh rotate-all              # Rotate all service keys (keeps authority)
#   ./keys.sh regenerate-figs         # Regenerate fig files with current keys
#   ./keys.sh list                    # List all keys
#   ./keys.sh export <output-dir>     # Export public keys for distribution
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TOPOLOGY_DIR="$SCRIPT_DIR"
KEYS_DIR="$TOPOLOGY_DIR/output/keys"
NODES_DIR="$TOPOLOGY_DIR/output/nodes"
CONFIG_FILE="$TOPOLOGY_DIR/chat-config.json"
TOPOLOGY_GEN="$(cd "$SCRIPT_DIR/../../topology-gen" && pwd)"

SERVICE_KEYS=("chat-static-key" "chat-ledger-key" "chat-mod-key" "chat-admin-key")
AUTHORITY_KEYS=("chat-authority")

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

generate_ed25519_key() {
    local output_file="$1"
    openssl genpkey -algorithm ED25519 -out "$output_file" 2>/dev/null
    chmod 600 "$output_file"
}

get_public_key_hex() {
    local pem_file="$1"
    local pub_pem=$(mktemp)
    openssl pkey -in "$pem_file" -pubout -out "$pub_pem" 2>/dev/null
    local der=$(mktemp)
    openssl pkey -in "$pem_file" -pubout -outform DER 2>/dev/null > "$der"
    local hex=$(xxd -p "$der" | tr -d '\n')
    rm -f "$pub_pem" "$der"
    echo "$hex"
}

init_keys() {
    log_info "Initializing all keys..."
    
    rm -rf "$KEYS_DIR" "$NODES_DIR"
    mkdir -p "$KEYS_DIR/authority"
    
    # Generate authority keys
    for key in "${AUTHORITY_KEYS[@]}"; do
        log_info "Generating authority key: $key"
        generate_ed25519_key "$KEYS_DIR/authority/${key}.pem"
    done
    
    # Generate service keys
    for key in "${SERVICE_KEYS[@]}"; do
        log_info "Generating service key: $key"
        generate_ed25519_key "$KEYS_DIR/${key}.pem"
    done
    
    # Generate key mapping
    update_key_mapping
    
    # Regenerate figs and nodes
    regenerate_all
}

rotate_key() {
    local key_name="$1"
    
    if [[ ! " ${SERVICE_KEYS[@]} " =~ " $key_name " ]]; then
        log_error "Unknown key: $key_name"
        log_info "Valid keys: ${SERVICE_KEYS[*]}"
        exit 1
    fi
    
    log_warn "Rotating key: $key_name"
    log_warn "This will invalidate all figs and node configs!"
    
    # Generate new key
    generate_ed25519_key "$KEYS_DIR/${key_name}.pem"
    log_info "Generated new key: $key_name"
    
    # Update key mapping
    update_key_mapping
    
    # Regenerate figs and nodes
    regenerate_all
}

rotate_all_service_keys() {
    log_warn "Rotating all service keys (keeping authority)..."
    
    for key in "${SERVICE_KEYS[@]}"; do
        log_info "Rotating: $key"
        generate_ed25519_key "$KEYS_DIR/${key}.pem"
    done
    
    update_key_mapping
    regenerate_all
    
    log_info "All service keys rotated. Distribute new keys to nodes!"
}

update_key_mapping() {
    log_info "Updating key mapping..."
    
    local mapping_file="$KEYS_DIR/key-mapping.json"
    echo "{" > "$mapping_file"
    
    local first=true
    
    # Authority keys
    for key_file in "$KEYS_DIR/authority"/*.pem; do
        if [[ -f "$key_file" ]]; then
            local name=$(basename "$key_file" .pem)
            local hex=$(get_public_key_hex "$key_file")
            if [[ "$first" == true ]]; then
                first=false
            else
                echo "," >> "$mapping_file"
            fi
            echo "  \"${name}.pem\": \"${hex}\"" >> "$mapping_file"
        fi
    done
    
    # Service keys
    for key_file in "$KEYS_DIR"/*.pem; do
        if [[ -f "$key_file" ]]; then
            local name=$(basename "$key_file" .pem)
            local hex=$(get_public_key_hex "$key_file")
            if [[ "$first" == true ]]; then
                first=false
            else
                echo "," >> "$mapping_file"
            fi
            echo "  \"${name}.pem\": \"${hex}\"" >> "$mapping_file"
        fi
    done
    
    echo "" >> "$mapping_file"
    echo "}" >> "$mapping_file"
    
    log_info "Key mapping updated"
}

regenerate_all() {
    log_info "Regenerating figs and node configs..."
    
    cd "$TOPOLOGY_GEN"
    go run . "$CONFIG_FILE" "$TOPOLOGY_DIR/output"
    
    log_info "Figs and nodes regenerated"
}

regenerate_figs() {
    log_info "Regenerating fig files only..."
    regenerate_all
}

list_keys() {
    log_info "Keys in $KEYS_DIR:"
    echo ""
    echo "Authority Keys:"
    for key_file in "$KEYS_DIR/authority"/*.pem; do
        if [[ -f "$key_file" ]]; then
            local name=$(basename "$key_file" .pem)
            local hex=$(get_public_key_hex "$key_file")
            echo "  $name: ${hex:0:32}..."
        fi
    done
    echo ""
    echo "Service Keys:"
    for key_file in "$KEYS_DIR"/*.pem; do
        if [[ -f "$key_file" ]]; then
            local name=$(basename "$key_file" .pem)
            local hex=$(get_public_key_hex "$key_file")
            echo "  $name: ${hex:0:32}..."
        fi
    done
}

export_public_keys() {
    local output_dir="$1"
    
    if [[ -z "$output_dir" ]]; then
        output_dir="$TOPOLOGY_DIR/output/public-keys"
    fi
    
    mkdir -p "$output_dir"
    
    log_info "Exporting public keys to $output_dir..."
    
    # Export authority public keys
    for key_file in "$KEYS_DIR/authority"/*.pem; do
        if [[ -f "$key_file" ]]; then
            local name=$(basename "$key_file")
            openssl pkey -in "$key_file" -pubout -out "$output_dir/$name" 2>/dev/null
            log_info "  Exported: $name"
        fi
    done
    
    # Export service public keys
    for key_file in "$KEYS_DIR"/*.pem; do
        if [[ -f "$key_file" ]]; then
            local name=$(basename "$key_file")
            openssl pkey -in "$key_file" -pubout -out "$output_dir/$name" 2>/dev/null
            log_info "  Exported: $name"
        fi
    done
    
    # Export key mapping
    cp "$KEYS_DIR/key-mapping.json" "$output_dir/"
    
    log_info "Public keys exported to $output_dir"
}

case "${1:-}" in
    init)
        init_keys
        ;;
    rotate)
        rotate_key "$2"
        ;;
    rotate-all)
        rotate_all_service_keys
        ;;
    regenerate-figs)
        regenerate_figs
        ;;
    list)
        list_keys
        ;;
    export)
        export_public_keys "$2"
        ;;
    *)
        echo "Banyan Chat Key Management"
        echo ""
        echo "Usage: $0 <command> [args]"
        echo ""
        echo "Commands:"
        echo "  init                  Initialize all keys and figs"
        echo "  rotate <key>          Rotate a specific key (e.g., chat-ledger-key)"
        echo "  rotate-all            Rotate all service keys (keeps authority)"
        echo "  regenerate-figs       Regenerate fig files with current keys"
        echo "  list                  List all keys"
        echo "  export [dir]          Export public keys for distribution"
        echo ""
        echo "Examples:"
        echo "  $0 init"
        echo "  $0 rotate chat-ledger-key"
        echo "  $0 export ./keys-to-distribute"
        ;;
esac
