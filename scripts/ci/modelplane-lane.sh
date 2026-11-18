#!/usr/bin/env bash
# Install Crossplane and Modelplane on one kind cluster, create Modelplane
# objects with no workload cluster and no model, and record what Modelplane
# writes and what cub-scout says. The maintainer chose this control-plane-only
# lane over Modelplane's two-cluster demo (2026-11-18).
#
# Asserted: the ModelDeployment and ModelService read as Modelplane's in map
# list, and trace answers for the ModelDeployment in JSON without saying "no
# owner marker". Recorded: every object in a Modelplane API group, the
# objects Modelplane composed, the detector's source, trace, explain and the
# composition tree.
set -euo pipefail
scout=${1:?usage: modelplane-lane.sh <cub-scout binary> <output dir>}
out=${2:?usage: modelplane-lane.sh <cub-scout binary> <output dir>}
root=${RUNNER_TEMP:?Expected disposable runner directory}/scout-modelplane
fixtures=test/e2e/fixtures/modelplane-lane
mkdir -p "$root" "$out"

fetch() { # url sha256 file
  curl --fail --silent --show-error --location --retry 3 --proto '=https' --proto-redir '=https' "$1" -o "$3"
  printf '%s  %s\n' "$2" "$3" | sha256sum --check --status
}
fetch https://get.helm.sh/helm-v4.3.2-linux-amd64.tar.gz d1c0278d46beab79582d7e50f5b3dc206944ba8e577cde618513fd07daa53983 "$root/helm.tar.gz"
tar -xzf "$root/helm.tar.gz" -C "$root" --strip-components 1 linux-amd64/helm
fetch https://charts.crossplane.io/stable/crossplane-2.4.3.tgz eeace77cdc7e727a8400cb7a229e290d4d28dd730b440a1b06fb6df4569cd33c "$root/crossplane.tgz"
fetch https://raw.githubusercontent.com/modelplaneai/modelplane/v0.7.0/docs/manifests/install/prerequisites.yaml f4e3e179b15ddbcd6d5f663dd1e648d4e1c57298b5bc18c036b185070278dbf6 "$root/prerequisites.yaml"

# As Modelplane's own setup script installs Crossplane.
"$root/helm" install crossplane "$root/crossplane.tgz" -n crossplane-system --create-namespace \
  --set "args={--enable-dependency-version-upgrades}" --set-json 'provider.defaultActivations=[]' --wait --timeout 5m
kubectl apply -f "$root/prerequisites.yaml" -f "$fixtures/configuration.yaml"
if ! kubectl wait configuration.pkg.crossplane.io/modelplane --for=condition=Healthy --timeout=15m; then
  kubectl get configuration.pkg.crossplane.io,providers.pkg.crossplane.io,functions.pkg.crossplane.io -o wide >&2 || true
  kubectl describe configuration.pkg.crossplane.io/modelplane | tail -n 30 >&2 || true
  exit 1
fi
kubectl wait xrd --all --for=condition=Established --timeout=5m
kubectl apply -f "$fixtures/objects.yaml"
# Give the composition functions a minute to act on what they can.
sleep 60

kubectl api-resources -o name | grep -E 'modelplane\.ai$' > "$out/api-resources.txt" || true
for resource in $(cat "$out/api-resources.txt"); do
  kubectl get "$resource" -A -o json --show-managed-fields > "$out/objects-$resource.json" 2>/dev/null || true
done
kubectl get all,configmaps,secrets -n ml-team -o json --show-managed-fields > "$out/ml-team.json"

"$scout" map list --json --ownership-evidence > "$out/map-list.json"
"$scout" trace modeldeployment/smollm -n ml-team --format json > "$out/trace-modeldeployment.json" 2> "$out/trace-modeldeployment.stderr" || true
"$scout" explain ModelDeployment/smollm -n ml-team --format json > "$out/explain-modeldeployment.json" 2> "$out/explain-modeldeployment.stderr" || true
"$scout" tree composition > "$out/tree-composition.txt" 2>&1 || true

python3 - "$out" <<'PY'
import glob, json, os, sys
out = sys.argv[1]
rows = json.load(open(out + "/map-list.json"))["resources"]
lines, bad = [], []
for path in sorted(glob.glob(out + "/objects-*.json")):
    for item in json.load(open(path)).get("items", []):
        meta = item["metadata"]
        labels = sorted(k for k in (meta.get("labels") or {}) if "modelplane" in k or "crossplane" in k)
        refs = ["%s %s" % (r.get("kind"), r.get("name")) for r in meta.get("ownerReferences") or []]
        lines.append("%s %s/%s: labels %s; owner refs %s" % (item["kind"], meta.get("namespace", ""), meta["name"], labels, refs or "none"))
for row in rows:
    if row.get("namespace") == "ml-team" or (row.get("apiVersion") or "").split("/")[0].endswith("modelplane.ai"):
        detection = row.get("ownershipDetection") or {}
        lines.append("map: %s %s/%s owner %s (%s)" % (row.get("kind"), row.get("namespace"), row.get("name"), row.get("owner"), detection.get("source")))
owners = {(r.get("kind"), r.get("name")): r.get("owner") for r in rows if r.get("namespace") == "ml-team"}
for key in (("ModelDeployment", "smollm"), ("ModelService", "assistant")):
    if owners.get(key) != "Modelplane":
        bad.append("map list owner of %s/%s is %s, want Modelplane" % (key[0], key[1], owners.get(key)))
try:
    answer = json.load(open(out + "/trace-modeldeployment.json")).get("answer") or {}
    lines.append("trace ModelDeployment/smollm: %s %s" % (answer.get("state"), answer.get("reason")))
    if answer.get("state") == "no-owner-marker":
        bad.append("trace says no owner marker on a Modelplane object")
except (OSError, ValueError):
    bad.append("trace printed no JSON for ModelDeployment/smollm (stderr in trace-modeldeployment.stderr)")
open(out + "/summary.txt", "w").write("\n".join(lines) + "\n")
print("\n".join(lines))
if bad:
    sys.exit("Modelplane lane:\n" + "\n".join(bad))
PY
