#!/bin/bash
# Demo 1: "The Monday Morning Panic"
#
# Scenario: 47 clusters, PagerDuty fires, ArgoCD shows green.
# Where's the problem?
#
# Usage:
#   ./demo.sh           # Run the full demo
#   ./demo.sh --setup   # Just set up the scenario
#   ./demo.sh --cleanup # Remove demo resources

set -eo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../../.." && pwd)"

# Source UI library if available
[[ -f "$REPO_ROOT/test/atk/lib/ui.sh" ]] && source "$REPO_ROOT/test/atk/lib/ui.sh"

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
    echo -e "${BOLD}  Demo 1: The Monday Morning Panic${NC}"
    echo -e "${BOLD}═══════════════════════════════════════════════════════════════${NC}"
    echo ""
}

print_scenario() {
    echo -e "${YELLOW}SCENARIO:${NC}"
    echo -e "  ${DIM}8:47 AM Monday. PagerDuty fires.${NC}"
    echo -e "  ${DIM}\"Payment API errors spiking in production.\"${NC}"
    echo ""
    echo -e "  You have ${BOLD}47 clusters${NC}. ArgoCD shows ${GREEN}green${NC} everywhere."
    echo -e "  Where do you even start?"
    echo ""
    sleep 2
}

print_old_way() {
    echo -e "${RED}THE OLD WAY:${NC}"
    echo ""
    echo -e "  ${DIM}# Check ArgoCD UI... cluster 1... looks fine${NC}"
    echo -e "  ${DIM}# Check ArgoCD UI... cluster 2... looks fine${NC}"
    echo -e "  ${DIM}# Check ArgoCD UI... cluster 3...${NC}"
    echo -e "  ${DIM}# ...${NC}"
    echo -e "  ${DIM}# (47 clusters later, 45 minutes wasted)${NC}"
    echo ""
    sleep 2
}

simulate_fleet_query() {
    echo -e "${GREEN}THE CONFIGHUB WAY:${NC}"
    echo ""
    echo -e "  ${CYAN}\$ cub unit list --where \"app=payment-api\"${NC}"
    echo ""
    sleep 1

    # Simulated output
    cat << 'EOF'
UNIT         CLUSTER          VERSION   PODS   STATUS
───────────────────────────────────────────────────────────────
payment-api  prod-us-east-1   v2.3.1    5/5    ✓ Synced
payment-api  prod-us-east-2   v2.3.1    5/5    ✓ Synced
payment-api  prod-us-west-1   v2.3.1    5/5    ✓ Synced
payment-api  prod-us-west-2   v2.3.1    5/5    ✓ Synced
payment-api  prod-eu-west-1   v2.3.1    5/5    ✓ Synced
EOF
    echo -e "payment-api  prod-eu-west-2   v2.3.0    3/5    ${YELLOW}⚠ BEHIND${NC}    ${RED}← FOUND IT${NC}"
    cat << 'EOF'
payment-api  prod-ap-south-1  v2.3.1    5/5    ✓ Synced
... (40 more, all v2.3.1)

Summary: 46 current, 1 behind (prod-eu-west-2 @ v2.3.0)
EOF
    echo ""
    sleep 2
}

simulate_investigation() {
    echo -e "${CYAN}\$ cub unit history payment-api --cluster prod-eu-west-2${NC}"
    echo ""
    sleep 1

    cat << 'EOF'
HISTORY: payment-api @ prod-eu-west-2

TIME                 VERSION   STATUS    DETAILS
───────────────────────────────────────────────────────────────
2026-01-13 05:23:00  v2.3.0    Synced    Last successful sync
2026-01-13 08:15:00  v2.3.1    Failed    OCI pull timeout
2026-01-13 08:30:00  v2.3.1    Failed    OCI pull timeout (retry 1)
2026-01-13 08:45:00  v2.3.1    Failed    OCI pull timeout (retry 2)

ROOT CAUSE: Registry connectivity issue in eu-west-2
ArgoCD status shows "Synced" because v2.3.0 sync succeeded.
ArgoCD does NOT know it's behind.
EOF
    echo ""
}

print_aha() {
    echo -e "${BOLD}THE \"AHA\" MOMENT:${NC}"
    echo ""
    echo -e "  ArgoCD said \"Synced\" because it synced ${BOLD}something${NC}."
    echo -e "  It didn't know it was behind. ${GREEN}ConfigHub knows.${NC}"
    echo ""
    echo -e "  ${DIM}Old way: 45 minutes checking 47 ArgoCD UIs${NC}"
    echo -e "  ${GREEN}New way: 30 seconds with one command${NC}"
    echo ""
}

run_demo() {
    print_header
    print_scenario
    print_old_way
    simulate_fleet_query
    simulate_investigation
    print_aha
}

case "${1:-}" in
    --setup)
        echo "Setting up Monday Panic scenario..."
        echo "(In a real demo, this would create 47 mock clusters)"
        ;;
    --cleanup)
        echo "Cleaning up Monday Panic scenario..."
        ;;
    *)
        run_demo
        ;;
esac
