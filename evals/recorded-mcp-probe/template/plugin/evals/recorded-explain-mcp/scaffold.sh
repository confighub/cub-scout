#!/usr/bin/env bash
# Authored fixture-only setup: each arm receives the same complete recorded List.
set -euo pipefail
case_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
expected='305614fa67327ba3ff6bea85c3c23f5ba9b57db181155b1c62af25d6f882eca8'
if command -v shasum >/dev/null 2>&1; then
	actual="$(shasum -a 256 "$case_dir/fixtures/deployments.yaml" | awk '{print $1}')"
elif command -v sha256sum >/dev/null 2>&1; then
	actual="$(sha256sum "$case_dir/fixtures/deployments.yaml" | awk '{print $1}')"
else
	echo 'recorded MCP probe requires shasum or sha256sum' >&2
	exit 70
fi
[[ "$actual" == "$expected" ]] || { echo 'recorded MCP probe fixture hash mismatch' >&2; exit 70; }
mkdir -p cluster
cp "$case_dir/fixtures/deployments.yaml" cluster/deployments.yaml
cp "$case_dir/fixtures/recording.json" cluster/recording.json
