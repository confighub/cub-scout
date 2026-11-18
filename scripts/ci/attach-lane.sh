#!/usr/bin/env bash
# Record a brownfield cluster before and after a real `cub cluster attach`
# (#888), against the disposable ConfigHub that setup-connected-server.sh made.
#
# The cluster already runs Argo CD and Flux, each with an app, and a
# Deployment applied by hand. The lane records what is there, attaches the
# cluster through CONTROLLER with --no-install and --prune, records again,
# and then attaches it a second time under another name to record what attach
# says to a cluster that is already wired. A pre-attach report is designed
# from these recordings, not from reading cub's source.
#
# Everything kept under OUT goes into a public artifact. Secrets are read by
# name and type only, never their data, and command output is passed through
# redact() first.
set -euo pipefail
umask 077

root=${1:?Usage: attach-lane.sh CONNECTED_ROOT OUT_DIR SCOUT_BINARY KIND_CLUSTER [argo|flux]}
out=${2:?Expected an output directory}
scout=${3:?Expected the cub-scout binary}
kind_cluster=${4:?Expected the kind cluster to attach}
controller=${5:-argo}
[[ "$controller" = argo || "$controller" = flux ]] || { echo 'Expected argo or flux' >&2; exit 1; }
[[ -f "$root/owns-server-cluster" && -x "$scout" ]] || {
  echo 'Expected the disposable server root and an executable cub-scout' >&2; exit 1;
}
: "${CUB_CONFIG:?Expected the disposable server cub config}"
: "${KUBECONFIG:?Expected the workload kubeconfig}"
here=$(cd "$(dirname "$0")" && pwd)
fixtures="$here/../../test/e2e/fixtures/attach-lane"
name=scout-attach
private="$root/attach-lane"
mkdir -p "$out" "$private"

redact() {
  sed -E \
    -e 's/eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_.-]+/[redacted-jwt]/g' \
    -e 's/ch_[A-Za-z0-9_-]{8,}/[redacted-key]/g' \
    -e 's/(password[^A-Za-z0-9]{1,4})[^ )"]+/\1[redacted]/Ig' \
    -e 's/[A-Za-z0-9+\/=_-]{40,}/[redacted]/g'
}

# What an attach meets, and what it leaves. Secrets by name and type only.
snapshot() {
  local dir=$1
  mkdir -p "$dir"
  "$scout" map list --json > "$dir/map-list.json" 2> "$dir/map-list.err" || true
  "$scout" doctor --format json > "$dir/doctor.json" 2> "$dir/doctor.err" || true
  kubectl get namespaces -o name > "$dir/namespaces.txt"
  kubectl get deployments.apps,statefulsets.apps,daemonsets.apps -A \
    -o custom-columns='KIND:.kind,NAMESPACE:.metadata.namespace,NAME:.metadata.name,IMAGES:.spec.template.spec.containers[*].image' \
    > "$dir/workloads.txt"
  kubectl get secrets -A \
    -o custom-columns='NAMESPACE:.metadata.namespace,NAME:.metadata.name,TYPE:.type,LABELS:.metadata.labels' \
    > "$dir/secrets-by-name.txt"
  kubectl get configmap argocd-cm -n argocd -o json 2>/dev/null | python3 -c '
import json, sys
cm = json.load(sys.stdin)
data = cm.get("data") or {}
key = "resource.customizations.ignoreDifferences.argoproj.io_Application"
print(json.dumps({"keys": sorted(data), key: data.get(key)}, indent=2))' > "$dir/argocd-cm.json" || true
  local kind
  for kind in applications.argoproj.io applicationsets.argoproj.io \
      kustomizations.kustomize.toolkit.fluxcd.io ocirepositories.source.toolkit.fluxcd.io \
      gitrepositories.source.toolkit.fluxcd.io helmreleases.helm.toolkit.fluxcd.io; do
    kubectl get "$kind" -A -o json 2>/dev/null | python3 -c '
import json, sys
items = json.load(sys.stdin)["items"]
for item in items:
    item["metadata"].pop("managedFields", None)
print(json.dumps(items, indent=2, sort_keys=True))' > "$dir/${kind%%.*}.json" || true
  done
  kubectl auth can-i create clusterroles > "$dir/can-i-create-clusterroles.txt" 2>&1 || true
}

