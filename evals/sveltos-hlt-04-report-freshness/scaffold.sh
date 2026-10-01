#!/usr/bin/env bash
# FIXTURE-OWNED: stage byte-identical authored synthetic replay evidence for both arms.
set -euo pipefail
case_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
mkdir -p cluster
for name in source-metadata.json scenario-evidence.json synthetic-check-execution-receipt.json; do
  cp -- "$case_dir/fixtures/$name" "cluster/$name"
done
