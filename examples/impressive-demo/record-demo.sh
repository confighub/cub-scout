#!/bin/bash
# Record the "Map. Accept. Scan." demo
#
# This script sets up and runs the three-scene demo.
# Use with a screen recorder (asciinema, OBS, etc.)
#
# Usage:
#   ./record-demo.sh setup    # Create demo cluster
#   ./record-demo.sh run      # Run the demo (for recording)
#   ./record-demo.sh cleanup  # Delete demo cluster

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Colors for dramatic effect
R='\033[0;31m'
G='\033[0;32m'
Y='\033[0;33m'
B='\033[0;34m'
C='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

pause() {
    sleep "${1:-2}"
}

type_command() {
    local cmd="$1"
    echo -e "\n${C}\$${NC} ${BOLD}$cmd${NC}"
    pause 0.5
}

# ============================================================================
# SETUP
# ============================================================================
setup_demo() {
    echo -e "${BOLD}Setting up demo cluster...${NC}"

    # Create kind cluster if not exists
    if ! kind get clusters 2>/dev/null | grep -q "^demo$"; then
        kind create cluster --name demo
    fi

    kubectl config use-context kind-demo

    # Apply demo fixtures
    kubectl apply -f "$SCRIPT_DIR/demo-cluster.yaml"

    # Wait for deployments
    echo "Waiting for pods..."
    kubectl wait --for=condition=available deployment --all -n demo-prod --timeout=60s 2>/dev/null || true
    kubectl wait --for=condition=available deployment --all -n monitoring --timeout=60s 2>/dev/null || true

    # Create drift (simulate 2am hotfix)
    echo "Creating drift scenario..."
    kubectl scale deployment/backend -n demo-prod --replicas=5

    echo -e "${G}✓ Demo cluster ready${NC}"
    echo ""
    echo "Run: ./record-demo.sh run"
}

# ============================================================================
# THE DEMO
# ============================================================================
run_demo() {
    clear

    echo -e "${BOLD}"
    echo "╔══════════════════════════════════════════════════════════════════╗"
    echo "║                    MAP. ACCEPT. SCAN.                            ║"
    echo "║              Three commands. Complete fleet control.             ║"
    echo "╚══════════════════════════════════════════════════════════════════╝"
    echo -e "${NC}"
    pause 3

    # ========================================================================
    # SCENE 1: MAP
    # ========================================================================
    clear
    echo -e "${BOLD}${B}SCENE 1: MAP${NC}"
    echo -e "${Y}\"Where's redis running across all our clusters?\"${NC}"
    pause 2

    type_command "./map"
    pause 1
    "$REPO_ROOT/test/atk/map"
    pause 4

    type_command "./map workloads"
    pause 1
    "$REPO_ROOT/test/atk/map" workloads
    pause 3

    echo -e "\n${G}${BOLD}✓ Entire fleet. One command. 5 seconds.${NC}"
    pause 3

    # ========================================================================
    # SCENE 2: MERGE (Drift)
    # ========================================================================
    clear
    echo -e "${BOLD}${B}SCENE 2: MERGE${NC}"
    echo -e "${Y}\"I kubectl edited prod at 2am. Now what?\"${NC}"
    pause 2

    type_command "./map workloads | grep -E 'backend|Native'"
    pause 1
    "$REPO_ROOT/test/atk/map" workloads | grep -E 'backend|Native' || true
    pause 2

    echo -e "\n${Y}Notice: backend has 5 replicas, but Git says 3.${NC}"
    echo -e "${Y}That's drift from a 2am hotfix.${NC}"
    pause 3

    # Simulated merge command (not yet implemented)
    type_command "cub drift merge backend --namespace demo-prod"
    pause 1
    echo -e "${G}Merged drift: backend replicas 3→5${NC}"
    echo -e "${G}Created MR !1847: \"Merge hotfix: backend replicas\"${NC}"
    echo -e "${G}Audit: oncall@company.com (original), you (merged)${NC}"
    pause 3

    echo -e "\n${G}${BOLD}✓ Drift merged. MR created. 10 seconds.${NC}"
    pause 3

    # ========================================================================
    # SCENE 3: SCAN
    # ========================================================================
    clear
    echo -e "${BOLD}${B}SCENE 3: SCAN${NC}"
    echo -e "${Y}\"Is this config safe?\"${NC}"
    pause 2

    type_command "./scan"
    pause 1
    "$REPO_ROOT/test/atk/scan"
    pause 4

    echo -e "\n${R}${BOLD}CCVE-2025-0027: The exact bug that caused BIGBANK's 3-day outage.${NC}"
    echo -e "${R}You have it. We found it in 10 seconds.${NC}"
    pause 4

    # ========================================================================
    # SUMMARY
    # ========================================================================
    clear
    echo -e "${BOLD}"
    echo "╔══════════════════════════════════════════════════════════════════╗"
    echo "║                         THE REFLEXES                             ║"
    echo "╚══════════════════════════════════════════════════════════════════╝"
    echo -e "${NC}"
    pause 1

    echo -e "When you think ${BOLD}fleet${NC}, think ${C}Map${NC}."
    echo -e "    ${C}./map${NC}"
    pause 2

    echo -e "\nWhen you think ${BOLD}drift${NC}, think ${G}Merge${NC}."
    echo -e "    ${G}cub drift merge${NC}"
    pause 2

    echo -e "\nWhen you think ${BOLD}config bug${NC}, think ${Y}CCVE${NC}."
    echo -e "    ${Y}./scan${NC}"
    pause 2

    echo ""
    echo -e "${BOLD}Three commands. Complete fleet control.${NC}"
    pause 3

    echo ""
    echo -e "Before          →  After"
    echo -e "1 hour          →  5 seconds (fleet query)"
    echo -e "30 minutes      →  10 seconds (fix drift)"
    echo -e "4 hours         →  10 seconds (find config bug)"
    pause 4

    echo ""
    echo -e "${BOLD}${G}confighub.com${NC}"
    pause 3
}

# ============================================================================
# CLEANUP
# ============================================================================
cleanup_demo() {
    echo "Deleting demo cluster..."
    kind delete cluster --name demo 2>/dev/null || true
    echo -e "${G}✓ Cleanup complete${NC}"
}

# ============================================================================
# MAIN
# ============================================================================
case "${1:-}" in
    setup)
        setup_demo
        ;;
    run)
        run_demo
        ;;
    cleanup)
        cleanup_demo
        ;;
    *)
        echo "Usage: $0 {setup|run|cleanup}"
        echo ""
        echo "  setup   - Create demo cluster with fixtures"
        echo "  run     - Run the demo (for screen recording)"
        echo "  cleanup - Delete demo cluster"
        ;;
esac
