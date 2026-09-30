#!/usr/bin/env bash
# Capture a real, bounded Flux/Kubernetes object set for HLT-02.
# This script creates and deletes only its own named disposable kind cluster.
set -Eeuo pipefail

readonly CLUSTER='scout-hlt02-ready-without-health'
readonly NODE_IMAGE='kindest/node:v1.35.0@sha256:4613778f3cfcd10e615029370f5786704559103cf27bef934597ba562b269661'
readonly FLUX_VERSION='2.8.6'
readonly SOURCE_REPOSITORY='https://github.com/confighub/cub-scout'
readonly SOURCE_COMMIT='7732dde28be8cf8c42c096d94efbd8ce4a9d0a19'
readonly SOURCE_PATH='./examples/combined-git-live/git-repo/apps/payment-worker/base'
readonly FIXTURE_NAMESPACE='scout-hlt02'
readonly DEPLOYMENT_NAME='payment-worker'
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
readonly SCRIPT_DIR

usage() {
  echo "Usage: $0 --execute OUTPUT_DIR" >&2
  echo 'Creates a fresh owned kind cluster, captures raw evidence, then deletes it.' >&2
}

if [[ $# -ne 2 || $1 != '--execute' || -z ${2//[[:space:]]/} ]]; then
  usage
  exit 2
fi

OUT=$2
case "$OUT" in
  -*|'') echo 'OUTPUT_DIR must be a non-empty path, not an option.' >&2; exit 2 ;;
esac
mkdir -p "$(dirname "$OUT")"
mkdir "$OUT" || { echo "Refusing to overwrite output directory: $OUT" >&2; exit 2; }
chmod 700 "$OUT"

for tool in kind docker kubectl flux jq python3; do
  command -v "$tool" >/dev/null || { echo "Required command not found: $tool" >&2; exit 2; }
done

if command -v sha256sum >/dev/null; then
  sha256() { sha256sum "$1" | awk '{print $1}'; }
elif command -v shasum >/dev/null; then
  sha256() { shasum -a 256 "$1" | awk '{print $1}'; }
else
  echo 'Required command not found: sha256sum or shasum.' >&2
  exit 2
fi

run_bounded() {
  local seconds=$1
  shift
  python3 - "$seconds" "$@" <<'PY'
import subprocess
import sys

limit = float(sys.argv[1])
command = sys.argv[2:]
try:
    result = subprocess.run(command, timeout=limit, check=False)
except subprocess.TimeoutExpired:
    print(f"Timed out after {limit:g}s: {command[0]}", file=sys.stderr)
    raise SystemExit(124)
raise SystemExit(result.returncode)
PY
}

kind_version=$(run_bounded 15 kind version)
[[ $kind_version == *'v0.31.0'* ]] || { echo "Need kind v0.31.0; got: $kind_version" >&2; exit 2; }
flux_version=$(run_bounded 15 flux --version)
[[ $flux_version == *"$FLUX_VERSION"* ]] || { echo "Need Flux CLI $FLUX_VERSION; got: $flux_version" >&2; exit 2; }
run_bounded 15 docker image inspect "$NODE_IMAGE" >/dev/null 2>&1 || {
  echo "Pinned node image is not already cached locally: $NODE_IMAGE" >&2
  exit 2
}

# Never adopt or delete an existing cluster, including one created by another run.
existing_clusters=$(run_bounded 15 kind get clusters) || { echo 'Cannot verify existing kind clusters; refusing to create.' >&2; exit 2; }
if awk -v name="$CLUSTER" '$0 == name { found=1 } END { exit !found }' <<< "$existing_clusters"; then
  echo "Refusing to reuse existing kind cluster '$CLUSTER'." >&2
  exit 2
fi

tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/scout-hlt02.XXXXXX")
chmod 700 "$tmp_dir"
private_kubeconfig="$tmp_dir/kubeconfig"
lock_dir="${TMPDIR:-/tmp}/scout-hlt02-ready-without-health.lock"
mkdir "$lock_dir" 2>/dev/null || { echo "Capture lock exists at $lock_dir; refusing concurrent or stale run." >&2; rm -rf "$tmp_dir"; exit 2; }
chmod 700 "$lock_dir"
create_attempted=0
cleanup() {
  status=$?
  trap - EXIT INT TERM
  if [[ $create_attempted -eq 1 ]]; then
    cluster_list=$(run_bounded 15 kind get clusters 2>/dev/null) || { echo 'Cannot verify owned cluster during cleanup.' >&2; cluster_list=''; status=1; }
    if awk -v name="$CLUSTER" '$0 == name { found=1 } END { exit !found }' <<< "$cluster_list"; then
      # Deliberately use the same private kubeconfig for deletion as creation.
      run_bounded 60 kind delete cluster --name "$CLUSTER" --kubeconfig "$private_kubeconfig" >/dev/null || status=1
      remaining=$(run_bounded 15 kind get clusters 2>/dev/null) || { echo 'Cannot verify cluster cleanup.' >&2; remaining=''; status=1; }
      if awk -v name="$CLUSTER" '$0 == name { found=1 } END { exit !found }' <<< "$remaining"; then
        echo "Owned kind cluster '$CLUSTER' remains after cleanup." >&2
        status=1
      fi
    fi
  fi
  rmdir "$lock_dir" 2>/dev/null || status=1
  rm -rf "$tmp_dir"
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# Do not let inherited kubeconfig selection affect any operation.
unset KUBECONFIG
create_attempted=1
run_bounded 150 kind create cluster --name "$CLUSTER" --image "$NODE_IMAGE" --kubeconfig "$private_kubeconfig" --wait 120s
chmod 600 "$private_kubeconfig"

kubectl_owned() {
  local limit=45 arg requested
  for arg in "$@"; do
    if [[ $arg =~ ^--timeout=([0-9]+)s$ ]]; then
      requested=${BASH_REMATCH[1]}
      limit=$((requested + 30))
    fi
  done
  run_bounded "$limit" kubectl --kubeconfig "$private_kubeconfig" --context "kind-$CLUSTER" --request-timeout=30s "$@"
}

started_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
printf '%s\n' "$kind_version" > "$OUT/kind-version.txt"
printf '%s\n' "$flux_version" > "$OUT/flux-cli-version.txt"
printf '%s\n' "$SOURCE_COMMIT" > "$OUT/source-commit.txt"
printf '%s\n' "$SOURCE_REPOSITORY" > "$OUT/source-repository.txt"
printf '%s\n' "$SOURCE_PATH" > "$OUT/source-path.txt"

run_bounded 20 flux install --version="$FLUX_VERSION" --components=source-controller,kustomize-controller --export > "$OUT/flux-install.yaml"
flux_manifest_sha=$(sha256 "$OUT/flux-install.yaml")
kubectl_owned apply -f "$OUT/flux-install.yaml"
kubectl_owned wait --for=condition=Available deployment/source-controller deployment/kustomize-controller -n flux-system --timeout=180s

kubectl_owned create namespace "$FIXTURE_NAMESPACE"
cp "$SCRIPT_DIR/gitrepository.yaml" "$OUT/gitrepository.yaml"
cp "$SCRIPT_DIR/kustomization.yaml" "$OUT/kustomization.yaml"
kubectl_owned apply -f "$OUT/gitrepository.yaml"
kubectl_owned apply -f "$OUT/kustomization.yaml"
kubectl_owned wait --for=condition=Ready kustomization/apps -n flux-system --timeout=240s

# Wait only for the bounded workload evidence to exist; never wait for it to be healthy.
deadline=$((SECONDS + 180))
while :; do
  if kubectl_owned get deployment "$DEPLOYMENT_NAME" -n "$FIXTURE_NAMESPACE" -o json > "$tmp_dir/deployment-probe.json" 2>/dev/null \
    && jq -e '.metadata.uid and .status.observedGeneration == .metadata.generation and (.spec.replicas // 1) == 1 and (.status.availableReplicas // 0) == 0 and (.status.unavailableReplicas // 0) >= 1' "$tmp_dir/deployment-probe.json" >/dev/null; then
    break
  fi
  (( SECONDS < deadline )) || { echo 'Timed out waiting for current-generation unavailable Deployment evidence.' >&2; exit 1; }
  sleep 2
done

kubectl_owned get gitrepository scout-hlt02-source -n flux-system --show-managed-fields=true -o json > "$OUT/gitrepository.json"
kubectl_owned get kustomization apps -n flux-system --show-managed-fields=true -o json > "$OUT/kustomization-start.json"
kubectl_owned get deployment "$DEPLOYMENT_NAME" -n "$FIXTURE_NAMESPACE" --show-managed-fields=true -o json > "$OUT/deployment.json"
deadline=$((SECONDS + 120))
while :; do
  if kubectl_owned get pods -n "$FIXTURE_NAMESPACE" -l app.kubernetes.io/name=payment-worker -o json > "$tmp_dir/pods-probe.json" 2>/dev/null \
    && jq -e --arg uid "$(jq -r '.metadata.uid' "$OUT/deployment.json")" 'any(.items[]; any(.metadata.ownerReferences[]?; .uid == $uid))' "$tmp_dir/pods-probe.json" >/dev/null; then
    break
  fi
  (( SECONDS < deadline )) || { echo 'Timed out waiting for Pod evidence owned by the fixture Deployment.' >&2; exit 1; }
  sleep 2
done
kubectl_owned get pods -n "$FIXTURE_NAMESPACE" -l app.kubernetes.io/name=payment-worker --show-managed-fields=true -o json > "$OUT/pods.json"

git_revision=$(jq -r '.status.artifact.revision // empty' "$OUT/gitrepository.json")
applied_revision=$(jq -r '.status.lastAppliedRevision // empty' "$OUT/kustomization-start.json")
ready=$(jq -r '[.status.conditions[]? | select(.type == "Ready" and .status == "True")][0] != null' "$OUT/kustomization-start.json")
generation=$(jq -r '.metadata.generation // 0' "$OUT/kustomization-start.json")
observed_generation=$(jq -r '.status.observedGeneration // 0' "$OUT/kustomization-start.json")
[[ $git_revision == *"$SOURCE_COMMIT"* && $applied_revision == "$git_revision" ]] || { echo 'GitRepository and applied Kustomization revisions do not match the pinned commit.' >&2; exit 1; }
[[ $ready == true && $generation == "$observed_generation" ]] || { echo 'Kustomization is not Ready for its current generation.' >&2; exit 1; }
jq -e '(.spec.wait == false) and ((.spec.healthChecks // []) | length == 0) and ((.spec.healthCheckExprs // []) | length == 0)' "$OUT/kustomization-start.json" >/dev/null || {
  echo 'Captured Kustomization does not prove wait=false and absent/empty health checks.' >&2; exit 1;
}
jq -e --arg commit "$SOURCE_COMMIT" '.status.artifact.revision | contains($commit)' "$OUT/gitrepository.json" >/dev/null || { echo 'GitRepository artifact revision does not contain the full pinned commit.' >&2; exit 1; }
# Validate workload identity/status and Flux ownership labels.
jq -e '.spec.replicas == 1 and .status.observedGeneration == .metadata.generation and (.status.availableReplicas // 0) == 0 and (.status.unavailableReplicas // 0) >= 1 and .metadata.labels["kustomize.toolkit.fluxcd.io/name"] == "apps" and .metadata.labels["kustomize.toolkit.fluxcd.io/namespace"] == "flux-system"' "$OUT/deployment.json" >/dev/null || {
  echo 'Captured Deployment is not current-generation, Flux-owned, and unavailable.' >&2; exit 1;
}
deployment_uid=$(jq -r '.metadata.uid' "$OUT/deployment.json")
jq -e --arg uid "$deployment_uid" 'any(.items[]; any(.metadata.ownerReferences[]?; .uid == $uid))' "$OUT/pods.json" >/dev/null || {
  echo 'No captured Pod is owned by the selected Deployment.' >&2; exit 1;
}

kubectl_owned get pods -n flux-system -l app=source-controller --show-managed-fields=true -o json > "$OUT/source-controller-pods.json"
kubectl_owned get pods -n flux-system -l app=kustomize-controller --show-managed-fields=true -o json > "$OUT/kustomize-controller-pods.json"
jq -s '[.[] | .items[]? as $pod | {pod: $pod.metadata.name, containers: [$pod.spec.containers[]? as $container | {name: $container.name, image: $container.image, imageID: ([$pod.status.containerStatuses[]? | select(.name == $container.name) | .imageID][0] // null)}]}]' \
  "$OUT/source-controller-pods.json" "$OUT/kustomize-controller-pods.json" > "$OUT/controller-images.json"
kubectl_owned get kustomization apps -n flux-system --show-managed-fields=true -o json > "$OUT/kustomization-end.json"
start_uid=$(jq -r '.metadata.uid' "$OUT/kustomization-start.json")
end_uid=$(jq -r '.metadata.uid' "$OUT/kustomization-end.json")
end_generation=$(jq -r '.metadata.generation // 0' "$OUT/kustomization-end.json")
end_revision=$(jq -r '.status.lastAppliedRevision // empty' "$OUT/kustomization-end.json")
[[ $start_uid == "$end_uid" && $generation == "$end_generation" && $applied_revision == "$end_revision" ]] || {
  echo 'Kustomization identity/generation/revision changed during the capture interval.' >&2; exit 1;
}
jq -e --arg generation "$generation" '.status.observedGeneration == ($generation | tonumber) and any(.status.conditions[]?; .type == "Ready" and .status == "True")' "$OUT/kustomization-end.json" >/dev/null || {
  echo 'Kustomization was no longer Ready for its captured generation at capture end.' >&2; exit 1;
}
ended_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
cat > "$OUT/provenance.json" <<EOF
{
  "format": "flux-hlt02-raw-capture/v1",
  "captureStartedAt": "$started_at",
  "captureEndedAt": "$ended_at",
  "atomicSnapshot": false,
  "cluster": "$CLUSTER",
  "kindVersion": $(jq -Rn --arg value "$kind_version" '$value'),
  "nodeImage": "$NODE_IMAGE",
  "fluxCLI": $(jq -Rn --arg value "$flux_version" '$value'),
  "fluxInstallManifestSHA256": "$flux_manifest_sha",
  "captureScriptSHA256": "$(sha256 "$SCRIPT_DIR/capture.sh")",
  "gitRepositoryManifestSHA256": "$(sha256 "$OUT/gitrepository.yaml")",
  "kustomizationManifestSHA256": "$(sha256 "$OUT/kustomization.yaml")",
  "sourceRepository": "$SOURCE_REPOSITORY",
  "sourceCommit": "$SOURCE_COMMIT",
  "sourcePath": "$SOURCE_PATH",
  "gitRepositoryArtifactRevision": $(jq -Rn --arg value "$git_revision" '$value'),
  "kustomizationAppliedRevision": $(jq -Rn --arg value "$applied_revision" '$value'),
  "kustomizationUID": "$start_uid",
  "kustomizationGeneration": $generation,
  "deploymentUID": "$deployment_uid",
  "objects": {
    "gitrepository.json": "$(sha256 "$OUT/gitrepository.json")",
    "kustomization-start.json": "$(sha256 "$OUT/kustomization-start.json")",
    "deployment.json": "$(sha256 "$OUT/deployment.json")",
    "pods.json": "$(sha256 "$OUT/pods.json")",
    "controller-images.json": "$(sha256 "$OUT/controller-images.json")",
    "kustomization-end.json": "$(sha256 "$OUT/kustomization-end.json")"
  },
  "excluded": ["Secrets", "kubeconfig", "credentials"]
}
EOF
jq -e . "$OUT/provenance.json" >/dev/null
echo "Captured HLT-02 raw evidence in $OUT (private cluster scheduled for deletion on exit)."
