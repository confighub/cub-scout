#!/usr/bin/env bash
# Disposable live smoke for Helm's Kubernetes Secret release decoding.
# It never reuses an existing kind cluster or the user's default kubeconfig.
set -euo pipefail

CLUSTER="cub-scout-helm-release-decode"
NS="helm-release-decode"
for tool in kind kubectl helm go; do
	command -v "$tool" >/dev/null 2>&1 || { echo "$tool is required" >&2; exit 1; }
done
if kind get clusters 2>/dev/null | grep -Fxq "$CLUSTER"; then
	echo "refusing to reuse existing kind cluster: $CLUSTER" >&2
	exit 1
fi

TMP_DIR="$(mktemp -d)"
KUBECONFIG="$TMP_DIR/kubeconfig"
CREATED=false
cleanup() {
	if [[ "$CREATED" == "true" ]]; then
		KUBECONFIG="$KUBECONFIG" kind delete cluster --name "$CLUSTER" >/dev/null
	fi
	rm -rf "$TMP_DIR"
}
trap cleanup EXIT

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
kind create cluster --name "$CLUSTER" --kubeconfig "$KUBECONFIG"
CREATED=true
export KUBECONFIG

helm create "$TMP_DIR/release-probe"
helm install release-probe "$TMP_DIR/release-probe" --namespace "$NS" --create-namespace
(cd "$REPO_ROOT" && go build -o "$TMP_DIR/cub-scout" ./cmd/cub-scout)

echo "Helm CLI: $(helm version --short)"
echo "Release Secret metadata (payload not printed):"
kubectl -n "$NS" get secrets -l owner=helm,name=release-probe -o name
echo "cub-scout trace of the generated Deployment:"
"$TMP_DIR/cub-scout" trace deployment/release-probe -n "$NS" --format json

