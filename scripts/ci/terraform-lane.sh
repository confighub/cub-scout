#!/usr/bin/env bash
# Apply two Deployments with Terraform's Kubernetes provider and record what
# Terraform writes on them and what cub-scout says.
#
# cub-scout's Terraform detector reads app.terraform.io/run-id and
# app.terraform.io/managed. This step records whether a real Terraform writes
# either. Asserted only that no other owner is claimed and that trace answers
# in JSON; what Terraform writes is recorded, not assumed.
set -euo pipefail
scout=${1:?usage: terraform-lane.sh <cub-scout binary> <output dir>}
out=${2:?usage: terraform-lane.sh <cub-scout binary> <output dir>}
root=${RUNNER_TEMP:?Expected disposable runner directory}/scout-terraform
mkdir -p "$root" "$out"

curl --fail --silent --show-error --location --retry 3 --proto '=https' --proto-redir '=https' \
  https://releases.hashicorp.com/terraform/1.17.2/terraform_1.17.2_linux_amd64.zip -o "$root/terraform.zip"
printf '%s  %s\n' e6f16b2f0b137ee5729caa8c4e79a3bae4ee261acd198a1fcfa8584a6296605b "$root/terraform.zip" | sha256sum --check --status
unzip -q -o "$root/terraform.zip" terraform -d "$root"
cp test/e2e/fixtures/terraform-lane/main.tf "$root/"
kubeconfig=${KUBECONFIG:-$HOME/.kube/config}
"$root/terraform" -chdir="$root" version > "$out/terraform-version.txt"
"$root/terraform" -chdir="$root" init -input=false > "$out/init.txt"
"$root/terraform" -chdir="$root" apply -input=false -auto-approve -var "kubeconfig=$kubeconfig" > "$out/apply.txt"
cp "$root/.terraform.lock.hcl" "$out/terraform.lock.hcl"

kubectl get deployments -n tf-lane -o json > "$out/deployments.json"
"$scout" map list --json --namespace tf-lane --ownership-evidence > "$out/map-list.json"
for name in tf-typed tf-manifest; do
  "$scout" trace "deployment/$name" -n tf-lane --format json > "$out/trace-$name.json" 2> "$out/trace-$name.stderr" || true
  "$scout" explain "Deployment/$name" -n tf-lane --format json > "$out/explain-$name.json" 2> "$out/explain-$name.stderr" || true
done

python3 - "$out" <<'PY'
import json, sys
out = sys.argv[1]
items = {d["metadata"]["name"]: d["metadata"] for d in json.load(open(out + "/deployments.json"))["items"]}
rows = {r["name"]: r for r in json.load(open(out + "/map-list.json"))["resources"] if r.get("kind") == "Deployment"}
lines, bad = [], []
for name in ("tf-typed", "tf-manifest"):
    meta = items.get(name, {})
    labels, annotations = meta.get("labels") or {}, meta.get("annotations") or {}
    managers = sorted({f.get("manager", "") + "/" + f.get("operation", "") for f in meta.get("managedFields") or []})
    row = rows.get(name, {})
    detection = row.get("ownershipDetection") or {}
    try:
        state = (json.load(open("%s/trace-%s.json" % (out, name))).get("answer") or {}).get("state")
    except (OSError, ValueError):
        state = None
    lines.append("%s: labels %s; annotations %s; field managers %s; map owner %s (%s); trace state %s" % (
        name, sorted(labels), sorted(k for k in annotations if k != "deployment.kubernetes.io/revision"), managers,
        row.get("owner"), detection.get("status"), state))
    if row.get("owner") not in ("Native", "Terraform"):
        bad.append("%s: map owner %s" % (name, row.get("owner")))
    if state is None:
        bad.append("%s: trace printed no JSON" % name)
open(out + "/summary.txt", "w").write("\n".join(lines) + "\n")
print("\n".join(lines))
if bad:
    sys.exit("Terraform lane:\n" + "\n".join(bad))
PY
