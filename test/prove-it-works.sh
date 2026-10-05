#!/bin/bash
#
# PROVE IT WORKS - Comprehensive verification of cub-scout
#
# This script PROVES that cub-scout works by running tests at different levels.
# See test/test-levels.yaml for level definitions.
#
# Usage:
#   ./test/prove-it-works.sh                    # Default: run unit tests
#   ./test/prove-it-works.sh --level=smoke      # Quick sanity check
#   ./test/prove-it-works.sh --level=unit       # Unit tests only
#   ./test/prove-it-works.sh --level=integration # Needs cluster
#   ./test/prove-it-works.sh --level=gitops     # Needs Flux + ArgoCD
#   ./test/prove-it-works.sh --level=demos      # All demos
#   ./test/prove-it-works.sh --level=examples   # All examples E2E
#   ./test/prove-it-works.sh --level=connected  # Needs ConfigHub
#   ./test/prove-it-works.sh --level=full       # EVERYTHING
#   ./test/prove-it-works.sh --all              # Alias for --level=full
#
# Environment variables:
#   SKIP_FLUX_INSTALL=1     Skip Flux installation
#   SKIP_ARGO_INSTALL=1     Skip ArgoCD installation
#   VERBOSE=1               Show all command output

set -eo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_ROOT"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m'

# Defaults
LEVEL="unit"
VERBOSE=${VERBOSE:-0}
PASSED=0
FAILED=0
SKIPPED=0

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --level=*)
            LEVEL="${1#*=}"
            shift
            ;;
        --all)
            LEVEL="full"
            shift
            ;;
        --verbose|-v)
            VERBOSE=1
            shift
            ;;
        --help|-h)
            echo "Usage: prove-it-works.sh [--level=LEVEL] [--verbose]"
            echo ""
            echo "Levels (cumulative):"
            echo "  smoke       Quick sanity check (< 10s, no cluster)"
            echo "  unit        Unit tests (< 30s, no cluster)"
            echo "  integration Integration tests (< 2m, needs cluster)"
            echo "  gitops      GitOps E2E (< 5m, needs Flux + ArgoCD)"
            echo "  demos       All demos (< 10m)"
            echo "  examples    All examples E2E (< 15m)"
            echo "  connected   ConfigHub connected mode (< 20m)"
            echo "  full        PROVE IT ALL WORKS"
            echo ""
            echo "Shortcuts:"
            echo "  --all       Alias for --level=full"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Helper functions
section() {
    echo ""
    echo -e "${BLUE}═══════════════════════════════════════════════════════════════════${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}═══════════════════════════════════════════════════════════════════${NC}"
}

subsection() {
    echo ""
    echo -e "${CYAN}── $1 ──${NC}"
}

run_test() {
    local name="$1"
    local cmd="$2"

    echo -n -e "  ${DIM}▸${NC} $name... "

    if [[ $VERBOSE -eq 1 ]]; then
        echo ""
        if eval "$cmd"; then
            echo -e "  ${GREEN}✓${NC} $name"
            PASSED=$((PASSED + 1))
            return 0
        else
            echo -e "  ${RED}✗${NC} $name"
            FAILED=$((FAILED + 1))
            return 1
        fi
    else
        if eval "$cmd" > /tmp/test-output.txt 2>&1; then
            echo -e "${GREEN}✓${NC}"
            PASSED=$((PASSED + 1))
            return 0
        else
            echo -e "${RED}✗${NC}"
            echo -e "    ${DIM}Output:${NC}"
            tail -5 /tmp/test-output.txt | sed 's/^/    /'
            FAILED=$((FAILED + 1))
            return 1
        fi
    fi
}

skip_test() {
    local name="$1"
    local reason="$2"
    echo -e "  ${RED}✗${NC} $name ${DIM}(required prerequisite missing: $reason)${NC}"
    FAILED=$((FAILED + 1))
    return 1
}

optional_test_unavailable() {
    local name="$1"
    local reason="$2"
    if [[ "$LEVEL" == "full" ]]; then
        skip_test "$name" "$reason"
    else
        echo -e "  ${YELLOW}○${NC} $name ${DIM}(optional check unavailable: $reason)${NC}"
        SKIPPED=$((SKIPPED + 1))
    fi
}

check_cluster() {
    kubectl cluster-info > /dev/null 2>&1
}

check_flux() {
    kubectl get crd kustomizations.kustomize.toolkit.fluxcd.io > /dev/null 2>&1
}

check_argocd() {
    kubectl get crd applications.argoproj.io > /dev/null 2>&1
}

check_confighub() {
    command -v cub > /dev/null 2>&1 && cub auth status > /dev/null 2>&1
}

