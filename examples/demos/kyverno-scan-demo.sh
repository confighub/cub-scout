#!/usr/bin/env bash
#
# Demo: Kyverno Policy Scan
#
# Shows the Kyverno policy scanning feature with colored output.
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
DIM="\033[38;5;245m"
BOLD="\033[1m"
NC="\033[0m"

# ═══════════════════════════════════════════════════════════════════════════════
# HEADER
# ═══════════════════════════════════════════════════════════════════════════════

ui_header "🛡️ KYVERNO POLICY SCAN DEMO"

echo ""
echo -e "${DIM}Scan for Kyverno policy violations and map them to KPOL CCVEs.${NC}"
echo -e "${DIM}Press 'c' in TUI or run: cub-agent scan${NC}"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# WHAT IT DOES
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "WHAT IT DOES" "read PolicyReports, map to KPOL database"

echo ""
echo -e "  ${CYAN}1.${NC} Reads Kyverno PolicyReport resources from cluster"
echo -e "  ${CYAN}2.${NC} Extracts policy violations (fail/warn results)"
echo -e "  ${CYAN}3.${NC} Maps to our KPOL-* database (460 patterns)"
echo -e "  ${CYAN}4.${NC} Shows findings grouped by severity"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# SAMPLE OUTPUT
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "SAMPLE OUTPUT" "cub-agent scan"

echo ""
echo -e "  ${BOLD}KYVERNO POLICY SCAN${NC}"
echo -e "  ${DIM}Scanned at: 2026-01-08 14:23:45${NC}"
echo ""
echo -e "  ${RED}${BOLD}CRITICAL (2)${NC}"
echo -e "  ────────────────────────────────────────────────────────────────────"
echo -e "  ${RED}[C]${NC} disallow-privileged${DIM}[KPOL-0042]${NC}"
echo -e "      ${DIM}Resource:${NC} prod/debug-pod"
echo -e "      ${DIM}Message:${NC}  Privileged container not allowed"
echo ""
echo -e "  ${RED}[C]${NC} require-run-as-non-root${DIM}[KPOL-0015]${NC}"
echo -e "      ${DIM}Resource:${NC} prod/legacy-app"
echo -e "      ${DIM}Message:${NC}  Container must run as non-root"
echo ""
echo -e "  ${YELLOW}${BOLD}WARNING (3)${NC}"
echo -e "  ────────────────────────────────────────────────────────────────────"
echo -e "  ${YELLOW}[W]${NC} require-labels${DIM}[KPOL-0001]${NC}"
echo -e "      ${DIM}Resource:${NC} default/nginx"
echo -e "      ${DIM}Message:${NC}  Missing required label 'team'"
echo ""
echo -e "  ${YELLOW}[W]${NC} require-resources${DIM}[KPOL-0023]${NC}"
echo -e "      ${DIM}Resource:${NC} default/api"
echo -e "      ${DIM}Message:${NC}  Resource limits not set"
echo ""
echo -e "  ${YELLOW}[W]${NC} require-probes${DIM}[KPOL-0031]${NC}"
echo -e "      ${DIM}Resource:${NC} default/worker"
echo -e "      ${DIM}Message:${NC}  Liveness probe not configured"
echo ""
echo -e "  ════════════════════════════════════════════════════════════════════"
echo -e "  Summary: ${RED}2 critical${NC}, ${YELLOW}3 warning${NC}, 0 info"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# KPOL DATABASE
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "KPOL DATABASE" "460 Kyverno policy patterns"

echo ""
echo -e "  ${BOLD}ID           SEV       NAME                              CATEGORY${NC}"
echo -e "  ${DIM}───────────────────────────────────────────────────────────────────────${NC}"
echo -e "  ${YELLOW}KPOL-0001    warning   Application Field Validation      Best Practices${NC}"
echo -e "  ${YELLOW}KPOL-0015    warning   Require Run As Non-Root           Pod Security${NC}"
echo -e "  ${YELLOW}KPOL-0023    warning   Require Resource Limits           Best Practices${NC}"
echo -e "  ${RED}KPOL-0042    critical  Disallow Privileged Containers    Pod Security${NC}"
echo -e "  ${RED}KPOL-0043    critical  Disallow Host Namespaces          Pod Security${NC}"
echo -e "  ${DIM}... (460 total)${NC}"
echo ""
echo -e "  ${DIM}List all: cub-agent scan --list${NC}"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# USAGE
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "USAGE" "commands and options"

echo ""
echo -e "  ${CYAN}cub-agent scan${NC}"
echo -e "  ${DIM}→ Scan entire cluster for policy violations${NC}"
echo ""
echo -e "  ${CYAN}cub-agent scan -n production${NC}"
echo -e "  ${DIM}→ Scan specific namespace${NC}"
echo ""
echo -e "  ${CYAN}cub-agent scan --json${NC}"
echo -e "  ${DIM}→ JSON output for CI/CD integration${NC}"
echo ""
echo -e "  ${CYAN}cub-agent scan --list${NC}"
echo -e "  ${DIM}→ List all KPOL policies in database${NC}"
echo ""
echo -e "  ${CYAN}./test/atk/map${NC} then press ${BOLD}c${NC}"
echo -e "  ${DIM}→ Interactive scan in TUI dashboard${NC}"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# CI/CD EXAMPLE
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "CI/CD GATE" "fail pipeline on critical violations"

echo ""
echo -e "  ${DIM}# In your CI pipeline:${NC}"
echo -e "  ${CYAN}cub-agent scan --json | jq -e '.summary.critical == 0'${NC}"
echo ""
echo -e "  ${DIM}# Or check specific namespace:${NC}"
echo -e "  ${CYAN}cub-agent scan -n prod --json > audit.json${NC}"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# REQUIREMENTS
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "REQUIREMENTS" "what you need"

echo ""
echo -e "  ${GREEN}✓${NC} Kyverno installed in cluster"
echo -e "  ${GREEN}✓${NC} PolicyReport CRD enabled (default in Kyverno)"
echo ""
echo -e "  ${DIM}Install Kyverno:${NC}"
echo -e "  ${CYAN}kubectl create -f https://github.com/kyverno/kyverno/releases/download/v1.11.0/install.yaml${NC}"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# CONFIGHUB HOOK
# ═══════════════════════════════════════════════════════════════════════════════

echo "────────────────────────────────────────────────────────────────────────────"
echo -e "🔗 ${BOLD}Track violations across your fleet?${NC}"
echo ""
echo -e "   ${CYAN}cub-agent scan --confighub${NC}"
echo ""
echo "   → Sign up at https://confighub.com"
echo "   → See violations by Unit, Space, and Target"
echo "   → Track remediation progress over time"
echo "────────────────────────────────────────────────────────────────────────────"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# COMMANDS SUMMARY
# ═══════════════════════════════════════════════════════════════════════════════

echo "  ┌────────────────────────────────────────────────────────────────────┐"
echo "  │ COMMANDS                                                           │"
echo "  │                                                                    │"
echo "  │  cub-agent scan                 Scan cluster for violations        │"
echo "  │  cub-agent scan -n <ns>         Scan specific namespace            │"
echo "  │  cub-agent scan --json          JSON output for tooling            │"
echo "  │  cub-agent scan --list          List all KPOL policies             │"
echo "  │  cub-agent scan --verbose       Show rule details                  │"
echo "  │  ./test/atk/map then 'c'        Interactive TUI scan               │"
echo "  │                                                                    │"
echo -e "  │  ${DIM}Full guide: docs/TUI-SCAN.md${NC}                                   │"
echo "  └────────────────────────────────────────────────────────────────────┘"
echo ""
