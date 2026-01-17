#!/usr/bin/env bash
#
# Demo: Real-Time Messaging Style App Config in ConfigHub
#
# This visualizes the example YAML files to show what the TUI would look like.
# NOTE: This is a mockup. Real implementation would read from ConfigHub API.
#

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Source the UI library
source "$REPO_ROOT/test/atk/lib/ui.sh"
ui_init "$REPO_ROOT"

clear

# Colors
GREEN="\033[32m"
YELLOW="\033[33m"
CYAN="\033[36m"
MAGENTA="\033[35m"
DIM="\033[2m"
BOLD="\033[1m"
NC="\033[0m"

# ═══════════════════════════════════════════════════════════════════════════════
# HEADER
# ═══════════════════════════════════════════════════════════════════════════════

ui_header "⚡ APP CONFIG: RTMSG EXAMPLE"

echo ""
echo -e "${DIM}This demo shows how ConfigHub manages app config (not K8s).${NC}"
echo -e "${DIM}Modeled after a platform's DynamoDB-backed configuration system.${NC}"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# HUB OVERVIEW
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "HUB" "rtmsg-platform"

echo ""
echo "  Templates                     Constraints"
echo "  ─────────────────────────     ─────────────────────────────────────"
echo "  • realtime-service            • production-requires-approval"
echo "  • health-server               • critical-tier-restricted"
echo "  • frontdoor                   • customer-config-audit"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# APP SPACES
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "APP SPACES" "2 App Spaces"

echo ""
echo -e "  ${CYAN}${BOLD}realtime-team${NC} (internal)          ${MAGENTA}${BOLD}customer-acme${NC} (self-serve)"
echo "  ───────────────────────────────     ───────────────────────────────"
echo "  Owner: realtime@rtmsg.io            Owner: platform-admin@acme.com"
echo "  Units: 9                           Units: 1"
echo ""
echo -e "  ${GREEN}✓${NC} production-blows                  ${GREEN}✓${NC} acme-realtime-config"
echo -e "  ${GREEN}✓${NC} production-cn                        └── inherits: production-blows"
echo -e "  ${GREEN}✓${NC} production-drill"
echo -e "  ${YELLOW}○${NC} nonprod-realtime-matth"
echo -e "  ${YELLOW}○${NC} dev-alice"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# UNITS BY ENVIRONMENT
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "UNITS" "by environment"

echo ""
printf "  %-20s %-25s %-15s %-12s %s\n" "ENVIRONMENT" "UNIT" "SERVICE" "TIER" "REVISION"
echo -e "  ${DIM}─────────────────────────────────────────────────────────────────────────────────${NC}"

echo -e "  ${GREEN}$(printf "%-20s %-25s %-15s %-12s %s" "production" "production-blows" "realtime" "critical" "20251223.3")${NC}"
echo -e "  ${GREEN}$(printf "%-20s %-25s %-15s %-12s %s" "production" "production-cn" "realtime" "critical" "20251223.1")${NC}"
echo -e "  ${GREEN}$(printf "%-20s %-25s %-15s %-12s %s" "production" "production-drill" "realtime" "critical" "20251222.5")${NC}"
echo -e "  ${MAGENTA}$(printf "%-20s %-25s %-15s %-12s %s" "production" "acme-realtime-config" "realtime" "enterprise" "20251220.2")${NC}"
echo -e "  ${YELLOW}$(printf "%-20s %-25s %-15s %-12s %s" "nonprod" "nonprod-realtime-matth" "realtime" "dev" "20251223.1")${NC}"
echo -e "  ${YELLOW}$(printf "%-20s %-25s %-15s %-12s %s" "nonprod" "nonprod-staging" "realtime" "staging" "20251222.8")${NC}"
echo -e "  ${DIM}$(printf "%-20s %-25s %-15s %-12s %s" "dev" "dev-alice" "realtime" "dev" "20251223.2")${NC}"
echo -e "  ${DIM}$(printf "%-20s %-25s %-15s %-12s %s" "dev" "dev-bob" "realtime" "dev" "20251223.1")${NC}"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# CUSTOMER SELF-SERVE VIEW
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "CUSTOMER VIEW" "acme-realtime-config"

