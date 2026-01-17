#!/usr/bin/env bash
#
# Demo: Fleet Queries
#
# Interactive demo showing query language for filtering resources.
# Can run live against your cluster or show example output.
#
# Usage:
#   ./fleet-queries-demo.sh          # Show examples (no cluster needed)
#   ./fleet-queries-demo.sh --live   # Run live queries against cluster
#
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Source the UI library
source "$REPO_ROOT/test/atk/lib/ui.sh"
ui_init "$REPO_ROOT"

# Parse args
LIVE_MODE=false
for arg in "$@"; do
    case "$arg" in
        --live) LIVE_MODE=true ;;
    esac
done

clear

# Colors
GREEN="\033[38;5;82m"
YELLOW="\033[38;5;214m"
CYAN="\033[38;5;51m"
PURPLE="\033[38;5;141m"
ORANGE="\033[38;5;208m"
RED="\033[38;5;196m"
DIM="\033[38;5;245m"
BOLD="\033[1m"
NC="\033[0m"

# ═══════════════════════════════════════════════════════════════════════════════
# HEADER
# ═══════════════════════════════════════════════════════════════════════════════

ui_header "🔍 FLEET QUERIES DEMO"

echo ""
echo -e "${DIM}Query language for filtering resources across your fleet.${NC}"
if $LIVE_MODE; then
    echo -e "${GREEN}Running LIVE queries against your cluster${NC}"
else
    echo -e "${DIM}Showing example output (run with --live for real queries)${NC}"
fi
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# QUERY SYNTAX
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "QUERY SYNTAX" "how to filter resources"

echo ""
echo -e "  ${BOLD}OPERATOR        EXAMPLE                      MEANING${NC}"
echo -e "  ${DIM}──────────────────────────────────────────────────────────────────${NC}"
echo -e "  ${CYAN}=${NC}               owner=Flux                   Exact match"
echo -e "  ${CYAN}!=${NC}              owner!=Native                Not equal"
echo -e "  ${CYAN}~=${NC}              name~=payment.*              Regex match"
echo -e "  ${CYAN}=val1,val2${NC}      owner=Flux,ArgoCD            IN list"
echo -e "  ${CYAN}=prefix*${NC}        namespace=prod*              Wildcard"
echo -e "  ${CYAN}AND${NC}             kind=Deployment AND owner=Flux"
echo -e "  ${CYAN}OR${NC}              owner=Flux OR owner=ArgoCD"
echo ""
echo -e "  ${BOLD}FIELDS:${NC} kind, namespace, name, owner, cluster, labels[key]"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# QUERY: GITOPS MANAGED
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "QUERY 1" "GitOps-managed resources only"

echo ""
echo -e "  ${CYAN}cub-scout map list -q \"owner!=Native\"${NC}"
echo ""

if $LIVE_MODE; then
    "$REPO_ROOT/cub-scout" map list --standalone -q "owner!=Native" 2>/dev/null | head -20 || echo -e "  ${DIM}(no results or error)${NC}"
else
    # Example output
    echo -e "  ${BOLD}NAMESPACE   KIND        NAME            OWNER${NC}"
    echo -e "  ${DIM}────────────────────────────────────────────────────${NC}"
    echo -e "  ${CYAN}prod-east${NC}   Deployment  payment-api     ${CYAN}Flux${NC}"
    echo -e "  ${CYAN}prod-east${NC}   Deployment  payment-worker  ${CYAN}Flux${NC}"
    echo -e "  ${PURPLE}prod-west${NC}   Deployment  order-api       ${PURPLE}ArgoCD${NC}"
    echo -e "  ${PURPLE}prod-west${NC}   Deployment  order-processor ${PURPLE}ArgoCD${NC}"
    echo -e "  ${GREEN}monitoring${NC}  Deployment  prometheus      ${GREEN}ConfigHub${NC}"
    echo -e "  ${GREEN}monitoring${NC}  Deployment  grafana         ${GREEN}ConfigHub${NC}"
    echo ""
    echo -e "  ${DIM}Total: 6 resources${NC}"
    echo -e "  ${DIM}By Owner: ArgoCD(2) ConfigHub(2) Flux(2)${NC}"
fi
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# QUERY: PRODUCTION NAMESPACES
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "QUERY 2" "Production namespaces (wildcard)"

echo ""
echo -e "  ${CYAN}cub-scout map list -q \"namespace=prod*\"${NC}"
echo ""

if $LIVE_MODE; then
    "$REPO_ROOT/cub-scout" map list --standalone -q "namespace=prod*" 2>/dev/null | head -15 || echo -e "  ${DIM}(no results or error)${NC}"
else
    echo -e "  ${BOLD}NAMESPACE   KIND        NAME            OWNER${NC}"
    echo -e "  ${DIM}────────────────────────────────────────────────────${NC}"
    echo -e "  prod-east   Deployment  payment-api     ${CYAN}Flux${NC}"
    echo -e "  prod-east   Deployment  payment-worker  ${CYAN}Flux${NC}"
    echo -e "  prod-east   Service     payment-api     ${CYAN}Flux${NC}"
    echo -e "  prod-west   Deployment  order-api       ${PURPLE}ArgoCD${NC}"
    echo -e "  prod-west   Deployment  order-processor ${PURPLE}ArgoCD${NC}"
    echo -e "  prod-west   Service     order-api       ${PURPLE}ArgoCD${NC}"
    echo ""
    echo -e "  ${DIM}Total: 6 resources${NC}"
fi
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# QUERY: ORPHAN HUNT
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "QUERY 3" "Orphan hunt (unmanaged resources)"

echo ""
echo -e "  ${CYAN}cub-scout map list -q \"owner=Native\"${NC}"
echo ""

