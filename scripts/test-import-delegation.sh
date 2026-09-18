#!/bin/bash
# Repeatable verification for cub-scout import delegation behavior.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
cd "$ROOT_DIR"

# cub removed the whole `gitops` group on 2026-07-25 (#573), so there is no
# delegation left to verify: Argo- and Flux-managed workloads are imported as a
# cub-scout snapshot like everything else. What this checks now is that the help
# says so, and that nothing claims a delegation that cannot happen.

echo "==> Verifying the runner refuses cub commands that no longer exist"
go test ./cmd/cub-scout \
  -run 'TestCubRunnerRefusesACallThatNamesNoSpace|TestNoCubCallUsesARemovedSubcommand' \
  -count=1

echo "==> Verifying import help states what happens to GitOps-managed workloads"
HELP_OUTPUT="$(go run ./cmd/cub-scout import --help)"
echo "$HELP_OUTPUT" | grep -q "imported the same way as the rest"
echo "$HELP_OUTPUT" | grep -q "cub variant upload"
echo "$HELP_OUTPUT" | grep -q -- "--connect"
echo "$HELP_OUTPUT" | grep -q -- "--no-connect"

echo "==> Verifying no code path still calls the removed command"
! grep -rn 'exec.Command("cub", "gitops"' cmd/ || {
  echo "FAIL: a cub gitops call is back" >&2
  exit 1
}

echo "==> Import delegation checks passed"
