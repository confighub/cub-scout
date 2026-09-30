#!/usr/bin/env bash
# FIXTURE-OWNED: copy the committed capture byte-for-byte; no generated facts.
set -euo pipefail
case_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
mkdir -p cluster
cp -- "$case_dir"/fixtures/2026-09-30/*.json cluster/
cp -- "$case_dir"/gitrepository.yaml "$case_dir"/kustomization.yaml cluster/
