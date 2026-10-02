#!/usr/bin/env bash
# FIXTURE-OWNED-SCAFFOLD: synthetic serialized contract, not a live observation.
set -euo pipefail
case_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
mkdir -p cluster
cp -- "$case_dir/fixtures/source-truth.json" cluster/source-truth.json
