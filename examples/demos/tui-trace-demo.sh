#!/usr/bin/env bash
#
# Demo: GitOps Trace
#
# Shows the trace feature - following resources back to their Git source.
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

ui_header "🔍 GITOPS TRACE DEMO"

echo ""
echo -e "${DIM}Trace any resource back to its Git source.${NC}"
echo -e "${DIM}Press 't' in TUI or run: cub-agent trace <resource>${NC}"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# WHAT IT DOES
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "WHAT IT DOES" "follow the delivery chain"

echo ""
echo -e "  ${PURPLE}Git Source${NC}  →  ${CYAN}Deployer${NC}  →  ${GREEN}Resource${NC}"
echo ""
echo -e "  ${DIM}For any Kubernetes resource, trace shows:${NC}"
echo -e "  ${CYAN}1.${NC} The Git repository it comes from"
echo -e "  ${CYAN}2.${NC} The deployer (Kustomization, HelmRelease, Application)"
echo -e "  ${CYAN}3.${NC} Status at each level of the chain"
echo -e "  ${CYAN}4.${NC} Where the chain is broken (if any)"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# HEALTHY CHAIN
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "HEALTHY CHAIN" "all levels in sync"

echo ""
echo -e "  ┌─────────────────────────────────────────────────────────────────────┐"
echo -e "  │ ${BOLD}TRACE: Deployment/nginx${NC}                                             │"
echo -e "  ├─────────────────────────────────────────────────────────────────────┤"
echo -e "  │                                                                     │"
echo -e "  │   ${GREEN}✓${NC} ${PURPLE}GitRepository/infra-repo${NC}                                     │"
echo -e "  │       │ URL: https://github.com/your-org/infra.git                  │"
echo -e "  │       │ Revision: main@sha1:abc123f                                 │"
echo -e "  │       │ Status: ${GREEN}Artifact is up to date${NC}                            │"
echo -e "  │       │                                                             │"
echo -e "  │       └─▶ ${GREEN}✓${NC} ${CYAN}Kustomization/apps${NC}                                 │"
echo -e "  │               │ Path: ./clusters/prod/apps                          │"
echo -e "  │               │ Status: ${GREEN}Applied revision main@sha1:abc123f${NC}       │"
echo -e "  │               │                                                     │"
echo -e "  │               └─▶ ${GREEN}✓${NC} Deployment/nginx                             │"
echo -e "  │                       Status: ${GREEN}3/3 ready${NC}                          │"
echo -e "  │                                                                     │"
echo -e "  ├─────────────────────────────────────────────────────────────────────┤"
echo -e "  │ ${GREEN}✓ All levels in sync.${NC} Managed by ${CYAN}flux${NC}.                          │"
echo -e "  └─────────────────────────────────────────────────────────────────────┘"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# BROKEN CHAIN
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "BROKEN CHAIN" "find the problem"

echo ""
echo -e "  ┌─────────────────────────────────────────────────────────────────────┐"
echo -e "  │ ${BOLD}TRACE: Deployment/broken-app${NC}                                        │"
echo -e "  ├─────────────────────────────────────────────────────────────────────┤"
echo -e "  │                                                                     │"
echo -e "  │   ${GREEN}✓${NC} ${PURPLE}GitRepository/infra-repo${NC}                                     │"
echo -e "  │       │ Revision: main@sha1:def456                                  │"
echo -e "  │       │ Status: ${GREEN}Artifact is up to date${NC}                            │"
echo -e "  │       │                                                             │"
echo -e "  │       └─▶ ${RED}✗${NC} ${CYAN}Kustomization/apps${NC}        ${RED}◀── PROBLEM HERE${NC}        │"
echo -e "  │               │ Status: ${YELLOW}Reconciliation failed${NC}                    │"
echo -e "  │               │ ${RED}Error: path './clusters/prod/apps' not found${NC}     │"
echo -e "  │               │                                                     │"
echo -e "  │               └─▶ Deployment/broken-app (stale)                     │"
echo -e "  │                                                                     │"
echo -e "  ├─────────────────────────────────────────────────────────────────────┤"
echo -e "  │ ${YELLOW}⚠ Chain broken at Kustomization/apps${NC}                             │"
echo -e "  └─────────────────────────────────────────────────────────────────────┘"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# ARGO CD
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "ARGO CD" "trace works with Argo too"