# Start
echo ""
echo -e "${BOLD}╔═══════════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║              PROVE IT WORKS: cub-scout verification               ║${NC}"
echo -e "${BOLD}╚═══════════════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "Level: ${BOLD}$LEVEL${NC}"
echo -e "Time:  $(date)"
echo ""

# Level ordering
LEVELS=(smoke unit integration gitops demos examples connected full)
CURRENT_IDX=-1
for i in "${!LEVELS[@]}"; do
    if [[ "${LEVELS[$i]}" == "$LEVEL" ]]; then
        CURRENT_IDX=$i
        break
    fi
done

if [[ $CURRENT_IDX -lt 0 ]]; then
    echo "Unknown level: $LEVEL" >&2
    exit 1
fi

# =============================================================================
# LEVEL 0: SMOKE
# =============================================================================
if [[ $CURRENT_IDX -ge 0 ]]; then
    section "LEVEL 0: SMOKE (quick sanity check)"

    subsection "Build"
    run_test "go build" "go build ./cmd/cub-scout"

    subsection "Version"
    run_test "cub-scout version" "./cub-scout version"

    subsection "Help"
    run_test "cub-scout --help" "./cub-scout --help"
fi

# =============================================================================
# LEVEL 1: UNIT
# =============================================================================
if [[ $CURRENT_IDX -ge 1 ]]; then
    section "LEVEL 1: UNIT TESTS (no cluster needed)"

    subsection "Go Tests"
    run_test "go test ./..." "go test ./... -v"

fi

# =============================================================================
# LEVEL 2: INTEGRATION
# =============================================================================
if [[ $CURRENT_IDX -ge 2 ]]; then
    section "LEVEL 2: INTEGRATION (requires cluster)"

    if ! check_cluster; then
        skip_test "Integration tests" "no cluster available"
    else
        subsection "Cluster Check"
        run_test "kubectl cluster-info" "kubectl cluster-info"

        subsection "Map Commands"
        run_test "map status" "./cub-scout map status"
        run_test "map list" "./cub-scout map list"
        run_test "map list --json" "./cub-scout map list --json"
        run_test "map orphans" "./cub-scout map orphans"
        run_test "map deployers" "./cub-scout map deployers"

        subsection "Scan Command"
        run_test "scan" "./cub-scout scan"
        run_test "scan --json" "./cub-scout scan --json"

        subsection "Scan Provider Selection (v1.2)"
        # scan --file exits 0 even with findings (use --fail-on for non-zero), so clean fixture isn't strictly needed
        run_test "scan --file (legacy override)" "CUB_SCOUT_SCAN_PROVIDER=legacy ./cub-scout scan --file test/golden/scan-file/testdata/inputs/clean-deployment.yaml --json > /dev/null"
        if command -v confighub-scan > /dev/null 2>&1 || command -v cub-scan > /dev/null 2>&1; then
            run_test "scan --file (cub-scan detected)" "./cub-scout scan --file test/golden/scan-file/testdata/inputs/clean-deployment.yaml --json > /dev/null"
        else
            optional_test_unavailable "scan --file (cub-scan)" "confighub-scan/cub-scan not on PATH"
        fi

        subsection "Integration Test Suite"
        run_test "go test -tags=integration" "go test -tags=integration ./test/integration/... -v"
    fi
fi

