#!/bin/bash
#=============================================================================
# ConfigHub Terminal UI Library
#=============================================================================
#
# A modern, beautiful terminal UI library powered by Charmbracelet's gum.
# Provides consistent styling, colors, and components for CLI tools.
#
# USAGE:
#   source "$(dirname "${BASH_SOURCE[0]}")/lib/ui.sh"
#   ui_init                        # Initialize (auto-downloads gum if needed)
#   ui_header "My App"             # Draw header
#   ui_section "Title" "content"   # Draw section box
#
# FEATURES:
#   - Auto-downloads gum binary on first use (~5MB, cached locally)
#   - Graceful fallback to basic ANSI when gum unavailable
#   - 256-color palette for modern terminals
#   - Context selector, progress bars, status icons
#   - Side-by-side panels with gum join
#
#=============================================================================

# Prevent double-sourcing
[[ -n "${_UI_LIB_LOADED:-}" ]] && return 0
_UI_LIB_LOADED=1

#=============================================================================
# COLOR PALETTE
#=============================================================================

# Basic colors (universal fallback)
UI_RED='\033[0;31m'
UI_GREEN='\033[0;32m'
UI_YELLOW='\033[0;33m'
UI_BLUE='\033[0;34m'
UI_CYAN='\033[0;36m'
UI_GRAY='\033[0;90m'
UI_BOLD='\033[1m'
UI_NC='\033[0m'  # No Color / Reset

# 256-color palette (modern terminals)
UI_FG_CYAN='\033[38;5;51m'      # Bright cyan (Flux)
UI_FG_PURPLE='\033[38;5;141m'   # Purple (ArgoCD)
UI_FG_ORANGE='\033[38;5;208m'   # Orange (Helm)
UI_FG_GREEN='\033[38;5;84m'     # Bright green (success/ConfigHub)
UI_FG_DIM='\033[38;5;245m'      # Dim gray (secondary text)
UI_FG_WHITE='\033[38;5;255m'    # Bright white (emphasis)
UI_FG_PINK='\033[38;5;212m'     # Pink (headers)

# Status colors
UI_OK='\033[38;5;82m'           # Bright green checkmark
UI_WARN='\033[38;5;214m'        # Amber warning
UI_ERR='\033[38;5;196m'         # Bright red error

# Border colors (for gum)
UI_BORDER_DEFAULT=240
UI_BORDER_ACCENT=212
UI_BORDER_ERROR=196
UI_BORDER_SUCCESS=82

#=============================================================================
# ICONS AND SYMBOLS
#=============================================================================

UI_CHECK='✓'
UI_CROSS='✗'
UI_WARN_ICON='⚠'
UI_BULLET_ACTIVE='●'
UI_BULLET_INACTIVE='○'
UI_ARROW='▶'
UI_ARROW_RIGHT='→'
UI_LIGHTNING='⚡'
UI_PIPE='──▶'

# Progress bar characters
UI_BAR_FULL='█'
UI_BAR_EMPTY='░'

# Box drawing (for fallback mode)
UI_BOX_TL='╭'
UI_BOX_TR='╮'
UI_BOX_BL='╰'
UI_BOX_BR='╯'
UI_BOX_H='─'
UI_BOX_V='│'

#=============================================================================
# CONFIGURATION
#=============================================================================

UI_WIDTH="${UI_WIDTH:-72}"                    # Default box width
UI_CACHE_DIR=""                               # Set by ui_init
UI_GUM=""                                     # Path to gum binary

#=============================================================================
# INITIALIZATION
#=============================================================================

# Initialize the UI library
# Call this once at script start
ui_init() {
    local script_dir="${1:-$(pwd)}"
    UI_CACHE_DIR="${script_dir}/.cache"

    _ui_ensure_gum 2>/dev/null || true
}

