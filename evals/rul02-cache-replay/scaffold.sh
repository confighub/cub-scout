#!/usr/bin/env bash
# FIXTURE-OWNED: stage identical actual synthetic replay and factual provenance for both arms.
set -euo pipefail
case_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
mkdir -p cluster
for name in capture-scope.json rul02-cache-replay.json; do
  cp -- "$case_dir/fixtures/$name" "cluster/$name"
done
