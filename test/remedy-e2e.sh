#!/bin/bash
# Remedy E2E Test Script
# Tests the remedy command with real Kubernetes resources

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
FIXTURES="$SCRIPT_DIR/fixtures/remedy"
NAMESPACE="test-remedy"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

pass() { echo -e "${GREEN}✓${NC} $1"; }
fail() { echo -e "${RED}✗${NC} $1"; exit 1; }
info() { echo -e "${YELLOW}→${NC} $1"; }

# Check prerequisites
check_prerequisites() {
    info "Checking prerequisites..."

    if ! command -v kubectl &> /dev/null; then
        fail "kubectl not found"
    fi

    if ! kubectl cluster-info &> /dev/null; then
        fail "No Kubernetes cluster available"
    fi

    if [ ! -f "$PROJECT_ROOT/cub-scout" ]; then
        info "Building cub-scout..."
        (cd "$PROJECT_ROOT" && go build ./cmd/cub-scout) || fail "Build failed"
    fi

    pass "Prerequisites OK"
}

# Setup test namespace and resources
setup() {
    info "Setting up test namespace..."

    kubectl apply -f "$FIXTURES/namespace.yaml" 2>/dev/null || true
    kubectl apply -f "$FIXTURES/deployment.yaml"
    kubectl apply -f "$FIXTURES/orphaned-service.yaml"
    kubectl apply -f "$FIXTURES/configmap-bad.yaml"

    # Wait for deployment
    kubectl wait --for=condition=available deployment/nginx-test -n $NAMESPACE --timeout=60s

    pass "Test resources created in $NAMESPACE"
}

# Test 1: remedy --list
test_list() {
    info "Test 1: remedy --list"

    output=$("$PROJECT_ROOT/cub-scout" remedy --list 2>&1)

    if echo "$output" | grep -q "config_fix"; then
        pass "Lists config_fix CCVEs"
    else
        fail "Missing config_fix in list"
    fi

    if echo "$output" | grep -q "TOTAL"; then
        pass "Shows total count"
    else
        fail "Missing total count"
    fi
}

# Test 2: Namespace validation
test_namespace_validation() {
    info "Test 2: Namespace validation"

    if "$PROJECT_ROOT/cub-scout" remedy CCVE-2025-0147 --dry-run -n nonexistent-ns-12345 2>&1 | grep -q "not found"; then
        pass "Rejects invalid namespace"
    else
        fail "Should reject invalid namespace"
    fi

    if "$PROJECT_ROOT/cub-scout" remedy CCVE-2025-0147 --dry-run -n $NAMESPACE 2>&1 | grep -q "REMEDY PLAN"; then
        pass "Accepts valid namespace"
    else
        fail "Should accept valid namespace"
    fi
}

# Test 3: Dry-run shows plan
test_dry_run() {
    info "Test 3: Dry-run mode"

    output=$("$PROJECT_ROOT/cub-scout" remedy CCVE-2025-0147 --dry-run -n $NAMESPACE 2>&1)

    if echo "$output" | grep -q "REMEDY PLAN"; then
        pass "Shows remedy plan"
    else
        fail "Missing remedy plan"
    fi

    if echo "$output" | grep -q "Risk Level"; then
        pass "Shows risk level"
    else
        fail "Missing risk level"
    fi

    if echo "$output" | grep -q "dry-run"; then
        pass "Shows dry-run warning"
    else
        fail "Missing dry-run warning"
    fi
}

# Test 4: Audit logging
test_audit_logging() {
    info "Test 4: Audit logging"

    AUDIT_FILE="/tmp/remedy-test-audit.log"
    rm -f "$AUDIT_FILE"

    "$PROJECT_ROOT/cub-scout" remedy CCVE-2025-0147 --dry-run -n $NAMESPACE --audit-file="$AUDIT_FILE" 2>&1 >/dev/null

    if [ -f "$AUDIT_FILE" ]; then
        pass "Audit file created"
    else
        fail "Audit file not created"
    fi

    if grep -q "DRY-RUN" "$AUDIT_FILE"; then
        pass "Audit contains DRY-RUN status"
    else
        fail "Missing DRY-RUN in audit"
    fi

    if grep -q "CCVE-2025-0147" "$AUDIT_FILE"; then
        pass "Audit contains CCVE ID"
    else
        fail "Missing CCVE ID in audit"
    fi

    rm -f "$AUDIT_FILE"
}

# Test 5: JSON output
test_json_output() {
    info "Test 5: JSON output"

    output=$("$PROJECT_ROOT/cub-scout" remedy CCVE-2025-0147 --dry-run -n $NAMESPACE --json 2>&1)

    if echo "$output" | jq . >/dev/null 2>&1; then
        pass "Valid JSON output"
    else
        fail "Invalid JSON output"
    fi

    if echo "$output" | jq -e '.ccve' >/dev/null 2>&1; then
        pass "JSON contains ccve field"
    else
        fail "Missing ccve in JSON"
    fi
}

# Cleanup
cleanup() {
    info "Cleaning up..."
    kubectl delete namespace $NAMESPACE --ignore-not-found=true 2>/dev/null || true
    pass "Cleanup complete"
}

# Main
main() {
    echo ""
    echo "================================"
    echo "  Remedy E2E Tests"
    echo "================================"
    echo ""

    check_prerequisites
    setup

    echo ""
    echo "Running tests..."
    echo ""

    test_list
    test_namespace_validation
    test_dry_run
    test_audit_logging
    test_json_output

    echo ""
    echo "================================"
    echo -e "  ${GREEN}All tests passed!${NC}"
    echo "================================"
    echo ""

    # Cleanup on success
    if [ "${KEEP_FIXTURES:-}" != "1" ]; then
        cleanup
    else
        info "Keeping test fixtures (KEEP_FIXTURES=1)"
    fi
}

# Handle cleanup on exit
trap 'cleanup' EXIT

main "$@"
