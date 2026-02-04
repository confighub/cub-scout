#!/usr/bin/env bash
# TUI Golden Update Script
#
# Updates the TUI snapshot golden files when intentional UI changes are made.
#
# Usage:
#   ./scripts/tui-golden-update.sh
#
# After running:
#   1. Review the git diff for test/ascii/tui/*.txt
#   2. Verify changes are intentional
#   3. Commit with message explaining the UI change
#
# Golden files:
#   test/ascii/tui/main_view_80x24.txt    - Main dashboard (80×24)
#   test/ascii/tui/main_view_120x40.txt   - Main dashboard (120×40)
#   test/ascii/tui/help_80x24.txt         - Help overlay (80×24)
#   test/ascii/tui/help_120x40.txt        - Help overlay (120×40)
#   test/ascii/tui/error_no_data.txt      - Error state (no data)
#   test/ascii/tui/workloads_80x24.txt    - Workloads panel (80×24)
#   test/ascii/tui/workloads_120x40.txt   - Workloads panel (120×40)
#   test/ascii/tui/tree.txt               - Tree panel
#   test/ascii/tui/tree_filtered.txt      - Tree panel (filtered)
#   test/ascii/tui/trace.txt              - Trace picker
#   test/ascii/tui/map.txt                - Maps panel
#   test/ascii/tui/suggest_workloads.txt  - Suggestions (workloads)
#   test/ascii/tui/suggest_dashboard.txt  - Suggestions (dashboard)
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

echo "Updating TUI snapshot golden files..."
echo ""

go test ./cmd/cub-scout -run 'TestTUISnapshot_' -update-tui -v 2>&1 | grep -E '(updated golden|PASS|FAIL)'

echo ""
echo "Done. Review changes:"
echo "  git diff test/ascii/tui/"
echo ""
echo "If changes are intentional, commit them."
