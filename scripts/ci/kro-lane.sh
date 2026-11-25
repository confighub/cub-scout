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
kubectl apply -f "$fixtures/rbac.yaml"
kubectl apply -f "$fixtures/rgd.yaml"
for _ in $(seq 1 60); do
  kubectl get crd laneapps.kro.run > /dev/null 2>&1 && break
  sleep 2
done
kubectl wait --for=condition=Established crd/laneapps.kro.run --timeout=120s
kubectl apply -f "$fixtures/instance.yaml"
created=''
for _ in $(seq 1 60); do
  kubectl get deployment lane-web -n kro-lane > /dev/null 2>&1 && { created=yes; break; }
  sleep 2
done
# What kro says about the definition and the instance, kept either way.
kubectl get resourcegraphdefinitions.kro.run laneapp.kro.run -o json > "$out/rgd.json" 2>&1 || true
kubectl get laneapps.kro.run lane -n kro-lane -o json > "$out/instance.json" 2>&1 || true
if [[ -z "$created" ]]; then
  echo 'kro created no Deployment lane-web in two minutes. Definition and instance status, and the end of kro'"'"'s log:' >&2
  python3 -c '
import json, sys
for path in sys.argv[1:]:
    try:
        d = json.load(open(path))
    except ValueError:
        print(path, open(path).read()[:400]); continue
    print(d.get("kind"), d["metadata"]["name"], json.dumps(d.get("status"), indent=1)[:2000])' "$out/rgd.json" "$out/instance.json" >&2 || true
  kubectl logs deployment/kro -n kro-system --tail=60 > "$out/kro.log" 2>&1 || true
  tail -n 40 "$out/kro.log" >&2
  exit 1
fi
kubectl rollout status deployment/lane-web -n kro-lane --timeout=60s || true
kubectl wait --for=condition=Ready laneapps.kro.run/lane -n kro-lane --timeout=60s || true
kubectl get laneapps.kro.run lane -n kro-lane -o json > "$out/instance.json" 2>&1 || true
kubectl get deployment lane-web -n kro-lane -o json > "$out/deployment.json"

"$scout" map list --json --namespace kro-lane --ownership-evidence > "$out/map-list.json"
"$scout" trace deployment/lane-web -n kro-lane --format json > "$out/trace.json" 2> "$out/trace.stderr" || true
"$scout" tree composition > "$out/tree-composition.txt" 2>&1 || true

kro_obs_gen=$(python3 -c '
import json, sys
d = json.load(open(sys.argv[1]))
st = d.get("status") or {}
gen = None
for c in st.get("conditions") or []:
    if c.get("type") in ("Ready", "InstanceReady") and c.get("observedGeneration"):
        gen = c["observedGeneration"]
        break
if not gen and st.get("observedGeneration"):
    gen = st["observedGeneration"]
print(gen or "")
' "$out/instance.json" 2>/dev/null || true)

if [[ -n "$kro_obs_gen" ]]; then
  "$scout" landed deployment/lane-web -n kro-lane --revision "$kro_obs_gen" --format json > "$out/landed-ok.json"
  "$scout" landed deployment/lane-web -n kro-lane --revision 99 --format json > "$out/landed-wrong.json"
else
  "$scout" landed deployment/lane-web -n kro-lane --revision 1 --format json > "$out/landed-no-anchor.json"
fi

python3 - "$out" "$kro_obs_gen" <<'PY'
import json, sys
out, kro_obs_gen = sys.argv[1], sys.argv[2]
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
if kro_obs_gen:
    ok_doc = json.load(open(out + "/landed-ok.json"))
    wrong_doc = json.load(open(out + "/landed-wrong.json"))
    ok_state = (ok_doc.get("answer") or {}).get("state")
    wrong_state = (wrong_doc.get("answer") or {}).get("state")
    lines.append("landed(--revision %s)=%s, landed(--revision 99)=%s" % (kro_obs_gen, ok_state, wrong_state))
    if ok_state != "landed":
        bad.append("landed --revision %s state %s, want landed" % (kro_obs_gen, ok_state))
    if wrong_state != "not-landed":
        bad.append("landed --revision 99 state %s, want not-landed" % wrong_state)
else:
    na_doc = json.load(open(out + "/landed-no-anchor.json"))
    na_state = (na_doc.get("answer") or {}).get("state")
    lines.append("landed(no observedGeneration)=%s" % na_state)
    if na_state != "not-observable":
        bad.append("landed without observedGeneration state %s, want not-observable" % na_state)
open(out + "/summary.txt", "w").write("\n".join(lines) + "\n")
if bad:
    sys.exit("kro lane:\n" + "\n".join(bad))
PY
