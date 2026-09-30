#!/usr/bin/env bash
# Prepared live protocol only. Do not run without reviewing the README and
# selecting an explicitly disposable environment.
set -euo pipefail

readonly KIND_IMAGE='kindest/node:v1.35.0@sha256:4613778f3cfcd10e615029370f5786704559103cf27bef934597ba562b269661'
readonly KIND_VERSION='v0.31.0'
readonly HELM3_VERSION='v3.22.0+g144ca65'
readonly HELM3_ARCHIVE_SHA='4c9982a6cdeb458b60258df66b55398ca5b19293f6877faffe2909ad6f23dfe0'
readonly HELM3_SHA='8566ea7d76445d174050eca068bc6d685ba3607de8430524a809a16f505a6138'
readonly HELM4_VERSION='v4.1.4+g05fa379'
readonly HELM4_SHA='11e3c9fb6548fa1661a72000a6a483a31f1c2a0bf300f3d2422270feee180d34'
readonly CLUSTER='scout-helm-version-matrix'

usage() {
	echo "usage: HELM3_BIN=/absolute/path/helm-v3.22.0 $0 EMPTY_EVIDENCE_DIR" >&2
}
if [[ $# -ne 1 ]]; then usage; exit 2; fi
EVIDENCE_DIR=$1
HELM3_BIN=${HELM3_BIN:-}
HELM4_BIN=${HELM4_BIN:-$(command -v helm || true)}
KIND_BIN=${KIND_BIN:-$(command -v kind || true)}
KUBECTL_BIN=${KUBECTL_BIN:-$(command -v kubectl || true)}
JQ_BIN=${JQ_BIN:-$(command -v jq || true)}
GO_BIN=${GO_BIN:-$(command -v go || true)}

fail() { echo "error: $*" >&2; exit 1; }
for tool in "$HELM3_BIN" "$HELM4_BIN" "$KIND_BIN" "$KUBECTL_BIN" "$JQ_BIN" "$GO_BIN"; do
	[[ -n "$tool" && -x "$tool" ]] || fail "required executable missing: ${tool:-HELM3_BIN}"
done
[[ -d "$EVIDENCE_DIR" && ! -L "$EVIDENCE_DIR" ]] || fail "evidence directory must be a real directory and already exist"
if find "$EVIDENCE_DIR" -mindepth 1 -print -quit | grep -q .; then
	fail "refusing to overwrite existing evidence: $EVIDENCE_DIR"
fi

sha256() { shasum -a 256 "$1" | awk '{print $1}'; }
[[ "$(sha256 "$HELM3_BIN")" == "$HELM3_SHA" ]] || fail "Helm 3 binary SHA-256 mismatch"
[[ "$(sha256 "$HELM4_BIN")" == "$HELM4_SHA" ]] || fail "Helm 4 binary SHA-256 mismatch"
HELM_PREFLIGHT_HOME="$(mktemp -d "${TMPDIR:-/tmp}/scout-helm-version-check.XXXXXX")"
trap 'rm -rf "$HELM_PREFLIGHT_HOME"' EXIT
mkdir -p "$HELM_PREFLIGHT_HOME"/{helm3,helm4}/{cache,config,data,home}
helm_preflight() {
	local which=$1 binary=$2
	env -i PATH="$PATH" HOME="$HELM_PREFLIGHT_HOME/$which/home" HELM_DRIVER=secret \
		HELM_CACHE_HOME="$HELM_PREFLIGHT_HOME/$which/cache" HELM_CONFIG_HOME="$HELM_PREFLIGHT_HOME/$which/config" \
		HELM_DATA_HOME="$HELM_PREFLIGHT_HOME/$which/data" "$binary" version --short
}
[[ "$(helm_preflight helm3 "$HELM3_BIN" 2>&1)" == "$HELM3_VERSION" ]] || fail "Helm 3 must be $HELM3_VERSION"
[[ "$(helm_preflight helm4 "$HELM4_BIN" 2>&1)" == "$HELM4_VERSION" ]] || fail "Helm 4 must be $HELM4_VERSION"
[[ "$("$KIND_BIN" version 2>&1)" == "kind $KIND_VERSION "* ]] || fail "kind must be $KIND_VERSION"
"$GO_BIN" version >/dev/null 2>&1 || fail "go is required"

# Only the fixed cluster name owned by this script blocks the lane. Other
# clusters are left untouched and are not used by this run.
EXISTING_CLUSTERS=$("$KIND_BIN" get clusters 2>/dev/null) || fail "could not verify kind cluster inventory"
if printf '%s\n' "$EXISTING_CLUSTERS" | grep -Fxq "$CLUSTER"; then
	fail "refusing to reuse owned kind cluster: $CLUSTER"
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
RUN_ID="$(date -u +%Y%m%d-%H%M%S)-$$"
CONTEXT="kind-$CLUSTER"
WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/scout-helm-matrix.XXXXXX")"
KUBECONFIG="$WORK_DIR/kubeconfig"
rm -rf "$HELM_PREFLIGHT_HOME"
trap 'rm -rf "$HELM_PREFLIGHT_HOME" "$WORK_DIR"' EXIT
mkdir -p "$WORK_DIR/helm3"/{cache,config,data,home} "$WORK_DIR/helm4"/{cache,config,data,home} "$WORK_DIR/cub-home"
CREATED=false
CREATE_ATTEMPTED=false
STATUS=0

cleanup() {
	STATUS=$?
	trap - EXIT
	if [[ "$CREATED" == true || "$CREATE_ATTEMPTED" == true ]]; then
		printf 'cleanup: env KUBECONFIG=%q %q delete cluster --name %q --kubeconfig %q\n' "$KUBECONFIG" "$KIND_BIN" "$CLUSTER" "$KUBECONFIG" >>"$EVIDENCE_DIR/commands.log"
		if env KUBECONFIG="$KUBECONFIG" "$KIND_BIN" delete cluster --name "$CLUSTER" --kubeconfig "$KUBECONFIG" >"$EVIDENCE_DIR/kind-delete.stdout" 2>"$EVIDENCE_DIR/kind-delete.stderr"; then
			CREATED=false
			CREATE_ATTEMPTED=false
		else
			echo "cleanup warning: owned kind cluster $CLUSTER could not be deleted; use kind delete cluster --name $CLUSTER" >&2
			STATUS=1
		fi
	fi
	printf '%s\n' "$STATUS" >"$EVIDENCE_DIR/exit-status.txt"
	rm -rf "$WORK_DIR" "$HELM_PREFLIGHT_HOME"
	exit "$STATUS"
}
trap cleanup EXIT

mkdir -p "$EVIDENCE_DIR/chart/templates" "$EVIDENCE_DIR/releases"
cat >"$EVIDENCE_DIR/chart/Chart.yaml" <<'EOF'
apiVersion: v2
name: release-probe
description: Pinned ordinary Deployment install/upgrade matrix.
type: application
version: 0.1.0
appVersion: "3.9"
EOF
cat >"$EVIDENCE_DIR/chart/values.yaml" <<'EOF'
replicaCount: 1
EOF
cat >"$EVIDENCE_DIR/chart/templates/deployment.yaml" <<'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: release-probe
  namespace: {{ .Release.Namespace }}
  labels:
    app.kubernetes.io/name: release-probe
spec:
  replicas: {{ .Values.replicaCount }}
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

cat >"$EVIDENCE_DIR/commands.log" <<EOF
run_id=$RUN_ID
cluster=$CLUSTER
kind_image=$KIND_IMAGE
helm3=$HELM3_BIN ($HELM3_VERSION, sha256:$HELM3_SHA)
helm3_archive_sha256=$HELM3_ARCHIVE_SHA
helm4=$HELM4_BIN ($HELM4_VERSION, sha256:$HELM4_SHA)
helm4_sha256=$HELM4_SHA
source_commit=$(git -C "$REPO_ROOT" rev-parse HEAD)
source_status_artifact=source-status.txt
preflight=kind get clusters (owned name must be absent)
build_cub_scout=(cd "$REPO_ROOT" && go build -o "$WORK_DIR/cub-scout" ./cmd/cub-scout)
chart files:
EOF
shasum -a 256 "$EVIDENCE_DIR/chart/Chart.yaml" "$EVIDENCE_DIR/chart/values.yaml" "$EVIDENCE_DIR/chart/templates/deployment.yaml" >>"$EVIDENCE_DIR/commands.log"
shasum -a 256 "$SCRIPT_DIR/run-helm-version-matrix.sh" >>"$EVIDENCE_DIR/commands.log"
git -C "$REPO_ROOT" status --short >"$EVIDENCE_DIR/source-status.txt"
printf 'version-kind: %q version\nversion-helm3: %q version --short\nversion-helm4: %q version --short\n' \
	"$KIND_BIN" "$HELM3_BIN" "$HELM4_BIN" >>"$EVIDENCE_DIR/commands.log"
	"$GO_BIN" version >"$EVIDENCE_DIR/go-version.txt" 2>&1
"$KIND_BIN" version >"$EVIDENCE_DIR/kind-version.txt" 2>&1
shasum -a 256 "$KIND_BIN" >"$EVIDENCE_DIR/kind-binary.sha256"
"$KUBECTL_BIN" version --client -o yaml >"$EVIDENCE_DIR/kubectl-version.yaml" 2>&1
shasum -a 256 "$KUBECTL_BIN" >"$EVIDENCE_DIR/kubectl-binary.sha256"
env -i PATH="$PATH" HOME="$WORK_DIR/helm3/home" HELM_DRIVER=secret HELM_CACHE_HOME="$WORK_DIR/helm3/cache" \
	HELM_CONFIG_HOME="$WORK_DIR/helm3/config" HELM_DATA_HOME="$WORK_DIR/helm3/data" \
	"$HELM3_BIN" version --template '{{.Version}} {{.GitCommit}} {{.GitTreeState}}' >"$EVIDENCE_DIR/helm3-version.txt"
env -i PATH="$PATH" HOME="$WORK_DIR/helm4/home" HELM_DRIVER=secret HELM_CACHE_HOME="$WORK_DIR/helm4/cache" \
	HELM_CONFIG_HOME="$WORK_DIR/helm4/config" HELM_DATA_HOME="$WORK_DIR/helm4/data" \
	"$HELM4_BIN" version --template '{{.Version}} {{.GitCommit}} {{.GitTreeState}}' >"$EVIDENCE_DIR/helm4-version.txt"
printf '%s\n' "$KIND_IMAGE" >"$EVIDENCE_DIR/kind-image.txt"

record_run() {
	local label=$1; shift
	{
		printf '%s:' "$label"
		printf ' %q' "$@"
		printf '\n'
	} >>"$EVIDENCE_DIR/commands.log"
	"$@"
}
helm_call() {
	local label=$1 version_home=$2; shift 2
	local binary=$1; shift
	record_run "$label" env -i PATH="$PATH" HOME="$WORK_DIR/$version_home/home" KUBECONFIG="$KUBECONFIG" HELM_DRIVER=secret \
		HELM_CACHE_HOME="$WORK_DIR/$version_home/cache" HELM_CONFIG_HOME="$WORK_DIR/$version_home/config" HELM_DATA_HOME="$WORK_DIR/$version_home/data" \
		"$binary" "$@" --kubeconfig "$KUBECONFIG" --kube-context "$CONTEXT"
}
kubectl_call() {
	local label=$1; shift
	record_run "$label" env KUBECONFIG="$KUBECONFIG" "$KUBECTL_BIN" --kubeconfig "$KUBECONFIG" \
		--context "$CONTEXT" "$@"
}
assert_trace() {
	local ns=$1 source=$2 out="$EVIDENCE_DIR/$3"
	local bin="$WORK_DIR/cub-scout"
	if [[ "$source" == plugin ]]; then
		record_run "trace-$ns-plugin" env -i PATH="$PATH" HOME="$WORK_DIR/cub-home" KUBECONFIG="$KUBECONFIG" CUB_PLUGIN=1 "$bin" trace deployment/release-probe -n "$ns" --format json >"$out" 2>"$out.stderr"
	else
		record_run "trace-$ns-standalone" env -i PATH="$PATH" HOME="$WORK_DIR/cub-home" KUBECONFIG="$KUBECONFIG" "$bin" trace deployment/release-probe -n "$ns" --format json >"$out" 2>"$out.stderr"
	fi
	# shellcheck disable=SC2016 # jq program is a literal expression.
	"$JQ_BIN" -e --arg ns "$ns" '
		.summary.ownerType == "Helm" and
		([.chain[]? | select(.id.kind == "Deployment" and .id.name == "release-probe" and .id.namespace == $ns)] | length) == 1
	' "$out" >/dev/null || fail "trace assertion failed for $ns ($source)"
	"$JQ_BIN" -S '{owner:.summary.ownerType, target:.target, chain:(.chain // [])}' "$out" >"$out.projection.json"
}
capture_release() {
	local label=$1 binary=$2 version_home=$3 ns=$4 expected_replicas=$5
	local dir="$EVIDENCE_DIR/releases/$label"
	mkdir -p "$dir"
	kubectl_call "get-$label-deployment" -n "$ns" get deployment release-probe --show-managed-fields=true -o json >"$dir/deployment.raw.json" 2>"$dir/deployment.stderr"
	# shellcheck disable=SC2016 # jq program is a literal expression.
	"$JQ_BIN" -e --arg ns "$ns" --argjson replicas "$expected_replicas" '
		.metadata.name == "release-probe" and .metadata.namespace == $ns and
		(.metadata.uid | type == "string" and length > 0) and
		.spec.replicas == $replicas and .status.readyReplicas == $replicas
	' "$dir/deployment.raw.json" >/dev/null || fail "deployment identity/readiness assertion failed for $label"
	"$JQ_BIN" '{identity:{apiVersion:.apiVersion,kind:.kind,name:.metadata.name,namespace:.metadata.namespace,uid:.metadata.uid},replicas:{desired:.spec.replicas,ready:.status.readyReplicas},managedFields:[.metadata.managedFields[]? | {manager,operation,apiVersion,time,fieldsType,fieldsV1}]}' \
		"$dir/deployment.raw.json" >"$dir/deployment-evidence.json"
	kubectl_call "get-$label-helm-secret-names" -n "$ns" get secrets -l owner=helm,name=release-probe -o name >"$dir/secret-names.txt" 2>"$dir/secret-names.stderr"
	helm_call "history-$label" "$version_home" "$binary" history release-probe -n "$ns" -o json >"$dir/release-history.json" 2>"$dir/release-history.stderr"
	assert_trace "$ns" standalone "${label}-standalone-trace.json"
	assert_trace "$ns" plugin "${label}-plugin-trace.json"
	cmp "$EVIDENCE_DIR/${label}-standalone-trace.json.projection.json" \
		"$EVIDENCE_DIR/${label}-plugin-trace.json.projection.json" || fail "standalone/plugin trace projections differ for $label"
}

printf 'build-cub-scout: (cd %q && %q build -o %q ./cmd/cub-scout)\n' "$REPO_ROOT" "$GO_BIN" "$WORK_DIR/cub-scout" >>"$EVIDENCE_DIR/commands.log"
(cd "$REPO_ROOT" && "$GO_BIN" build -o "$WORK_DIR/cub-scout" ./cmd/cub-scout)
shasum -a 256 "$WORK_DIR/cub-scout" >"$EVIDENCE_DIR/cub-scout-binary.sha256"
CREATE_ATTEMPTED=true
	record_run create-cluster env KUBECONFIG="$KUBECONFIG" "$KIND_BIN" create cluster --name "$CLUSTER" --image "$KIND_IMAGE" --kubeconfig "$KUBECONFIG" --wait 5m >"$EVIDENCE_DIR/kind-create.stdout" 2>"$EVIDENCE_DIR/kind-create.stderr"
CREATED=true
mkdir -p "$WORK_DIR/helm3"/{cache,config,data} "$WORK_DIR/helm4"/{cache,config,data}

# Case 1: fresh Helm 3 install (default client-side behavior).
helm_call install-helm3-fresh helm3 "$HELM3_BIN" install release-probe "$EVIDENCE_DIR/chart" -n matrix-h3 --create-namespace --wait --timeout 3m >"$EVIDENCE_DIR/install-helm3-fresh.stdout" 2>"$EVIDENCE_DIR/install-helm3-fresh.stderr"
capture_release fresh-helm3 "$HELM3_BIN" helm3 matrix-h3 1

# Case 2: fresh Helm 4 install with pinned Helm 4 defaults.
helm_call install-helm4-fresh helm4 "$HELM4_BIN" install release-probe "$EVIDENCE_DIR/chart" -n matrix-h4 --create-namespace --wait --timeout 3m >"$EVIDENCE_DIR/install-helm4-fresh.stdout" 2>"$EVIDENCE_DIR/install-helm4-fresh.stderr"
capture_release fresh-helm4 "$HELM4_BIN" helm4 matrix-h4 1

# Case 3: Helm 3 install, then Helm 4 explicitly requests auto on upgrade.
helm_call install-helm3-upgrade-case helm3 "$HELM3_BIN" install release-probe "$EVIDENCE_DIR/chart" -n matrix-h3-to-h4 --create-namespace --wait --timeout 3m >"$EVIDENCE_DIR/install-helm3-upgrade-case.stdout" 2>"$EVIDENCE_DIR/install-helm3-upgrade-case.stderr"
capture_release pre-upgrade-helm3 "$HELM3_BIN" helm3 matrix-h3-to-h4 1
helm_call upgrade-helm4-auto helm4 "$HELM4_BIN" upgrade release-probe "$EVIDENCE_DIR/chart" -n matrix-h3-to-h4 --set replicaCount=2 --server-side=auto --wait --timeout 3m >"$EVIDENCE_DIR/upgrade-helm4-auto.stdout" 2>"$EVIDENCE_DIR/upgrade-helm4-auto.stderr"
capture_release upgraded-helm4-auto "$HELM4_BIN" helm4 matrix-h3-to-h4 2
BEFORE_UID=$("$JQ_BIN" -r '.identity.uid' "$EVIDENCE_DIR/releases/pre-upgrade-helm3/deployment-evidence.json")
AFTER_UID=$("$JQ_BIN" -r '.identity.uid' "$EVIDENCE_DIR/releases/upgraded-helm4-auto/deployment-evidence.json")
[[ -n "$BEFORE_UID" && "$BEFORE_UID" != null && "$BEFORE_UID" == "$AFTER_UID" ]] || fail "Helm 4 upgrade replaced the Deployment instead of updating the same identity"

SOURCE_DIRTY=false
if [[ -n "$(git -C "$REPO_ROOT" status --short)" ]]; then SOURCE_DIRTY=true; fi
cat >"$EVIDENCE_DIR/summary.json" <<EOF
{
  "status": "completed",
  "cluster": "$CLUSTER",
  "context": "$CONTEXT",
  "kindImage": "$KIND_IMAGE",
  "helm3Version": "$HELM3_VERSION",
  "helm3ArchiveSha256": "$HELM3_ARCHIVE_SHA",
  "helm3BinarySha256": "$HELM3_SHA",
  "helm4Version": "$HELM4_VERSION",
  "helm4BinarySha256": "$HELM4_SHA",
  "kindVersion": "$KIND_VERSION",
  "sourceCommit": "$(git -C "$REPO_ROOT" rev-parse HEAD)",
  "sourceDirty": $SOURCE_DIRTY,
  "coverage": ["fresh-helm3-install", "fresh-helm4-default-install", "helm3-install-then-helm4-upgrade-server-side-auto"],
  "notCovered": ["hooks", "CRDs", "rollback", "server-side-conflict-modes"],
  "managedFieldsInterpretation": "recorded observations only; manager metadata alone does not establish apply method"
}
EOF