if $LIVE_MODE; then
    "$REPO_ROOT/cub-scout" map list --standalone -q "owner=Native" 2>/dev/null | head -15 || echo -e "  ${DIM}(no results or error)${NC}"
else
    echo -e "  ${BOLD}NAMESPACE   KIND        NAME            OWNER${NC}"
    echo -e "  ${DIM}────────────────────────────────────────────────────${NC}"
    echo -e "  ${YELLOW}staging${NC}     Deployment  debug-pod       ${YELLOW}Native${NC}"
    echo -e "  ${YELLOW}default${NC}     ConfigMap   mystery-config  ${YELLOW}Native${NC}"
    echo -e "  ${YELLOW}prod-east${NC}   Secret      manual-secret   ${YELLOW}Native${NC}"
    echo ""
    echo -e "  ${YELLOW}⚠ These resources have no GitOps owner${NC}"
    echo -e "  ${DIM}Someone kubectl apply'd them — security/rebuild risk${NC}"
fi
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# QUERY: COMBINE OWNERS
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "QUERY 4" "Flux OR Argo managed"

echo ""
echo -e "  ${CYAN}cub-scout map list -q \"owner=Flux OR owner=ArgoCD\"${NC}"
echo ""

if $LIVE_MODE; then
    "$REPO_ROOT/cub-scout" map list --standalone -q "owner=Flux OR owner=ArgoCD" 2>/dev/null | head -15 || echo -e "  ${DIM}(no results or error)${NC}"
else
    echo -e "  ${BOLD}NAMESPACE   KIND        NAME            OWNER${NC}"
    echo -e "  ${DIM}────────────────────────────────────────────────────${NC}"
    echo -e "  prod-east   Deployment  payment-api     ${CYAN}Flux${NC}"
    echo -e "  prod-east   Deployment  payment-worker  ${CYAN}Flux${NC}"
    echo -e "  prod-west   Deployment  order-api       ${PURPLE}ArgoCD${NC}"
    echo -e "  staging     Deployment  frontend        ${CYAN}Flux${NC}"
    echo ""
    echo -e "  ${DIM}Total: 4 resources${NC}"
    echo -e "  ${DIM}By Owner: ArgoCD(1) Flux(3)${NC}"
fi
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# QUERY: REGEX
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "QUERY 5" "Regex pattern matching"

echo ""
echo -e "  ${CYAN}cub-scout map list -q \"name~=payment.*\"${NC}"
echo ""

if $LIVE_MODE; then
    "$REPO_ROOT/cub-scout" map list --standalone -q "name~=payment.*" 2>/dev/null | head -15 || echo -e "  ${DIM}(no results or error)${NC}"
else
    echo -e "  ${BOLD}NAMESPACE   KIND        NAME            OWNER${NC}"
    echo -e "  ${DIM}────────────────────────────────────────────────────${NC}"
    echo -e "  prod-east   Deployment  payment-api     ${CYAN}Flux${NC}"
    echo -e "  prod-east   Deployment  payment-worker  ${CYAN}Flux${NC}"
    echo -e "  prod-east   Service     payment-api     ${CYAN}Flux${NC}"
    echo ""
    echo -e "  ${DIM}Regex finds all resources matching pattern${NC}"
fi
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# SAVED QUERIES
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "SAVED QUERIES" "reusable named queries"

echo ""
echo -e "  ${DIM}Built-in queries ship with the agent:${NC}"
echo ""
echo -e "  ${BOLD}NAME          EXPANDS TO${NC}"
echo -e "  ${DIM}────────────────────────────────────────────────────${NC}"
echo -e "  ${YELLOW}unmanaged${NC}     owner=Native"
echo -e "  ${GREEN}gitops${NC}        owner=Flux OR owner=Argo"
echo -e "  ${CYAN}flux${NC}          owner=Flux"
echo -e "  ${PURPLE}argo${NC}          owner=Argo"
echo -e "  ${DIM}prod${NC}          namespace=prod* OR namespace=production*"
echo ""
echo -e "  ${BOLD}Usage:${NC}"
echo -e "  ${CYAN}cub-scout map list -q unmanaged${NC}"
echo -e "  ${CYAN}cub-scout map list -q \"unmanaged AND namespace=prod*\"${NC}"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# IITS QUESTIONS
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "REAL-WORLD QUESTIONS" "what this solves"

echo ""
echo -e "  ${BOLD}QUESTION                              QUERY${NC}"
echo -e "  ${DIM}────────────────────────────────────────────────────────────────${NC}"
echo -e "  What's managed by GitOps?           ${CYAN}-q \"owner!=Native\"${NC}"
echo -e "  What's orphaned / unmanaged?        ${CYAN}-q \"owner=Native\"${NC}"
echo -e "  What's in production?               ${CYAN}-q \"namespace=prod*\"${NC}"
echo -e "  What's managed by Flux?             ${CYAN}-q flux${NC}"
echo -e "  Find debug resources                ${CYAN}-q \"name~=debug.*\"${NC}"
echo -e "  Deployments only                    ${CYAN}-q \"kind=Deployment\"${NC}"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# FOOTER
# ═══════════════════════════════════════════════════════════════════════════════

echo "────────────────────────────────────────────────────────────────────────────"
echo -e "${BOLD}Try it:${NC}"
echo ""
echo -e "  ${CYAN}./test/atk/demo query${NC}              Run live against cluster (applies fixtures)"
echo -e "  ${CYAN}cub-scout map list -q \"...\"${NC}       Run any query"
echo -e "  ${CYAN}cub-scout map queries${NC}             List saved queries"
echo ""
echo -e "${DIM}Full reference: docs/FLEET-QUERIES-REFERENCE.md${NC}"
echo "────────────────────────────────────────────────────────────────────────────"
echo ""
