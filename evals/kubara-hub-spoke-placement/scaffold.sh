#!/usr/bin/env bash
# FIXTURE-OWNED: stage identical pinned desired/source artifacts for both arms.
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
mkdir -p cluster
cp "$root/fixtures/desired-matrix.json" cluster/desired-matrix.json
cp "$root/fixtures/config.yaml" cluster/config.yaml
cp "$root/fixtures/source-provenance.json" cluster/source-provenance.json
