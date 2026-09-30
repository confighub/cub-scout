#!/usr/bin/env bash
# FIXTURE-OWNED: offline source-pinned contract input; no cluster or paid call.
set -euo pipefail
script_dir="$(cd -- "$(dirname -- "$0")" && pwd)"
mkdir -p cluster evidence
cat > cluster/deployments.yaml <<'CUB_SCOUT_EVAL_EOF'
apiVersion: v1
kind: List
items:
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: checkout
    namespace: shop
    labels:
      kustomize.toolkit.fluxcd.io/name: shop-app
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: api
    namespace: shop
CUB_SCOUT_EVAL_EOF
cp "$script_dir/fixtures/map-ownership-evidence.json" evidence/map-ownership-evidence.json