# Internal: ensure gum is available
_ui_ensure_gum() {
    # Already have gum system-wide?
    if command -v gum &>/dev/null; then
        UI_GUM="gum"
        return 0
    fi

    # Already cached locally?
    local gum_path="${UI_CACHE_DIR}/gum"
    if [[ -x "$gum_path" ]]; then
        UI_GUM="$gum_path"
        return 0
    fi

    # Determine OS and architecture
    local os arch
    case "$(uname -s)" in
        Darwin) os="Darwin" ;;
        Linux)  os="Linux" ;;
        *)      return 1 ;;
    esac
    case "$(uname -m)" in
        x86_64)       arch="x86_64" ;;
        aarch64|arm64) arch="arm64" ;;
        *)            return 1 ;;
    esac

    # Download gum
    local version="0.14.5"
    local url="https://github.com/charmbracelet/gum/releases/download/v${version}/gum_${version}_${os}_${arch}.tar.gz"
    local extract_dir="gum_${version}_${os}_${arch}"

    echo -e "${UI_FG_DIM}Downloading gum for beautiful output...${UI_NC}" >&2
    mkdir -p "$UI_CACHE_DIR"

    local tmp_dir
    tmp_dir=$(mktemp -d)
    if ! curl -sL "$url" | tar -xz -C "$tmp_dir" 2>/dev/null; then
        rm -rf "$tmp_dir"
        return 1
    fi

    mv "$tmp_dir/$extract_dir/gum" "$gum_path"
    rm -rf "$tmp_dir"
    chmod +x "$gum_path"
    UI_GUM="$gum_path"

    echo -e "${UI_OK}${UI_CHECK}${UI_NC} ${UI_FG_DIM}gum cached at ${UI_CACHE_DIR}${UI_NC}" >&2
}

# Check if gum is available
ui_has_gum() {
    [[ -n "$UI_GUM" ]]
}

#=============================================================================
# CORE UI COMPONENTS
#=============================================================================

# Draw a header with optional context selector
# Usage: ui_header "Title" [contexts_array] [current_context]
ui_header() {
    local title="$1"
    local all_contexts="${2:-}"
    local current_context="${3:-}"

    echo ""

    if ui_has_gum; then
        # Main title
        echo -e "$title" | $UI_GUM style \
            --border rounded \
            --border-foreground "$UI_BORDER_ACCENT" \
            --foreground "$UI_BORDER_ACCENT" \
            --bold \
            --padding "0 2" \
            --width "$UI_WIDTH"

        # Context selector (if provided)
        if [[ -n "$all_contexts" ]]; then
            local context_line=""
            while IFS= read -r ctx; do
                [[ -z "$ctx" ]] && continue
                local display="${ctx#kind-}"  # Strip kind- prefix
                if [[ "$ctx" == "$current_context" ]]; then
                    context_line+="${UI_FG_GREEN}${UI_BULLET_ACTIVE} ${display}${UI_NC}  "
                else
                    context_line+="${UI_FG_DIM}${UI_BULLET_INACTIVE} ${display}${UI_NC}  "
                fi
            done <<< "$all_contexts"

            echo -e "  ${UI_FG_DIM}CONTEXT${UI_NC}  ${context_line}" | $UI_GUM style \
                --padding "0 1" \
                --width "$UI_WIDTH"
        fi
    else
        # Fallback
        echo -e "${UI_BOLD}$title${UI_NC}"
        [[ -n "$current_context" ]] && echo -e "${UI_FG_DIM}Context: ${current_context}${UI_NC}"
    fi

    echo ""
}

# Draw a section with title and content
# Usage: ui_section "Title" "content" [border_color]
ui_section() {
    local title="$1"
    local content="$2"
    local border_color="${3:-$UI_BORDER_DEFAULT}"

    if ui_has_gum; then
        echo "$title" | $UI_GUM style --foreground 255 --bold
        echo -e "$content" | $UI_GUM style \
            --border normal \
            --border-foreground "$border_color" \
            --padding "0 2" \
            --width "$UI_WIDTH"
    else
        echo -e "${UI_BOLD}$title${UI_NC}"
        echo -e "$content"
        echo ""
    fi
}

