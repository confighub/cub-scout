#!/usr/bin/env bash
#
# Demo: TUI Saved Queries
#
# Shows the saved queries feature with colored output.
#
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Source the UI library
source "$REPO_ROOT/test/atk/lib/ui.sh"
ui_init "$REPO_ROOT"

clear

# Colors
GREEN="\033[38;5;82m"
YELLOW="\033[38;5;214m"
CYAN="\033[38;5;51m"
PURPLE="\033[38;5;141m"
ORANGE="\033[38;5;208m"
DIM="\033[38;5;245m"
BOLD="\033[1m"
NC="\033[0m"

# ═══════════════════════════════════════════════════════════════════════════════
# HEADER
# ═══════════════════════════════════════════════════════════════════════════════

ui_header "🔍 SAVED QUERIES DEMO"

echo ""
echo -e "${DIM}Saved queries are named, reusable filters for resources.${NC}"
echo -e "${DIM}Run with: cub-agent map list -q <name>${NC}"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# BUILT-IN QUERIES
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "BUILT-IN QUERIES" "9 queries ship with the agent"

echo ""
printf "  ${BOLD}%-14s %-45s %-8s${NC}\n" "NAME" "DESCRIPTION" "MATCHES"
echo -e "  ${DIM}─────────────────────────────────────────────────────────────────────────${NC}"

# Colored rows showing different owners
echo -e "  ${YELLOW}$(printf "%-14s %-45s" "unmanaged" "Resources with no GitOps owner")${NC}  ${YELLOW}${BOLD}47${NC}"
echo -e "  ${GREEN}$(printf "%-14s %-45s" "gitops" "Resources managed by GitOps (Flux or Argo)")${NC}  ${GREEN}${BOLD}23${NC}"
echo -e "  ${CYAN}$(printf "%-14s %-45s" "flux" "All Flux-managed resources")${NC}  ${CYAN}${BOLD}15${NC}"
echo -e "  ${PURPLE}$(printf "%-14s %-45s" "argo" "All Argo CD-managed resources")${NC}  ${PURPLE}${BOLD}8${NC}"
echo -e "  ${ORANGE}$(printf "%-14s %-45s" "helm-only" "Helm-managed resources (no GitOps)")${NC}  ${ORANGE}${BOLD}5${NC}"
echo -e "  ${GREEN}$(printf "%-14s %-45s" "confighub" "Resources managed by ConfigHub")${NC}  ${GREEN}${BOLD}0${NC}"
echo -e "  ${DIM}$(printf "%-14s %-45s" "deployments" "All Deployments across namespaces")${NC}  ${DIM}12${NC}"
echo -e "  ${DIM}$(printf "%-14s %-45s" "services" "All Services across namespaces")${NC}  ${DIM}18${NC}"
echo -e "  ${DIM}$(printf "%-14s %-45s" "prod" "Resources in production namespaces")${NC}  ${DIM}31${NC}"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# QUERY EXPRESSIONS
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "UNDER THE HOOD" "query expressions"

echo ""
echo -e "${DIM}Each saved query expands to a query expression:${NC}"
echo ""
echo -e "  ${BOLD}unmanaged${NC}    → ${CYAN}owner=Native${NC}"
echo -e "  ${BOLD}gitops${NC}       → ${CYAN}owner=Flux OR owner=Argo${NC}"
echo -e "  ${BOLD}flux${NC}         → ${CYAN}owner=Flux${NC}"
echo -e "  ${BOLD}prod${NC}         → ${CYAN}namespace=prod* OR namespace=production*${NC}"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# USING QUERIES
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "USAGE" "run saved queries"

