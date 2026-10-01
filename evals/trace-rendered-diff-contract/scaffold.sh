#!/usr/bin/env bash
# FIXTURE-OWNED: synthetic contract-shaped example only; no live/recorded cluster evidence.
set -euo pipefail
case_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
mkdir -p evidence
for name in desired.yaml comparison.json; do
  cp -- "$case_dir/fixtures/$name" "evidence/$name"
done