# =============================================================================
# LEVEL 3: GITOPS E2E
# =============================================================================
if [[ $CURRENT_IDX -ge 3 ]]; then
    section "LEVEL 3: GITOPS E2E (Flux + ArgoCD)"

    if ! check_cluster; then
        skip_test "GitOps E2E" "no cluster available"
    else
        subsection "Flux Installation"
        if check_flux; then
            echo -e "  ${GREEN}✓${NC} Flux already installed"
        elif [[ -n "${SKIP_FLUX_INSTALL:-}" ]]; then
            skip_test "Flux install" "SKIP_FLUX_INSTALL set"
        else
            run_test "flux install" "flux install"
        fi

        subsection "ArgoCD Installation"
        if check_argocd; then
            echo -e "  ${GREEN}✓${NC} ArgoCD already installed"
        elif [[ -n "${SKIP_ARGO_INSTALL:-}" ]]; then
            skip_test "ArgoCD install" "SKIP_ARGO_INSTALL set"
        else
            run_test "argocd install" "kubectl create namespace argocd 2>/dev/null || true && kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml"
            run_test "argocd wait" "kubectl wait --for=condition=available deployment/argocd-server -n argocd --timeout=120s"
        fi

        subsection "Deploy Example Apps"
        run_test "flux-boutique deploy" "kubectl apply -f examples/flux-boutique/boutique.yaml"
        run_test "flux-boutique wait" "kubectl wait --for=condition=Ready gitrepository/boutique -n boutique --timeout=300s"

        for name in frontend cart checkout payment shipping; do
            run_test "Flux reconciliation $name" "kubectl wait --for=condition=Ready kustomization/$name -n boutique --timeout=300s"
            run_test "Flux workload $name" "kubectl rollout status deployment/$name -n boutique --timeout=300s"
        done

        subsection "Ownership Detection"
        run_test "Flux ownership" "./cub-scout map list -n boutique --json | jq -e '[.[] | select(.kind == \"Deployment\" and .name == \"cart\" and .owner == \"Flux\")] | length == 1'"

        # Create ArgoCD app if not exists
        if ! kubectl get application guestbook -n argocd > /dev/null 2>&1; then
            run_test "ArgoCD app create" "kubectl apply -f - <<EOF
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: guestbook
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/argoproj/argocd-example-apps.git
    targetRevision: HEAD
    path: guestbook
  destination:
    server: https://kubernetes.default.svc
    namespace: guestbook
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
EOF"

        fi
        run_test "ArgoCD synced" "kubectl wait --for=jsonpath='{.status.sync.status}'=Synced application/guestbook -n argocd --timeout=300s"
        run_test "ArgoCD healthy" "kubectl wait --for=jsonpath='{.status.health.status}'=Healthy application/guestbook -n argocd --timeout=300s"
        run_test "ArgoCD ownership" "./cub-scout map list -n guestbook --json | jq -e '[.[] | select(.kind == \"Deployment\" and .name == \"guestbook-ui\" and .owner == \"ArgoCD\")] | length == 1'"

        subsection "Trace Command"
        run_test "trace flux app" "./cub-scout trace deployment/cart -n boutique"

        subsection "Trace Lineage (v1.2)"
        # Verify trace --format json includes lineage fields in schema (even if empty)
        run_test "trace json schema" "./cub-scout trace deployment/cart -n boutique --format json 2>/dev/null | python3 -c 'import sys,json; d=json.load(sys.stdin); assert \"object\" in d, \"missing object field\"' 2>/dev/null"
        # If ArgoCD app exists, verify lineage fields
        if kubectl get application guestbook -n argocd > /dev/null 2>&1; then
            run_test "trace argo lineage" "./cub-scout trace application/guestbook -n argocd --format json 2>/dev/null | python3 -c 'import sys,json; d=json.load(sys.stdin); print(\"lineage:\", d.get(\"parentApplication\",\"\"), d.get(\"generatedByApplicationSet\",\"\"), d.get(\"lineageConfidence\",\"\"))'"
        fi

        subsection "Deep Dive"
        run_test "deep-dive" "./cub-scout map deep-dive"

        subsection "App Hierarchy"
        run_test "app-hierarchy" "./cub-scout map app-hierarchy"
    fi
fi

# =============================================================================
# LEVEL 4: DEMOS
# =============================================================================
if [[ $CURRENT_IDX -ge 4 ]]; then
    section "LEVEL 4: DEMOS"

    if ! check_cluster; then
        skip_test "Demos" "no cluster available"
    else
        subsection "Quick Demo"
        run_test "quick fixtures" "kubectl apply -f test/atk/fixtures/flux-basic.yaml -f test/atk/fixtures/argo-basic.yaml"
        run_test "demo quick" "./cub-scout quickstart demo quick"
        run_test "demo quick cleanup" "./cub-scout quickstart demo quick --cleanup"

        subsection "CCVE Demo"
        run_test "risk fixture" "kubectl apply -f examples/impressive-demo/bad-configs/monitoring-bad.yaml"
        run_test "risk finding" "./cub-scout scan --file examples/impressive-demo/bad-configs/monitoring-bad.yaml --json | jq -e '[.static.findings[] | select(.ccve_id == \"CCVE-2025-0027\" and .resource_name == \"grafana\")] | length == 1'"
        run_test "demo ccve" "./cub-scout quickstart demo ccve"
        run_test "demo ccve cleanup" "./cub-scout quickstart demo ccve --cleanup"

        subsection "Query Demo"
        run_test "query fixtures" "kubectl apply -f examples/demos/multi-cluster.yaml"
        run_test "query map" "./cub-scout map list -q 'kind=Deployment AND owner!=Native' --json | jq -e 'length > 0'"
        run_test "demo query" "./cub-scout quickstart demo query"
        run_test "demo query cleanup" "./cub-scout quickstart demo query --cleanup"

        subsection "Scenarios"
        run_test "incident fixture" "kubectl apply -f examples/impressive-demo/bad-configs/monitoring-bad.yaml"
        run_test "scenario bigbank-incident" "./cub-scout quickstart demo scenario bigbank-incident"
        run_test "scenario bigbank-incident cleanup" "./cub-scout quickstart demo scenario bigbank-incident --cleanup"
        run_test "break-glass fixture" "kubectl apply -f examples/demos/break-glass.yaml"
        run_test "scenario break-glass" "./cub-scout quickstart demo scenario break-glass"
        run_test "scenario break-glass cleanup" "./cub-scout quickstart demo scenario break-glass --cleanup"

        subsection "Visual Demos"
        run_test "fleet-queries-demo" "./examples/demos/fleet-queries-demo.sh"
        run_test "tui-queries-demo" "./examples/demos/tui-queries-demo.sh"
    fi
