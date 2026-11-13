#!/usr/bin/env bash
# Prove the live-status cluster check (#887) against a real attached cluster.
#
# `cub cluster up` creates a kind cluster, installs Flux or Argo CD and argobot in it and
# attaches it to the disposable ConfigHub that setup-connected-server.sh made.
# argobot records a LiveStatus on each Release; cub-scout then compares that
# record with the controller's objects in the same cluster.
#
# Everything read is kept under OUT for the run's artifact. Command output can
# carry tokens of the disposable instance, so what reaches the job log is passed
# through redact() first.
set -euo pipefail
umask 077

root=${1:?Usage: live-status-proof.sh CONNECTED_ROOT OUT_DIR SCOUT_BINARY [flux|argo|sveltos]}
out=${2:?Expected an output directory}
scout=${3:?Expected the cub-scout binary}
controller=${4:-flux}
[[ "$controller" = flux || "$controller" = argo || "$controller" = sveltos ]] || { echo 'Expected flux, argo or sveltos' >&2; exit 1; }
[[ -f "$root/owns-server-cluster" && -x "$scout" ]] || {
  echo 'Expected the disposable server root and an executable cub-scout' >&2; exit 1;
}
: "${CUB_CONFIG:?Expected the disposable server cub config}"
mkdir -p "$out"
name=scout-live
private="$root/live-status"
mkdir -p "$private"

redact() {
  sed -E \
    -e 's/eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_.-]+/[redacted-jwt]/g' \
    -e 's/ch_[A-Za-z0-9_-]{8,}/[redacted-key]/g' \
    -e 's/[A-Za-z0-9+\/=_-]{40,}/[redacted]/g'
}

touch "$private/owns-live-cluster"
if ! cub cluster up "$name" --controller "$controller" > "$private/cluster-up.log" 2>&1; then
  echo 'cub cluster up failed; the end of its output, redacted:' >&2
  tail -n 40 "$private/cluster-up.log" | redact >&2
  exit 1
fi
tail -n 15 "$private/cluster-up.log" | redact

kubeconfig="$CUB_CONFIG/clusters/$name.kubeconfig"
[[ -s "$kubeconfig" ]] || { echo "cub cluster up left no kubeconfig at clusters/$name.kubeconfig" >&2; ls "$CUB_CONFIG/clusters" >&2 || true; exit 1; }

cub space list -o json > "$private/space-list.json"
python3 - "$private/space-list.json" > "$out/spaces.txt" <<'PY'
import json, sys
for item in json.load(open(sys.argv[1])):
    print(item["Space"]["Slug"])
PY
echo 'Spaces after cluster up:'; cat "$out/spaces.txt"

# Wait for a Release whose LiveStatus says Synced: argobot writes one as soon
# as the controller starts on it ("Provisioning" under Sveltos, seen in run
# 43403750799) and again once it has reconciled. Ten minutes, since images are
# pulled cold.
found=''
for _ in $(seq 1 60); do
  : > "$out/release-list.json"
  cub release list --space '*' -o json > "$out/release-list.json" 2>"$private/release-list.err" || true
  if python3 - "$out/release-list.json" <<'PY'
import json, sys
try:
    items = json.load(open(sys.argv[1]))
except Exception:
    sys.exit(1)
sys.exit(0 if any(((i.get("Release") or {}).get("LiveStatus") or {}).get("Sync") == "Synced" for i in items) else 1)
PY
  then found=yes; break; fi
  sleep 10
done

if [[ "$controller" = flux ]]; then
  KUBECONFIG="$kubeconfig" kubectl get ocirepositories.source.toolkit.fluxcd.io,kustomizations.kustomize.toolkit.fluxcd.io -A -o json > "$out/flux-objects.json" 2>"$private/flux.err" || true
elif [[ "$controller" = argo ]]; then
  KUBECONFIG="$kubeconfig" kubectl get applications.argoproj.io -A -o json > "$out/argo-objects.json" 2>"$private/argo.err" || true
