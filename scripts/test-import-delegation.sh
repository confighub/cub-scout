#!/bin/bash
# Repeatable verification for cub-scout import delegation behavior.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
cd "$ROOT_DIR"

echo "==> Running import delegation unit tests"
go test ./cmd/cub-scout \
  -run 'TestSelectGitOpsTargets|TestFilterScoutWorkloadsAfterDelegation|TestGitOpsNamespacesForOwner' \
  -count=1 -v

echo "==> Verifying import help documents delegation and flags"
HELP_OUTPUT="$(go run ./cmd/cub-scout import --help)"
echo "$HELP_OUTPUT" | grep -q "cub gitops discover + cub gitops import"
echo "$HELP_OUTPUT" | grep -q -- "--connect"
echo "$HELP_OUTPUT" | grep -q -- "--no-connect"

echo "==> Import delegation checks passed"
