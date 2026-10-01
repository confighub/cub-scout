#!/usr/bin/env bash
# FIXTURE-OWNED: stage byte-identical raw responses and factual capture metadata for both arms.
set -euo pipefail
case_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
mkdir -p cluster
for name in capture-scope.json observer-context-map.json rul03-denied-deployments.body rul03-readable-deployments.body; do
  cp -- "$case_dir/fixtures/$name" "cluster/$name"
done
