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
# Everything kept under OUT goes into a public artifact. Secrets are listed
# through the API's server-side table with metadata only, so their data is
# never fetched. Attach's output is passed through redact(), and every file
# kept is passed through redact_credentials() before the job uploads it.
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

# Credential shapes: a JWT, a ConfigHub key, a password value, a private key.
redact_credentials() {
  sed -E \
    -e 's/eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_.-]+/[redacted-jwt]/g' \
    -e 's/ch_[A-Za-z0-9_-]{8,}/[redacted-key]/g' \
    -e 's/(password[^A-Za-z0-9]{1,4})[^ )"]+/\1[redacted]/Ig' \
    -e 's/-----BEGIN [A-Z ]*PRIVATE KEY-----.*/[redacted-private-key]/g'
}
# Attach's own output also loses any long opaque string. Too broad for the
# recorded objects, whose commit SHAs and IDs it would erase.
redact() {
  redact_credentials | sed -E -e 's/[A-Za-z0-9+\/=_-]{40,}/[redacted]/g'
}

# Secrets are listed through a local proxy so the request can ask for the
# server-side table with metadata only (includeObject=Metadata): the API
# server sends the Type column and each Secret's metadata, never its data.
proxy_port=8199
kubectl proxy --port="$proxy_port" > "$private/proxy.log" 2>&1 &
proxy_pid=$!
# On any exit, a failed step included: stop the proxy, and pass every kept
# file through redact_credentials(), so no credential shape is uploaded.
finish() {
  kill "$proxy_pid" 2>/dev/null || true
  local file
  while IFS= read -r -d '' file; do
    redact_credentials < "$file" > "$file.redacted" && mv "$file.redacted" "$file"
  done < <(find "$out" -type f -print0)
}
trap finish EXIT
for _ in $(seq 1 30); do
  curl -sf "http://127.0.0.1:$proxy_port/version" > /dev/null && break
  sleep 1
done
secrets_by_name() {
  curl -sf -H 'Accept: application/json;as=Table;v=v1;g=meta.k8s.io' \
    "http://127.0.0.1:$proxy_port/api/v1/secrets?includeObject=Metadata" | python3 -c '
import json, sys
table = json.load(sys.stdin)
type_at = [c["name"] for c in table["columnDefinitions"]].index("Type")
rows = []
for row in table["rows"]:
    meta = row["object"]["metadata"]
    labels = meta.get("labels") or {}
    shown = "map[" + " ".join("%s:%s" % kv for kv in sorted(labels.items())) + "]" if labels else "<none>"
    rows.append((meta["namespace"], meta["name"], row["cells"][type_at], shown))
print("%-14s%-30s%-33s%s" % ("NAMESPACE", "NAME", "TYPE", "LABELS"))
for r in sorted(rows):
    print("%-14s%-30s%-33s%s" % r)'
}

# What an attach meets, and what it leaves. Secrets by name and type only.
snapshot() {
  local dir=$1
  mkdir -p "$dir"
  "$scout" map list --json > "$dir/map-list.json" 2> "$dir/map-list.err" || true
  "$scout" doctor --format json > "$dir/doctor.json" 2> "$dir/doctor.err" || true
  "$scout" doctor --before-attach --format json > "$dir/doctor-before-attach.json" 2> "$dir/doctor-before-attach.err" || true
  "$scout" doctor --before-attach > "$dir/doctor-before-attach.txt" 2>&1 || true
  kubectl get namespaces -o name > "$dir/namespaces.txt"
  kubectl get deployments.apps,statefulsets.apps,daemonsets.apps -A \
    -o custom-columns='KIND:.kind,NAMESPACE:.metadata.namespace,NAME:.metadata.name,IMAGES:.spec.template.spec.containers[*].image' \
    > "$dir/workloads.txt"
  secrets_by_name > "$dir/secrets-by-name.txt"
  kubectl get configmap argocd-cm -n argocd -o json | python3 -c '
import json, sys
cm = json.load(sys.stdin)
data = cm.get("data") or {}
key = "resource.customizations.ignoreDifferences.argoproj.io_Application"
print(json.dumps({"keys": sorted(data), key: data.get(key)}, indent=2))' > "$dir/argocd-cm.json"
  local kind
  for kind in applications.argoproj.io applicationsets.argoproj.io \
      kustomizations.kustomize.toolkit.fluxcd.io ocirepositories.source.toolkit.fluxcd.io \
      gitrepositories.source.toolkit.fluxcd.io helmreleases.helm.toolkit.fluxcd.io; do
    # Every kind here is served in this lane: a failed read fails the lane.
    kubectl get "$kind" -A -o json | python3 -c '
import json, sys
items = json.load(sys.stdin)["items"]
for item in items:
    item["metadata"].pop("managedFields", None)
print(json.dumps(items, indent=2, sort_keys=True))' > "$dir/${kind%%.*}.json"
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

# What cub-scout said before each attach (#888), against what attach did.
attach_would() {
  python3 - "$1" "$2" <<'PY'
import json, sys
report = json.load(open(sys.argv[1]))["attach"]
print(next(c["attachWould"] for c in report["controllers"] if c["controller"] == sys.argv[2]))
PY
}
said_before=$(attach_would "$out/before/doctor-before-attach.json" "$controller")
said_after=$(attach_would "$out/after/doctor-before-attach.json" "$controller")
echo "doctor --before-attach said $said_before before the attach (it succeeded) and $said_after before the second (it exited $again_exit)"

# What the brownfield still runs: the same generation, spec and state after
# the attach as before it. These are the facts the report rests on.
failed=0
python3 - "$out/before" "$out/after" <<'PY' || failed=1
import json, sys
before, after = sys.argv[1], sys.argv[2]
def state(item):
    st = item.get("status") or {}
    sync = (st.get("sync") or {}).get("status")
    ready = [c["status"] for c in st.get("conditions") or [] if c.get("type") == "Ready"]
    return (item["metadata"]["generation"], json.dumps(item["spec"], sort_keys=True), sync, ready)
bad = []
for file, ns, name in (("applications.json", "argocd", "guestbook"), ("kustomizations.json", "podinfo", "podinfo")):
    def find(d):
        return next((i for i in json.load(open(f"{d}/{file}")) if i["metadata"]["namespace"] == ns and i["metadata"]["name"] == name), None)
    b, a = find(before), find(after)
    if a is None:
        bad.append(f"{ns}/{name} is gone after attach")
    elif state(b) != state(a):
        bad.append(f"{ns}/{name} changed: {state(b)[0::2]} -> {state(a)[0::2]}")
def legacy(d):
    return [l for l in open(f"{d}/workloads.txt") if " legacy-web " in f" {l} "]
if legacy(before) != legacy(after) or not legacy(after):
    bad.append("legacy-web changed or is gone after attach")
if bad:
    sys.exit("\n".join(bad))
print("guestbook, podinfo and legacy-web: same generation, spec and state after attach")
PY
[[ "$again_exit" != 0 ]] || { echo 'A second attach of a wired cluster succeeded' >&2; failed=1; }
cmp -s <(awk '{print $1}' "$out/spaces-after-attach.txt" | sort) <(sort "$out/spaces-after-second-attach.txt") \
  || { echo 'The refused second attach changed the Space list' >&2; failed=1; }
[[ "$said_before" = use-existing ]] || { echo "doctor --before-attach said $said_before for an installed $controller before attach" >&2; failed=1; }
[[ "$said_after" = refuse ]] || { echo "doctor --before-attach said $said_after for a cluster attach then refused" >&2; failed=1; }
exit "$failed"
