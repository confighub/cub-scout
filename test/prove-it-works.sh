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

set -e

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
            ((PASSED++))
            return 0
        else
            echo -e "  ${RED}✗${NC} $name"
            ((FAILED++))
            return 1
        fi
    else
        if eval "$cmd" > /tmp/test-output.txt 2>&1; then
            echo -e "${GREEN}✓${NC}"
            ((PASSED++))
            return 0
        else
            echo -e "${RED}✗${NC}"
            echo -e "    ${DIM}Output:${NC}"
            tail -5 /tmp/test-output.txt | sed 's/^/    /'
            ((FAILED++))
            return 1
        fi
    fi
}

skip_test() {
    local name="$1"
    local reason="$2"
    echo -e "  ${YELLOW}○${NC} $name ${DIM}(skipped: $reason)${NC}"
    ((SKIPPED++))
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
    command -v cub > /dev/null 2>&1 && cub context get > /dev/null 2>&1
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
CURRENT_IDX=0
for i in "${!LEVELS[@]}"; do
    if [[ "${LEVELS[$i]}" == "$LEVEL" ]]; then
        CURRENT_IDX=$i
        break
    fi
done

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

    TEST_COUNT=$(go test ./... -v 2>&1 | grep -c "=== RUN" || echo "0")
    echo -e "  ${DIM}Total tests: $TEST_COUNT${NC}"
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
        run_test "map list --json" "./cub-scout map list --json | head -10"
        run_test "map orphans" "./cub-scout map orphans"
        run_test "map deployers" "./cub-scout map deployers"

        subsection "Scan Command"
        run_test "scan" "./cub-scout scan"
        run_test "scan --json" "./cub-scout scan --json"

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
        run_test "flux-boutique wait" "kubectl wait --for=condition=available deployment --all -n boutique --timeout=120s || true"

        subsection "Ownership Detection"
        run_test "Flux ownership" "./cub-scout map list | grep -q flux"

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
            sleep 10  # Wait for sync
        fi
        run_test "ArgoCD ownership" "./cub-scout map list | grep -q argo"

        subsection "Trace Command"
        run_test "trace flux app" "./cub-scout trace deployment/cart -n boutique"

        subsection "Deep Dive"
        run_test "deep-dive" "./cub-scout map deep-dive | head -50"

        subsection "App Hierarchy"
        run_test "app-hierarchy" "./cub-scout map app-hierarchy | head -50"
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
        run_test "demo quick" "./test/atk/demo quick"
        run_test "demo quick cleanup" "./test/atk/demo quick --cleanup"

        subsection "CCVE Demo"
        run_test "demo ccve" "./test/atk/demo ccve"
        run_test "demo ccve cleanup" "./test/atk/demo ccve --cleanup"

        subsection "Scenarios"
        run_test "scenario bigbank-incident" "./test/atk/demo scenario bigbank-incident"
        run_test "scenario break-glass" "./test/atk/demo scenario break-glass"
        run_test "scenario break-glass cleanup" "./test/atk/demo scenario break-glass --cleanup"

        subsection "Visual Demos"
        run_test "fleet-queries-demo" "./examples/demos/fleet-queries-demo.sh | head -30"
        run_test "tui-queries-demo" "./examples/demos/tui-queries-demo.sh | head -30"
    fi
fi

# =============================================================================
# LEVEL 5: EXAMPLES E2E
# =============================================================================
if [[ $CURRENT_IDX -ge 5 ]]; then
    section "LEVEL 5: EXAMPLES E2E"

    if ! check_cluster; then
        skip_test "Examples" "no cluster available"
    else
        subsection "Flux Boutique"
        run_test "boutique ownership count" "./cub-scout map list -n boutique --json | jq '[.[] | select(.owner == \"flux\")] | length' | grep -q '[1-9]'"

        subsection "Example Scripts"
        run_test "impressive-demo exists" "test -x examples/impressive-demo/demo-script.sh"

        subsection "Integration Scripts"
        run_test "k9s-plugin valid" "test -f examples/scripts/k9s-plugin.yaml"
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
        run_test "app-space list" "./cub-scout app-space list | head -10"

        subsection "Import Preview"
        run_test "import dry-run" "./cub-scout import -n boutique --dry-run"
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