echo ""
echo -e "${DIM}Customer ACME sees only their config. They can edit highlighted fields.${NC}"
echo ""
echo -e "  ${MAGENTA}${BOLD}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "  ${MAGENTA}${BOLD}║${NC} ${BOLD}acme-realtime-config${NC}                                        ${MAGENTA}${BOLD}║${NC}"
echo -e "  ${MAGENTA}${BOLD}║${NC} Upstream: production-blows │ Revision: 20251220.2            ${MAGENTA}${BOLD}║${NC}"
echo -e "  ${MAGENTA}${BOLD}╠══════════════════════════════════════════════════════════════╣${NC}"
echo -e "  ${MAGENTA}${BOLD}║${NC} ${BOLD}EDITABLE BY CUSTOMER${NC}                                        ${MAGENTA}${BOLD}║${NC}"
echo -e "  ${MAGENTA}${BOLD}║${NC}                                                              ${MAGENTA}${BOLD}║${NC}"
echo -e "  ${MAGENTA}${BOLD}║${NC} rate_limit:                                                  ${MAGENTA}${BOLD}║${NC}"
echo -e "  ${MAGENTA}${BOLD}║${NC}   messages_per_second: ${GREEN}100000${NC}     ← 2x default              ${MAGENTA}${BOLD}║${NC}"
echo -e "  ${MAGENTA}${BOLD}║${NC}   connections_per_channel: ${GREEN}200000${NC}                          ${MAGENTA}${BOLD}║${NC}"
echo -e "  ${MAGENTA}${BOLD}║${NC}                                                              ${MAGENTA}${BOLD}║${NC}"
echo -e "  ${MAGENTA}${BOLD}║${NC} feature_flags:                                               ${MAGENTA}${BOLD}║${NC}"
echo -e "  ${MAGENTA}${BOLD}║${NC}   enable_reactor: ${GREEN}true${NC}            ← enabled                 ${MAGENTA}${BOLD}║${NC}"
echo -e "  ${MAGENTA}${BOLD}║${NC}   enable_firehose: ${GREEN}true${NC}           ← enabled                 ${MAGENTA}${BOLD}║${NC}"
echo -e "  ${MAGENTA}${BOLD}║${NC}                                                              ${MAGENTA}${BOLD}║${NC}"
echo -e "  ${MAGENTA}${BOLD}║${NC} custom_domain: ${GREEN}realtime.acme.com${NC}                            ${MAGENTA}${BOLD}║${NC}"
echo -e "  ${MAGENTA}${BOLD}║${NC} message_retention_days: ${GREEN}14${NC}                                  ${MAGENTA}${BOLD}║${NC}"
echo -e "  ${MAGENTA}${BOLD}║${NC}                                                              ${MAGENTA}${BOLD}║${NC}"
echo -e "  ${MAGENTA}${BOLD}║${NC} webhooks:                                                    ${MAGENTA}${BOLD}║${NC}"
echo -e "  ${MAGENTA}${BOLD}║${NC}   on_message: ${GREEN}https://hooks.acme.com/rtmsg/message${NC}           ${MAGENTA}${BOLD}║${NC}"
echo -e "  ${MAGENTA}${BOLD}║${NC}   on_error: ${GREEN}https://hooks.acme.com/rtmsg/error${NC}               ${MAGENTA}${BOLD}║${NC}"
echo -e "  ${MAGENTA}${BOLD}╠══════════════════════════════════════════════════════════════╣${NC}"
echo -e "  ${MAGENTA}${BOLD}║${NC} ${DIM}INHERITED FROM PLATFORM (read-only)${NC}                         ${MAGENTA}${BOLD}║${NC}"
echo -e "  ${MAGENTA}${BOLD}║${NC}                                                              ${MAGENTA}${BOLD}║${NC}"
echo -e "  ${MAGENTA}${BOLD}║${NC} ${DIM}image_tags:${NC}                                                  ${MAGENTA}${BOLD}║${NC}"
echo -e "  ${MAGENTA}${BOLD}║${NC} ${DIM}  core: prod-20251220.1-a1b2c3d${NC}                              ${MAGENTA}${BOLD}║${NC}"
echo -e "  ${MAGENTA}${BOLD}║${NC} ${DIM}  frontdoor: prod-20251218.2-e4f5g6h${NC}                         ${MAGENTA}${BOLD}║${NC}"
echo -e "  ${MAGENTA}${BOLD}║${NC}                                                              ${MAGENTA}${BOLD}║${NC}"
echo -e "  ${MAGENTA}${BOLD}║${NC} ${DIM}service_endpoints:${NC}                                           ${MAGENTA}${BOLD}║${NC}"
echo -e "  ${MAGENTA}${BOLD}║${NC} ${DIM}  api: https://api.rtmsg.io${NC}                                   ${MAGENTA}${BOLD}║${NC}"
echo -e "  ${MAGENTA}${BOLD}║${NC} ${DIM}  realtime: wss://realtime-blows.rtmsg.io${NC}                     ${MAGENTA}${BOLD}║${NC}"
echo -e "  ${MAGENTA}${BOLD}║${NC}                                                              ${MAGENTA}${BOLD}║${NC}"
echo -e "  ${MAGENTA}${BOLD}║${NC} ${DIM}internal_settings:${NC}                                           ${MAGENTA}${BOLD}║${NC}"
echo -e "  ${MAGENTA}${BOLD}║${NC} ${DIM}  cluster_size: 12${NC}                                           ${MAGENTA}${BOLD}║${NC}"
echo -e "  ${MAGENTA}${BOLD}╚══════════════════════════════════════════════════════════════╝${NC}"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# AUDIT TRAIL
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "AUDIT" "recent changes"

