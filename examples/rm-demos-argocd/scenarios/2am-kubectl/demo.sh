#!/bin/bash
# Demo 2: "The 2AM kubectl"
#
# Scenario: Someone scaled prod manually during an incident.
# GitOps says "Synced". But who changed what? When? Why?
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
    echo -e "${BOLD}  Demo 2: The 2AM kubectl${NC}"
    echo -e "${BOLD}═══════════════════════════════════════════════════════════════${NC}"
    echo ""
}

print_scenario() {
    echo -e "${YELLOW}SCENARIO:${NC}"
    echo -e "  ${DIM}Tuesday morning standup.${NC}"
    echo -e "  \"Why did prod-us-east have ${BOLD}8 replicas${NC} overnight?\""
    echo ""
    echo -e "  Someone scaled it manually. GitOps says \"Synced.\""
    echo -e "  But ${BOLD}who${NC}? ${BOLD}When${NC}? ${BOLD}Why${NC}?"
    echo ""
    sleep 2
}

print_dirty_secret() {
    echo -e "${RED}THE DIRTY SECRET OF GITOPS:${NC}"
    echo ""
    echo -e "  kubectl still works. People use it."
    echo -e "  At 2am. During incidents. Without PRs."
    echo ""
    sleep 2
}

simulate_confusion() {
    echo -e "${DIM}# What's the current state? (ArgoCD says \"Synced\")${NC}"
    echo -e "${CYAN}\$ kubectl get deploy payment-api -n payments -o jsonpath='{.spec.replicas}'${NC}"
    echo "8"
    echo ""

    echo -e "${DIM}# But Git says...${NC}"
    echo -e "${CYAN}\$ cat overlays/prod-us-east/replicas-patch.yaml${NC}"
    echo "replicas: 5"
    echo ""

    echo -e "${DIM}# ArgoCD says...${NC}"
    echo -e "${CYAN}\$ argocd app get payment-api-prod-us-east${NC}"
    echo -e "Status: ${GREEN}Synced ✓${NC}"
    echo -e "Health: ${GREEN}Healthy ✓${NC}"
    echo ""

    echo -e "${RED}Wait, what?${NC} Git says 5, cluster has 8, ArgoCD says \"Synced\"?"
    echo ""
    sleep 2
}

simulate_configub_diff() {
    echo -e "${GREEN}THE CONFIGHUB WAY:${NC}"
    echo ""
    echo -e "${CYAN}\$ cub unit diff payment-api --cluster prod-us-east${NC}"
    echo ""
    sleep 1

    cat << 'EOF'
DRIFT DETECTED: payment-api @ prod-us-east

┌──────────────────────────────────────────────────────────────────┐
│  ConfigHub (Desired)              Cluster (Live)                 │
├──────────────────────────────────────────────────────────────────┤
│  spec.replicas: 5                 spec.replicas: 8               │
│                                                                  │
│  DRIFT SOURCE                                                    │
│  ─────────────                                                   │
│  Changed: 2026-01-12 02:47:23 UTC                               │
│  By: kubectl (user: oncall-sarah@acme.com)                      │
│  Context: Incident INC-4521 (payment latency spike)             │
│                                                                  │
│  Annotation found on resource:                                   │
│    kubernetes.io/change-cause: "emergency scale for INC-4521"   │
└──────────────────────────────────────────────────────────────────┘

OPTIONS:
  cub unit revert payment-api --cluster prod-us-east    # Force back to 5
  cub unit accept payment-api --cluster prod-us-east    # Accept 8 as new desired
  cub unit ignore payment-api --cluster prod-us-east    # Mark as expected drift
EOF
    echo ""
    sleep 2
}

simulate_fleet_drift_check() {
    echo -e "${CYAN}\$ cub unit list --where \"drift=true\"${NC}"
    echo ""

    cat << 'EOF'
DRIFTED UNITS (3)
───────────────────────────────────────────────────────────────────
payment-api     prod-us-east   replicas: 5→8    02:47 UTC  INC-4521
redis-cache     prod-us-east   replicas: 3→6    02:52 UTC  INC-4521
order-api       prod-us-east   replicas: 3→5    02:55 UTC  INC-4521

All 3 drifts are from the same incident.
EOF
    echo ""
    sleep 2
}

simulate_remediation() {
    echo -e "${GREEN}REMEDIATION (Make Changes THROUGH ConfigHub):${NC}"
    echo ""
    echo -e "${CYAN}\$ cub unit accept --where \"drift.cause=INC-4521\" \\${NC}"
    echo -e "${CYAN}    --reason \"INC-4521 scale-up should be permanent\"${NC}"
    echo ""
    sleep 1

    cat << 'EOF'
Accepting drift for 3 units...

Creating ChangeSet CS-892:
  payment-api (prod-us-east): replicas 5→8
  redis-cache (prod-us-east): replicas 3→6
  order-api (prod-us-east): replicas 3→5

Syncing back to Git...
  Created PR #1247: "Accept INC-4521 scale changes"
  URL: https://github.com/acme/configs/pull/1247

Desired state updated. Drift resolved. ✓
EOF
    echo ""
}

print_aha() {
    echo -e "${BOLD}THE \"AHA\" MOMENTS:${NC}"
    echo ""
    echo -e "  1. ConfigHub caught drift that ArgoCD missed"
    echo -e "  2. It shows ${BOLD}WHO${NC} made the change and ${BOLD}WHY${NC}"
    echo -e "  3. It finds ${BOLD}ALL${NC} related drift across the fleet"
    echo -e "  4. You can bulk revert or bulk accept"
    echo -e "  5. Changes sync back to Git automatically"
    echo ""
    echo -e "  ${GREEN}ConfigHub doesn't just detect drift — it resolves it properly.${NC}"
    echo ""
}

run_demo() {
    print_header
    print_scenario
    print_dirty_secret
    simulate_confusion
    simulate_configub_diff
    simulate_fleet_drift_check
    simulate_remediation
    print_aha
}

case "${1:-}" in
    --setup)
        echo "Setting up 2AM kubectl scenario..."
        echo "(In a real demo, this would cause drift on a cluster)"
        ;;
    --cleanup)
        echo "Cleaning up 2AM kubectl scenario..."
        ;;
    *)
        run_demo
        ;;
esac