echo ""
echo -e "${DIM}Run a saved query by name:${NC}"
echo ""
echo -e "  ${CYAN}cub-agent map list -q unmanaged${NC}"
echo -e "  ${DIM}→ Shows all resources with no GitOps owner${NC}"
echo ""
echo -e "  ${CYAN}cub-agent map list -q gitops${NC}"
echo -e "  ${DIM}→ Shows Flux and Argo managed resources${NC}"
echo ""
echo -e "${DIM}Combine queries with filters:${NC}"
echo ""
echo -e "  ${CYAN}cub-agent map list -q \"unmanaged AND namespace=prod*\"${NC}"
echo -e "  ${DIM}→ Unmanaged resources in production namespaces${NC}"
echo ""
echo -e "  ${CYAN}cub-agent map list -q \"deployments AND flux\"${NC}"
echo -e "  ${DIM}→ Deployments managed by Flux${NC}"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# USER QUERIES
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "YOUR QUERIES" "saved to ~/.confighub/queries.yaml"

echo ""
echo -e "${DIM}Save your own queries:${NC}"
echo ""
echo -e "  ${CYAN}cub-agent map queries save my-team \"labels[team]=payments\"${NC}"
echo -e "  ${GREEN}✓ Saved query \"my-team\"${NC}"
echo -e "    Query: labels[team]=payments"
echo -e "    File:  ~/.confighub/queries.yaml"
echo ""
echo -e "${DIM}Run it:${NC}"
echo ""
echo -e "  ${CYAN}cub-agent map list -q my-team${NC}"
echo ""
echo -e "${DIM}Delete it:${NC}"
echo ""
echo -e "  ${CYAN}cub-agent map queries delete my-team${NC}"
echo -e "  ${GREEN}✓ Deleted query \"my-team\"${NC}"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# SAMPLE OUTPUT
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "SAMPLE OUTPUT" "cub-agent map list -q unmanaged"

echo ""
printf "  ${BOLD}%-20s %-15s %-35s %-8s${NC}\n" "NAMESPACE" "KIND" "NAME" "OWNER"
echo -e "  ${DIM}──────────────────────────────────────────────────────────────────────────────${NC}"
echo -e "  ${YELLOW}$(printf "%-20s %-15s %-35s %-8s" "argocd" "StatefulSet" "argocd-application-controller" "Native")${NC}"
echo -e "  ${YELLOW}$(printf "%-20s %-15s %-35s %-8s" "argocd" "Deployment" "argocd-server" "Native")${NC}"
echo -e "  ${YELLOW}$(printf "%-20s %-15s %-35s %-8s" "argocd" "Service" "argocd-server" "Native")${NC}"
echo -e "  ${YELLOW}$(printf "%-20s %-15s %-35s %-8s" "default" "ConfigMap" "kube-root-ca.crt" "Native")${NC}"
echo -e "  ${YELLOW}$(printf "%-20s %-15s %-35s %-8s" "monitoring" "Deployment" "prometheus" "Native")${NC}"
echo -e "  ${DIM}... (47 total)${NC}"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# CONFIGHUB HOOK
# ═══════════════════════════════════════════════════════════════════════════════

echo "────────────────────────────────────────────────────────────────────────────"
echo -e "🔗 ${BOLD}Want team-shared queries, alerts, and history?${NC}"
echo ""
echo -e "   ${CYAN}cub-agent map queries connect${NC}"
echo ""
echo "   → Sign up at https://confighub.com"
echo "   → Import workloads: cub-agent import --namespace <ns>"
echo "────────────────────────────────────────────────────────────────────────────"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# COMMANDS SUMMARY
# ═══════════════════════════════════════════════════════════════════════════════

echo "  ┌────────────────────────────────────────────────────────────────────┐"
echo "  │ COMMANDS                                                           │"
echo "  │                                                                    │"
echo "  │  cub-agent map queries           List all saved queries            │"
echo "  │  cub-agent map list -q <name>    Run a saved query                 │"
echo "  │  cub-agent map queries save ...  Save a user query                 │"
echo "  │  cub-agent map queries delete .. Delete a user query               │"
echo "  │  cub-agent map queries connect   Connect to ConfigHub              │"
echo "  │  ./test/atk/map queries          TUI view with live counts         │"
echo "  │                                                                    │"
echo -e "  │  ${DIM}Full guide: docs/TUI-SAVED-QUERIES.md${NC}                          │"
echo "  └────────────────────────────────────────────────────────────────────┘"
echo ""
