#!/usr/bin/env bash
# FIXTURE-OWNED: both arms receive the same raw Pod and neutral time evidence.
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
mkdir -p cluster
cp "$root/fixtures/after-pod.json" cluster/after-pod.json
cp "$root/fixtures/capture-time-receipt.json" cluster/capture-time-receipt.json
cp "$root/fixtures/test-clocks.json" cluster/test-clocks.json
