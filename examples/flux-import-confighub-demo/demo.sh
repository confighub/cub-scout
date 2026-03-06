#!/bin/bash
# demo.sh - Flux import demo: Management and Discovery with D2 pattern
#
# Creates a kind cluster with real Flux, populates it with two fixture sets
# (real reconciled podinfo + label-only D2 brownfield), then shows how
# cub gitops import (management) and cub-scout import (discovery) complement
# each other for Flux-managed clusters.
#
# Usage:
#   ./demo.sh                # Observe only (dry-run, ~5 min)
#   ./demo.sh --live         # Full import into ConfigHub (~6 min, requires cub auth)
#   ./demo.sh --keep         # Keep cluster running for exploration
#   ./demo.sh --live --keep  # Import + keep cluster
#   ./demo.sh --live --seed-history  # Also seed tagged synthetic ChangeSet history
#   ./demo.sh --live --confighub-url=http://localhost:9090  # Local dev override
#
# Prerequisites: docker, kind, kubectl, go, flux
#                --live also requires: cub (cub auth login)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
D2_FIXTURES="$SCRIPT_DIR/fixtures"
CLUSTER_NAME="flux-import-demo"
KEEP=false
LIVE=false
SEED_HISTORY=false
CONFIGHUB_URL_OVERRIDE=""
WORKER_PID=""
WORKER_LIFECYCLE_SCRIPT="$SCRIPT_DIR/../scripts/demo-worker-lifecycle.sh"
VERIFY_CONNECTED_SCRIPT="$SCRIPT_DIR/../scripts/verify-connected-demo.sh"
SEED_HISTORY_SCRIPT="$SCRIPT_DIR/../scripts/seed-connected-demo-history.sh"
DISCOVERY_WORKER_PID_FILE=""
DISCOVERY_WORKER_LOG=""

for arg in "$@"; do
    case "$arg" in
        --keep) KEEP=true ;;
        --live) LIVE=true ;;
        --seed-history) SEED_HISTORY=true ;;
        --confighub-url=*) CONFIGHUB_URL_OVERRIDE="${arg#*=}" ;;
    esac
done

# Colors
CYAN='\033[0;36m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
DIM='\033[0;90m'
BOLD='\033[1m'
NC='\033[0m'

banner()  { echo -e "\n${CYAN}${BOLD}=== $1 ===${NC}\n"; }
step()    { echo -e "${GREEN}>>${NC} $1"; }
note()    { echo -e "${DIM}   $1${NC}"; }
warn()    { echo -e "${YELLOW}!!${NC} $1"; }
fail()    { echo -e "${RED}!!${NC} $1"; }

cleanup() {
    if [[ -n "$DISCOVERY_WORKER_PID_FILE" ]]; then
        if $KEEP; then
            bash "$WORKER_LIFECYCLE_SCRIPT" cleanup --pid-file "$DISCOVERY_WORKER_PID_FILE" --keep >/dev/null 2>&1 || true
            step "Discovery worker left running"
            if [[ -n "$DISCOVERY_WORKER_LOG" ]]; then
                note "Discovery worker log: $DISCOVERY_WORKER_LOG"
            fi
        else
            bash "$WORKER_LIFECYCLE_SCRIPT" cleanup --pid-file "$DISCOVERY_WORKER_PID_FILE" >/dev/null 2>&1 || true
            step "Discovery worker stopped"
        fi
    fi
    if $KEEP; then
        warn "Cluster '$CLUSTER_NAME' kept running. Delete with: kind delete cluster --name $CLUSTER_NAME"
    else
        step "Tearing down cluster..."
        kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || true
    fi
}
trap cleanup EXIT

# ============================================================
# PREFLIGHT
# ============================================================
banner "Preflight"

for tool in kind kubectl docker go flux; do
    if ! command -v "$tool" >/dev/null 2>&1; then
        fail "Missing: $tool"
        exit 1
    fi
done

