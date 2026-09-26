#!/usr/bin/env bash
# Hand edits made after evals/fixtures/scenario.yaml is applied, as during an
# incident. Each uses a different kubectl verb, so each leaves a different
# field manager in managedFields. A plain `kubectl get -o yaml` export omits
# managedFields; cub-scout's explain reports them as mutationCause and
# mutationManager. hotfix-worker is left untouched as the control.
#
# Usage: evals/fixtures/incident.sh <kube-context>
set -euo pipefail
ctx="${1:?usage: incident.sh <kube-context>}"
k() { kubectl --context "$ctx" "$@"; }

# Flux-labelled: the image changed by hand, bypassing Git.
k -n shop set image deployment/checkout checkout=registry.k8s.io/pause:3.10
# Argo-labelled: scaled by hand.
k -n shop scale deployment/cart --replicas=3
# ConfigHub-labelled: an environment variable patched in.
k -n inventory patch deployment/inventory --type=json \
  -p '[{"op":"add","path":"/spec/template/spec/containers/0/env","value":[{"name":"FEATURE_BULK_IMPORT","value":"true"}]}]'
