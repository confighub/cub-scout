#!/bin/bash
# Demo 3: "The Critical Security Patch"
#
# Scenario: CVE announced. 847 services need updating. How long?
#
# Usage:
#   ./demo.sh           # Run the full demo
#   ./demo.sh --setup   # Just set up the scenario
#   ./demo.sh --cleanup # Remove demo resources

set -eo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../../.." && pwd)"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m'

print_header() {
    echo ""
    echo -e "${YELLOW}╔═══════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${YELLOW}║  ⚠️  SIMULATION - This demo shows what ConfigHub WILL do      ║${NC}"
    echo -e "${YELLOW}║     when Rendered Manifest features are implemented.          ║${NC}"
    echo -e "${YELLOW}║     Output is simulated, not real cluster data.               ║${NC}"
    echo -e "${YELLOW}╚═══════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo -e "${BOLD}═══════════════════════════════════════════════════════════════${NC}"
    echo -e "${BOLD}  Demo 3: The Critical Security Patch${NC}"
    echo -e "${BOLD}═══════════════════════════════════════════════════════════════${NC}"
    echo ""
}

print_scenario() {
    echo -e "${RED}SCENARIO:${NC}"
    echo -e "  ${DIM}Friday 4pm. Slack explodes.${NC}"
    echo -e "  \"${BOLD}CVE-2026-1234${NC} — critical vulnerability in base image.\""
    echo ""
    echo -e "  You have ${BOLD}847 microservices${NC} across ${BOLD}47 clusters${NC}."
    echo -e "  How long to patch everything?"
    echo ""
    sleep 2
}

print_old_way() {
    echo -e "${RED}THE OLD WAY:${NC}"
    echo ""
    echo -e "  ${DIM}# For each of 847 services:${NC}"
    echo -e "  ${DIM}#   1. Find the repo${NC}"
    echo -e "  ${DIM}#   2. Update the base image${NC}"
    echo -e "  ${DIM}#   3. Create PR${NC}"
    echo -e "  ${DIM}#   4. Wait for CI${NC}"
    echo -e "  ${DIM}#   5. Get approval${NC}"
    echo -e "  ${DIM}#   6. Merge${NC}"
    echo -e "  ${DIM}#   7. Wait for ArgoCD to sync${NC}"
    echo -e "  ${DIM}#   8. Verify deployment${NC}"
    echo ""
    echo -e "  ${DIM}Estimated time: 847 PRs × 15 min each = ${RED}212 hours${NC}"
    echo -e "  ${DIM}Reality: \"We'll do it next sprint\"${NC}"
    echo ""
    sleep 2
}

simulate_impact_query() {
    echo -e "${GREEN}THE CONFIGHUB WAY:${NC}"
    echo ""
    echo -e "${DIM}# Step 1: How bad is it? (30 seconds)${NC}"
    echo -e "${CYAN}\$ cub unit list --where \"image.base=alpine:3.18*\"${NC}"
    echo ""
    sleep 1

    cat << 'EOF'
AFFECTED UNITS: 847

By Team:
  payments-team:     127 units
  orders-team:       89 units
  inventory-team:    234 units
  platform-team:     397 units

By Environment:
  production:        312 units (47 clusters)
  staging:           285 units (12 clusters)
  development:       250 units (3 clusters)

Oldest image: alpine:3.18.0 (deployed 2025-09-14)
Newest image: alpine:3.18.4 (deployed 2026-01-10)
EOF
    echo ""
    sleep 2
}

simulate_dry_run() {
    echo -e "${DIM}# Step 2: What's the fix? (Preview without applying)${NC}"
    echo -e "${CYAN}\$ cub unit update \\${NC}"
    echo -e "${CYAN}    --where \"image.base=alpine:3.18*\" \\${NC}"
    echo -e "${CYAN}    --set image.base=alpine:3.19.1 \\${NC}"
    echo -e "${CYAN}    --dry-run${NC}"
    echo ""
    sleep 1

    cat << 'EOF'
DRY RUN: Would update 847 units

Changes by team (requires their approval):
  payments-team:     127 units → ChangeSet for @payments-leads
  orders-team:       89 units  → ChangeSet for @orders-leads
  inventory-team:    234 units → ChangeSet for @inventory-leads
  platform-team:     397 units → ChangeSet for @platform-leads

Rollout strategy (based on policies):
  Phase 1: development (250 units) — auto-approve
  Phase 2: staging (285 units) — auto-approve after dev healthy
  Phase 3: production (312 units) — requires manual approval

Estimated time: 2-4 hours (phased rollout)
EOF
    echo ""
    sleep 2
}

