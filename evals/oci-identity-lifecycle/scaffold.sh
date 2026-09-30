#!/usr/bin/env bash
# FIXTURE-OWNED: copy the same pinned public receipts into each eval arm.
set -euo pipefail

case_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
fixture_dir="$case_dir/fixtures/evidence"
dest="$PWD/evidence/oci-identity-lifecycle"
mkdir -p "$dest"
for name in \
  oci-evidence-chain.yaml \
  installer-publication-receipt.yaml \
  render-intent.yaml \
  render-receipt.yaml \
  catalog-delivery-proof.yaml; do
  cp -- "$fixture_dir/$name" "$dest/$name"
done