wait_for() {
  local what=$1; shift
  for _ in $(seq 1 60); do
    if "$@" >/dev/null 2>&1; then return 0; fi
    sleep 5
  done
  echo "Timed out waiting for $what" >&2
  return 1
}

# The brownfield: an app under each controller, and one applied by hand.
kubectl apply -f "$fixtures/brownfield.yaml"
wait_for 'guestbook to sync' bash -c \
  '[[ $(kubectl get application guestbook -n argocd -o jsonpath="{.status.sync.status}/{.status.health.status}") = Synced/Healthy ]]'
kubectl wait --for=condition=Ready kustomization/podinfo -n podinfo --timeout=300s
kubectl rollout status deployment/legacy-web -n legacy --timeout=300s
kubectl rollout status deployment/podinfo -n podinfo --timeout=300s

snapshot "$out/before"

# The attach under test.
set +e
cub cluster attach "$name" --provider kind --cluster "$kind_cluster" \
  --controller "$controller" --no-install --prune > "$private/attach.log" 2>&1
attach_exit=$?
set -e
redact < "$private/attach.log" > "$out/attach.log"
echo "$attach_exit" > "$out/attach.exit"
echo "cub cluster attach exited $attach_exit; the end of its output:"
tail -n 25 "$out/attach.log"
[[ "$attach_exit" = 0 ]] || exit 1

# The root syncs on its own; wait for it before reading what it changed.
if [[ "$controller" = argo ]]; then
  wait_for 'the root Application to sync' bash -c \
    "[[ \$(kubectl get application '$name-argo-apps' -n argocd -o jsonpath='{.status.sync.status}') = Synced ]]" || true
else
  kubectl wait --for=condition=Ready kustomization/confighub-root -n flux-system --timeout=300s || true
fi

snapshot "$out/after"

cub space list -o json > "$private/space-list.json"
python3 - "$private/space-list.json" > "$out/spaces-after-attach.txt" <<'PY'
import json, sys
for item in json.load(open(sys.argv[1])):
    space = item["Space"]
    print(space["Slug"], json.dumps(space.get("Annotations") or {}, sort_keys=True))
PY

# The same cluster, attached again under another name. Attach is expected to
# refuse; what it says, and whether it leaves anything behind, is recorded.
set +e
cub cluster attach "$name-again" --provider kind --cluster "$kind_cluster" \
  --controller "$controller" --no-install > "$private/attach-again.log" 2>&1
again_exit=$?
set -e
redact < "$private/attach-again.log" > "$out/attach-again.log"
echo "$again_exit" > "$out/attach-again.exit"
echo "A second attach exited $again_exit; the end of its output:"
tail -n 15 "$out/attach-again.log"
cub space list -o json > "$private/space-list-again.json"
python3 - "$private/space-list-again.json" > "$out/spaces-after-second-attach.txt" <<'PY'
import json, sys
for item in json.load(open(sys.argv[1])):
    print(item["Space"]["Slug"])
PY

# What the brownfield still runs. The attach is recorded either way; these
# are the facts a pre-attach report would rest on.
failed=0
kubectl get application guestbook -n argocd -o name || { echo 'guestbook Application is gone after attach' >&2; failed=1; }
kubectl get kustomization podinfo -n podinfo -o name || { echo 'podinfo Kustomization is gone after attach' >&2; failed=1; }
kubectl get deployment legacy-web -n legacy -o name || { echo 'legacy-web is gone after attach' >&2; failed=1; }
[[ "$again_exit" != 0 ]] || { echo 'A second attach of a wired cluster succeeded' >&2; failed=1; }
exit "$failed"
