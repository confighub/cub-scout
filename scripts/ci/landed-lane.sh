#!/usr/bin/env bash
# Proves `cub-scout landed` on real Flux and Argo CD deliveries in the GitOps
# E2E lane: both positive controls (actual applied/synced revision -> landed)
# and negative controls (wrong revision -> not-landed, zero false-landed).
set -euo pipefail

scout="${1:?usage: landed-lane.sh <cub-scout-binary> <output-dir>}"
out="${2:?usage: landed-lane.sh <cub-scout-binary> <output-dir>}"
mkdir -p "$out"

ctx="$(kubectl config current-context)"
echo "landed-lane: context=$ctx"

# 1. Flux real delivery: deployment/cart in namespace boutique.
flux_rev="$(kubectl get kustomizations.kustomize.toolkit.fluxcd.io -n boutique -o jsonpath='{.items[0].status.lastAppliedRevision}')"
if [[ -z "$flux_rev" ]]; then
  echo "ERROR: Flux Kustomization in boutique has no status.lastAppliedRevision" >&2
  exit 1
fi
echo "landed-lane: Flux revision=$flux_rev"

"$scout" landed deployment/cart -n boutique --kube-context "$ctx" --revision "$flux_rev" --format json > "$out/flux-landed.json"
jq -e '.answer.state == "landed" and .answer.cost.requestsMade > 0' "$out/flux-landed.json" >/dev/null

"$scout" landed deployment/cart -n boutique --kube-context "$ctx" --revision 0000000000000000000000000000000000000000 --format json > "$out/flux-not-landed.json"
jq -e '.answer.state == "not-landed" and .brokenLink.stage == "deployer"' "$out/flux-not-landed.json" >/dev/null

# 2. Argo CD real delivery: deployment/guestbook-ui in namespace guestbook.
argo_rev="$(kubectl get applications.argoproj.io guestbook -n argocd -o jsonpath='{.status.sync.revision}')"
if [[ -z "$argo_rev" ]]; then
  echo "ERROR: Argo CD Application/guestbook in argocd has no status.sync.revision" >&2
  exit 1
fi
echo "landed-lane: Argo CD revision=$argo_rev"

"$scout" landed deployment/guestbook-ui -n guestbook --kube-context "$ctx" --revision "$argo_rev" --format json > "$out/argo-landed.json"
jq -e '.answer.state == "landed" and .answer.cost.requestsMade > 0' "$out/argo-landed.json" >/dev/null

"$scout" landed deployment/guestbook-ui -n guestbook --kube-context "$ctx" --revision 0000000000000000000000000000000000000000 --format json > "$out/argo-not-landed.json"
jq -e '.answer.state == "not-landed" and .brokenLink.stage == "deployer"' "$out/argo-not-landed.json" >/dev/null

echo "landed-lane: PASS (Flux and Argo CD positive + negative controls verified)"
