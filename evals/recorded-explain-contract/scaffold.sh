#!/usr/bin/env bash
# FIXTURE-OWNED: complete source-derived recorded List; not generated live evidence.
set -euo pipefail
case_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
mkdir -p cluster
cp "$case_dir/fixtures/deployments.yaml" cluster/deployments.yaml
cp "$case_dir/fixtures/recording.json" cluster/recording.json
