#!/usr/bin/env bash
#
# cleanup.sh — tear down whatever setup.sh created.

set -euo pipefail

CLUSTER="cub-scout-helm-expt"
CONTEXT="kind-${CLUSTER}"
NS="helm-expt-demo"
NO_CLUSTER=false

usage() {
    cat <<'USAGE'
Usage: ./cleanup.sh [--no-cluster]

Options:
  --no-cluster   Only delete the demo namespace from the current context;
                 do not delete a kind cluster.
  -h, --help     Show this help.
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
    echo "==> Deleting namespace '$NS' from context '$CONTEXT'"
    kubectl --context "$CONTEXT" delete namespace "$NS" --ignore-not-found
else
    if kind get clusters 2>/dev/null | grep -qx "$CLUSTER"; then
        echo "==> Deleting kind cluster '$CLUSTER'"
        kind delete cluster --name "$CLUSTER"
    else
        echo "==> kind cluster '$CLUSTER' not present; nothing to delete"
    fi
fi

echo "Cleanup complete."