if ! docker info >/dev/null 2>&1; then
    fail "Docker is not running"
    exit 1
fi

if $LIVE; then
    LIVE_MISSING=""
    for tool in cub; do
        command -v "$tool" >/dev/null 2>&1 || LIVE_MISSING="$LIVE_MISSING $tool"
    done
    if [[ -n "$LIVE_MISSING" ]]; then
        fail "Missing tools required for --live:$LIVE_MISSING"
        exit 1
    fi
    if ! cub auth get-token >/dev/null 2>&1; then
        fail "ConfigHub auth required for --live. Run: cub auth login"
        exit 1
    fi

    # Resolve ConfigHub URL for in-cluster renderer worker
    if [[ -n "$CONFIGHUB_URL_OVERRIDE" ]]; then
        CONFIGHUB_SERVER_URL="$CONFIGHUB_URL_OVERRIDE"
    elif [[ -n "${CONFIGHUB_URL:-}" ]]; then
        CONFIGHUB_SERVER_URL="$CONFIGHUB_URL"
    else
        CONFIGHUB_SERVER_URL=$(cub info 2>/dev/null | grep "Server URL:" | awk '{print $NF}')
    fi
    if [[ -z "$CONFIGHUB_SERVER_URL" ]]; then
        fail "Could not determine ConfigHub URL. Set CONFIGHUB_URL or use --confighub-url=<url>"
        exit 1
    fi
    for helper in "$WORKER_LIFECYCLE_SCRIPT" "$VERIFY_CONNECTED_SCRIPT" "$SEED_HISTORY_SCRIPT"; do
        if [[ ! -f "$helper" ]]; then
            fail "Missing helper script: $helper"
            exit 1
        fi
    done
    step "ConfigHub: $CONFIGHUB_SERVER_URL"
    step "Auth OK (cub present)"
fi

step "Building cub-scout..."
(cd "$REPO_ROOT" && go build -o cub-scout ./cmd/cub-scout)
CUB="$REPO_ROOT/cub-scout"
step "Build OK"

# ============================================================
# ACT 1 - THE CLUSTER
# ============================================================
banner "Act 1: The Cluster"
note "Creating kind cluster with real Flux + D2 brownfield fixtures + podinfo"
echo ""

# Delete stale cluster
kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || true

step "Creating kind cluster..."
kind create cluster --name "$CLUSTER_NAME" --wait 60s
step "Cluster ready: kind-$CLUSTER_NAME"
echo ""

# --- Install real Flux ---
step "Installing Flux (this takes 1-2 minutes)..."
flux install --timeout=5m
step "Flux installed"
echo ""

# --- Wait for Flux CRDs ---
step "Waiting for Flux CRDs..."
if ! kubectl wait --for=condition=Established --timeout=120s \
    crd/gitrepositories.source.toolkit.fluxcd.io \
    crd/kustomizations.kustomize.toolkit.fluxcd.io \
    crd/helmreleases.helm.toolkit.fluxcd.io >/dev/null 2>&1; then
    fail "Flux CRDs not established in time"
    exit 1
fi

# --- Apply D2 brownfield fixtures ---
step "Applying D2 brownfield fixtures (Control Plane pattern)..."
D2_APPLIED=false
for i in $(seq 1 5); do
    if kubectl apply -f "$D2_FIXTURES/d2-brownfield.yaml"; then
        D2_APPLIED=true
        break
    fi
    warn "D2 fixture apply failed (attempt $i/5), retrying in 5s..."
    sleep 5
done

if ! $D2_APPLIED; then
    fail "Could not apply D2 fixtures"
    exit 1
fi
echo ""
note "Flux CRs: GitRepository, 2 Kustomizations (infrastructure + apps)"
note "          2 per-app Kustomizations (payment-api, frontend)"
note "          2 HelmReleases (cert-manager, monitoring)"
note "Infrastructure: cert-manager, prometheus, grafana (Helm-via-Flux labels)"
note "Applications:   payment-api, frontend (Kustomization labels)"
note "Helm-only:      redis (no Flux labels)"
note "Native:         debug-config (no labels)"
echo ""