# Draw two panels side by side
# Usage: ui_panels "Left Title" "left_content" "Right Title" "right_content"
ui_panels() {
    local left_title="$1"
    local left_content="$2"
    local right_title="$3"
    local right_content="$4"

    if ui_has_gum; then
        local left_width=$(( (UI_WIDTH - 4) / 2 ))
        local right_width=$(( UI_WIDTH - left_width - 4 ))

        local left_box right_box
        left_box=$(echo -e "$left_content" | $UI_GUM style \
            --border normal \
            --border-foreground "$UI_BORDER_DEFAULT" \
            --padding "0 1" \
            --width "$left_width")
        right_box=$(echo -e "$right_content" | $UI_GUM style \
            --border normal \
            --border-foreground "$UI_BORDER_DEFAULT" \
            --padding "0 1" \
            --width "$right_width")

        local left_panel right_panel
        left_panel=$($UI_GUM style --foreground 255 --bold "$left_title")$'\n'"$left_box"
        right_panel=$($UI_GUM style --foreground 255 --bold "$right_title")$'\n'"$right_box"

        $UI_GUM join --horizontal "$left_panel" "  " "$right_panel"
    else
        echo -e "${UI_BOLD}$left_title${UI_NC}"
        echo -e "$left_content"
        echo ""
        echo -e "${UI_BOLD}$right_title${UI_NC}"
        echo -e "$right_content"
    fi
}

# Draw a simple box around content
# Usage: ui_box "content" [border_color]
ui_box() {
    local content="$1"
    local border_color="${2:-$UI_BORDER_DEFAULT}"

    if ui_has_gum; then
        echo -e "$content" | $UI_GUM style \
            --border normal \
            --border-foreground "$border_color" \
            --padding "0 2" \
            --width "$UI_WIDTH"
    else
        echo -e "$content"
        echo ""
    fi
}

#=============================================================================
# PROGRESS BARS
#=============================================================================

# Generate a progress bar string
# Usage: ui_progress_bar percentage [width] [color]
# Returns: string like "██████████░░░░░░"
ui_progress_bar() {
    local pct="$1"
    local width="${2:-30}"
    local color="${3:-}"

    local filled=$((pct * width / 100))
    local empty=$((width - filled))

    local bar=""
    for ((i=0; i<filled; i++)); do bar+="$UI_BAR_FULL"; done
    for ((i=0; i<empty; i++)); do bar+="$UI_BAR_EMPTY"; done

    if [[ -n "$color" ]]; then
        echo -e "${color}${bar}${UI_NC}"
    else
        echo "$bar"
    fi
}

# Generate a health-colored progress bar (auto-colors based on percentage)
# Usage: ui_health_bar percentage [width]
ui_health_bar() {
    local pct="$1"
    local width="${2:-30}"

    local color
    if [[ $pct -ge 90 ]]; then
        color="$UI_OK"
    elif [[ $pct -ge 70 ]]; then
        color="$UI_WARN"
    else
        color="$UI_ERR"
    fi

    ui_progress_bar "$pct" "$width" "$color"
}

# Generate a mini bar for inline display (e.g., ownership counts)
# Usage: ui_mini_bar count max_bars color
ui_mini_bar() {
    local count="$1"
    local max="${2:-8}"
    local color="${3:-$UI_FG_DIM}"

    local bar=""
    local show=$((count < max ? count : max))
    for ((i=0; i<show; i++)); do bar+="${color}${UI_BAR_FULL}${UI_NC}"; done
    echo -e "$bar"
}

#=============================================================================
# STATUS ICONS
#=============================================================================

# Get a colored status icon
# Usage: ui_status_icon "ok"|"warn"|"error"
ui_status_icon() {
    local status="$1"
    case "$status" in
        ok|success|ready)    echo -e "${UI_OK}${UI_CHECK}${UI_NC}" ;;
        warn|warning|paused) echo -e "${UI_WARN}${UI_WARN_ICON}${UI_NC}" ;;
        error|failed|*)      echo -e "${UI_ERR}${UI_CROSS}${UI_NC}" ;;
    esac
}

# Get an owner-colored label
# Usage: ui_owner_label "Flux"|"ArgoCD"|"Helm"|"Native"
ui_owner_label() {
    local owner="$1"
    case "$owner" in
        Flux)      echo -e "${UI_FG_CYAN}Flux${UI_NC}" ;;
        ArgoCD)    echo -e "${UI_FG_PURPLE}ArgoCD${UI_NC}" ;;
        Helm)      echo -e "${UI_FG_ORANGE}Helm${UI_NC}" ;;
        ConfigHub) echo -e "${UI_FG_GREEN}ConfigHub${UI_NC}" ;;
        Native|*)  echo -e "${UI_FG_DIM}Native${UI_NC}" ;;
    esac
}

