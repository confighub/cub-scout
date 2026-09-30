#!/usr/bin/env bash
# Disposable valid-release live smoke for Helm Secret release decoding.
# It never reuses an existing kind cluster or the user's default kubeconfig.
set -euo pipefail

CLUSTER="cub-scout-helm-release-decode"
NS="helm-release-decode"
if [[ -n "${1:-}" ]]; then
	EVIDENCE_DIR="$1"
	if [[ -e "$EVIDENCE_DIR" ]]; then
		if [[ ! -d "$EVIDENCE_DIR" ]] || find "$EVIDENCE_DIR" -mindepth 1 -print -quit | grep -q .; then
			echo "refusing to overwrite existing evidence: $EVIDENCE_DIR" >&2
			exit 1
		fi
	fi
	mkdir -p "$EVIDENCE_DIR"
else
	EVIDENCE_DIR="$(mktemp -d "${TMPDIR:-/tmp}/cub-scout-helm-release-evidence.XXXXXX")"
fi
echo "Evidence directory: $EVIDENCE_DIR"

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
	status=$?
	printf '%s\n' "$status" >"$EVIDENCE_DIR/exit-status.txt"
	if [[ "$CREATED" == "true" ]]; then
		KUBECONFIG="$KUBECONFIG" kind delete cluster --name "$CLUSTER" >/dev/null
	fi
	rm -rf "$TMP_DIR"
	exit "$status"
}
trap cleanup EXIT

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
(cd "$REPO_ROOT" && go build -o "$TMP_DIR/cub-scout" ./cmd/cub-scout)
helm version --short >"$EVIDENCE_DIR/helm-version.txt" 2>&1
helm create "$TMP_DIR/release-probe"
helm template release-probe "$TMP_DIR/release-probe" >"$TMP_DIR/rendered.yaml"
shasum -a 256 "$TMP_DIR/rendered.yaml" >"$EVIDENCE_DIR/chart-render.sha256"

kind create cluster --name "$CLUSTER" --kubeconfig "$KUBECONFIG" >"$EVIDENCE_DIR/kind-create.stdout" 2>"$EVIDENCE_DIR/kind-create.stderr"
CREATED=true
export KUBECONFIG

helm install release-probe "$TMP_DIR/release-probe" --namespace "$NS" --create-namespace >"$EVIDENCE_DIR/helm-install.stdout" 2>"$EVIDENCE_DIR/helm-install.stderr"

kubectl -n "$NS" get secrets -l owner=helm,name=release-probe -o name >"$EVIDENCE_DIR/secret-metadata.txt"
"$TMP_DIR/cub-scout" trace deployment/release-probe -n "$NS" --format json >"$EVIDENCE_DIR/trace.stdout" 2>"$EVIDENCE_DIR/trace.stderr"
