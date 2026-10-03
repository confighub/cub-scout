#!/usr/bin/env bash
# FIXTURE-OWNED: synthetic, unrun; identical private evidence for both arms, no network calls.
set -euo pipefail
case_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
mkdir -p cluster
cp -- "$case_dir/fixtures/evidence.json" cluster/evidence.json
