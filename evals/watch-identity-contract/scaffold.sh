#!/usr/bin/env bash
# FIXTURE-OWNED: authored product contract, outside the frozen benchmark.
set -euo pipefail
script_dir="$(cd -- "$(dirname -- "$0")" && pwd)"
mkdir -p evidence
cp "$script_dir/fixtures/inputs.json" evidence/watch-identity.json
