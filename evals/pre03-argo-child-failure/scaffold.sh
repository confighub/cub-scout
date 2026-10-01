#!/usr/bin/env bash
set -euo pipefail

# FIXTURE-OWNED-SCAFFOLD: identical historical files for each eval arm.
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
mkdir -p cluster
for name in capture-scope.json argocd-core-root.json argocd-core-child.json argocd-core-root-tree.txt argocd-core-child-tree.txt pod-describe.txt events.txt; do
  cp "$script_dir/fixtures/$name" "cluster/$name"
done