# --- Apply real podinfo Kustomization (Flux will reconcile this) ---
step "Applying podinfo Kustomization (real Flux reconciliation)..."
kubectl apply -f "$D2_FIXTURES/podinfo-kustomizations.yaml"
note "GitRepository + Kustomization pointing at stefanprodan/podinfo"
note "Flux will pull the source and create real Deployments/Services"
echo ""

# --- Wait for podinfo sync ---
step "Waiting for podinfo to sync (up to 120s)..."
PODINFO_SYNCED=false
for i in $(seq 1 24); do
    if kubectl wait --for=condition=available --timeout=5s deployment/podinfo -n podinfo >/dev/null 2>&1 \
        && kubectl wait --for=condition=Ready --timeout=5s kustomization/podinfo -n flux-system >/dev/null 2>&1; then
        PODINFO_SYNCED=true
        break
    fi
    printf "."
    sleep 5
done
echo ""

if $PODINFO_SYNCED; then
    step "Podinfo synced - Flux created real workloads"
    kubectl get deployments -n podinfo 2>/dev/null || true
else
    warn "Podinfo hasn't synced yet (Flux may still be pulling)"
    note "This is OK - the demo still shows the contrast"
fi
echo ""

# --- Wait for brownfield deployments ---
step "Waiting for brownfield workloads..."
kubectl wait --for=condition=available deployment --all -n payments --timeout=60s >/dev/null 2>&1 || true
kubectl wait --for=condition=available deployment --all -n store --timeout=60s >/dev/null 2>&1 || true
kubectl wait --for=condition=available deployment --all -n cert-manager --timeout=60s >/dev/null 2>&1 || true
kubectl wait --for=condition=available deployment --all -n monitoring --timeout=60s >/dev/null 2>&1 || true

# --- Summary ---
echo ""
step "Cluster populated:"
note "Podinfo = real Flux-reconciled (GitRepository -> Kustomization -> Deployment)"
note "D2 infrastructure = cert-manager, prometheus, grafana (Helm-via-Flux labels)"
note "D2 applications = payment-api, frontend (Kustomization labels)"
note "Redis = Helm-only (invisible to Flux-specific tools)"
note "debug-config = Native (invisible to everything except cub-scout)"

# ============================================================
# ACT 2 - THE OBSERVER: cub-scout
# ============================================================
banner "Act 2: The Observer (cub-scout)"
note "cub-scout sees EVERYTHING: all workloads, all ownership types"
echo ""

step "cub-scout map list  (full resource inventory)"
echo ""
"$CUB" map list 2>/dev/null || true
echo ""

step "cub-scout gitops status  (Flux health + sync status)"
echo ""
"$CUB" gitops status 2>/dev/null || true
echo ""

step "cub-scout import --dry-run  (workload grouping proposal)"
echo ""
"$CUB" import --dry-run 2>/dev/null || true
echo ""

note "cub-scout found: Flux workloads + Helm redis + Native debug-config"
note "It groups by ownership labels, not by what the controller actually reconciled"

# ============================================================
# ACT 3 - THE FLUX VIEW: trace + patterns
# ============================================================
banner "Act 3: The Flux View (trace + patterns)"
note "No import-flux command exists — instead, cub-scout reveals Flux-specific"
note "structure: ownership chains, D2 pattern detection, trace from workload to Git"
echo ""

step "cub-scout tree ownership  (Flux Kustomization chains)"
echo ""
"$CUB" tree ownership 2>/dev/null || true
echo ""

step "cub-scout tree patterns  (D2/Control Plane pattern detection)"
echo ""
"$CUB" tree patterns 2>/dev/null || true
echo ""

