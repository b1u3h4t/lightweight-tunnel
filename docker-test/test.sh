#!/usr/bin/env bash
set -euo pipefail

# =============================================================================
# Lightweight Tunnel - Multi-Client Integration Test
#
# Topology:
#
#   client1 (10.99.0.2) ──┐
#                          ├── server (10.99.0.1) ── relay
#   client2 (10.99.0.3) ──┘
#
# Tests:
#   Phase 1: Basic connectivity (each client <-> server)
#   Phase 2: Client-to-client through server relay
#   Phase 3: Burst / stress test
#
# Usage:  docker exec lt-client1 bash /test.sh
#    or:  run as the tester entrypoint
# =============================================================================

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

pass() { echo -e "  ${GREEN}[PASS]${NC} $1"; }
fail() { echo -e "  ${RED}[FAIL]${NC} $1"; FAILURES=$((FAILURES + 1)); }
info() { echo -e "  ${YELLOW}[INFO]${NC} $1"; }
section() { echo -e "\n${CYAN}── $1 ──${NC}"; }

FAILURES=0
TOTAL=0

run_test() {
    local name="$1"
    shift
    TOTAL=$((TOTAL + 1))
    if "$@" > /dev/null 2>&1; then
        pass "${name}"
    else
        fail "${name}"
    fi
}

# Ping helper: returns 0 if all packets received
ping_test() {
    local host="$1"
    local count="${2:-5}"
    ping -c "$count" -W 3 -i 0.2 "$host" 2>&1
}

# Ping burst helper: returns summary line
ping_burst() {
    local host="$1"
    local count="${2:-50}"
    ping -c "$count" -W 3 -i 0.05 "$host" 2>&1
}

echo ""
echo "============================================================"
echo "   Lightweight Tunnel - Multi-Client Integration Tests"
echo "============================================================"
echo ""
echo "   Topology:"
echo "     client1 (10.99.0.2) ──> server (10.99.0.1) <── client2 (10.99.0.3)"
echo "     client1 (10.99.0.2) <──── server relay ────> client2 (10.99.0.3)"
echo ""

# ---------------------------------------------------------------------------
# This script is designed to run INSIDE one of the tunnel containers
# (e.g., lt-client1) where tun0 is available. When run from the
# external "tester" container we can only verify docker-network health.
# ---------------------------------------------------------------------------

# Detect environment: do we have tun0?
HAS_TUN=false
if ip link show tun0 > /dev/null 2>&1; then
    HAS_TUN=true
fi

if [ "$HAS_TUN" = false ]; then
    echo "  ⚠  No tun0 detected — this script should run inside a tunnel container."
    echo "  ⚠  Falling back to docker-network-only checks."
    echo ""
fi

# =========================================================================
#  Phase 1: Basic Connectivity
# =========================================================================
section "Phase 1: Basic Connectivity"

if [ "$HAS_TUN" = true ]; then
    MY_IP=$(ip -4 addr show tun0 | grep -oP 'inet \K[\d.]+')
    info "Running inside tunnel container, tun0 IP = ${MY_IP}"

    # Determine peer IPs
    SERVER_TUN="10.99.0.1"
    CLIENT1_TUN="10.99.0.2"
    CLIENT2_TUN="10.99.0.3"

    run_test "Ping server (${SERVER_TUN})" ping_test "$SERVER_TUN"

    if [ "$MY_IP" = "$CLIENT1_TUN" ]; then
        PEER_TUN="$CLIENT2_TUN"
        SELF_LABEL="client1"
        PEER_LABEL="client2"
    else
        PEER_TUN="$CLIENT1_TUN"
        SELF_LABEL="client2"
        PEER_LABEL="client1"
    fi

    run_test "Ping ${PEER_LABEL} (${PEER_TUN}) via server relay" ping_test "$PEER_TUN"
else
    # Docker network checks only
    info "Checking docker-network reachability..."
    apt-get update -qq > /dev/null 2>&1 && apt-get install -y -qq iputils-ping netcat-openbsd > /dev/null 2>&1 || true

    run_test "Server reachable (172.30.0.10)" ping_test "172.30.0.10" 3
    run_test "Client1 reachable (172.30.0.20)" ping_test "172.30.0.20" 3
    run_test "Client2 reachable (172.30.0.21)" ping_test "172.30.0.21" 3
    run_test "Server port 9000 open" bash -c "nc -z -w 3 172.30.0.10 9000"
