#!/usr/bin/env bash
# FIXTURE-OWNED-SCAFFOLD: stages identical raw capture bytes for both arms.
set -euo pipefail
case_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
mkdir -p cluster
for name in capture-scope.json before-pod.json before-nodes.json before-events.json after-pod.json after-nodes.json after-events.json; do
  cp -- "$case_dir/fixtures/$name" "cluster/$name"
done
