#!/usr/bin/env bash
#
# Demo: Import with GitOps Path Inference
#
# Shows how import infers variant from Flux/Argo deployer paths.
#
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Source the UI library
source "$REPO_ROOT/test/atk/lib/ui.sh"
ui_init "$REPO_ROOT"

clear

# Colors
RED="\033[38;5;196m"
GREEN="\033[38;5;82m"
YELLOW="\033[38;5;214m"
CYAN="\033[38;5;51m"
PURPLE="\033[38;5;141m"
BLUE="\033[38;5;75m"
DIM="\033[38;5;245m"
BOLD="\033[1m"
NC="\033[0m"

# ═══════════════════════════════════════════════════════════════════════════════
# HEADER
# ═══════════════════════════════════════════════════════════════════════════════

ui_header "IMPORT DEMO"

echo ""
echo -e "${DIM}Import workloads into ConfigHub with smart variant inference.${NC}"
echo -e "${DIM}Reads GitOps deployer paths directly from the cluster.${NC}"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# WHAT IT DOES
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "WHAT IT DOES" "infer variant from GitOps paths"

echo ""
echo -e "  ${CYAN}Deployer${NC}  ->  ${PURPLE}Path${NC}  ->  ${GREEN}Variant${NC}"
echo ""
echo -e "  ${DIM}For GitOps-managed workloads, import reads the path:${NC}"
echo -e "  ${CYAN}1.${NC} Flux Kustomization: ${PURPLE}spec.path${NC}"
echo -e "  ${CYAN}2.${NC} Argo Application:   ${PURPLE}spec.source.path${NC}"
echo -e "  ${CYAN}3.${NC} Extract variant from path pattern"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# FLUX EXAMPLE
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "FLUX EXAMPLE" "Kustomization spec.path"

echo ""
echo -e "  ${DIM}Kustomization object in cluster:${NC}"
echo ""
echo -e "  apiVersion: kustomize.toolkit.fluxcd.io/v1"
echo -e "  kind: Kustomization"
echo -e "  metadata:"
echo -e "    name: apps"
echo -e "  spec:"
echo -e "    path: ${GREEN}./staging${NC}              ${DIM}<- variant=staging${NC}"
echo -e "    sourceRef:"
echo -e "      kind: GitRepository"
echo -e "      name: infra-repo"
echo ""
echo -e "  ${DIM}Import reads path from LIVE cluster, not Git:${NC}"
echo ""
echo -e "  +---------------------------------------------------------------------+"
echo -e "  | ${BOLD}cub-agent import -n myapp --dry-run${NC}                              |"
echo -e "  +---------------------------------------------------------------------+"
echo -e "  |                                                                     |"
echo -e "  |  Workload: ${CYAN}myapp/payment-api${NC}                                       |"
echo -e "  |    Owner: ${PURPLE}Flux${NC}                                                      |"
echo -e "  |    Kustomization: apps (path: ${GREEN}./staging${NC})                          |"
echo -e "  |                                                                     |"
echo -e "  |  Suggested:                                                         |"
echo -e "  |    App Space: ${BLUE}myapp-team${NC}                                            |"
echo -e "  |    Unit: ${CYAN}payment-api-staging${NC}                                       |"
echo -e "  |      Labels: app=payment-api, ${GREEN}variant=staging${NC}                     |"
echo -e "  |                                                                     |"
echo -e "  +---------------------------------------------------------------------+"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# ARGO EXAMPLE
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "ARGO EXAMPLE" "Application spec.source.path"

