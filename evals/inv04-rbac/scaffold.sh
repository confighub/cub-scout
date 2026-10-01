#!/usr/bin/env bash
# FIXTURE-OWNED: stage identical raw API responses and bounded capture metadata for both arms.
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
mkdir -p cluster
for name in capture-scope.json readable-populated-deployments.json readable-empty-deployments.json denied-deployments.json; do
  cp "$root/fixtures/$name" "cluster/$name"
done
