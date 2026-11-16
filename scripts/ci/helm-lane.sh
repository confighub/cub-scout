#!/usr/bin/env bash
# Install one small chart with Helm 4 and with Helm 3, each with and without
# the conventional labels, and record what cub-scout says owns each object.
#
# Helm 3 and 4 label every object they install app.kubernetes.io/managed-by=
# Helm and annotate it with the release's name and namespace, whatever the
# chart sets. Asserted: every object reads as owned by Helm in map list;
# trace answers traced, from Helm's record of the release, naming the release
# that installed it, with the chart as intermediate and the release as
# deployer; and two copies, one keeping
# Helm's annotations and one only the chart's instance label, are credited
# to no release.
# Recorded, not asserted: explain's answer.
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

# A copy made the way people make them: the live object, renamed, applied.
# It keeps Helm's label and annotations, and no release lists it.
kubectl get deployment v4-bare -n "$ns" -o json | python3 -c '
import json, sys
d = json.load(sys.stdin)
m = d["metadata"]
for k in ("uid", "resourceVersion", "creationTimestamp", "generation", "managedFields"):
    m.pop(k, None)
m["name"] = "copied"
d.pop("status", None)
d["spec"]["selector"]["matchLabels"]["app"] = "copied"
d["spec"]["template"]["metadata"]["labels"]["app"] = "copied"
json.dump(d, sys.stdout)' | kubectl apply -f -

# A copy of a labelled object without Helm's annotations: only the chart's
# app.kubernetes.io/instance label names a release, and it does not list it.
kubectl get deployment v3-labelled -n "$ns" -o json | python3 -c '
import json, sys
d = json.load(sys.stdin)
m = d["metadata"]
for k in ("uid", "resourceVersion", "creationTimestamp", "generation", "managedFields"):
    m.pop(k, None)
m["annotations"] = {}
m["name"] = "relabelled"
d.pop("status", None)
d["spec"]["selector"]["matchLabels"]["app"] = "relabelled"
d["spec"]["template"]["metadata"]["labels"]["app"] = "relabelled"
json.dump(d, sys.stdout)' | kubectl apply -f -

kubectl get deployments -n "$ns" -o json > "$out/deployments.json"
"$scout" map list --json --namespace "$ns" > "$out/map-list.json"
for name in v4-labelled v4-bare v3-labelled v3-bare copied relabelled; do
  "$scout" trace "deployment/$name" -n "$ns" --format json > "$out/trace-$name.json" 2> "$out/trace-$name.stderr" || true
  "$scout" explain "Deployment/$name" -n "$ns" --format json > "$out/explain-$name.json" 2> "$out/explain-$name.stderr" || true
done

python3 - "$out" <<'PY'
import json, sys
out = sys.argv[1]
deployments = {d["metadata"]["name"]: d["metadata"] for d in json.load(open(out + "/deployments.json"))["items"]}
owners = {r["name"]: r.get("owner") for r in json.load(open(out + "/map-list.json")) if r.get("kind") == "Deployment"}
lines, bad = [], []
for name in ["v4-labelled", "v4-bare", "v3-labelled", "v3-bare", "copied", "relabelled"]:
    meta = deployments.get(name, {})
    labels, annotations = meta.get("labels") or {}, meta.get("annotations") or {}
    helm_marks = sorted("%s=%s" % (k, v) for k, v in list(labels.items()) + list(annotations.items()) if "helm" in (k + v).lower())
    try:
        trace = json.load(open("%s/trace-%s.json" % (out, name)))
    except (OSError, ValueError):
        trace = None
    releases = [l.get("id") or {} for l in (trace or {}).get("chain") or [] if (l.get("id") or {}).get("kind") == "Release"]
    state = ((trace or {}).get("answer") or {}).get("state")
    lines.append("%s: map owner %s; trace state %s, release %s; on the object: %s" % (
        name, owners.get(name), state, "/".join([releases[-1].get("namespace", ""), releases[-1].get("name", "")]) if releases else None, ", ".join(helm_marks)))
    if owners.get(name) != "Helm":
        bad.append("%s: map list owner %s, want Helm" % (name, owners.get(name)))
    if trace is None:
        bad.append("%s: trace printed no JSON (stderr in trace-%s.stderr)" % (name, name))
    elif not state:
        bad.append("%s: trace JSON has no answer.state" % name)
    elif name in ("copied", "relabelled"):
        if releases:
            bad.append("%s: trace credits the copy to Release %s, which does not list it" % (name, releases[-1].get("name")))
        if state == "no-owner-marker":
            bad.append("%s: trace says no owner marker, but the object carries Helm's" % name)
    elif not releases or releases[-1].get("name") != name:
        bad.append("%s: trace chain does not name Release %s" % (name, name))
    else:
        if state != "traced":
            bad.append("%s: trace state %s, want traced (Helm records the release as deployed)" % (name, state))
        roles = {(l.get("id") or {}).get("kind"): l.get("role") for l in trace.get("chain") or []}
        if roles.get("HelmChart") != "intermediate" or roles.get("Release") != "deployer":
            bad.append("%s: chain roles %s, want HelmChart intermediate and Release deployer" % (name, roles))
open(out + "/summary.txt", "w").write("\n".join(lines) + "\n")
print("\n".join(lines))
if bad:
    sys.exit("Helm lane:\n" + "\n".join(bad))
PY