simulate_apply() {
    echo -e "${DIM}# Step 3: Do it. (One command)${NC}"
    echo -e "${CYAN}\$ cub unit update \\${NC}"
    echo -e "${CYAN}    --where \"image.base=alpine:3.18*\" \\${NC}"
    echo -e "${CYAN}    --set image.base=alpine:3.19.1 \\${NC}"
    echo -e "${CYAN}    --reason \"CVE-2026-1234 critical security patch\"${NC}"
    echo ""
    sleep 1

    echo "Creating ChangeSets..."
    echo ""
    echo "CS-901: payments-team (127 units)     → Pending approval from @payments-leads"
    echo "CS-902: orders-team (89 units)        → Pending approval from @orders-leads"
    echo "CS-903: inventory-team (234 units)    → Pending approval from @inventory-leads"
    echo "CS-904: platform-team (397 units)     → Pending approval from @platform-leads"
    echo ""
    echo "Development phase auto-approved (250 units rendering...)"

    # Animated progress bar
    for i in {1..40}; do
        printf "\r  "
        for j in $(seq 1 $i); do printf "█"; done
        for j in $(seq $i 39); do printf "░"; done
        printf " %d%%" $((i * 100 / 40))
        sleep 0.05
    done
    echo ""
    echo "  Pushing to OCI registries... done"
    echo "  ArgoCD syncing... 250/250 synced"
    echo ""
    echo "Staging phase starting in 10 minutes (waiting for dev health checks)..."
    echo ""
    sleep 2
}

simulate_rollout_status() {
    echo -e "${DIM}# Step 4: Watch the rollout${NC}"
    echo -e "${CYAN}\$ cub rollout status --where \"reason~=CVE-2026-1234\" --watch${NC}"
    echo ""

    cat << 'EOF'
CVE-2026-1234 PATCH ROLLOUT

Phase 1: Development ████████████████████ 250/250 ✓ Complete
Phase 2: Staging     ████████████░░░░░░░░ 187/285   Progressing
Phase 3: Production  ░░░░░░░░░░░░░░░░░░░░   0/312   Waiting for approval

Approvals:
  ✓ @payments-leads approved CS-901 (127 units)
  ✓ @orders-leads approved CS-902 (89 units)
  ⏳ @inventory-leads pending CS-903 (234 units)
  ⏳ @platform-leads pending CS-904 (397 units)

ETA to full rollout: 1h 45m (after approvals)
EOF
    echo ""
    sleep 2
}

print_comparison() {
    echo -e "${BOLD}THE COMPARISON:${NC}"
    echo ""
    cat << 'EOF'
┌─────────────────────────────────────────────────────────────────────┐
│  Traditional GitOps              ConfigHub RM Pattern               │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  847 repos × PR workflow         1 command                          │
│  ─────────────────────           ─────────                          │
│                                                                     │
│  Week 1: 200 PRs created         Minute 1: Preview impact           │
│  Week 2: 400 PRs merged          Minute 2: Create ChangeSets        │
│  Week 3: Still 247 pending       Minute 15: Dev complete            │
│  Week 4: "Can we close these?"   Hour 2: Staging complete           │
│                                  Hour 4: Production complete        │
│                                                                     │
│  Audit: "Check 847 PR threads"   Audit: cub audit CVE-2026-1234    │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
EOF
    echo ""
}

print_aha() {
    echo -e "${BOLD}THE \"AHA\" MOMENTS:${NC}"
    echo ""
    echo -e "  1. ${BOLD}847 services patched with ONE command${NC} (not 847 PRs)"
    echo -e "  2. Respects team ownership — each team approves their own"
    echo -e "  3. Phased rollout built-in — dev → staging → prod"
    echo -e "  4. Full audit trail — every change tied to CVE-2026-1234"
    echo -e "  5. ArgoCD does the deployment — ConfigHub orchestrates"
    echo ""
    echo -e "  ${GREEN}Security patch in 4 hours, not 4 weeks.${NC}"
    echo ""
}

run_demo() {
    print_header
    print_scenario
    print_old_way
    simulate_impact_query
    simulate_dry_run
    simulate_apply
    simulate_rollout_status
    print_comparison
    print_aha
}

case "${1:-}" in
    --setup)
        echo "Setting up Security Patch scenario..."
        echo "(In a real demo, this would create 847 mock services)"
        ;;
    --cleanup)
        echo "Cleaning up Security Patch scenario..."
        ;;
    *)
        run_demo
        ;;
esac
