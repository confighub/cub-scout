#!/bin/bash
# demo.sh - Combined demo: Three tools, same cluster, different lenses
#
# Creates a kind cluster with real ArgoCD, populates it with two fixture sets
# (real synced apps + label-only "Arnie" pattern), then runs three import tools
# to show what each sees and misses.
#
# Usage:
#   ./demo.sh                # Observe only (dry-run, ~5 min)
#   ./demo.sh --live         # Full import into ConfigHub (~6 min, requires cub auth)
#   ./demo.sh --keep         # Keep cluster running for exploration
#   ./demo.sh --live --keep  # Import + keep cluster
#   ./demo.sh --live --seed-history  # Also seed tagged synthetic ChangeSet history
#   ./demo.sh --live --confighub-url=http://localhost:9090  # Local dev override
#
# Prerequisites: docker, kind, kubectl, go
#                --live also requires: cub, curl, python3 (cub auth login)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
ARNIE_FIXTURES="$SCRIPT_DIR/../import-from-live/fixtures"
GUESTBOOK_FIXTURES="$SCRIPT_DIR/fixtures"
CLUSTER_NAME="argo-import-demo"
ARGOCD_VERSION="v3.3.2"
ARGOCD_INSTALL_URL="https://raw.githubusercontent.com/argoproj/argo-cd/${ARGOCD_VERSION}/manifests/install.yaml"
KEEP=false
LIVE=false
SEED_HISTORY=false
CONFIGHUB_URL_OVERRIDE=""
WORKER_PID=""
PORT_FORWARD_PID=""
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

guestbook_app_ready() {
    local app="$1"
    local sync_status health_status

    sync_status="$(kubectl get application "$app" -n argocd -o jsonpath='{.status.sync.status}' 2>/dev/null || true)"
    health_status="$(kubectl get application "$app" -n argocd -o jsonpath='{.status.health.status}' 2>/dev/null || true)"

    [[ "$sync_status" == "Synced" && "$health_status" == "Healthy" ]]
}