else
  KUBECONFIG="$kubeconfig" kubectl api-resources -o name > "$out/api-resources.txt" 2>"$private/api-resources.err" || true
  KUBECONFIG="$kubeconfig" kubectl get clusterprofiles.config.projectsveltos.io,profiles.config.projectsveltos.io,clustersummaries.config.projectsveltos.io,clusterconfigurations.config.projectsveltos.io,clusterreports.config.projectsveltos.io -A -o json > "$out/sveltos-objects.json" 2>"$private/sveltos.err" || true
  [[ -s "$private/sveltos.err" ]] && redact < "$private/sveltos.err" >&2
fi
KUBECONFIG="$kubeconfig" kubectl get deployments -A -o wide > "$out/deployments.txt" 2>&1 || true
curl --fail --silent --show-error --max-time 10 "$(cub context get -o json | python3 -c 'import json,sys; print(json.load(sys.stdin)["coordinate"]["serverURL"])')/api/info" > "$out/api-info.json" 2>/dev/null || true

KUBECONFIG="$kubeconfig" CUB_SCOUT_TELEMETRY='' "$scout" gitops status --with-confighub --confighub-space '*' --format json > "$out/gitops-status.json" 2>"$out/gitops-status.err" || {
  echo 'cub-scout gitops status failed:' >&2; redact < "$out/gitops-status.err" >&2; exit 1;
}
KUBECONFIG="$kubeconfig" "$scout" gitops status --with-confighub --confighub-space '*' > "$out/gitops-status.txt" 2>/dev/null || true

[[ -n "$found" ]] || { echo 'No Release carried a Synced LiveStatus within ten minutes; see the artifact' >&2; exit 1; }

python3 - "$out/gitops-status.json" "$controller" <<'PY'
import json, sys
report = json.load(open(sys.argv[1]))
def walk(value):
    if isinstance(value, dict):
        if isinstance(value.get("liveStatuses"), list):
            yield from value["liveStatuses"]
        for inner in value.values():
            yield from walk(inner)
    elif isinstance(value, list):
        for inner in value:
            yield from walk(inner)
statuses = [s for s in walk(report) if s.get("recordedOn") == "release"]
for s in statuses:
    check = s.get("clusterCheck") or {}
    health = check.get("health") or {}
    print(s.get("space"), s.get("release"), "sync=%s" % s.get("syncStatus"), "health=%s" % s.get("healthStatus"), "matchedBy=%s" % check.get("matchedBy"), "check=%s" % check.get("state"), check.get("reason", ""))
    print("   health check=%s" % health.get("state"), "listed=%s ready=%s" % (health.get("workloadsListed"), health.get("workloadsReady")), health.get("reason", ""))
if not statuses:
    sys.exit("cub-scout reported no live status recorded on a Release")
checks = [s.get("clusterCheck") or {} for s in statuses]
if sys.argv[2] == "sveltos":
    # Sveltos reports no digest for what it applied: the honest answer is
    # that the cluster cannot say, never that it agrees.
    if any(c.get("state") != "not-observable" for c in checks):
        sys.exit("a status delivered by Sveltos was compared, though Sveltos reports no digest")
    if not any("Sveltos" in c.get("reason", "") for c in checks):
        sys.exit("no answer named the Sveltos ClusterProfile as the reason nothing was compared")
    print("Sveltos: the ClusterProfile is named, and nothing is compared")
    sys.exit(0)
if not any(c.get("state") == "agrees" for c in checks):
    sys.exit("no recorded live status was found to agree with the cluster")
if not any(c.get("state") == "agrees" and (c.get("health") or {}).get("state") == "agrees" for c in checks):
    sys.exit("no recorded health was found to agree with the workloads the controller lists")
print("A recorded live status agrees with the controllers in the cluster, and its health with the workloads they list")
PY
