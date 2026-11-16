#!/usr/bin/env bash
# Install kro, create one instance of a small ResourceGraphDefinition, and
# record what kro writes on the object it creates and what cub-scout says.
#
# Asserted: the created Deployment reads as owned by kro in map list, by
# kro's own kro.run/instance-name label, naming the instance; and trace
# answers in JSON without saying "no owner marker". Recorded: the object's
# labels and owner references, trace, and the composition tree.
set -euo pipefail
scout=${1:?usage: kro-lane.sh <cub-scout binary> <output dir>}
out=${2:?usage: kro-lane.sh <cub-scout binary> <output dir>}
root=${RUNNER_TEMP:?Expected disposable runner directory}/scout-kro
fixtures=test/e2e/fixtures/kro-lane
mkdir -p "$root" "$out"

curl --fail --silent --show-error --location --retry 3 --proto '=https' --proto-redir '=https' \
  https://github.com/kubernetes-sigs/kro/releases/download/v0.10.0/kro-core-install-manifests.yaml -o "$root/kro.yaml"
printf '%s  %s\n' 6ec555646be855c9858781107b77ab462494ee0396c61e3c56dd3a02c5102ec9 "$root/kro.yaml" | sha256sum --check --status
# The release manifest does not create its own namespace.
kubectl create namespace kro-system
kubectl apply --server-side -f "$root/kro.yaml"
kubectl rollout status deployment/kro -n kro-system --timeout=300s

kubectl create namespace kro-lane
kubectl apply -f "$fixtures/rgd.yaml"
for _ in $(seq 1 60); do
  kubectl get crd laneapps.kro.run > /dev/null 2>&1 && break
  sleep 2
done
kubectl wait --for=condition=Established crd/laneapps.kro.run --timeout=120s
kubectl apply -f "$fixtures/instance.yaml"
for _ in $(seq 1 60); do
  kubectl get deployment lane-web -n kro-lane > /dev/null 2>&1 && break
  sleep 2
done
kubectl get deployment lane-web -n kro-lane -o json > "$out/deployment.json"
kubectl get laneapps.kro.run lane -n kro-lane -o json > "$out/instance.json"

"$scout" map list --json --namespace kro-lane --ownership-evidence > "$out/map-list.json"
"$scout" trace deployment/lane-web -n kro-lane --format json > "$out/trace.json" 2> "$out/trace.stderr" || true
"$scout" tree composition > "$out/tree-composition.txt" 2>&1 || true

python3 - "$out" <<'PY'
import json, sys
out = sys.argv[1]
meta = json.load(open(out + "/deployment.json"))["metadata"]
labels = meta.get("labels") or {}
owner_refs = ["%s/%s %s" % (r.get("apiVersion"), r.get("kind"), r.get("name")) for r in meta.get("ownerReferences") or []]
rows = [r for r in json.load(open(out + "/map-list.json"))["resources"] if r.get("kind") == "Deployment" and r.get("name") == "lane-web"]
row = rows[0] if rows else {}
detection = row.get("ownershipDetection") or {}
try:
    trace = json.load(open(out + "/trace.json"))
except (OSError, ValueError):
    trace = None
state = ((trace or {}).get("answer") or {}).get("state")
lines = [
    "kro labels: " + ", ".join("%s=%s" % (k, v) for k, v in sorted(labels.items()) if k.startswith("kro.run/") or k == "app.kubernetes.io/managed-by"),
    "owner references: " + (", ".join(owner_refs) or "none"),
    "map owner %s, detected by %s" % (row.get("owner"), detection.get("source")),
    "trace state %s" % state,
]
open(out + "/summary.txt", "w").write("\n".join(lines) + "\n")
print("\n".join(lines))
bad = []
if row.get("owner") != "kro":
    bad.append("map list owner %s, want kro" % row.get("owner"))
if labels.get("kro.run/instance-name") != "lane":
    bad.append("kro did not label the Deployment kro.run/instance-name=lane; the detector's premise needs checking")
elif detection.get("source") != "label:kro.run/instance-name":
    bad.append("detected by %s, want label:kro.run/instance-name" % detection.get("source"))
if trace is None:
    bad.append("trace printed no JSON (stderr in trace.stderr)")
elif state == "no-owner-marker":
    bad.append("trace says no owner marker, but kro's labels are on the object")
if bad:
    sys.exit("kro lane:\n" + "\n".join(bad))
PY
