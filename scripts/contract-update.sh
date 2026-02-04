#!/usr/bin/env bash
# Update contract baselines (schema snapshots and golden hashes).
#
# Run this ONLY when you have intentionally changed a locked schema
# and have bumped the schema version appropriately.
#
# Usage:
#   ./scripts/contract-update.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

echo "== Contract update: regenerating baselines"
echo ""
echo "WARNING: Only run this if you have intentionally changed locked schemas"
echo "         and have bumped the schema version appropriately."
echo ""

GENERATE_SNAPSHOTS=1 go test ./test/contract/... -run TestGenerateSnapshots -v

echo ""
echo "== Contract update: DONE"
echo ""
echo "Verify changes with: git diff test/contract/testdata/"
echo "Then commit if correct."