cleanup() {
    if [[ -n "$PORT_FORWARD_PID" ]]; then
        kill "$PORT_FORWARD_PID" 2>/dev/null || true
        wait "$PORT_FORWARD_PID" 2>/dev/null || true
    fi
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

for tool in kind kubectl docker go; do
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
    for tool in cub curl python3; do
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
    step "Auth OK (cub, curl, python3 present)"
fi

step "Building cub-scout..."
(cd "$REPO_ROOT" && go build -o cub-scout ./cmd/cub-scout)
CUB="$REPO_ROOT/cub-scout"
step "Build OK"

# ============================================================
# ACT 1 - THE CLUSTER
# ============================================================
banner "Act 1: The Cluster"
note "Creating kind cluster with real ArgoCD + two fixture sets"
echo ""

# Delete stale cluster
kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || true

step "Creating kind cluster..."
kind create cluster --name "$CLUSTER_NAME" --wait 60s
step "Cluster ready: kind-$CLUSTER_NAME"
echo ""

# --- Install real ArgoCD ---
step "Installing ArgoCD ${ARGOCD_VERSION} (this takes 2-4 minutes)..."
kubectl create namespace argocd --dry-run=client -o yaml | kubectl apply -f -
kubectl apply --server-side -n argocd -f "$ARGOCD_INSTALL_URL"

step "Waiting for ArgoCD deployments..."
ARGOCD_READY=true
if ! kubectl wait --for=condition=Available --timeout=300s \
    -n argocd deployment/argocd-server \
    deployment/argocd-repo-server \
    deployment/argocd-applicationset-controller \
    deployment/argocd-notifications-controller \
    deployment/argocd-redis \
    deployment/argocd-dex-server 2>/dev/null; then
    warn "Some ArgoCD components not ready yet; continuing anyway"
    ARGOCD_READY=false
fi

if $ARGOCD_READY; then
    step "ArgoCD installed and ready"
else
    warn "ArgoCD partially ready - guestbook apps may not sync"
fi
echo ""

# --- Namespaces for Arnie fixtures ---
step "Creating namespaces for Arnie fixtures..."
for ns in myapp-dev myapp-staging myapp-prod; do
    kubectl create namespace "$ns" 2>/dev/null || true
done

# --- Apply Arnie fixtures (from import-from-live) ---
step "Applying Arnie fixtures..."

kubectl apply -f "$ARNIE_FIXTURES/applications.yaml"
note "3 ArgoCD Application CRs (myapp-dev/staging/prod) - CRs exist but not synced"

kubectl apply -f "$ARNIE_FIXTURES/deployments.yaml"
note "6 Deployments (api + worker x 3 envs) - ArgoCD labels, applied by kubectl"

kubectl apply -f "$ARNIE_FIXTURES/statefulsets.yaml"
note "3 StatefulSets (redis x 3 envs) - Helm-managed"

kubectl apply -f "$ARNIE_FIXTURES/native.yaml"
note "1 ConfigMap (debug-config) - Native/unmanaged"
echo ""

# --- Apply guestbook apps (real ArgoCD-synced) ---
step "Applying guestbook Applications (real ArgoCD sync)..."
kubectl apply -f "$GUESTBOOK_FIXTURES/guestbook-apps.yaml"
note "2 ArgoCD Applications: helm-guestbook + kustomize-guestbook"
note "These have syncPolicy.automated - ArgoCD will create real workloads"
echo ""

# --- Wait for guestbook sync ---
step "Waiting for guestbook apps to sync (up to 120s)..."
GUESTBOOK_SYNCED=false
for i in $(seq 1 24); do
    if guestbook_app_ready helm-guestbook && guestbook_app_ready kustomize-guestbook; then
        GUESTBOOK_SYNCED=true
        break
    fi
    printf "."
    sleep 5
done
echo ""

if $GUESTBOOK_SYNCED; then
    step "Guestbook apps synced - ArgoCD created real workloads"
    kubectl get deployments -n guestbook 2>/dev/null || true
else
    warn "Guestbook apps haven't synced yet (ArgoCD may still be working)"
    note "This is OK - the demo still shows the contrast between tools"
fi
echo ""

# Wait for Arnie deployments
step "Waiting for Arnie workloads..."
kubectl wait --for=condition=available deployment --all -n myapp-dev --timeout=60s >/dev/null 2>&1 || true
kubectl wait --for=condition=available deployment --all -n myapp-staging --timeout=60s >/dev/null 2>&1 || true
kubectl wait --for=condition=available deployment --all -n myapp-prod --timeout=60s >/dev/null 2>&1 || true

# --- Summary ---
echo ""
step "Cluster populated:"
note "Guestbook = real ArgoCD-synced (helm + kustomize)"
note "Arnie Deployments = ArgoCD-labeled but kubectl-applied"
note "Redis = Helm-managed (invisible to ArgoCD)"
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

step "cub-scout gitops status  (ArgoCD health + sync status)"
echo ""
"$CUB" gitops status 2>/dev/null || true
echo ""

step "cub-scout import --dry-run  (workload grouping proposal)"
echo ""
"$CUB" import --dry-run 2>/dev/null || true
echo ""

note "cub-scout found: ArgoCD workloads + Helm redis + Native debug-config"
note "It groups by ownership labels, not by what the controller actually synced"

# ============================================================
# ACT 3 - THE APPLICATION IMPORTER: import argocd
# ============================================================
banner "Act 3: The Application Importer (import argocd)"
note "import argocd reads ArgoCD Application CRs specifically"
echo ""

step "cub-scout import argocd --list  (all ArgoCD Applications)"
echo ""
"$CUB" import argocd --list 2>/dev/null || true
echo ""

step "cub-scout import argocd helm-guestbook --dry-run  (real synced app)"
echo ""
"$CUB" import argocd helm-guestbook --dry-run 2>/dev/null || warn "helm-guestbook not ready yet"
echo ""

step "cub-scout import argocd myapp-dev --dry-run  (Arnie app - labels only)"
echo ""
"$CUB" import argocd myapp-dev --dry-run 2>/dev/null || warn "myapp-dev not accessible"
echo ""

note "import argocd sees 5 Applications. helm-guestbook has real managed"
note "resources; myapp-dev has resources but ArgoCD didn't create them."
note "Redis and debug-config are invisible to this tool."

# ============================================================
# ACT 4 - THE PIPELINE: cub gitops import (--live only)
# ============================================================
if $LIVE; then
    banner "Act 4: The Pipeline (cub gitops import)"
    note "Full render pipeline with dry/wet unit pairs"
    note "Requires ConfigHub infrastructure + in-cluster renderer"
    echo ""

    SPACE="argo-import-demo"
    KUBE_CONTEXT=$(kubectl config current-context)
    DISCOVERY_WORKER_SLUG="discovery-worker"
    WORKER_STATE_DIR="${TMPDIR:-/tmp}/cub-scout-demo-workers"
    mkdir -p "$WORKER_STATE_DIR"
    DISCOVERY_WORKER_PID_FILE="$WORKER_STATE_DIR/${SPACE}-${DISCOVERY_WORKER_SLUG}.pid"
    DISCOVERY_WORKER_LOG="$WORKER_STATE_DIR/${SPACE}-${DISCOVERY_WORKER_SLUG}.log"

    # Get ArgoCD auth token
    step "Getting ArgoCD auth token..."
    ARGOCD_ADMIN_PASSWORD=$(kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d)
    ARGOCD_TOKEN_MAX_ATTEMPTS=30
    ARGOCD_TOKEN_RETRY_SECONDS=4
    ARGOCD_PROXY_PORT=8001
    ARGOCD_PROXY_LOG="$(mktemp)"
    ARGOCD_SESSION_PROXY_URL="http://127.0.0.1:${ARGOCD_PROXY_PORT}/api/v1/namespaces/argocd/services/https:argocd-server:https/proxy/api/v1/session"
    note "Retrying the ArgoCD session endpoint through the Kubernetes service proxy"

    kubectl proxy --port "$ARGOCD_PROXY_PORT" >"$ARGOCD_PROXY_LOG" 2>&1 &
    ARGOCD_PROXY_PID=$!
    sleep 2

    ARGOCD_TOKEN=""
    LAST_ARGOCD_SERVER_STATE=""
    LAST_ARGOCD_SESSION_STATUS=""
    for i in $(seq 1 "$ARGOCD_TOKEN_MAX_ATTEMPTS"); do
        LAST_ARGOCD_SERVER_STATE="$(kubectl -n argocd get pod -l app.kubernetes.io/name=argocd-server \
            -o jsonpath='{range .items[0]}{.metadata.name} phase={.status.phase} ready={.status.containerStatuses[0].ready} restarts={.status.containerStatuses[0].restartCount}{end}' 2>/dev/null || true)"

        RESPONSE=$(curl -sS --max-time 5 -H "Content-Type: application/json" "$ARGOCD_SESSION_PROXY_URL" \
            -d "{\"username\":\"admin\",\"password\":\"${ARGOCD_ADMIN_PASSWORD}\"}" 2>/dev/null || true)
        LAST_ARGOCD_SESSION_STATUS="$(printf '%s' "$RESPONSE" | tr '\n' ' ' | sed 's/[[:space:]]\+/ /g' | cut -c1-200)"
        if [ -n "$RESPONSE" ]; then
            ARGOCD_TOKEN=$(echo "$RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin).get('token',''))" 2>/dev/null || true)
        fi

        if [ -n "$ARGOCD_TOKEN" ]; then
            break
        fi

        printf "."
        sleep "$ARGOCD_TOKEN_RETRY_SECONDS"
    done
    echo ""

    kill "$ARGOCD_PROXY_PID" 2>/dev/null || true
    wait "$ARGOCD_PROXY_PID" 2>/dev/null || true

    LAST_PROXY_STATUS=""
    if [[ -s "$ARGOCD_PROXY_LOG" ]]; then
        LAST_PROXY_STATUS=$(tail -n 3 "$ARGOCD_PROXY_LOG" | tr '\n' ' ' | sed 's/[[:space:]]\+/ /g')
    fi
    rm -f "$ARGOCD_PROXY_LOG"

    if [ -z "$ARGOCD_TOKEN" ]; then
        warn "Could not get ArgoCD token after ${ARGOCD_TOKEN_MAX_ATTEMPTS} attempts - skipping cub gitops import"
        if [[ -n "$LAST_PROXY_STATUS" ]]; then
            note "Last proxy status: $LAST_PROXY_STATUS"
        fi
        if [[ -n "$LAST_ARGOCD_SERVER_STATE" ]]; then
            note "Last argocd-server state: $LAST_ARGOCD_SERVER_STATE"
        fi
        if [[ -n "$LAST_ARGOCD_SESSION_STATUS" ]]; then
            note "Last session response: $LAST_ARGOCD_SESSION_STATUS"
        fi
        warn "The demo still shows Acts 1-3"
    else
        step "ArgoCD auth token obtained"

        # Create ConfigHub space
        step "Creating ConfigHub space '$SPACE'..."
        cub space create "$SPACE" 2>/dev/null || true

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

        # Deploy ArgoCD renderer in-cluster
        step "Deploying ArgoCD renderer worker in-cluster..."

        # For public URLs (https://hub.confighub.com), the in-cluster worker
        # can reach them directly. For local dev (localhost:*), we need the
        # docker gateway IP so the kind container can reach the host.
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

        WORKER_MANIFEST=$(mktemp)
        cub worker install --space "$SPACE" \
            --export \
            --include-secret \
            -t argocdrenderer \
            -n confighub-worker \
            --image-pull-policy IfNotPresent \
            -e "CONFIGHUB_URL=$RENDERER_CONFIGHUB_URL" \
            -e "ARGOCD_SERVER=https://argocd-server.argocd.svc.cluster.local" \
            -e "ARGOCD_AUTH_TOKEN=$ARGOCD_TOKEN" \
            -e "ARGOCD_INSECURE=true" \
            argocd-renderer > "$WORKER_MANIFEST"

        RENDERER_IMAGE="$(grep -m1 'image:' "$WORKER_MANIFEST" | awk '{print $2}' || true)"
        if [[ -n "$RENDERER_IMAGE" ]]; then
            step "Preloading renderer image into kind..."
            if ! docker image inspect "$RENDERER_IMAGE" >/dev/null 2>&1; then
                note "Pulling renderer image: $RENDERER_IMAGE"
                docker pull "$RENDERER_IMAGE" >/dev/null 2>&1 || warn "Renderer image pull failed; cluster will pull directly"
            fi
            if docker image inspect "$RENDERER_IMAGE" >/dev/null 2>&1; then
                kind load docker-image --name "$CLUSTER_NAME" "$RENDERER_IMAGE" >/dev/null 2>&1 || warn "Renderer image preload failed"
            fi
        fi

        kubectl create namespace confighub-worker 2>/dev/null || true
        kubectl apply -f "$WORKER_MANIFEST"
        rm -f "$WORKER_MANIFEST"

        step "Waiting for renderer deployment..."
        kubectl -n confighub-worker wait --for=condition=available \
            deployment --all --timeout=120s 2>/dev/null || warn "Renderer not ready"

        # Wait for discovery target
        step "Waiting for targets to register..."
        TARGETS_READY=false
        for i in $(seq 1 60); do
            TARGET_OUTPUT=$(cub target list --space "$SPACE" 2>/dev/null || true)
            if echo "$TARGET_OUTPUT" | awk '$3=="Kubernetes" {found=1} END {exit found ? 0 : 1}' \
                && echo "$TARGET_OUTPUT" | awk '$3=="ArgoCDRenderer" {found=1} END {exit found ? 0 : 1}'; then
                TARGETS_READY=true
                break
            fi
            printf "."
            sleep 2
        done
        echo ""

        if ! $TARGETS_READY; then
            warn "Not all targets registered - cub gitops import may fail"
            cub target list --space "$SPACE" 2>/dev/null || true
        else
            step "Both targets registered"
        fi

        # Extract target slugs
        TARGET_OUTPUT=$(cub target list --space "$SPACE" 2>/dev/null || true)
        K8S_TARGET=$(echo "$TARGET_OUTPUT" | awk '$3=="Kubernetes" {print $1; exit}' || true)
        RENDERER_TARGET=$(echo "$TARGET_OUTPUT" | awk '$3=="ArgoCDRenderer" {print $1; exit}' || true)

        if [[ -n "$K8S_TARGET" ]] && [[ -n "$RENDERER_TARGET" ]]; then
            step "cub gitops discover  (finding ArgoCD Applications)"
            echo ""
            cub gitops discover --space "$SPACE" "$K8S_TARGET" 2>&1 || warn "Discover failed"
            echo ""

            step "cub gitops import  (creating dry/wet unit pairs)"
            echo ""
            cub gitops import --space "$SPACE" "$K8S_TARGET" "$RENDERER_TARGET" 2>&1 || warn "Import failed"
            echo ""

            step "Units created by cub gitops import:"
            cub unit list --space "$SPACE" 2>/dev/null || true
            echo ""

            note "cub gitops import creates dry/wet unit pairs with auto-updating links."
            note "The renderer produces the exact YAML ArgoCD would apply."
        else
            warn "Targets not found - skipping cub gitops import"
            note "K8S_TARGET=$K8S_TARGET  RENDERER_TARGET=$RENDERER_TARGET"
        fi

        step "Connected readiness check"
        if ! bash "$VERIFY_CONNECTED_SCRIPT" --space "$SPACE" --renderer argocdrenderer; then
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
    fi
else
    banner "Act 4: The Pipeline (cub gitops import)"
    note "Not running - add --live to enable (requires ConfigHub auth)"
    echo ""
    note "This is the production import path for ArgoCD/Flux clusters."
    note "cub gitops import creates a live render pipeline:"
    echo ""
    note "  1. Discover ArgoCD Applications on the cluster"
    note "  2. Deploy an in-cluster renderer worker"
    note "  3. Render each Application through the actual ArgoCD renderer"
    note "  4. Create dry/wet unit pairs with MergeUnits links"
    note "  5. Wet units auto-update as Git changes — no re-import needed"
    echo ""
    note "For ArgoCD-managed apps, this produces the best result: controller-rendered"
    note "manifests with continuous updates. Acts 2-3 capture static snapshots only."
    echo ""
    note "  ./demo.sh --live                                 # with cub auth"
    note "  ./demo.sh --live --seed-history                  # add synthetic history for demo storytelling"
    note "  ./demo.sh --live --confighub-url=http://localhost:9090  # local dev"
fi

# ============================================================
# ACT 5 - THE COMPARISON
# ============================================================
banner "Act 5: Management and Discovery"
echo ""
cat <<'TABLE'
                     MANAGEMENT                    DISCOVERY
                     cub gitops     import argocd  cub-scout
                     import         (per-app)      import
---------------------------------------------------------------
helm-guestbook          Y              Y              Y
kustomize-guestbook     Y              Y              Y
myapp-dev               Y              Y              Y
myapp-staging           Y              Y              Y
myapp-prod              Y              Y              Y
redis (Helm)            .              .              Y
debug-config (Native)   .              .              Y
---------------------------------------------------------------
Unit model           dry/wet pairs  per-app        flat groups
Rendering            controller     raw snapshot   raw snapshot
Pipeline             auto-updating  static         static

Y = found/imported    . = not visible to this tool
TABLE
echo ""

echo -e "${BOLD}Management:${NC} cub gitops import"
echo "  Rendered pipeline with auto-updating dry/wet unit pairs."
echo "  Use for ArgoCD/Flux apps you want to manage continuously."
echo ""
echo -e "${BOLD}Discovery:${NC} cub-scout import + import argocd"
echo "  Broad cluster inventory (import) or quick per-app detail (import argocd)."
echo "  Use to find everything, including Helm/Native resources outside ArgoCD."
echo ""
echo -e "${BOLD}Together:${NC} cub gitops import for ArgoCD apps, then cub-scout import for the rest."
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
    echo "    $CUB import argocd --list             # List ArgoCD Applications"
    echo "    $CUB import argocd helm-guestbook --show-yaml  # Show rendered YAML"
    echo "    $CUB gitops status                    # GitOps health dashboard"
    echo "    $CUB trace deploy/api -n myapp-prod   # Trace ownership chain"
    echo ""
    echo "  Teardown:"
    if [[ -n "$DISCOVERY_WORKER_PID_FILE" ]]; then
        echo "    bash \"$WORKER_LIFECYCLE_SCRIPT\" stop --pid-file \"$DISCOVERY_WORKER_PID_FILE\""
    fi
    echo "    kind delete cluster --name $CLUSTER_NAME"
else
    echo "Cluster torn down. Run with --keep to explore interactively."
fi
