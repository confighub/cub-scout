#!/usr/bin/env bash
# FIXTURE-OWNED: stage the same pinned source excerpts and receipt for both arms.
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
mkdir -p cluster
cp "$root/fixtures/source-doc/onboard-excerpt.md" cluster/onboard-excerpt.md
cp "$root/fixtures/source-doc/known-behaviours-excerpt.md" cluster/known-behaviours-excerpt.md
cp "$root/fixtures/receipt/sveltos-oci-delivery-proof.yaml" cluster/sveltos-oci-delivery-proof.yaml
