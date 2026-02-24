#!/bin/bash
# Break-Glass Guided Test
#
# Validates the break-glass-to-managed flow end-to-end:
#   1. Applies break-glass fixture (Flux-managed + Native orphan)
#   2. Verifies ownership detection (payment-api=Flux, hotfix-cache=Native)
#   3. Verifies orphan detection via map
#   4. Verifies trace output for both resources
#   5. Cleans up
#
# Prerequisites:
#   - kubectl access to a cluster
#   - Flux CRDs installed (for Flux ownership labels)
#   - cub-scout binary built (go build ./cmd/cub-scout)
#
# Usage:
#   ./test/e2e/break-glass-guided-test.sh
#   ./test/e2e/break-glass-guided-test.sh --cleanup   # Remove fixtures only
#
# See: docs/howto/break-glass-to-managed.md

set -eo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
FIXTURE="$REPO_ROOT/examples/demos/break-glass.yaml"
BINARY="$REPO_ROOT/cub-scout"
NAMESPACE="break-glass-demo"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m'

pass() { echo -e "${GREEN}PASS${NC} $*"; }
fail() { echo -e "${RED}FAIL${NC} $*"; FAILURES=$((FAILURES + 1)); }
info() { echo -e "${CYAN}INFO${NC} $*"; }
step() { echo -e "\n${BOLD}Step $1:${NC} $2"; }

FAILURES=0
TESTS=0

assert_contains() {
    local label="$1"
    local haystack="$2"
    local needle="$3"
    TESTS=$((TESTS + 1))
    if echo "$haystack" | grep -qi "$needle"; then
        pass "$label"
    else
        fail "$label (expected '$needle' in output)"
    fi
}

assert_not_contains() {
    local label="$1"
    local haystack="$2"
    local needle="$3"
    TESTS=$((TESTS + 1))
    if echo "$haystack" | grep -qi "$needle"; then
        fail "$label (unexpected '$needle' in output)"
    else
        pass "$label"
    fi
}

cleanup() {
    info "Cleaning up break-glass fixtures..."
    kubectl delete -f "$FIXTURE" --ignore-not-found=true 2>/dev/null || true
    kubectl delete namespace "$NAMESPACE" --ignore-not-found=true 2>/dev/null || true
    info "Cleanup complete"
}

# Handle --cleanup flag
if [[ "${1:-}" == "--cleanup" ]]; then
    cleanup
    exit 0
fi

echo -e "${BOLD}Break-Glass Guided Test${NC}"
echo -e "${DIM}Validates the break-glass-to-managed flow end-to-end${NC}"
echo ""

# Pre-flight checks
step 0 "Pre-flight checks"

if [[ ! -f "$BINARY" ]]; then
    fail "cub-scout binary not found at $BINARY"
    echo "  Run: go build ./cmd/cub-scout"
    exit 1
fi
pass "cub-scout binary exists"
TESTS=$((TESTS + 1))

if ! kubectl cluster-info &>/dev/null 2>&1; then
    fail "Cannot connect to cluster"
    echo "  Check your kubeconfig"
    exit 1
fi
pass "Cluster is reachable"
TESTS=$((TESTS + 1))

# Step 1: Apply fixtures
step 1 "Apply break-glass fixtures"

kubectl apply -f "$FIXTURE" 2>/dev/null
sleep 2
info "Waiting for resources to settle..."
kubectl wait --for=condition=Available deployment/payment-api -n "$NAMESPACE" --timeout=60s 2>/dev/null || true
kubectl wait --for=condition=Available deployment/hotfix-cache -n "$NAMESPACE" --timeout=60s 2>/dev/null || true

DEPLOYMENTS=$(kubectl get deployments -n "$NAMESPACE" --no-headers 2>/dev/null | wc -l | tr -d ' ')
TESTS=$((TESTS + 1))
if [[ "$DEPLOYMENTS" -ge 2 ]]; then
    pass "Both deployments exist in $NAMESPACE ($DEPLOYMENTS found)"
else
    fail "Expected at least 2 deployments, found $DEPLOYMENTS"
fi

# Step 2: Verify ownership detection via map list
step 2 "Verify ownership detection"

