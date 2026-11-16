#!/usr/bin/env bash
# Install one small chart with Helm 4 and with Helm 3, each with and without
# the conventional labels, and record what cub-scout says owns each object.
#
# Asserted: an object carrying app.kubernetes.io/managed-by=Helm reads as
# owned by Helm; no object reads as owned by anything other than Helm or
# Native (a false claim). Recorded, not asserted: what cub-scout says of an
# object with only the metadata Helm itself adds.
set -euo pipefail
scout=${1:?usage: helm-lane.sh <cub-scout binary> <output dir>}
out=${2:?usage: helm-lane.sh <cub-scout binary> <output dir>}
root=${RUNNER_TEMP:?Expected disposable runner directory}/scout-helm
chart=test/e2e/fixtures/helm-lane
ns=helm-lane
mkdir -p "$root" "$out"

fetch() { # version sha256
  curl --fail --silent --show-error --location --retry 3 --proto '=https' --proto-redir '=https' \
    "https://get.helm.sh/helm-$1-linux-amd64.tar.gz" -o "$root/helm-$1.tar.gz"
  printf '%s  %s\n' "$2" "$root/helm-$1.tar.gz" | sha256sum --check --status
  mkdir -p "$root/$1"
  tar -xzf "$root/helm-$1.tar.gz" -C "$root/$1" --strip-components 1 linux-amd64/helm
}
fetch v4.3.2 d1c0278d46beab79582d7e50f5b3dc206944ba8e577cde618513fd07daa53983
fetch v3.22.1 896cf63684a42a723d61c980369d6e588330927deb13676ca0d85006dd6da0ea

kubectl create namespace "$ns"
: > "$out/helm-versions.txt"
for v in v4.3.2 v3.22.1; do
  helm="$root/$v/helm"
  major=${v%%.*}
  echo "$v: $("$helm" version --short)" >> "$out/helm-versions.txt"
  "$helm" install "$major-labelled" "$chart" -n "$ns" --set conventionalLabels=true
  "$helm" install "$major-bare" "$chart" -n "$ns" --set conventionalLabels=false
done
cat "$out/helm-versions.txt"

kubectl get deployments -n "$ns" -o json > "$out/deployments.json"
"$scout" map list --json --namespace "$ns" > "$out/map-list.json"
for name in v4-labelled v4-bare v3-labelled v3-bare; do
  "$scout" trace "deployment/$name" -n "$ns" --format json > "$out/trace-$name.json" || true
done

python3 - "$out" <<'PY'
import json, sys
out = sys.argv[1]
deployments = {d["metadata"]["name"]: d["metadata"] for d in json.load(open(out + "/deployments.json"))["items"]}
owners = {r["name"]: r.get("owner") for r in json.load(open(out + "/map-list.json")) if r.get("kind") == "Deployment"}
lines, bad = [], []
for name in ["v4-labelled", "v4-bare", "v3-labelled", "v3-bare"]:
    meta = deployments.get(name, {})
    labels, annotations = meta.get("labels") or {}, meta.get("annotations") or {}
    helm_marks = sorted(k for k in list(labels) + list(annotations) if "helm" in k)
    lines.append("%s: cub-scout owner %s; helm keys on the object: %s" % (name, owners.get(name), ", ".join(helm_marks) or "none"))
    if name not in owners:
        bad.append("%s: not listed by map list" % name)
    elif owners[name] not in ("Helm", "Native"):
        bad.append("%s: claimed by %s" % (name, owners[name]))
    elif labels.get("app.kubernetes.io/managed-by") == "Helm" and owners[name] != "Helm":
        bad.append("%s: carries managed-by=Helm but reads as %s" % (name, owners[name]))
open(out + "/summary.txt", "w").write("\n".join(lines) + "\n")
print("\n".join(lines))
if bad:
    sys.exit("Helm lane:\n" + "\n".join(bad))
PY