fi

# =========================================================================
#  Phase 2: Client-to-Client via Server Relay
# =========================================================================
section "Phase 2: Client-to-Client Relay"

if [ "$HAS_TUN" = true ]; then
    info "Testing ${SELF_LABEL} -> ${PEER_LABEL} relay through server..."

    # 5-packet connectivity
    run_test "${SELF_LABEL} -> ${PEER_LABEL}: 5 pings" ping_test "$PEER_TUN" 5

    # Also test round-trip to server
    run_test "${SELF_LABEL} -> server: 5 pings" ping_test "$SERVER_TUN" 5

    # Larger ping payload (1000 bytes to test MTU handling)
    run_test "${SELF_LABEL} -> ${PEER_LABEL}: large payload (1000B)" \
        ping -c 3 -W 3 -s 1000 "$PEER_TUN"
else
    info "Skipping relay tests (no tun0 — run inside tunnel container)"
fi

# =========================================================================
#  Phase 3: Burst / Stress
# =========================================================================
section "Phase 3: Burst Stress Test"

if [ "$HAS_TUN" = true ]; then
    info "50-packet burst to ${PEER_LABEL} (${PEER_TUN})..."
    BURST_OUT=$(ping_burst "$PEER_TUN" 50 2>&1) || true
    BURST_LOSS=$(echo "$BURST_OUT" | grep -oP '\d+(?=% packet loss)' || echo "100")
    BURST_RTT=$(echo "$BURST_OUT" | grep -oP 'rtt min/avg/max/mdev = [\d.]+/([\d.]+)' | grep -oP '/\K[\d.]+' || echo "N/A")

    TOTAL=$((TOTAL + 1))
    if [ "$BURST_LOSS" = "0" ]; then
        pass "50-pkt burst ${SELF_LABEL} -> ${PEER_LABEL}: 0% loss, avg ${BURST_RTT}ms"
    elif [ "$BURST_LOSS" -le 2 ]; then
        pass "50-pkt burst ${SELF_LABEL} -> ${PEER_LABEL}: ${BURST_LOSS}% loss (acceptable), avg ${BURST_RTT}ms"
    else
        fail "50-pkt burst ${SELF_LABEL} -> ${PEER_LABEL}: ${BURST_LOSS}% loss, avg ${BURST_RTT}ms"
    fi

    info "50-packet burst to server (${SERVER_TUN})..."
    BURST_OUT2=$(ping_burst "$SERVER_TUN" 50 2>&1) || true
    BURST_LOSS2=$(echo "$BURST_OUT2" | grep -oP '\d+(?=% packet loss)' || echo "100")
    BURST_RTT2=$(echo "$BURST_OUT2" | grep -oP 'rtt min/avg/max/mdev = [\d.]+/([\d.]+)' | grep -oP '/\K[\d.]+' || echo "N/A")

    TOTAL=$((TOTAL + 1))
    if [ "$BURST_LOSS2" = "0" ]; then
        pass "50-pkt burst ${SELF_LABEL} -> server: 0% loss, avg ${BURST_RTT2}ms"
    elif [ "$BURST_LOSS2" -le 2 ]; then
        pass "50-pkt burst ${SELF_LABEL} -> server: ${BURST_LOSS2}% loss (acceptable), avg ${BURST_RTT2}ms"
    else
        fail "50-pkt burst ${SELF_LABEL} -> server: ${BURST_LOSS2}% loss, avg ${BURST_RTT2}ms"
    fi
else
    info "Skipping burst tests (no tun0)"
fi

# =========================================================================
#  Summary
# =========================================================================
echo ""
echo "============================================================"
echo "  Results: $((TOTAL - FAILURES))/${TOTAL} passed"
if [ ${FAILURES} -gt 0 ]; then
    echo -e "  ${RED}${FAILURES} test(s) FAILED${NC}"
    echo "============================================================"
    exit 1
else
    echo -e "  ${GREEN}All tests passed!${NC}"
    echo "============================================================"
    echo ""
    if [ "$HAS_TUN" = true ]; then
        info "Multi-client tunnel relay verified."
        info "  Server  : 10.99.0.1"
        info "  Client1 : 10.99.0.2"
        info "  Client2 : 10.99.0.3"
        info "  Relay   : client1 <-> server <-> client2"
    fi
    exit 0
fi