step "cub-scout trace deploy/payment-api -n payments  (Kustomization chain)"
echo ""
"$CUB" trace deploy/payment-api -n payments 2>/dev/null || true
echo ""

step "cub-scout trace deploy/cert-manager -n cert-manager  (Helm-via-Flux chain)"
echo ""
"$CUB" trace deploy/cert-manager -n cert-manager 2>/dev/null || true
echo ""

note "tree ownership shows infrastructure vs application layers (D2 separation)"
note "tree patterns auto-detects the Control Plane reference architecture"
note "trace follows the full chain: Deployment -> Kustomization/HelmRelease -> GitRepository"

# ============================================================
# ACT 4 - THE PIPELINE: cub gitops import
# ============================================================
if $LIVE; then
    banner "Act 4: The Pipeline (cub gitops import)"
    note "The production import path: rendered manifests with auto-updating dry/wet pairs"
    echo ""

    SPACE="flux-import-demo"
    DISCOVERY_WORKER_SLUG="discovery-worker"
    RENDERER_WORKER_SLUG="flux-renderer-worker"
    WORKER_STATE_DIR="${TMPDIR:-/tmp}/cub-scout-demo-workers"
    mkdir -p "$WORKER_STATE_DIR"
    DISCOVERY_WORKER_PID_FILE="$WORKER_STATE_DIR/${SPACE}-${DISCOVERY_WORKER_SLUG}.pid"
    DISCOVERY_WORKER_LOG="$WORKER_STATE_DIR/${SPACE}-${DISCOVERY_WORKER_SLUG}.log"
    step "Creating ConfigHub space '$SPACE'..."
    cub space create "$SPACE" 2>/dev/null || warn "Space may already exist"
    echo ""

    # Start discovery worker with persistent lifecycle handling
    step "Starting discovery worker..."
    if ! bash "$WORKER_LIFECYCLE_SCRIPT" start \
        --space "$SPACE" \
        --worker "$DISCOVERY_WORKER_SLUG" \
        --target Kubernetes \
        --pid-file "$DISCOVERY_WORKER_PID_FILE" \
        --log-file "$DISCOVERY_WORKER_LOG"; then
        warn "Discovery worker start failed. Log: $DISCOVERY_WORKER_LOG"
        tail -n 20 "$DISCOVERY_WORKER_LOG" 2>/dev/null || true
    fi
    if [[ -f "$DISCOVERY_WORKER_PID_FILE" ]]; then
        WORKER_PID="$(cat "$DISCOVERY_WORKER_PID_FILE" 2>/dev/null || true)"
        note "Discovery worker PID: ${WORKER_PID:-unknown}"
    fi

    # Deploy kustomize renderer in-cluster
    step "Deploying kustomize renderer worker in-cluster..."

    # For public URLs the in-cluster worker can reach them directly.
    # For local dev (localhost:*), rewrite to docker gateway IP.
    RENDERER_CONFIGHUB_URL="$CONFIGHUB_SERVER_URL"
    if echo "$CONFIGHUB_SERVER_URL" | grep -qE "localhost|127\.0\.0\.1"; then
        DOCKER_HOST_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.Gateway}}{{end}}' "${CLUSTER_NAME}-control-plane" 2>/dev/null | head -1)
        if [ -z "$DOCKER_HOST_IP" ]; then
            DOCKER_HOST_IP="host.docker.internal"
        fi
        PORT=$(echo "$CONFIGHUB_SERVER_URL" | grep -oE ':[0-9]+$' | tr -d ':')
        PORT="${PORT:-9090}"
        RENDERER_CONFIGHUB_URL="http://${DOCKER_HOST_IP}:${PORT}"
        note "Local dev detected, renderer will use: $RENDERER_CONFIGHUB_URL"
    fi

    step "Recreating renderer worker '$RENDERER_WORKER_SLUG'..."
    cub worker delete --space "$SPACE" "$RENDERER_WORKER_SLUG" >/dev/null 2>&1 || true
    cub worker create --space "$SPACE" "$RENDERER_WORKER_SLUG" >/dev/null

    WORKER_MANIFEST=$(mktemp)
    cub worker install --space "$SPACE" \
        --export \
        --include-secret \
        -t fluxrenderer \
        -n confighub-worker \
        --image-pull-policy IfNotPresent \
        -e "CONFIGHUB_URL=$RENDERER_CONFIGHUB_URL" \
        "$RENDERER_WORKER_SLUG" > "$WORKER_MANIFEST"

    kubectl create namespace confighub-worker 2>/dev/null || true
    kubectl apply -f "$WORKER_MANIFEST"
    rm -f "$WORKER_MANIFEST"

    step "Waiting for renderer deployment..."
    kubectl -n confighub-worker wait --for=condition=available \
        "deployment/$RENDERER_WORKER_SLUG" --timeout=120s 2>/dev/null || warn "Renderer not ready"

    # Wait for targets
    step "Waiting for targets to register..."
    TARGETS_READY=false
    for i in $(seq 1 60); do
        TARGET_OUTPUT=$(cub target list --space "$SPACE" 2>/dev/null || true)
        if echo "$TARGET_OUTPUT" | grep -q "kubernetes" && echo "$TARGET_OUTPUT" | grep -q "fluxrenderer"; then
            TARGETS_READY=true
            break
        fi
        printf "."
        sleep 2
    done
    echo ""

    if ! $TARGETS_READY; then
        warn "Targets not registered - cub gitops import may fail"
        cub target list --space "$SPACE" 2>/dev/null || true
        kubectl -n confighub-worker get pods 2>/dev/null || true
        kubectl -n confighub-worker logs "deployment/$RENDERER_WORKER_SLUG" --tail=40 2>/dev/null || true
    else
        step "Targets registered"
    fi

    # Extract target slugs
    TARGET_OUTPUT=$(cub target list --space "$SPACE" 2>/dev/null || true)
    K8S_TARGET=$(echo "$TARGET_OUTPUT" | awk '/kubernetes/ {print $1; exit}' || true)
    RENDERER_TARGET=$(echo "$TARGET_OUTPUT" | awk '/fluxrenderer/ {print $1; exit}' || true)

    if [[ -n "$K8S_TARGET" ]]; then
        step "cub gitops discover  (finding Flux Kustomizations)"
        echo ""
        cub gitops discover --space "$SPACE" "$K8S_TARGET" 2>&1 || warn "Discover failed"
        echo ""

        if [[ -n "$RENDERER_TARGET" ]]; then
            step "cub gitops import  (creating dry/wet unit pairs)"
            echo ""
            cub gitops import --space "$SPACE" "$K8S_TARGET" "$RENDERER_TARGET" 2>&1 || warn "Import failed"
            echo ""

            step "Units created by cub gitops import:"
            cub unit list --space "$SPACE" 2>/dev/null || true
            echo ""

            note "cub gitops import creates dry/wet unit pairs with auto-updating links."
            note "The renderer produces the exact YAML Flux would apply."
        else
            warn "Renderer target not found - skipping cub gitops import"
            note "Discovery still ran - cub gitops discover found the Kustomizations"
        fi
    else
        warn "Targets not found - skipping cub gitops discover/import"
    fi

    step "Connected readiness check"
    if ! bash "$VERIFY_CONNECTED_SCRIPT" --space "$SPACE" --renderer fluxrenderer; then
        warn "Connected readiness check failed (inspect worker/target state before demoing history)"
    fi

    if $SEED_HISTORY; then
        step "Seeding synthetic demo ChangeSet history"
        seed_args=(--space "$SPACE" --allow-synthetic --apply)
        if [[ -n "${CI:-}" ]]; then
            seed_args+=(--allow-ci)
        fi
        if ! bash "$SEED_HISTORY_SCRIPT" "${seed_args[@]}"; then
            warn "Synthetic history seeding failed"
        else
            note "Synthetic markers: demo=true, synthetic=true, source=cub-scout-demo-seed"
        fi
    else
        note "Synthetic history seeding skipped (use --seed-history for storytelling demos)"
    fi
