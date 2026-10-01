#!/usr/bin/env bash
# FIXTURE-OWNED: stage byte-identical raw capture files for both arms.
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
mkdir -p cluster
for source in "$root"/fixtures/*; do
  cp "$source" "cluster/$(basename "$source")"
done