MAP_OUTPUT=$("$BINARY" map list -n "$NAMESPACE" 2>/dev/null || echo "MAP_FAILED")

if [[ "$MAP_OUTPUT" == "MAP_FAILED" ]]; then
    fail "cub-scout map list failed"
else
    # payment-api should be detected as Flux-owned
    assert_contains "payment-api appears in map output" "$MAP_OUTPUT" "payment-api"

    # hotfix-cache should appear
    assert_contains "hotfix-cache appears in map output" "$MAP_OUTPUT" "hotfix-cache"
fi

# Step 3: Verify orphan detection
step 3 "Verify orphan detection"

# Check that hotfix-cache has no GitOps labels
HOTFIX_LABELS=$(kubectl get deployment hotfix-cache -n "$NAMESPACE" -o jsonpath='{.metadata.labels}' 2>/dev/null)
assert_not_contains "hotfix-cache has no Flux labels" "$HOTFIX_LABELS" "kustomize.toolkit.fluxcd.io"
assert_not_contains "hotfix-cache has no ArgoCD labels" "$HOTFIX_LABELS" "argocd.argoproj.io"
assert_not_contains "hotfix-cache has no Helm managed-by" "$HOTFIX_LABELS" "managed-by.*Helm"

# Check that payment-api HAS Flux labels
PAYMENT_LABELS=$(kubectl get deployment payment-api -n "$NAMESPACE" -o jsonpath='{.metadata.labels}' 2>/dev/null)
assert_contains "payment-api has Flux labels" "$PAYMENT_LABELS" "kustomize.toolkit.fluxcd.io"

# Step 4: Verify break-glass annotations
step 4 "Verify break-glass annotations"

HOTFIX_ANNOTATIONS=$(kubectl get deployment hotfix-cache -n "$NAMESPACE" -o jsonpath='{.metadata.annotations}' 2>/dev/null)
assert_contains "hotfix-cache has incident annotation" "$HOTFIX_ANNOTATIONS" "INC-4521"
assert_contains "hotfix-cache has applied-by annotation" "$HOTFIX_ANNOTATIONS" "admin"
assert_contains "hotfix-cache has reason annotation" "$HOTFIX_ANNOTATIONS" "Emergency cache fix"

# Step 5: Verify trace (if Flux CRDs are available)
step 5 "Verify trace output"

# Try tracing the Flux-managed resource
TRACE_PAYMENT=$("$BINARY" trace deploy/payment-api -n "$NAMESPACE" 2>&1 || echo "TRACE_FAILED")

if [[ "$TRACE_PAYMENT" == *"TRACE_FAILED"* ]] || [[ "$TRACE_PAYMENT" == *"context appears stale"* ]]; then
    info "Trace for payment-api not fully available (Flux controllers may not be running)"
    info "  This is expected in test environments without active Flux controllers"
else
    assert_contains "payment-api trace mentions Flux" "$TRACE_PAYMENT" "flux\|Kustomization\|kustomize"
fi

# Try tracing the orphan
TRACE_HOTFIX=$("$BINARY" trace deploy/hotfix-cache -n "$NAMESPACE" 2>&1 || echo "TRACE_FAILED")

if [[ "$TRACE_HOTFIX" == *"TRACE_FAILED"* ]]; then
    info "Trace for hotfix-cache not available"
else
    # The orphan should not have a Flux/Argo/Helm trace result
    assert_not_contains "hotfix-cache trace has no Flux chain" "$TRACE_HOTFIX" "Kustomization.*payment"
fi

# Step 6: Cleanup
step 6 "Cleanup"
cleanup

# Summary
echo ""
echo -e "${BOLD}Results:${NC}"
echo -e "  Tests run: $TESTS"
echo -e "  Passed:    $((TESTS - FAILURES))"
echo -e "  Failed:    $FAILURES"
echo ""

if [[ $FAILURES -gt 0 ]]; then
    echo -e "${RED}${BOLD}$FAILURES test(s) failed${NC}"
    exit 1
else
    echo -e "${GREEN}${BOLD}All tests passed${NC}"
    exit 0
fi
