#!/usr/bin/env bash
set -euo pipefail
source_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
mkdir -p cluster
cp "$source_dir/fixtures/deployments.json" cluster/deployments.json