echo ""
printf "  %-12s %-25s %-25s %s\n" "DATE" "USER" "UNIT" "CHANGE"
echo -e "  ${DIM}─────────────────────────────────────────────────────────────────────────────────${NC}"
echo -e "  $(printf "%-12s %-25s %-25s %s" "Dec 23" "alice@rtmsg.io" "production-blows" "Increased cluster size")"
echo -e "  ${MAGENTA}$(printf "%-12s %-25s %-25s %s" "Dec 20" "devops@acme.com" "acme-realtime-config" "Increased rate limits")${NC}"
echo -e "  $(printf "%-12s %-25s %-25s %s" "Dec 18" "bob@rtmsg.io" "production-cn" "Updated frontdoor image")"
echo -e "  ${MAGENTA}$(printf "%-12s %-25s %-25s %s" "Dec 15" "admin@acme.com" "acme-realtime-config" "Added error webhook")${NC}"
echo -e "  ${YELLOW}$(printf "%-12s %-25s %-25s %s" "Dec 15" "matt@rtmsg.io" "nonprod-matth" "Testing new build")${NC}"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# QUERIES
# ═══════════════════════════════════════════════════════════════════════════════

ui_section "QUERIES" "cross-cutting visibility"

echo ""
echo -e "${DIM}Examples of queries that DynamoDB can't do:${NC}"
echo ""

echo -e "  ${CYAN}cub query \"environment=production\"${NC}"
echo -e "  ${DIM}→ production-blows, production-cn, production-drill, acme-realtime-config${NC}"
echo ""

echo -e "  ${CYAN}cub query \"customer=acme\"${NC}"
echo -e "  ${DIM}→ acme-realtime-config${NC}"
echo ""

echo -e "  ${CYAN}cub query \"config.rate_limit.messages_per_second>50000\"${NC}"
echo -e "  ${DIM}→ acme-realtime-config (100000)${NC}"
echo ""

echo -e "  ${CYAN}cub query \"modified>7d AND tier=critical\"${NC}"
echo -e "  ${DIM}→ production-blows (cluster size change)${NC}"
echo ""

# ═══════════════════════════════════════════════════════════════════════════════
# SUMMARY
# ═══════════════════════════════════════════════════════════════════════════════

echo ""
echo "  ┌────────────────────────────────────────────────────────────────────┐"
echo "  │ WHAT THIS DEMO SHOWS                                               │"
echo "  │                                                                    │"
echo "  │ 1. Hub as catalog      - Templates + constraints in one place     │"
echo "  │ 2. App Spaces as boundaries - Internal team vs customer self-serve    │"
echo "  │ 3. Units with labels   - Queryable across all environments        │"
echo "  │ 4. Customer self-serve - ACME edits their slice, platform rest    │"
echo "  │ 5. Audit trail         - Who changed what, when, why              │"
echo "  │ 6. Cross-cutting queries - Visibility DynamoDB can't provide      │"
echo "  │                                                                    │"
echo -e "  │ ${DIM}NOTE: This is a mockup. Real impl would read from ConfigHub API.${NC} │"
echo "  └────────────────────────────────────────────────────────────────────┘"
echo ""
