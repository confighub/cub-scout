#!/usr/bin/env bash
# FIXTURE-OWNED-SCAFFOLD: copy the exact synthetic summaries into the eval workspace.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
mkdir -p recorded
cp "$SCRIPT_DIR/fixtures/alpha.json" recorded/alpha.json
cp "$SCRIPT_DIR/fixtures/beta.json" recorded/beta.json
