#!/usr/bin/env bash
# FIXTURE-OWNED: copy pinned public source files byte-for-byte; no cluster access.
set -euo pipefail
case_root="$(cd -- "$(dirname -- "$0")" && pwd)"
mkdir -p cluster
cp "$case_root/fixtures/receipt.yaml" cluster/receipt.yaml
cp "$case_root/fixtures/argocd-child.json" cluster/argocd-child.json
cp "$case_root/fixtures/source-provenance.json" cluster/source-provenance.json
