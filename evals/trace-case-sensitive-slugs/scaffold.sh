#!/usr/bin/env bash
# FIXTURE-OWNED: copies synthetic Trace model output and mocked connected rows.
set -euo pipefail
case_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
mkdir -p cluster
for file in trace-evidence.json connected-rows.json; do
  cp -- "$case_dir/fixtures/$file" "cluster/$file"
done
