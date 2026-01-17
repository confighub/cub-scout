#!/usr/bin/env bash
set -euo pipefail

# Cleanup kind test clusters
# Usage: ./cleanup-clusters.sh [cluster-name]
#        ./cleanup-clusters.sh all

TARGET="${1:-}"

if [[ -z "$TARGET" ]]; then
    echo "Usage: $0 <cluster-name|all>"
    echo ""
    echo "Current kind clusters:"
    kind get clusters
    exit 0
fi

if [[ "$TARGET" == "all" ]]; then
    echo "Deleting all kind clusters..."
    for cluster in $(kind get clusters); do
        echo "  Deleting ${cluster}..."
        kind delete cluster --name "$cluster"
    done
else
    echo "Deleting kind cluster '${TARGET}'..."
    kind delete cluster --name "$TARGET"
fi

echo "Done."