# Get owner color code (for building custom strings)
# Usage: color=$(ui_owner_color "Flux")
ui_owner_color() {
    local owner="$1"
    case "$owner" in
        Flux)      echo "$UI_FG_CYAN" ;;
        ArgoCD)    echo "$UI_FG_PURPLE" ;;
        Helm)      echo "$UI_FG_ORANGE" ;;
        ConfigHub) echo "$UI_FG_GREEN" ;;
        Native|*)  echo "$UI_FG_DIM" ;;
    esac
}

#=============================================================================
# KUBERNETES CONTEXT HELPERS
#=============================================================================

# Get current kubernetes context (cleaned)
# Usage: ctx=$(ui_k8s_context)
ui_k8s_context() {
    local ctx
    ctx=$(kubectl config current-context 2>/dev/null || echo "none")
    echo "${ctx#kind-}"  # Strip kind- prefix
}

# Get all kubernetes contexts
# Usage: ui_k8s_contexts [limit]
ui_k8s_contexts() {
    local limit="${1:-5}"
    kubectl config get-contexts -o name 2>/dev/null | head -"$limit"
}

# Build a context selector line
# Usage: line=$(ui_context_selector)
ui_context_selector() {
    local current all_contexts line=""
    current=$(kubectl config current-context 2>/dev/null || echo "none")
    all_contexts=$(ui_k8s_contexts 5)

    while IFS= read -r ctx; do
        [[ -z "$ctx" ]] && continue
        local display="${ctx#kind-}"
        if [[ "$ctx" == "$current" ]]; then
            line+="${UI_FG_GREEN}${UI_BULLET_ACTIVE} ${display}${UI_NC}  "
        else
            line+="${UI_FG_DIM}${UI_BULLET_INACTIVE} ${display}${UI_NC}  "
        fi
    done <<< "$all_contexts"

    echo -e "$line"
}

#=============================================================================
# UTILITY FUNCTIONS
#=============================================================================

# Print a dimmed hint/instruction
# Usage: ui_hint "Run ./map help for more info"
ui_hint() {
    echo -e "${UI_FG_DIM}${UI_ARROW_RIGHT} $1${UI_NC}"
}

# Print a message with icon
# Usage: ui_msg "ok" "All systems healthy"
ui_msg() {
    local status="$1"
    local message="$2"
    echo -e "$(ui_status_icon "$status")  $message"
}

# Print a labeled value
# Usage: ui_label_value "Pods" "8/10"
ui_label_value() {
    local label="$1"
    local value="$2"
    echo -e "${UI_FG_DIM}${label}${UI_NC}  ${value}"
}

# Repeat a character N times
# Usage: line=$(ui_repeat "─" 40)
ui_repeat() {
    local char="$1"
    local count="$2"
    local result=""
    for ((i=0; i<count; i++)); do result+="$char"; done
    echo "$result"
}

#=============================================================================
# FORMATTED OUTPUT
#=============================================================================

# Format a pipeline line (source -> deployer -> target)
# Usage: ui_pipeline "ok" "github.com/repo@main" "my-app" "12 resources"
ui_pipeline() {
    local status="$1"
    local source="$2"
    local deployer="$3"
    local target="$4"

    echo -e "$(ui_status_icon "$status")  ${source} ${UI_PIPE} ${deployer} ${UI_PIPE} ${target}"
}

# Format an ownership summary line
# Usage: ui_ownership_line flux_count argo_count helm_count native_count
ui_ownership_line() {
    local flux="$1" argo="$2" helm="$3" native="$4"
    local line=""

    [[ $flux -gt 0 ]] && line+="$(ui_owner_label Flux) $(ui_mini_bar $flux 8 "$UI_FG_CYAN") ${flux}    "
    [[ $argo -gt 0 ]] && line+="$(ui_owner_label ArgoCD) $(ui_mini_bar $argo 6 "$UI_FG_PURPLE") ${argo}    "
    [[ $helm -gt 0 ]] && line+="$(ui_owner_label Helm) $(ui_mini_bar $helm 6 "$UI_FG_ORANGE") ${helm}    "
    [[ $native -gt 0 ]] && line+="$(ui_owner_label Native) $(ui_mini_bar $native 6 "$UI_FG_DIM") ${native}"

    echo -e "$line"
}

#=============================================================================
# EXPORT ALIASES FOR CONVENIENCE
#=============================================================================

# Short aliases for common colors (optional, for brevity in scripts)
NC="$UI_NC"
BOLD="$UI_BOLD"
DIM="$UI_FG_DIM"