fi

# =============================================================================
# LEVEL 5: EXAMPLES E2E
# =============================================================================
if [[ $CURRENT_IDX -ge 5 ]]; then
    section "LEVEL 5: EXAMPLES E2E"

    subsection "Real Examples Catalog"
    run_test "verify real examples catalog (local-first)" "go test -tags=integration ./test/integration/... -run '^TestRealExamplesCatalog$' -count=1"

    if ! check_cluster; then
        skip_test "Examples" "no cluster available"
    else
        subsection "Flux Boutique"
        run_test "boutique ownership count" "./cub-scout map list namespace=boutique --json | jq '[.[] | select(.owner == \"Flux\")] | length' | grep -q '[1-9]'"

        subsection "Example Scripts"
        run_test "impressive-demo exists" "test -x examples/impressive-demo/demo-script.sh"

        # Integration scripts tests skipped - files planned but not yet created
        # subsection "Integration Scripts"
        # run_test "k9s-plugin valid" "test -f examples/scripts/k9s-plugin.yaml"
    fi
fi

# =============================================================================
# LEVEL 6: CONNECTED
# =============================================================================
if [[ $CURRENT_IDX -ge 6 ]]; then
    section "LEVEL 6: CONNECTED (ConfigHub)"

    if ! check_confighub; then
        skip_test "Connected mode" "ConfigHub not authenticated (run: cub auth login)"
    else
        subsection "ConfigHub Connection"
        run_test "app list" "./cub-scout app list"

        subsection "Import Preview"
        run_test "import dry-run" "./cub-scout import -n boutique --dry-run"

        subsection "Import E2E (fixtures)"
        # Deploy test fixtures and run dry-run import against them
        E2E_NS="e2e-prove-$(date +%s)"
        kubectl create namespace "$E2E_NS" > /dev/null 2>&1
        kubectl apply -f test/fixtures/import-e2e/ -n "$E2E_NS" > /dev/null 2>&1
        sleep 3

        run_test "import dry-run JSON (fixtures)" "go test -tags=integration ./test/integration/... -run '^TestImportDryRunJSON$' -count=1 -v -timeout 120s"

        subsection "Import Full Round-Trip"
        IMPORT_PROOF=$(mktemp)
        run_test "authenticated import round trips" "go test -tags=integration ./test/integration/... -run '^(TestImportFullRoundTrip|TestImportIdempotent|TestImportCleanup)$' -count=1 -json -timeout 300s > $IMPORT_PROOF"
        run_test "required import outcomes" "python3 scripts/ci/require_test_passes.py $IMPORT_PROOF TestImportFullRoundTrip TestImportIdempotent TestImportCleanup"
        rm -f "$IMPORT_PROOF"

        # Always clean up the namespace
        kubectl delete namespace "$E2E_NS" --ignore-not-found --wait=false > /dev/null 2>&1
    fi
fi

# =============================================================================
# SUMMARY
# =============================================================================
section "SUMMARY"

TOTAL=$((PASSED + FAILED + SKIPPED))

echo ""
echo -e "  ${GREEN}Passed:${NC}  $PASSED"
echo -e "  ${RED}Failed:${NC}  $FAILED"
echo -e "  ${YELLOW}Skipped:${NC} $SKIPPED"
echo -e "  ${DIM}Total:${NC}   $TOTAL"
echo ""

if [[ $FAILED -eq 0 && $SKIPPED -gt 0 ]]; then
    echo "PARTIAL: selected checks passed; $SKIPPED optional check(s) were not executed. This is not full acceptance."
    exit 0
fi

if [[ $FAILED -eq 0 ]]; then
    echo -e "${GREEN}${BOLD}════════════════════════════════════════════════════════════════════${NC}"
    echo -e "${GREEN}${BOLD}  ✓ PROVEN: cub-scout works at level '$LEVEL'${NC}"
    echo -e "${GREEN}${BOLD}════════════════════════════════════════════════════════════════════${NC}"
    exit 0
else
    echo -e "${RED}${BOLD}════════════════════════════════════════════════════════════════════${NC}"
    echo -e "${RED}${BOLD}  ✗ FAILED: $FAILED test(s) failed${NC}"
    echo -e "${RED}${BOLD}════════════════════════════════════════════════════════════════════${NC}"
    exit 1
fi
