#!/bin/bash
#=============================================================================
# Example: Using the ConfigHub Terminal UI Library
#=============================================================================
#
# This script demonstrates how to use lib/ui.sh in your own tools.
# Run: ./lib/example.sh
#
#=============================================================================

set -euo pipefail

# Get the directory where this script lives
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source the UI library (go up one level to find lib/)
source "${SCRIPT_DIR}/ui.sh"

# Initialize (this auto-downloads gum if needed, caches to .cache/)
ui_init "${SCRIPT_DIR}/.."

#=============================================================================
# EXAMPLE 1: Simple Header
#=============================================================================

echo "=== Example 1: Header with Context Selector ==="
echo ""

# Get kubernetes contexts for the selector
all_contexts=$(ui_k8s_contexts 5)
current=$(kubectl config current-context 2>/dev/null || echo "none")

ui_header "$UI_LIGHTNING MY TOOL" "$all_contexts" "$current"

#=============================================================================
# EXAMPLE 2: Health Section with Progress Bar
#=============================================================================

echo "=== Example 2: Health Section ==="
echo ""

# Simulate some metrics
total=10
healthy=8
pct=$((healthy * 100 / total))

# Build content with colored progress bar
content=$(printf "%s  %d%%   %d/%d healthy\n\n%s Pods %d/%d      %s Services 5/5" \
    "$(ui_health_bar $pct 30)" \
    "$pct" "$healthy" "$total" \
    "$(ui_status_icon ok)" "$healthy" "$total" \
    "$(ui_status_icon ok)")

ui_section "HEALTH" "$content"
echo ""

#=============================================================================
# EXAMPLE 3: Ownership Line
#=============================================================================

echo "=== Example 3: Ownership Display ==="
echo ""

# Show ownership distribution
ownership=$(ui_ownership_line 4 2 1 3)
content="${ownership}\n\nGitOps Coverage   $(ui_health_bar 70 40)  70%"

ui_section "OWNERSHIP" "$content"
echo ""

#=============================================================================
# EXAMPLE 4: Side-by-Side Panels
#=============================================================================

echo "=== Example 4: Side-by-Side Panels ==="
echo ""

left_content="$(ui_status_icon ok) 5 synced\n${UI_FG_DIM}  no drift${UI_NC}\n$(ui_hint './map drift')"
right_content="Sources   3 repos\nTools     $(ui_owner_label Flux) $(ui_owner_label ArgoCD)\n$(ui_hint './map sprawl')"

ui_panels "DRIFT" "$left_content" "SPRAWL" "$right_content"
echo ""

#=============================================================================
# EXAMPLE 5: Pipeline Lines
#=============================================================================

echo "=== Example 5: Pipelines ==="
echo ""

pipelines=""
pipelines+="$(ui_pipeline ok 'github.com/app@main' 'production' '12 resources')\n"
pipelines+="$(ui_pipeline ok 'github.com/app@main' 'staging' '8 resources')\n"
pipelines+="$(ui_pipeline warn 'bitnami/redis@6.2' 'cache' 'helm')\n"
pipelines+="$(ui_pipeline error 'github.com/broken@main' 'failed' 'error')"

ui_section "PIPELINES" "$pipelines"
echo ""

#=============================================================================
# EXAMPLE 6: Trace Chain
#=============================================================================

echo "=== Example 6: Trace Chain (GitOps ownership) ==="
echo ""

# Build a sample trace chain display
echo -e "${UI_BOLD}${UI_FG_CYAN}TRACE:${UI_NC} ${UI_BOLD}Deployment/nginx${UI_NC}"
echo ""
echo -e "  ${UI_OK}✓${UI_NC} ${UI_FG_PURPLE}GitRepository${UI_NC}/${UI_BOLD}infra-repo${UI_NC}"
echo -e "    ${UI_FG_DIM}│${UI_NC} ${UI_FG_DIM}URL:${UI_NC} ${UI_BLUE}https://github.com/your-org/infra.git${UI_NC}"
echo -e "    ${UI_FG_DIM}│${UI_NC} ${UI_FG_DIM}Revision:${UI_NC} ${UI_FG_PURPLE}main@sha1:abc123f${UI_NC}"
echo -e "    ${UI_FG_DIM}│${UI_NC} ${UI_FG_DIM}Status:${UI_NC} ${UI_OK}Artifact is up to date${UI_NC}"
echo -e "    ${UI_FG_DIM}│${UI_NC}"
echo -e "    ${UI_FG_DIM}└─▶${UI_NC} ${UI_OK}✓${UI_NC} ${UI_FG_CYAN}Kustomization${UI_NC}/${UI_BOLD}apps${UI_NC}"
echo -e "        ${UI_FG_DIM}│${UI_NC} ${UI_FG_DIM}Path:${UI_NC} ./clusters/prod/apps"
echo -e "        ${UI_FG_DIM}│${UI_NC} ${UI_FG_DIM}Status:${UI_NC} ${UI_OK}Applied revision main@sha1:abc123f${UI_NC}"
echo -e "        ${UI_FG_DIM}│${UI_NC}"
echo -e "        ${UI_FG_DIM}└─▶${UI_NC} ${UI_OK}✓${UI_NC} ${UI_FG_GREEN}Deployment${UI_NC}/${UI_BOLD}nginx${UI_NC}"
echo -e "              ${UI_FG_DIM}Status:${UI_NC} ${UI_OK}Synced / Healthy${UI_NC}"
echo ""
echo -e "${UI_BOLD}${UI_OK}✓ All levels in sync.${UI_NC} Managed by ${UI_FG_CYAN}flux${UI_NC}."
echo ""

