#!/usr/bin/env bash
# Disposable valid-release live smoke for Helm Secret release decoding and
# exact manifest-to-resource identity.
# It never reuses an existing kind cluster or the user's default kubeconfig.
set -euo pipefail

CLUSTER="cub-scout-helm-release-decode"
NS="helm-release-decode"
NEGATIVE_IDENTITY=false
if [[ "${1:-}" == "--negative-identity" ]]; then
	NEGATIVE_IDENTITY=true
	shift
fi
if [[ $# -gt 1 ]]; then
	echo "usage: $0 [--negative-identity] [evidence-directory]" >&2
	exit 2
fi
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

for tool in kind kubectl helm go jq; do
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
SCOUT_BIN="$TMP_DIR/cub-scout"
if [[ -n "${CUB_SCOUT_BASELINE_BIN:-}" ]]; then
	if [[ ! -x "$CUB_SCOUT_BASELINE_BIN" ]]; then
		echo "CUB_SCOUT_BASELINE_BIN must name an executable" >&2
		exit 1
	fi
	cp "$CUB_SCOUT_BASELINE_BIN" "$TMP_DIR/cub-scout-baseline"
fi
(cd "$REPO_ROOT" && go build -o "$SCOUT_BIN" ./cmd/cub-scout)
helm version --short >"$EVIDENCE_DIR/helm-version.txt" 2>&1
CHART_DIR="$TMP_DIR/release-probe"
mkdir -p "$CHART_DIR/templates"
cat >"$CHART_DIR/Chart.yaml" <<'EOF'
apiVersion: v2
name: release-probe
description: Minimal deterministic chart for the Helm release trace smoke.
type: application
version: 0.1.0
appVersion: "3.9"
EOF
cat >"$CHART_DIR/templates/deployment.yaml" <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: release-probe
  namespace: $NS
  labels:
    app.kubernetes.io/name: release-probe
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: release-probe
  template:
    metadata:
      labels:
        app.kubernetes.io/name: release-probe
    spec:
      containers:
        - name: pause
          image: registry.k8s.io/pause:3.9
EOF
helm template release-probe "$CHART_DIR" --namespace "$NS" >"$TMP_DIR/rendered.yaml"
shasum -a 256 "$TMP_DIR/rendered.yaml" >"$EVIDENCE_DIR/chart-render.sha256"

kind create cluster --name "$CLUSTER" --kubeconfig "$KUBECONFIG" >"$EVIDENCE_DIR/kind-create.stdout" 2>"$EVIDENCE_DIR/kind-create.stderr"
CREATED=true
export KUBECONFIG

helm install release-probe "$CHART_DIR" --namespace "$NS" --create-namespace >"$EVIDENCE_DIR/helm-install.stdout" 2>"$EVIDENCE_DIR/helm-install.stderr"

kubectl -n "$NS" get secrets -l owner=helm,name=release-probe -o name >"$EVIDENCE_DIR/secret-metadata.txt"
"$SCOUT_BIN" trace deployment/release-probe -n "$NS" --format json >"$EVIDENCE_DIR/trace.stdout" 2>"$EVIDENCE_DIR/trace.stderr"
jq -e --arg ns "$NS" '
  .summary.ownerType == "Helm" and
  ([.chain[] | select(.id.kind == "Deployment" and .id.name == "release-probe" and .id.namespace == $ns)] | length) == 1
' "$EVIDENCE_DIR/trace.stdout" >"$EVIDENCE_DIR/trace-assertion.txt"

if [[ "$NEGATIVE_IDENTITY" == "true" ]]; then
	SECRET_NAME="$(kubectl -n "$NS" get secrets -l owner=helm,name=release-probe -o jsonpath='{.items[0].metadata.name}')"
	if [[ -z "$SECRET_NAME" ]]; then
		echo "Helm release Secret was not found by the owner=helm,name selector" >&2
		exit 1
	fi
	# Record only identity metadata. Never persist Secret .data or decoded payload.
	kubectl -n "$NS" get secret "$SECRET_NAME" -o json | jq '{metadata:{name:.metadata.name,namespace:.metadata.namespace,labels:.metadata.labels},type:.type}' >"$EVIDENCE_DIR/secret-identity-before.json"
	if [[ -n "${CUB_SCOUT_BASELINE_BIN:-}" ]]; then
		set +e
		"$TMP_DIR/cub-scout-baseline" trace deployment/release-probe -n "$NS" --format json >"$EVIDENCE_DIR/baseline-corrupt-label.stdout" 2>"$EVIDENCE_DIR/baseline-corrupt-label.stderr"
		BASELINE_STATUS=$?
		set -e
		printf '%s\n' "$BASELINE_STATUS" >"$EVIDENCE_DIR/baseline-corrupt-label.exit-status.txt"
	fi
	kubectl -n "$NS" label secret "$SECRET_NAME" version=999 --overwrite >/dev/null
	kubectl -n "$NS" get secret "$SECRET_NAME" -o json | jq '{metadata:{name:.metadata.name,namespace:.metadata.namespace,labels:.metadata.labels},type:.type}' >"$EVIDENCE_DIR/secret-identity-after.json"
	set +e
	"$SCOUT_BIN" trace deployment/release-probe -n "$NS" --format json >"$EVIDENCE_DIR/trace-corrupt-label.stdout" 2>"$EVIDENCE_DIR/trace-corrupt-label.stderr"
	TRACE_STATUS=$?
	set -e
	printf '%s\n' "$TRACE_STATUS" >"$EVIDENCE_DIR/trace-corrupt-label.exit-status.txt"
	if [[ "$TRACE_STATUS" -eq 0 ]] || ! grep -q 'incomplete or inconsistent identity metadata' "$EVIDENCE_DIR/trace-corrupt-label.stderr"; then
		echo "expected identity mismatch to fail with explicit incomplete/error evidence" >&2
		exit 1
	fi
	if [[ -n "${CUB_SCOUT_BASELINE_BIN:-}" ]]; then
		jq -e --arg ns "$NS" '
		  .summary.ownerType == "Helm" and
		  ([.chain[] | select(.id.kind == "Deployment" and .id.name == "release-probe" and .id.namespace == $ns)] | length) == 1
		' "$EVIDENCE_DIR/baseline-corrupt-label.stdout" >"$EVIDENCE_DIR/baseline-corrupt-label-assertion.txt"
	fi
fi
