#!/usr/bin/env bash
# FIXTURE-OWNED: byte-identical recorded CLI evidence in both arms; no live tools.
set -euo pipefail
case_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
mkdir -p cluster
for name in doctor-allowed.json doctor-denied.json scan-allowed.json scan-denied.json provenance.json; do
  cp -- "$case_dir/fixtures/$name" "cluster/$name"
done
