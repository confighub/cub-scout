#!/bin/bash
# teardown-cluster.sh - Delete the TUI E2E test cluster
#
# Usage:
#   ./teardown-cluster.sh [cluster-name]

set -euo pipefail

CLUSTER_NAME="${1:-tui-e2e}"

echo "Deleting kind cluster '$CLUSTER_NAME'..."

if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
    kind delete cluster --name "$CLUSTER_NAME"
    echo "Cluster deleted"
else
    echo "Cluster '$CLUSTER_NAME' does not exist"
fi
