#!/usr/bin/env bash
# Install Crossplane, compose one Deployment from a namespaced composite, and
# record what Crossplane writes on it and what cub-scout says.
#
# Asserted: the composed Deployment reads as owned by Crossplane in map list,
# and trace answers in JSON without saying "no owner marker". Recorded: the
# object's labels, annotations and owner references, the detector's source,
# trace, and the composition tree. Needs the Helm step's helm binary.
set -euo pipefail
scout=${1:?usage: crossplane-lane.sh <cub-scout binary> <output dir>}
out=${2:?usage: crossplane-lane.sh <cub-scout binary> <output dir>}
root=${RUNNER_TEMP:?Expected disposable runner directory}/scout-crossplane
helm=$RUNNER_TEMP/scout-helm/v4.3.2/helm
fixtures=test/e2e/fixtures/crossplane-lane
mkdir -p "$root" "$out"
[[ -x "$helm" ]] || { echo "expected the Helm step's helm at $helm" >&2; exit 1; }

curl --fail --silent --show-error --location --retry 3 --proto '=https' --proto-redir '=https' \
  https://charts.crossplane.io/stable/crossplane-2.4.3.tgz -o "$root/crossplane-2.4.3.tgz"
printf '%s  %s\n' eeace77cdc7e727a8400cb7a229e290d4d28dd730b440a1b06fb6df4569cd33c "$root/crossplane-2.4.3.tgz" | sha256sum --check --status
"$helm" install crossplane "$root/crossplane-2.4.3.tgz" -n crossplane-system --create-namespace --wait --timeout 5m

kubectl apply -f "$fixtures/rbac.yaml"
kubectl apply -f "$fixtures/function.yaml"
kubectl wait --for=condition=Healthy function.pkg.crossplane.io/function-patch-and-transform --timeout=300s
kubectl apply -f "$fixtures/definition.yaml"
kubectl wait --for=condition=Established compositeresourcedefinition.apiextensions.crossplane.io/xlaneapps.lane.example.org --timeout=120s
kubectl apply -f "$fixtures/composition.yaml"
kubectl create namespace xp-lane
kubectl apply -f "$fixtures/composite.yaml"

deployment=''
for _ in $(seq 1 60); do
  deployment=$(kubectl get deployments -n xp-lane -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)
  [[ -n "$deployment" ]] && break
  sleep 2
done
kubectl get xlaneapps.lane.example.org lane -n xp-lane -o json > "$out/composite.json" 2>&1 || true
if [[ -z "$deployment" ]]; then
  echo 'Crossplane composed no Deployment in two minutes. The composite, and the end of Crossplane'"'"'s log:' >&2
  cat "$out/composite.json" >&2
  kubectl logs deployment/crossplane -n crossplane-system --tail=60 > "$out/crossplane.log" 2>&1 || true
  tail -n 40 "$out/crossplane.log" >&2
  exit 1
fi
echo "$deployment" > "$out/deployment-name.txt"
kubectl get deployment "$deployment" -n xp-lane -o json > "$out/deployment.json"

"$scout" map list --json --namespace xp-lane --ownership-evidence > "$out/map-list.json"
"$scout" trace "deployment/$deployment" -n xp-lane --format json > "$out/trace.json" 2> "$out/trace.stderr" || true
"$scout" tree composition > "$out/tree-composition.txt" 2>&1 || true

python3 - "$out" "$deployment" <<'PY'
import json, sys
out, name = sys.argv[1], sys.argv[2]
meta = json.load(open(out + "/deployment.json"))["metadata"]
labels, annotations = meta.get("labels") or {}, meta.get("annotations") or {}
owner_refs = ["%s/%s %s" % (r.get("apiVersion"), r.get("kind"), r.get("name")) for r in meta.get("ownerReferences") or []]
rows = [r for r in json.load(open(out + "/map-list.json"))["resources"] if r.get("kind") == "Deployment" and r.get("name") == name]
row = rows[0] if rows else {}
detection = row.get("ownershipDetection") or {}
try:
    trace = json.load(open(out + "/trace.json"))
except (OSError, ValueError):
    trace = None
state = ((trace or {}).get("answer") or {}).get("state")
lines = [
    "crossplane labels: " + ", ".join("%s=%s" % (k, v) for k, v in sorted(labels.items()) if "crossplane" in k),
    "crossplane annotations: " + ", ".join("%s=%s" % (k, v) for k, v in sorted(annotations.items()) if "crossplane" in k),
    "owner references: " + (", ".join(owner_refs) or "none"),
    "map owner %s, detected by %s" % (row.get("owner"), detection.get("source")),
    "trace state %s" % state,
]
open(out + "/summary.txt", "w").write("\n".join(lines) + "\n")
print("\n".join(lines))
bad = []
if row.get("owner") != "Crossplane":
    bad.append("map list owner %s, want Crossplane" % row.get("owner"))
if trace is None:
    bad.append("trace printed no JSON (stderr in trace.stderr)")
elif state == "no-owner-marker":
    bad.append("trace says no owner marker, but Crossplane's metadata is on the object")
if bad:
    sys.exit("Crossplane lane:\n" + "\n".join(bad))
PY
