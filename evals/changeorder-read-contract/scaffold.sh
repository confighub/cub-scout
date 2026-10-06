#!/usr/bin/env bash
# FIXTURE-OWNED-SCAFFOLD: copy authored evidence only.
set -euo pipefail
case_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
mkdir -p cluster
cp -- "$case_dir/fixtures/inputs.json" cluster/inputs.json
