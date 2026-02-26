#!/bin/bash
# demo.sh — End-to-end import demo with a kind cluster
#
# Creates a kind cluster, populates it with the "Arnie" pattern fixtures
# (ArgoCD + Helm + Native), and runs cub-scout import.
#
# Usage:
#   ./demo.sh                # Dry-run only (no ConfigHub changes), teardown after
#   ./demo.sh --keep         # Keep cluster running after demo
#   ./demo.sh --live         # Actually import into ConfigHub (requires cub auth)
#   ./demo.sh --live --keep  # Import + keep cluster
#
# Prerequisites: kind, kubectl, docker (running)
#                --live also requires: cub CLI authenticated (cub auth login)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
FIXTURES="$SCRIPT_DIR/fixtures"
CLUSTER_NAME="import-demo"
KEEP=false
LIVE=false

for arg in "$@"; do
    case "$arg" in
        --keep) KEEP=true ;;
        --live) LIVE=true ;;
    esac
done

# Colors
CYAN='\033[0;36m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
DIM='\033[0;90m'
BOLD='\033[1m'
NC='\033[0m'

banner()  { echo -e "\n${CYAN}${BOLD}═══ $1 ═══${NC}\n"; }
step()    { echo -e "${GREEN}▸${NC} $1"; }
note()    { echo -e "${DIM}  $1${NC}"; }
warn()    { echo -e "${YELLOW}⚠${NC} $1"; }

cleanup() {
    if $KEEP; then
        warn "Cluster '$CLUSTER_NAME' kept running. Delete with: kind delete cluster --name $CLUSTER_NAME"
    else
        step "Tearing down cluster..."
        kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || true
    fi
}
trap cleanup EXIT

# --- Preflight ---
banner "Import Demo — Preflight"

for tool in kind kubectl docker; do
    command -v "$tool" >/dev/null 2>&1 || { echo "Missing: $tool"; exit 1; }
done

if ! docker info >/dev/null 2>&1; then
    echo "Docker is not running"; exit 1
fi

if $LIVE; then
    command -v cub >/dev/null 2>&1 || { echo "Missing: cub (required for --live). Install from https://confighub.com/docs/cli"; exit 1; }
    if ! cub auth get-token >/dev/null 2>&1; then
        echo "ConfigHub auth required for --live. Run: cub auth login"; exit 1
    fi
    step "ConfigHub auth OK"
fi

# Build cub-scout
step "Building cub-scout..."
(cd "$REPO_ROOT" && go build -o cub-scout ./cmd/cub-scout)
CUB="$REPO_ROOT/cub-scout"

# --- Cluster ---
banner "Creating Kind Cluster"

# Delete stale cluster if it exists
kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || true

kind create cluster --name "$CLUSTER_NAME" --wait 30s
step "Cluster ready"
echo ""

# --- ArgoCD CRD ---
# We only need the Application CRD (not the full ArgoCD install) so
# cub-scout can discover Application objects and read spec.source.path.
banner "Installing ArgoCD Application CRD"

kubectl apply -f - <<'CRDEOF'
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: applications.argoproj.io
spec:
  group: argoproj.io
  names:
    kind: Application
    listKind: ApplicationList
    plural: applications
    singular: application
    shortNames:
      - app
  scope: Namespaced
  versions:
    - name: v1alpha1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          x-kubernetes-preserve-unknown-fields: true
CRDEOF
step "ArgoCD CRD installed"

# --- Namespaces ---
banner "Creating Namespaces"

for ns in argocd myapp-dev myapp-staging myapp-prod; do
    kubectl create namespace "$ns" 2>/dev/null || true
done
step "Namespaces: argocd, myapp-dev, myapp-staging, myapp-prod"

# --- Fixtures ---
banner "Applying Fixtures"

kubectl apply -f "$FIXTURES/applications.yaml"
note "3 ArgoCD Applications (myapp-dev, myapp-staging, myapp-prod)"

kubectl apply -f "$FIXTURES/deployments.yaml"
note "6 Deployments (api + worker × 3 envs)"

kubectl apply -f "$FIXTURES/statefulsets.yaml"
note "3 StatefulSets (redis × 3 envs)"

kubectl apply -f "$FIXTURES/native.yaml"
note "1 ConfigMap (debug-config, unmanaged)"

echo ""
step "Cluster populated: 13 resources across 4 namespaces"

# Wait for deployments to be available
step "Waiting for workloads to be ready..."
kubectl wait --for=condition=available deployment --all -n myapp-dev --timeout=60s >/dev/null 2>&1 || true
kubectl wait --for=condition=available deployment --all -n myapp-staging --timeout=60s >/dev/null 2>&1 || true
kubectl wait --for=condition=available deployment --all -n myapp-prod --timeout=60s >/dev/null 2>&1 || true

# --- Discovery ---
banner "Running: cub-scout import --dry-run"
note "Read-only. No changes to the cluster or ConfigHub."
echo ""

"$CUB" import --dry-run

# --- JSON ---
banner "Running: cub-scout import --dry-run --json"
note "Machine-readable proposal for scripting/GUI integration."
echo ""

"$CUB" import --dry-run --json 2>/dev/null | python3 -m json.tool 2>/dev/null || "$CUB" import --dry-run --json

# --- Live Import ---
if $LIVE; then
    banner "Running: cub-scout import --yes (LIVE)"
    note "Creating App + Units in ConfigHub..."
    echo ""

    "$CUB" import --yes
    step "Import complete — resources created in ConfigHub"
fi

# --- Map ---
banner "Bonus: cub-scout map list"
note "How the TUI sees the same cluster."
echo ""

"$CUB" map list 2>/dev/null || true

# --- Done ---
banner "Demo Complete"

if $KEEP; then
    echo -e "Cluster '${BOLD}$CLUSTER_NAME${NC}' is still running."
    echo ""
    echo "  Try:"
    echo "    $CUB import --wizard        # Interactive TUI wizard"
    echo "    $CUB map                    # Interactive TUI map"
    echo "    $CUB scan                   # Security scan"
    echo "    $CUB trace deploy/api -n myapp-prod  # Trace ownership"
    echo ""
    echo "  Teardown:"
    echo "    kind delete cluster --name $CLUSTER_NAME"
else
    echo "Cluster torn down. Run with --keep to explore interactively."
fi
