#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
KUBE_CONTEXT="kind-flux-import-demo"
SPACE="flux-import-demo"
RENDERER_TOKEN="fluxrenderer"
VERIFY_CONNECTED_SCRIPT="$SCRIPT_DIR/../scripts/verify-connected-demo.sh"
WORKER_STATE_DIR="${TMPDIR:-/tmp}/cub-scout-demo-workers"

usage() {
    cat <<'USAGE'
Usage: ./verify.sh [--space <slug>] [--renderer <token>]

Options:
  --space <slug>       ConfigHub space to inspect when connected mode was used
                       default: flux-import-demo
  --renderer <token>   Renderer token expected by the connected readiness helper
                       default: fluxrenderer
  -h, --help           Show this help

Notes:
  - Cluster evidence is always checked.
  - ConfigHub evidence is checked when the live worker pid file is present.
  - cub-scout evidence is checked from the repo-root cub-scout binary when available.
USAGE
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --space)
            [[ $# -ge 2 ]] || { echo "missing value for --space" >&2; usage; exit 2; }
            SPACE="$2"
            shift 2
            ;;
        --space=*)
            SPACE="${1#*=}"
            shift
            ;;
        --renderer)
            [[ $# -ge 2 ]] || { echo "missing value for --renderer" >&2; usage; exit 2; }
            RENDERER_TOKEN="$2"
            shift 2
            ;;
        --renderer=*)
            RENDERER_TOKEN="${1#*=}"
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            echo "unknown argument: $1" >&2
            usage
            exit 2
            ;;
    esac
done

DISCOVERY_WORKER_PID_FILE="$WORKER_STATE_DIR/${SPACE}-discovery-worker.pid"

fail() {
    echo "FAIL verify"
    echo "- $1"
    exit 1
}

resolve_cub_scout() {
    if [[ -n "${CUB_SCOUT_BIN:-}" ]]; then
        printf '%s\n' "$CUB_SCOUT_BIN"
        return
    fi
    if [[ -x "$REPO_ROOT/cub-scout" ]]; then
        printf '%s\n' "$REPO_ROOT/cub-scout"
        return
    fi
    if command -v cub-scout >/dev/null 2>&1; then
        command -v cub-scout
        return
    fi
    echo ""
}

echo "==> Checking cluster connectivity"
kubectl --context "$KUBE_CONTEXT" get nodes >/dev/null 2>&1 || fail "cluster context '$KUBE_CONTEXT' is not reachable"

echo "==> Checking Flux namespace"
kubectl --context "$KUBE_CONTEXT" get namespace flux-system >/dev/null 2>&1 || fail "flux-system namespace is missing"

echo "==> Checking Flux controllers and deployers"
kubectl --context "$KUBE_CONTEXT" get gitrepositories,kustomizations,helmreleases -A >/dev/null 2>&1 || fail "Flux API resources are not listable"
for resource in \
    "gitrepository/podinfo -n flux-system" \
    "gitrepository/platform-config -n flux-system" \
    "kustomization/podinfo -n flux-system" \
    "kustomization/infrastructure -n flux-system" \
    "kustomization/apps -n flux-system" \
    "kustomization/payment-api -n flux-system" \
    "kustomization/frontend -n flux-system" \
    "helmrelease/cert-manager -n cert-manager" \
    "helmrelease/monitoring -n monitoring"; do
    kubectl --context "$KUBE_CONTEXT" get $resource >/dev/null 2>&1 || fail "expected Flux resource missing: $resource"
done

echo "==> Checking Flux overview"
command -v flux >/dev/null 2>&1 || fail "flux CLI not found"
flux --context "$KUBE_CONTEXT" get all -A >/dev/null 2>&1 || fail "flux get all -A failed"
flux --context "$KUBE_CONTEXT" get all -A

if [[ -f "$DISCOVERY_WORKER_PID_FILE" ]]; then
    echo "==> Checking ConfigHub evidence"
    command -v cub >/dev/null 2>&1 || fail "cub CLI not found while connected worker pid file exists"
    cub auth get-token >/dev/null 2>&1 || fail "cub auth is required while connected worker pid file exists"

    echo "==> Listing ConfigHub targets"
    cub target list --space "$SPACE" || fail "failed to list targets for space '$SPACE'"

    echo "==> Listing ConfigHub units"
    unit_output="$(cub unit list --space "$SPACE" 2>&1)" || fail "failed to list units for space '$SPACE'"
    if [[ -z "${unit_output//[[:space:]]/}" ]]; then
        fail "unit list output was empty for space '$SPACE'"
    fi
    printf '%s\n' "$unit_output"

    echo "==> Checking connected readiness"
    bash "$VERIFY_CONNECTED_SCRIPT" --space "$SPACE" --renderer "$RENDERER_TOKEN"
else
    echo "==> Connected worker pid file not present"
    echo "Skipping ConfigHub evidence and connected readiness checks"
fi

CUB_SCOUT="$(resolve_cub_scout)"
[[ -n "$CUB_SCOUT" ]] || fail "cub-scout binary not found (run ./setup.sh or build ./cmd/cub-scout)"

echo "==> Checking cub-scout gitops status"
status_output="$("$CUB_SCOUT" gitops status 2>&1 || true)"
if [[ -z "${status_output//[[:space:]]/}" ]]; then
    fail "cub-scout gitops status produced no output"
fi
printf '%s\n' "$status_output"

echo "==> Checking cub-scout tree ownership"
tree_output="$("$CUB_SCOUT" tree ownership 2>&1 || true)"
if [[ -z "${tree_output//[[:space:]]/}" ]]; then
    fail "cub-scout tree ownership produced no output"
fi
printf '%s\n' "$tree_output"

echo "Verification completed."
echo "This verifies three evidence surfaces: cluster, ConfigHub, and cub-scout."
echo "Post-import scan/finding evidence is not part of this verify contract yet."
