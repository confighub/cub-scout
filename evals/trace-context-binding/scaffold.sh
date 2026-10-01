#!/usr/bin/env bash
# FIXTURE-OWNED: identical recorded MCP evidence in both arms; no live access.
set -euo pipefail
case_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
mkdir -p cluster
for name in trace-allowed.json trace-denied.json provenance.json; do
  cp -- "$case_dir/fixtures/$name" "cluster/$name"
done