# Show a broken chain example
echo -e "${UI_BOLD}${UI_FG_CYAN}TRACE:${UI_NC} ${UI_BOLD}Deployment/broken-app${UI_NC} ${UI_FG_DIM}(broken chain example)${UI_NC}"
echo ""
echo -e "  ${UI_OK}✓${UI_NC} ${UI_FG_PURPLE}GitRepository${UI_NC}/${UI_BOLD}infra-repo${UI_NC}"
echo -e "    ${UI_FG_DIM}│${UI_NC} ${UI_FG_DIM}Revision:${UI_NC} ${UI_FG_PURPLE}main@sha1:def456${UI_NC}"
echo -e "    ${UI_FG_DIM}│${UI_NC}"
echo -e "    ${UI_FG_DIM}└─▶${UI_NC} ${UI_ERR}✗${UI_NC} ${UI_FG_CYAN}Kustomization${UI_NC}/${UI_BOLD}apps${UI_NC}"
echo -e "        ${UI_FG_DIM}│${UI_NC} ${UI_FG_DIM}Status:${UI_NC} ${UI_WARN}Reconciliation failed${UI_NC}"
echo -e "        ${UI_FG_DIM}│${UI_NC} ${UI_ERR}Error:${UI_NC} ${UI_ERR}path './clusters/prod/apps' not found${UI_NC}"
echo ""
echo -e "${UI_BOLD}${UI_WARN}⚠ Chain broken at Kustomization/apps${UI_NC}"
echo ""

#=============================================================================
# EXAMPLE 7: Status Messages
#=============================================================================

echo "=== Example 7: Status Messages ==="
echo ""

ui_msg ok "All systems operational"
ui_msg warn "1 deployment pending"
ui_msg error "Database connection failed"
echo ""

#=============================================================================
# EXAMPLE 8: Simple Box
#=============================================================================

echo "=== Example 8: Custom Box ==="
echo ""

ui_box "This is a simple bordered box.\nYou can put any content here.\n\n$(ui_hint 'Press q to quit')"
echo ""

#=============================================================================
# DONE
#=============================================================================

echo "=== Library Functions Available ==="
echo ""
echo "Initialization:"
echo "  ui_init SCRIPT_DIR           Initialize (auto-downloads gum)"
echo "  ui_has_gum                   Check if gum is available"
echo ""
echo "Components:"
echo "  ui_header TITLE [CTX] [CUR]  Header with optional context selector"
echo "  ui_section TITLE CONTENT     Section with title and bordered content"
echo "  ui_panels L_T L_C R_T R_C    Two panels side by side"
echo "  ui_box CONTENT               Simple bordered box"
echo ""
echo "Progress Bars:"
echo "  ui_progress_bar PCT WIDTH    Plain progress bar"
echo "  ui_health_bar PCT WIDTH      Auto-colored by percentage"
echo "  ui_mini_bar COUNT MAX COLOR  Small inline bar"
echo ""
echo "Status & Labels:"
echo "  ui_status_icon ok|warn|error Colored status icon"
echo "  ui_owner_label Flux|ArgoCD   Colored owner label"
echo "  ui_owner_color Flux|ArgoCD   Get color code for owner"
echo ""
echo "Kubernetes:"
echo "  ui_k8s_context               Current context (cleaned)"
echo "  ui_k8s_contexts LIMIT        List contexts"
echo "  ui_context_selector          Build context selector line"
echo ""
echo "Utilities:"
echo "  ui_hint TEXT                 Dimmed instruction text"
echo "  ui_msg STATUS TEXT           Message with status icon"
echo "  ui_pipeline STATUS SRC DEP T Pipeline line"
echo "  ui_ownership_line F A H N    Ownership summary"
echo "  ui_repeat CHAR COUNT         Repeat character N times"
