#!/usr/bin/env bash
#
# Demo: Meta-Pattern Detection — What Kyverno Misses
#
# Shows the 5 meta-patterns that cover 90% of config failures
# and demonstrates how ConfigHub detects runtime issues.
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
ORANGE="\033[38;5;208m"
DIM="\033[38;5;245m"
BOLD="\033[1m"
NC="\033[0m"

# ═══════════════════════════════════════════════════════════════════════════════
# HEADER
# ═══════════════════════════════════════════════════════════════════════════════

ui_header "🧠 META-PATTERN DETECTION"

echo ""
echo -e "${DIM}The 5 patterns that cover 90% of config failures.${NC}"
echo -e "${DIM}What Kyverno can't see — runtime state issues.${NC}"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# THE GAP
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "THE GAP" "why Kyverno isn't enough"

echo ""
echo -e "  ${YELLOW}⚠${NC}  Kyverno catches ~40% of issues at admission time."
echo -e "  ${RED}✗${NC}  60% of failures are ${BOLD}runtime state issues${NC}."
echo ""
echo -e "  ${DIM}Example: HelmRelease YAML passes validation,${NC}"
echo -e "  ${DIM}but then sits 'pending' for 3 hours in production.${NC}"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# THE 5 META-PATTERNS
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "5 META-PATTERNS" "90% coverage of 660 CCVEs"

echo ""
echo -e "  ${BOLD}PATTERN               COVERAGE   WHAT IT CATCHES${NC}"
echo -e "  ${DIM}────────────────────────────────────────────────────────────────────${NC}"
echo -e "  ${RED}STATE-STUCK${NC}           26%       Reconciliation loops, finalizer deadlocks"
echo -e "  ${ORANGE}CROSS-REF${NC}             18%       Cross-namespace blocked, case mismatch"
echo -e "  ${YELLOW}REF-NOT-FOUND${NC}         17%       Missing Secret/ConfigMap/ServiceAccount"
echo -e "  ${PURPLE}UPGRADE-BREAKING${NC}      15%       StatefulSet immutable, CRD removed"
echo -e "  ${CYAN}SILENT-CONFIG${NC}         14%       Annotation typo, template not rendering"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# KYVERNO COMPARISON
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "KYVERNO vs CONFIGHUB" "admission vs runtime"

echo ""
echo -e "  ${BOLD}LAYER                 KYVERNO    CONFIGHUB${NC}"
echo -e "  ${DIM}────────────────────────────────────────────────────────────────────${NC}"
echo -e "  Admission-time         ${GREEN}✓${NC}          ${GREEN}✓${NC}"
echo -e "  Post-apply validation  ${RED}✗${NC}          ${GREEN}✓${NC}"
echo -e "  Runtime state          ${RED}✗${NC}          ${GREEN}✓${NC}"
echo -e "  Reconciliation health  ${RED}✗${NC}          ${GREEN}✓${NC}"
echo -e "  Cross-resource refs    ${YELLOW}partial${NC}    ${GREEN}✓${NC}"
echo ""
echo -e "  ${DIM}Combined coverage: 95%+ vs 40% with Kyverno alone${NC}"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# SAMPLE OUTPUT
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "SAMPLE OUTPUT" "cub-agent scan --meta-patterns"

echo ""
echo -e "  ${BOLD}META-PATTERN FINDINGS${NC}"
echo -e "  ${DIM}══════════════════════════════════════════════════════════════════${NC}"
echo ""
echo -e "  ${RED}${BOLD}STATE-STUCK (2)${NC}"
echo -e "  ────────────────────────────────────────────────────────────────────"
echo -e "  ${RED}[STUCK]${NC} CCVE-2025-0632  HelmRelease/redis-cluster"
echo -e "         ${DIM}Status:${NC} pending 47 minutes"
echo -e "         ${DIM}Root cause:${NC} ArgoCD Redis init job deadlock"
echo -e "         ${CYAN}FIX:${NC} kubectl delete job argocd-redis-init"
echo ""
echo -e "  ${RED}[STUCK]${NC} CCVE-2025-0656  CSIDriver/csi-hostpath"
echo -e "         ${DIM}Status:${NC} terminating loop (12 cycles)"
echo -e "         ${DIM}Root cause:${NC} Scheduler preemption deadlock"
echo -e "         ${CYAN}FIX:${NC} kubectl delete pod -n kube-system csi-*"
echo ""
echo -e "  ${CYAN}${BOLD}SILENT-CONFIG (1)${NC}"
echo -e "  ────────────────────────────────────────────────────────────────────"
echo -e "  ${CYAN}[SILENT]${NC} CCVE-2025-0027  ConfigMap/grafana-sidecar"
echo -e "          ${DIM}Field:${NC} NAMESPACE"
echo -e "          ${DIM}Value:${NC} \"monitoring, grafana\"  ${YELLOW}<- space causes silent failure${NC}"
echo -e "          ${CYAN}FIX:${NC} Remove spaces: \"monitoring,grafana\""
echo ""
echo -e "  ${DIM}══════════════════════════════════════════════════════════════════${NC}"
echo -e "  Summary: ${BOLD}3 issues found${NC}. Kyverno detected: ${RED}0/3${NC}"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# PATTERN TREE
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "PATTERN TREE" "full taxonomy"

