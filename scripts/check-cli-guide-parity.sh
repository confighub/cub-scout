#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

# The canonical A-Z command catalog moved from CLI-GUIDE.md to
# docs/reference/cli-reference.md in the doc consolidation under #374.
# CLI-GUIDE.md is now the workflow-first guide and intentionally does
# not list every top-level command. Parity is enforced against the
# canonical catalog instead.
GUIDE_PATH="docs/reference/cli-reference.md"

echo "Checking CLI reference top-level command parity (${GUIDE_PATH})..."

help_cmds="$(
  go run ./cmd/cub-scout --help \
    | awk 'f && NF { if ($1 == "Flags:") exit; print $1 } /Available Commands:/ { f = 1 }' \
    | sort -u
)"

guide_cmds="$(
  awk '
    BEGIN { inside = 0 }
    /^## Top-Level Commands/ { inside = 1; next }
    inside && /^## / { inside = 0 }
    inside && /^\| `/ {
      cmd = $0
      sub(/^\| `/, "", cmd)
      sub(/`.*/, "", cmd)
      print cmd
    }
  ' "$GUIDE_PATH" | sort -u
)"

missing_from_guide="$(comm -23 <(printf "%s\n" "$help_cmds") <(printf "%s\n" "$guide_cmds") || true)"
extra_in_guide="$(comm -13 <(printf "%s\n" "$help_cmds") <(printf "%s\n" "$guide_cmds") || true)"

failed=0

if [[ -n "$missing_from_guide" ]]; then
  echo ""
  echo "Commands in 'cub-scout --help' but missing from ${GUIDE_PATH} top-level table:"
  printf "%s\n" "$missing_from_guide"
  failed=1
fi

if [[ -n "$extra_in_guide" ]]; then
  echo ""
  echo "Commands in ${GUIDE_PATH} top-level table but not in 'cub-scout --help':"
  printf "%s\n" "$extra_in_guide"
  failed=1
fi

if [[ "$failed" -ne 0 ]]; then
  echo ""
  echo "CLI reference parity check failed."
  exit 1
fi

echo "CLI reference parity check passed."