else
    banner "Act 4: The Pipeline (cub gitops import)"
    note "Not running - add --live to enable (requires ConfigHub auth)"
    echo ""
    note "This is the production import path for Flux clusters."
    note "cub gitops import creates a live render pipeline:"
    echo ""
    note "  1. Discover Flux Kustomizations and HelmReleases on the cluster"
    note "  2. Deploy an in-cluster kustomize renderer worker"
    note "  3. Render each Kustomization through the actual Flux renderer"
    note "  4. Create dry/wet unit pairs with MergeUnits links"
    note "  5. Wet units auto-update as Git changes — no re-import needed"
    echo ""
    note "For Flux-managed apps, this produces the best result: controller-rendered"
    note "manifests with continuous updates. Acts 2-3 capture static snapshots only."
    echo ""
    note "  ./demo.sh --live                                 # with cub auth"
    note "  ./demo.sh --live --seed-history                  # add synthetic history for demo storytelling"
    note "  ./demo.sh --live --confighub-url=http://localhost:9090  # local dev"
fi

# ============================================================
# ACT 5 - MANAGEMENT AND DISCOVERY
# ============================================================
banner "Act 5: Management and Discovery"
echo ""
cat <<'TABLE'
                     MANAGEMENT                    DISCOVERY
                     cub gitops     tree/trace     cub-scout
                     import         (patterns)     import