echo ""
echo -e "  ${DIM}Application object in cluster:${NC}"
echo ""
echo -e "  apiVersion: argoproj.io/v1alpha1"
echo -e "  kind: Application"
echo -e "  metadata:"
echo -e "    name: cart-prod"
echo -e "  spec:"
echo -e "    source:"
echo -e "      repoURL: https://github.com/org/apps"
echo -e "      path: ${GREEN}tenants/checkout/cart/overlays/prod${NC}"
echo ""
echo -e "  ${DIM}Import extracts variant from path:${NC}"
echo ""
echo -e "  +---------------------------------------------------------------------+"
echo -e "  | ${BOLD}cub-agent import -n checkout --dry-run${NC}                           |"
echo -e "  +---------------------------------------------------------------------+"
echo -e "  |                                                                     |"
echo -e "  |  Workload: ${CYAN}checkout/cart${NC}                                           |"
echo -e "  |    Owner: ${BLUE}ArgoCD${NC}                                                    |"
echo -e "  |    Application: cart-prod                                           |"
echo -e "  |    Path: tenants/checkout/cart/overlays/${GREEN}prod${NC}                      |"
echo -e "  |                                                                     |"
echo -e "  |  Suggested:                                                         |"
echo -e "  |    App Space: ${BLUE}checkout-team${NC}                                         |"
echo -e "  |    Unit: ${CYAN}cart-prod${NC}                                                  |"
echo -e "  |      Labels: app=cart, ${GREEN}variant=prod${NC}                               |"
echo -e "  |                                                                     |"
echo -e "  +---------------------------------------------------------------------+"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# INFERENCE PRIORITY
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "INFERENCE PRIORITY" "how variant is determined"

echo ""
echo -e "  ${BOLD}Priority order for variant inference:${NC}"
echo ""
echo -e "  ${GREEN}0${NC}  Flux Kustomization ${PURPLE}spec.path${NC}        ${DIM}<- Most reliable${NC}"
echo -e "  ${GREEN}0${NC}  Argo Application ${PURPLE}spec.source.path${NC}"
echo -e "  ${YELLOW}1${NC}  K8s label ${CYAN}app.kubernetes.io/instance${NC}"
echo -e "  ${YELLOW}2${NC}  K8s label ${CYAN}environment${NC} or ${CYAN}env${NC}"
echo -e "  ${DIM}3${NC}  Namespace pattern (myapp-prod)"
echo -e "  ${DIM}4${NC}  Workload name (fallback)"
echo ""
echo -e "  ${DIM}GitOps paths take priority because the deployer explicitly stores them.${NC}"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# USAGE
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "USAGE" "commands"

echo ""
echo -e "  ${CYAN}cub-agent import --dry-run${NC}"
echo -e "  ${DIM}-> Preview import (discovers all namespaces)${NC}"
echo ""
echo -e "  ${CYAN}cub-agent import -n myapp --dry-run${NC}"
echo -e "  ${DIM}-> Preview import for one namespace${NC}"
echo ""
echo -e "  ${CYAN}cub-agent import -y${NC}"
echo -e "  ${DIM}-> Import without confirmation${NC}"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# KEY INSIGHT
# ═══════════════════════════════════════════════════════════════════════════════

echo "--------------------------------------------------------------------------------"
echo -e "KEY INSIGHT"
echo ""
echo -e "   You don't need to parse Git to infer variant."
echo -e "   The deployer objects store the path in the cluster."
echo ""
echo -e "   ${DIM}LIVE cluster -> Kustomization/Application -> spec.path -> variant${NC}"
echo "--------------------------------------------------------------------------------"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# COMMANDS SUMMARY
# ═══════════════════════════════════════════════════════════════════════════════

echo "  +------------------------------------------------------------------------+"
echo "  | COMMANDS                                                               |"
echo "  |                                                                        |"
echo "  |  cub-agent import                    Import all namespaces             |"
echo "  |  cub-agent import -n <ns>            Import one namespace              |"
echo "  |  cub-agent import --dry-run          Preview only                      |"
echo "  |  cub-agent import --json             JSON output for GUI               |"
echo "  |  cub-agent import --no-log           Disable logging                   |"
echo "  |                                                                        |"
echo -e "  |  ${DIM}Logs saved to: .confighub/logs/import-*.log${NC}                        |"
echo -e "  |  ${DIM}Full guide: docs/IMPORTING-WORKLOADS.md${NC}                            |"
echo "  +------------------------------------------------------------------------+"
echo ""
