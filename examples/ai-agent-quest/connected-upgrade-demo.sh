#!/usr/bin/env bash
#
# connected-upgrade-demo.sh — The Wall → The Upgrade
#
# Demonstrates the moment ConfigHub transforms AI capabilities.
# Designed for asciinema recordings and conference demos.
#
# Usage:
#   ./connected-upgrade-demo.sh              # Full demo (requires ConfigHub auth)
#   ./connected-upgrade-demo.sh --wall-only  # Just Stage 4 (no auth needed)
#
# Prerequisites:
#   - cub-scout binary built (./cub-scout version)
#   - kubectl context pointing at a cluster
#   - For full demo: cub auth login completed
#
set -euo pipefail

WALL_ONLY=false
if [[ "${1:-}" == "--wall-only" ]]; then
  WALL_ONLY=true
fi

# --- Helpers ---

banner() {
  echo ""
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "  $1"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo ""
}

pause() {
  if [[ -t 0 ]]; then
    read -r -p "  [press enter to continue] "
  fi
}

# --- Act 1: What the AI CAN see ---

banner "ACT 1: What Your AI Can See (Standalone)"

echo "  Your AI uses cub-scout to observe the cluster..."
echo ""

echo "  → Ownership map:"
./cub-scout map list --json 2>/dev/null | jq -r '.[].owner' | sort | uniq -c | sort -rn | sed 's/^/    /'
echo ""

echo "  → Pipeline health:"
./cub-scout gitops status 2>/dev/null | head -15 | sed 's/^/    /'
echo ""

echo "  → Risk scan:"
./cub-scout scan 2>/dev/null | head -20 | sed 's/^/    /'
echo ""

echo "  Your AI can see WHAT exists, WHO owns it, and WHAT is broken."
pause

# --- Stage 4: The Wall ---

banner "STAGE 4: The Wall"

echo "  Now ask your AI the hard questions..."
echo ""

echo '  Q: "What changed in the last week?"'
echo ""
echo "    The cluster API has no change history."
echo "    Your AI cannot answer this."
echo ""

echo '  Q: "Is this cluster different from the others?"'
echo ""
echo "    The cluster API only sees this one cluster."
echo "    Your AI cannot compare."
echo ""

echo '  Q: "What is the blast radius of changing shared-db-config?"'
echo ""
echo "    The cluster API has no dependency graph."
echo "    Your AI cannot predict impact."
echo ""

echo '  Q: "What SHOULD be running here?"'
echo ""
echo "    The cluster API shows observed state, not intent."
echo "    Your AI sees WHAT, but not WHY."
echo ""

echo "  ┌─────────────────────────────────────────────────┐"
echo "  │  Sight without memory is not enough.            │"
echo "  │                                                 │"
echo "  │  The Kubernetes API is a snapshot of NOW.       │"
echo "  │  It has no memory. No fleet context.            │"
echo "  │  No intent model.                               │"
echo "  │                                                 │"
echo "  │  Your AI needs a data resource that provides    │"
echo "  │  history, intent, and fleet context.            │"
echo "  │                                                 │"
echo "  │  That resource is ConfigHub.                    │"
echo "  └─────────────────────────────────────────────────┘"
echo ""

if [[ "$WALL_ONLY" == true ]]; then
  echo "  (--wall-only mode: stopping here)"
  echo ""
  echo "  To see the upgrade, run: cub auth login && $0"
  exit 0
fi

pause

# --- Stage 5: The Upgrade ---

banner "STAGE 5: The Upgrade"

echo "  Checking ConfigHub connection..."
echo ""

if ! command -v cub >/dev/null 2>&1; then
  echo "  ✗ cub CLI not found."
  echo "    Install: https://confighub.com/docs/cli"
  echo "    Then: cub auth login"
  exit 1
fi

if ! cub auth get-token >/dev/null 2>&1; then
  echo "  ✗ Not authenticated to ConfigHub."
  echo "    Run: cub auth login"
  exit 1
fi

echo "  ✓ Connected to ConfigHub."
echo ""

echo "  → Connection status:"
./cub-scout status 2>/dev/null | sed 's/^/    /'
echo ""

pause

banner "THE SAME QUESTIONS — NOW WITH ANSWERS"

echo '  Q: "What changed in the last week?"'
echo ""
echo "    With ConfigHub, your AI queries ChangeSets:"
echo ""
echo "    (run: ./cub-scout history deploy/api -n myapp-prod)"
echo ""

echo '  Q: "Is this cluster different from the others?"'
echo ""
echo "    With ConfigHub, your AI compares across the fleet:"
echo ""
echo "    (run: ./cub-scout fleet outliers)"
echo ""

echo '  Q: "What SHOULD be running here?"'
echo ""
echo "    With ConfigHub, your AI compares intent vs reality:"
echo ""
echo "    (run: ./cub-scout compare deploy/api -n myapp-prod)"
echo ""

echo "  ┌─────────────────────────────────────────────────┐"
echo "  │  My AI remembers. My AI compares.               │"
echo "  │  My AI predicts.                                │"
echo "  │                                                 │"
echo "  │  Same AI. Same cub-scout.                       │"
echo "  │  The only difference: ConfigHub as a data       │"
echo "  │  resource behind the MCP gateway.               │"
echo "  └─────────────────────────────────────────────────┘"
echo ""

banner "QUEST COMPLETE"

echo "  Title earned: Agent of Intent and Evidence"
echo ""
echo "  Generate your full report:"
echo "    Ask your AI to fill in oracle-report-template.md"
echo ""
