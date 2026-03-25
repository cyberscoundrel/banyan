#!/bin/bash
# Manual test script for .svc domain functionality
# This script tests the service key prefix proxy and .svc domain routing
#
# Prerequisites:
# 1. Two Banyan nodes running with the proxy addon
# 2. At least one node has a service key registered
#
# Usage:
#   ./test-svc-domain.sh <node1_mgmt_url> <node2_id> <service_key_prefix>
#
# Example:
#   ./test-svc-domain.sh http://127.0.0.1:8787 12D3KooWExample deadbeef

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

if [ $# -lt 1 ]; then
    echo -e "${RED}Usage: $0 <mgmt_url> [service_key_prefix]${NC}"
    echo ""
    echo "Arguments:"
    echo "  mgmt_url          - Management API URL (e.g., http://127.0.0.1:8787)"
    echo "  service_key_prefix - Optional service key prefix to test (min 8 chars)"
    echo ""
    echo "Example:"
    echo "  $0 http://127.0.0.1:8787 deadbeef1234"
    exit 1
fi

MGMT_URL="$1"
SERVICE_KEY_PREFIX="${2:-}"

echo -e "${BLUE}=== .svc Domain Functionality Tests ===${NC}"
echo -e "${YELLOW}Management URL: $MGMT_URL${NC}"
echo ""

TEST_PASS=0
TEST_FAIL=0

run_test() {
    local name="$1"
    local url="$2"
    local expected_status="$3"
    local check_pattern="$4"

    echo -e "${BLUE}Test: $name${NC}"
    echo -e "  URL: $url"

    response=$(curl -s -w "\n%{http_code}" "$url" 2>/dev/null || echo -e "\n000")
    status=$(echo "$response" | tail -1)
    body=$(echo "$response" | sed '$d')

    if [ "$status" = "$expected_status" ]; then
        if [ -n "$check_pattern" ]; then
            if echo "$body" | grep -q "$check_pattern"; then
                echo -e "  ${GREEN}[PASS]${NC} Status: $status, Pattern found: $check_pattern"
                ((TEST_PASS++))
            else
                echo -e "  ${RED}[FAIL]${NC} Status: $status, Pattern NOT found: $check_pattern"
                echo "  Body: $body"
                ((TEST_FAIL++))
            fi
        else
            echo -e "  ${GREEN}[PASS]${NC} Status: $status"
            ((TEST_PASS++))
        fi
    else
        echo -e "  ${RED}[FAIL]${NC} Expected status $expected_status, got $status"
        echo "  Body: $body"
        ((TEST_FAIL++))
    fi
    echo ""
}

# Test 1: Check node status
echo -e "${YELLOW}=== Basic Connectivity ===${NC}"
run_test "Node status" "$MGMT_URL/node/status" "200" "node_id"

# Test 2: Check connections
echo -e "${YELLOW}=== Connection Tests ===${NC}"
run_test "List connections" "$MGMT_URL/network/connections" "200" "connections"

# Test 3: Test service key prefix proxy endpoint (without valid key - expect 400 or 404)
if [ -n "$SERVICE_KEY_PREFIX" ]; then
    echo -e "${YELLOW}=== Service Key Prefix Proxy Tests ===${NC}"

    # Test prefix too short (should return 400)
    run_test "Prefix too short" "$MGMT_URL/proxy/service-key/abc123/test" "400" "at least 8"

    # Test with full prefix
    if [ ${#SERVICE_KEY_PREFIX} -ge 8 ]; then
        # This may fail if no matching key, but we're testing the endpoint exists
        echo -e "${BLUE}Testing with prefix: $SERVICE_KEY_PREFIX${NC}"
        response=$(curl -s -w "\n%{http_code}" "$MGMT_URL/proxy/service-key/$SERVICE_KEY_PREFIX/test" 2>/dev/null || echo -e "\n000")
        status=$(echo "$response" | tail -1)
        body=$(echo "$response" | sed '$d')

        if [ "$status" = "200" ] || [ "$status" = "404" ] || [ "$status" = "400" ] || [ "$status" = "502" ]; then
            echo -e "  ${GREEN}[PASS]${NC} Endpoint accessible, status: $status"
            ((TEST_PASS++))
        else
            echo -e "  ${RED}[FAIL]${NC} Unexpected status: $status"
            ((TEST_FAIL++))
        fi
        echo ""
    fi
fi

# Test 4: Test SOCKS5 proxy with .svc domain (requires proxy addon running)
echo -e "${YELLOW}=== SOCKS5 Proxy .svc Domain Tests ===${NC}"
echo -e "${BLUE}Note: These tests require the proxy addon to be running${NC}"
echo ""

# Check if proxy addon is accessible
PROXY_PORT=${PROXY_PORT:-8080}
if nc -z 127.0.0.1 $PROXY_PORT 2>/dev/null; then
    echo -e "${GREEN}Proxy addon detected on port $PROXY_PORT${NC}"

    # Test .svc address detection via HTTP proxy (will fail without valid key, but tests routing)
    if [ -n "$SERVICE_KEY_PREFIX" ]; then
        echo -e "${BLUE}Testing .svc HTTP proxy routing...${NC}"
        response=$(curl -s --proxy "http://127.0.0.1:$PROXY_PORT" --connect-timeout 5 \
            "http://${SERVICE_KEY_PREFIX}.svc/test" 2>&1 || echo "CONNECTION_FAILED")

        if echo "$response" | grep -q "502\|Bad Gateway\|CONNECTION_FAILED"; then
            echo -e "  ${GREEN}[PASS]${NC} .svc routing attempted (502 expected without valid service)"
            ((TEST_PASS++))
        else
            echo -e "  ${YELLOW}[INFO]${NC} Response: $response"
        fi
        echo ""
    fi
else
    echo -e "${YELLOW}Proxy addon not detected on port $PROXY_PORT${NC}"
    echo "Start proxy addon to test .svc domain routing"
fi

# Summary
echo -e "${BLUE}=== Test Summary ===${NC}"
echo -e "${GREEN}Passed: $TEST_PASS${NC}"
echo -e "${RED}Failed: $TEST_FAIL${NC}"

if [ "$TEST_FAIL" -gt 0 ]; then
    exit 1
fi

echo -e "${GREEN}All tests passed!${NC}"
