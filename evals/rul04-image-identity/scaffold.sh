#!/usr/bin/env bash
# FIXTURE-OWNED: stages only the same raw authored/live evidence for both arms.
set -euo pipefail
case_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
mkdir -p cluster
for name in capture-scope.json desired-statefulset.yaml statefulset.json pods.json; do
  cp -- "$case_dir/fixtures/$name" "cluster/$name"
done