echo ""
echo -e "  ┌─────────────────────────────────────────────────────────────────────┐"
echo -e "  │ ${BOLD}TRACE: Application/frontend-app${NC}                                     │"
echo -e "  ├─────────────────────────────────────────────────────────────────────┤"
echo -e "  │                                                                     │"
echo -e "  │   ${GREEN}✓${NC} ${PURPLE}Source/your-org/frontend${NC}                                     │"
echo -e "  │       │ URL: https://github.com/your-org/frontend.git               │"
echo -e "  │       │ Revision: v2.1.0                                            │"
echo -e "  │       │                                                             │"
echo -e "  │       └─▶ ${GREEN}✓${NC} ${BLUE}Application/frontend-app${NC}                          │"
echo -e "  │               │ Status: ${GREEN}Synced / Healthy${NC}                          │"
echo -e "  │               │                                                     │"
echo -e "  │               ├─▶ ${GREEN}✓${NC} Deployment/frontend                          │"
echo -e "  │               ├─▶ ${GREEN}✓${NC} Service/frontend                             │"
echo -e "  │               └─▶ ${GREEN}✓${NC} ConfigMap/frontend-config                    │"
echo -e "  │                                                                     │"
echo -e "  ├─────────────────────────────────────────────────────────────────────┤"
echo -e "  │ ${GREEN}✓ All levels in sync.${NC} Managed by ${BLUE}argocd${NC}.                        │"
echo -e "  └─────────────────────────────────────────────────────────────────────┘"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# ORPHAN
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "ORPHAN DETECTION" "resources with no GitOps owner"

echo ""
echo -e "  ┌─────────────────────────────────────────────────────────────────────┐"
echo -e "  │ ${BOLD}TRACE: Deployment/mystery-app${NC}                                       │"
echo -e "  ├─────────────────────────────────────────────────────────────────────┤"
echo -e "  │                                                                     │"
echo -e "  │   ${YELLOW}⚠ No GitOps owner detected${NC}                                       │"
echo -e "  │       │ Labels: app=mystery-app                                     │"
echo -e "  │       │ Created: 2025-12-15 via kubectl                             │"
echo -e "  │       │                                                             │"
echo -e "  │       └─▶ Deployment/mystery-app                                    │"
echo -e "  │               Status: Running (no sync tracking)                    │"
echo -e "  │                                                                     │"
echo -e "  ├─────────────────────────────────────────────────────────────────────┤"
echo -e "  │ ${YELLOW}⚠ Resource not managed by GitOps${NC}                                  │"
echo -e "  │   Consider adding to a Kustomization or Argo Application            │"
echo -e "  └─────────────────────────────────────────────────────────────────────┘"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# USAGE
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "USAGE" "commands and options"

echo ""
echo -e "  ${CYAN}cub-agent trace deployment/nginx -n demo${NC}"
echo -e "  ${DIM}→ Trace a specific resource${NC}"
echo ""
echo -e "  ${CYAN}cub-agent trace --app frontend-app${NC}"
echo -e "  ${DIM}→ Trace an Argo CD application by name${NC}"
echo ""
echo -e "  ${CYAN}cub-agent trace deployment/nginx --json${NC}"
echo -e "  ${DIM}→ JSON output for scripting${NC}"
echo ""
echo -e "  ${CYAN}./test/atk/map${NC} then press ${BOLD}t${NC}"
echo -e "  ${DIM}→ Interactive trace picker in TUI${NC}"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# USE CASES
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "USE CASES" "when to use trace"

echo ""
echo -e "  ${BOLD}\"Why isn't my change deployed?\"${NC}"
echo -e "  ${DIM}Trace shows if Git→Deployer→Resource chain is healthy${NC}"
echo ""
echo -e "  ${BOLD}\"What manages this resource?\"${NC}"
echo -e "  ${DIM}Trace shows the full ownership chain${NC}"
echo ""
echo -e "  ${BOLD}\"Find the broken link\"${NC}"
echo -e "  ${DIM}Trace highlights exactly where the chain failed${NC}"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# CONFIGHUB HOOK
# ═══════════════════════════════════════════════════════════════════════════════

echo "────────────────────────────────────────────────────────────────────────────"
echo -e "🔗 ${BOLD}Want fleet-wide tracing?${NC}"
echo ""
echo -e "   ${CYAN}cub-agent trace --confighub${NC}"
echo ""
echo "   → Sign up at https://confighub.com"
echo "   → See trace chains across all clusters"
echo "   → Track broken chains and orphans over time"
echo "────────────────────────────────────────────────────────────────────────────"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# COMMANDS SUMMARY
# ═══════════════════════════════════════════════════════════════════════════════

echo "  ┌────────────────────────────────────────────────────────────────────┐"
echo "  │ COMMANDS                                                           │"
echo "  │                                                                    │"
echo "  │  cub-agent trace <kind/name>    Trace a resource                   │"
echo "  │  cub-agent trace -n <ns> ...    Specify namespace                  │"
echo "  │  cub-agent trace --app <name>   Trace Argo CD app by name          │"
echo "  │  cub-agent trace --json         JSON output for scripting          │"
echo "  │  ./test/atk/map then 't'        Interactive TUI trace              │"
echo "  │                                                                    │"
echo -e "  │  ${DIM}Full guide: docs/TUI-TRACE.md${NC}                                  │"
echo "  └────────────────────────────────────────────────────────────────────┘"
echo ""
