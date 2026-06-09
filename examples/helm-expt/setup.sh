#!/usr/bin/env bash
#
# setup.sh — stand up a self-contained cluster state that reproduces helm-expt
# finding F3, so verify.sh can show where object-set-matches falls short today.
#
# Self-contained: uses only the fixture in this directory. It never reads or
# touches a helm-expt checkout.
#
# Default: create an ephemeral kind cluster named cub-scout-helm-expt.
#   --no-cluster : skip kind, apply into your CURRENT kube-context instead.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLUSTER="cub-scout-helm-expt"
CONTEXT="kind-${CLUSTER}"
NS="helm-expt-demo"
MANIFESTS="$SCRIPT_DIR/fixtures/release-objects.yaml"
NO_CLUSTER=false

usage() {
    cat <<'USAGE'
Usage: ./setup.sh [--no-cluster]

Options:
  --no-cluster   Apply the fixture into your current kube-context instead of
                 creating an ephemeral kind cluster.
  -h, --help     Show this help.

What it does (read it before running):
  - creates kind cluster "cub-scout-helm-expt" (unless --no-cluster)
  - creates namespace "helm-expt-demo"
  - applies fixtures/release-objects.yaml (a ConfigMap, Service, Deployment)
  - does NOT create the Secret the Deployment requires -> pod will sit in
    CreateContainerConfigError. That is intentional.
  - creates one extra ConfigMap that is NOT in the object set, to exercise the
    closed-world / extra-live-object coverage gap.
USAGE
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --no-cluster) NO_CLUSTER=true; shift ;;
        -h|--help) usage; exit 0 ;;
        *) echo "unknown argument: $1" >&2; usage; exit 2 ;;
    esac
done

if [[ "$NO_CLUSTER" == "true" ]]; then
    CONTEXT="$(kubectl config current-context)"
    echo "==> Using current kube-context: $CONTEXT"
else
    command -v kind >/dev/null 2>&1 || { echo "kind is required (or pass --no-cluster)" >&2; exit 1; }
    if kind get clusters 2>/dev/null | grep -qx "$CLUSTER"; then
        echo "==> kind cluster '$CLUSTER' already exists; reusing it"
    else
        echo "==> Creating kind cluster '$CLUSTER'"
        kind create cluster --name "$CLUSTER"
    fi
fi

echo "==> Ensuring namespace '$NS'"
kubectl --context "$CONTEXT" create namespace "$NS" --dry-run=client -o yaml | kubectl --context "$CONTEXT" apply -f -

echo "==> Applying the rendered object set (no Secret -> intentional F3 reproduction)"
kubectl --context "$CONTEXT" apply -f "$MANIFESTS"

echo "==> Planting one extra live ConfigMap that is NOT in the object set"
kubectl --context "$CONTEXT" create configmap drift-not-in-object-set \
    -n "$NS" --from-literal=note="object-set-matches will not notice me" \
    --dry-run=client -o yaml | kubectl --context "$CONTEXT" apply -f -

echo "==> Waiting a few seconds for the pod to reach its (failed) steady state"
kubectl --context "$CONTEXT" -n "$NS" wait --for=condition=Available deployment/web --timeout=20s || true

echo
echo "Setup complete. Pod state:"
kubectl --context "$CONTEXT" -n "$NS" get pods -o wide || true
echo
echo "Now run:  ./verify.sh"
