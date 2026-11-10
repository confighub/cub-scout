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

root=${1:?Usage: live-status-proof.sh CONNECTED_ROOT OUT_DIR SCOUT_BINARY [flux|argo]}
out=${2:?Expected an output directory}
scout=${3:?Expected the cub-scout binary}
controller=${4:-flux}
[[ "$controller" = flux || "$controller" = argo ]] || { echo 'Expected flux or argo' >&2; exit 1; }
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

# Wait for a Release that carries a LiveStatus: argobot writes it once Flux
# has reconciled. Ten minutes, since images are pulled cold.
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
sys.exit(0 if any((i.get("Release") or {}).get("LiveStatus") for i in items) else 1)
PY
  then found=yes; break; fi
  sleep 10
done

if [[ "$controller" = flux ]]; then
  KUBECONFIG="$kubeconfig" kubectl get ocirepositories.source.toolkit.fluxcd.io,kustomizations.kustomize.toolkit.fluxcd.io -A -o json > "$out/flux-objects.json" 2>"$private/flux.err" || true
else
  KUBECONFIG="$kubeconfig" kubectl get applications.argoproj.io -A -o json > "$out/argo-objects.json" 2>"$private/argo.err" || true
fi
KUBECONFIG="$kubeconfig" kubectl get deployments -A -o wide > "$out/deployments.txt" 2>&1 || true
curl --fail --silent --show-error --max-time 10 "$(cub context get -o json | python3 -c 'import json,sys; print(json.load(sys.stdin)["coordinate"]["serverURL"])')/api/info" > "$out/api-info.json" 2>/dev/null || true

KUBECONFIG="$kubeconfig" CUB_SCOUT_TELEMETRY='' "$scout" gitops status --with-confighub --confighub-space '*' --format json > "$out/gitops-status.json" 2>"$out/gitops-status.err" || {
  echo 'cub-scout gitops status failed:' >&2; redact < "$out/gitops-status.err" >&2; exit 1;
}
KUBECONFIG="$kubeconfig" "$scout" gitops status --with-confighub --confighub-space '*' > "$out/gitops-status.txt" 2>/dev/null || true

[[ -n "$found" ]] || { echo 'No Release carried a LiveStatus within ten minutes; see the artifact' >&2; exit 1; }

python3 - "$out/gitops-status.json" <<'PY'
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
    print(s.get("space"), s.get("release"), "sync=%s" % s.get("syncStatus"), "matchedBy=%s" % (s.get("clusterCheck") or {}).get("matchedBy"), "check=%s" % check.get("state"), check.get("reason", ""))
if not statuses:
    sys.exit("cub-scout reported no live status recorded on a Release")
if not any((s.get("clusterCheck") or {}).get("state") == "agrees" for s in statuses):
    sys.exit("no recorded live status was found to agree with the cluster")
PY
echo 'A live status recorded by the deploying tool agrees with the controllers in the cluster'