---------------------------------------------------------------
podinfo (Flux)          Y              Y              Y
payment-api (Flux)      Y              Y              Y
frontend (Flux)         Y              Y              Y
cert-manager (Helm)     .              Y              Y
monitoring (Helm)       .              Y              Y
redis (Helm-only)       .              .              Y
debug-config (Native)   .              .              Y
---------------------------------------------------------------
Unit model           dry/wet pairs  trace chains   flat groups
Rendering            controller     label-based    raw snapshot
Pipeline             auto-updating  read-only      static

Y = found/imported    . = not visible to this tool
TABLE
echo ""

echo -e "${BOLD}Management:${NC} cub gitops import"
echo "  Rendered pipeline with auto-updating dry/wet unit pairs."
echo "  Use for renderable Flux deployers you want to manage continuously."
echo ""
echo -e "${BOLD}Discovery:${NC} cub-scout import + tree/trace"
echo "  Broad cluster inventory (import) or Flux-specific structure (tree/trace)."
echo "  Use to find everything, including Helm/Native resources outside Flux."
echo ""
echo -e "${BOLD}Together:${NC} cub gitops import for Flux apps, then cub-scout import for the rest."
echo ""

# ============================================================
# DONE
# ============================================================
banner "Demo Complete"

if $KEEP; then
    echo -e "Cluster '${BOLD}$CLUSTER_NAME${NC}' is still running."
    echo ""
    echo "  Explore:"
    echo "    $CUB map                              # Interactive TUI"
    echo "    $CUB import --dry-run                 # Workload import proposal"
    echo "    $CUB tree ownership                   # Flux ownership chains"
    echo "    $CUB tree patterns                    # D2 pattern detection"
    echo "    $CUB trace deploy/payment-api -n payments  # Trace to Git source"
    echo "    $CUB gitops status                    # Flux health dashboard"
    echo ""
    echo "  Teardown:"
    if [[ -n "$DISCOVERY_WORKER_PID_FILE" ]]; then
        echo "    bash \"$WORKER_LIFECYCLE_SCRIPT\" stop --pid-file \"$DISCOVERY_WORKER_PID_FILE\""
    fi
    echo "    kind delete cluster --name $CLUSTER_NAME"
else
    echo "Cluster torn down. Run with --keep to explore interactively."
fi