echo ""
echo -e "  ${RED}STATE-STUCK (26%)${NC}"
echo -e "  ├── Reconciliation loops"
echo -e "  ├── Finalizer deadlocks"
echo -e "  ├── Ownership conflicts"
echo -e "  └── Version rollback failures"
echo ""
echo -e "  ${ORANGE}CROSS-REF (18%)${NC}"
echo -e "  ├── Cross-namespace blocked"
echo -e "  ├── Case mismatch in selectors"
echo -e "  └── API version mismatch"
echo ""
echo -e "  ${YELLOW}REF-NOT-FOUND (17%)${NC}"
echo -e "  ├── Missing Secret"
echo -e "  ├── Missing ConfigMap"
echo -e "  └── Missing ServiceAccount"
echo ""
echo -e "  ${PURPLE}UPGRADE-BREAKING (15%)${NC}"
echo -e "  ├── StatefulSet immutable fields"
echo -e "  ├── CRD storedVersions removed"
echo -e "  └── Default behavior changes"
echo ""
echo -e "  ${CYAN}SILENT-CONFIG (14%)${NC}"
echo -e "  ├── Annotation typo ignored"
echo -e "  ├── Template not rendering"
echo -e "  └── Duplicate YAML keys merged"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# USAGE
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "USAGE" "commands"

echo ""
echo -e "  ${CYAN}cub-agent scan${NC}"
echo -e "  ${DIM}→ Full scan including all patterns${NC}"
echo ""
echo -e "  ${CYAN}cub-agent scan --pattern state-stuck${NC}"
echo -e "  ${DIM}→ Find stuck reconciliations only${NC}"
echo ""
echo -e "  ${CYAN}cub-agent scan --pattern cross-ref${NC}"
echo -e "  ${DIM}→ Find broken cross-references${NC}"
echo ""
echo -e "  ${CYAN}./test/atk/map${NC} then press ${BOLD}c${NC}"
echo -e "  ${DIM}→ Interactive scan in TUI dashboard${NC}"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# DETECTION LAYERS
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "DETECTION LAYERS" "multi-layer architecture"

echo ""
echo -e "  ${BOLD}LAYER            METHOD                   COVERAGE${NC}"
echo -e "  ${DIM}────────────────────────────────────────────────────────────────────${NC}"
echo -e "  Static          YAML parse + reference check    40%"
echo -e "  Admission       Kyverno integration             20%"
echo -e "  Runtime         Status + event monitoring       35%"
echo -e "  ML (future)     Anomaly detection               +4%"
echo ""
echo -e "  ${GREEN}Total:${NC} 99% detection coverage"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# RESEARCH
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "RESEARCH" "based on 660 CCVEs"

echo ""
echo -e "  The 5 meta-patterns were identified by analyzing 660 CCVEs"
echo -e "  from production incidents across:"
echo ""
echo -e "    ${DIM}•${NC} Kubernetes core (165 CCVEs)"
echo -e "    ${DIM}•${NC} Flux CD (28)"
echo -e "    ${DIM}•${NC} Argo CD (13)"
echo -e "    ${DIM}•${NC} Traefik (28)"
echo -e "    ${DIM}•${NC} cert-manager (20)"
echo -e "    ${DIM}•${NC} ingress-nginx (28)"
echo -e "    ${DIM}•${NC} + 40 more tools"
echo ""
echo -e "  ${DIM}See: cve/ccve/META-PATTERN-DETECTION-RESEARCH.md${NC}"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# COMMANDS SUMMARY
# ═══════════════════════════════════════════════════════════════════════════════

echo "  ┌────────────────────────────────────────────────────────────────────┐"
echo "  │ COMMANDS                                                           │"
echo "  │                                                                    │"
echo "  │  cub-agent scan                 Full cluster scan                  │"
echo "  │  cub-agent scan --pattern <p>   Scan for specific pattern          │"
echo "  │  cub-agent scan --json          JSON output for CI/CD              │"
echo "  │  ./test/atk/map then 'c'        Interactive TUI scan               │"
echo "  │                                                                    │"
echo -e "  │  ${DIM}Research: cve/ccve/META-PATTERN-DETECTION-RESEARCH.md${NC}          │"
echo -e "  │  ${DIM}WOW demo: examples/demos/WOW-MOMENTS.md #9${NC}                      │"
echo "  └────────────────────────────────────────────────────────────────────┘"
echo ""
