#!/usr/bin/env bash
# Contract audit: verifies schema immutability and output determinism
# for locked v0.16+ schemas.
#
# This script is run in CI to catch:
# - Schema drift (struct field/tag changes)
# - Output non-determinism
# - Semantic regression (hash mismatches)
#
# To update baselines after intentional changes:
#   ./scripts/contract-update.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

echo "== Contract audit: build"
go build ./cmd/cub-scout

echo "== Contract audit: schema immutability + golden determinism"
go test ./test/contract/... -v

echo "== Contract audit: PASSED"
