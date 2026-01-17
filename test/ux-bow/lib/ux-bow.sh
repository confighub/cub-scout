#!/usr/bin/env bash
# UX-BOW: User Experience Benchmark for Observability Workflows
# Run scenarios, measure difficulty, generate reports

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UXBOW_DIR="$(dirname "$SCRIPT_DIR")"
PROJECT_ROOT="$(dirname "$(dirname "$UXBOW_DIR")")"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color
BOLD='\033[1m'

# Usage
usage() {
    cat <<EOF
UX-BOW: User Experience Benchmark for Observability Workflows

Usage: $(basename "$0") [OPTIONS]

Options:
    --scenario=NAME     Run specific scenario (e.g., debug-pod-crash)
    --persona=NAME      Run all scenarios for persona (e.g., developer)
    --matrix            Run full test matrix (all scenarios × all entry points)
    --report            Generate summary report from results
    --list              List all available scenarios
    --list-personas     List all available personas
    --baseline          Create baseline measurements
    --compare=FILE      Compare against previous baseline
    --verbose           Show detailed output
    --help              Show this help

Examples:
    $(basename "$0") --list                     # List scenarios
    $(basename "$0") --scenario=debug-pod-crash # Run one scenario
    $(basename "$0") --persona=developer        # Run for persona
    $(basename "$0") --matrix                   # Run everything
    $(basename "$0") --report                   # Generate report
EOF
}

# List scenarios
list_scenarios() {
    echo -e "${BOLD}Available Scenarios:${NC}"
    echo
    for f in "$UXBOW_DIR"/scenarios/*.yaml; do
        [[ -f "$f" ]] || continue
        local id name category target
        id=$(grep '^id:' "$f" | head -1 | awk '{print $2}')
        name=$(grep '^name:' "$f" | head -1 | cut -d: -f2- | xargs)
        category=$(grep '^category:' "$f" | head -1 | awk '{print $2}')
        target=$(grep '^difficulty_target:' "$f" | head -1 | awk '{print $2}')

        printf "  ${CYAN}%-25s${NC} %-40s ${YELLOW}[%s]${NC} ${GREEN}%s${NC}\n" \
            "$id" "$name" "$category" "$target"
    done
    echo
}

# List personas
list_personas() {
    echo -e "${BOLD}Available Personas:${NC}"
    echo
    for f in "$UXBOW_DIR"/personas/*.yaml; do
        [[ -f "$f" ]] || continue
        local id name level
        id=$(grep '^id:' "$f" | head -1 | awk '{print $2}')
        name=$(grep '^name:' "$f" | head -1 | cut -d: -f2- | xargs)
        level=$(grep '^expertise_level:' "$f" | head -1 | awk '{print $2}')

        printf "  ${CYAN}%-20s${NC} %-30s ${YELLOW}[%s]${NC}\n" "$id" "$name" "$level"
    done
    echo
}

# Parse scenario YAML and extract scores
parse_scenario() {
    local scenario_file="$1"
    local entry_point="$2"

    # Extract composite scores from YAML
    # Look for pattern like "  tui: 4.85" under composite_scores:
    local score
    score=$(awk -v ep="$entry_point" '
        /^composite_scores:/ { in_scores=1; next }
        in_scores && /^[^ ]/ { in_scores=0 }
        in_scores && $1 == ep":" { gsub(/[^0-9.]/, "", $2); print $2; exit }
    ' "$scenario_file")
    echo "${score:-0}"
}

# Run a single scenario
run_scenario() {
    local scenario_id="$1"
    local scenario_file="$UXBOW_DIR/scenarios"/*"$scenario_id"*.yaml

    if [[ ! -f $scenario_file ]]; then
        echo -e "${RED}Error: Scenario '$scenario_id' not found${NC}"
        return 1
    fi

    local name category
    name=$(grep '^name:' "$scenario_file" | head -1 | cut -d: -f2- | xargs)
    category=$(grep '^category:' "$scenario_file" | head -1 | awk '{print $2}')

    echo -e "${BOLD}Scenario: $name${NC}"
    echo -e "Category: ${YELLOW}$category${NC}"
    echo

    # Extract scores for each entry point
    echo -e "${BOLD}Composite Scores:${NC}"
    for ep in tui cli hub; do
        local score
        score=$(parse_scenario "$scenario_file" "$ep")
        local color=$GREEN
        if (( $(echo "$score < 4.0" | bc -l 2>/dev/null || echo 1) )); then
            color=$YELLOW
        fi
        if (( $(echo "$score < 3.0" | bc -l 2>/dev/null || echo 0) )); then
            color=$RED
        fi
        printf "  %-6s ${color}%s${NC}\n" "$ep:" "$score"
    done
    echo
}

# Run all scenarios for a persona
run_persona() {
    local persona_id="$1"
    local persona_file="$UXBOW_DIR/personas/$persona_id.yaml"

    if [[ ! -f "$persona_file" ]]; then
        echo -e "${RED}Error: Persona '$persona_id' not found${NC}"
        return 1
    fi

    local name
    name=$(grep '^name:' "$persona_file" | head -1 | cut -d: -f2- | xargs)

    echo -e "${BOLD}Running scenarios for persona: $name${NC}"
    echo

    # Run each scenario and collect scores
    local total_score=0
    local count=0

    for f in "$UXBOW_DIR"/scenarios/*.yaml; do
        [[ -f "$f" ]] || continue

        # Check if persona is relevant to this scenario
        if grep -q "$persona_id" "$f" 2>/dev/null; then
            local scenario_id
            scenario_id=$(grep '^id:' "$f" | head -1 | awk '{print $2}')
            run_scenario "$scenario_id"

            # Track average (using TUI scores for now)
            local score
            score=$(parse_scenario "$f" "tui")
            total_score=$(echo "$total_score + $score" | bc -l 2>/dev/null || echo "$total_score")
            ((count++)) || true
        fi
    done

    if [[ $count -gt 0 ]]; then
        local avg
        avg=$(echo "scale=2; $total_score / $count" | bc -l 2>/dev/null || echo "N/A")
        echo -e "${BOLD}Average TUI Score: ${GREEN}$avg${NC}"
    fi
}

# Generate summary report
generate_report() {
    local date_str
    date_str=$(date +%Y-%m-%d)
    local results_dir="$UXBOW_DIR/results/$date_str"

    echo -e "${BOLD}UX-BOW Summary Report${NC}"
    echo -e "Date: $date_str"
    echo

    echo -e "${BOLD}Scenario Scores:${NC}"
    echo
    printf "%-30s %-8s %-8s %-8s %-8s\n" "Scenario" "TUI" "CLI" "Hub" "Avg"
    echo "------------------------------------------------------------"

    local total_tui=0 total_cli=0 total_hub=0
    local count=0

    for f in "$UXBOW_DIR"/scenarios/*.yaml; do
        [[ -f "$f" ]] || continue

        local id
        id=$(grep '^id:' "$f" | head -1 | awk '{print $2}')
        local tui cli hub
        tui=$(parse_scenario "$f" "tui")
        cli=$(parse_scenario "$f" "cli")
        hub=$(parse_scenario "$f" "hub")

        local avg
        avg=$(echo "scale=2; ($tui + $cli + $hub) / 3" | bc -l 2>/dev/null || echo "N/A")

        printf "%-30s %-8s %-8s %-8s %-8s\n" "$id" "$tui" "$cli" "$hub" "$avg"

        total_tui=$(echo "$total_tui + $tui" | bc -l 2>/dev/null || echo "$total_tui")
        total_cli=$(echo "$total_cli + $cli" | bc -l 2>/dev/null || echo "$total_cli")
        total_hub=$(echo "$total_hub + $hub" | bc -l 2>/dev/null || echo "$total_hub")
        ((count++)) || true
    done

    echo "------------------------------------------------------------"

    if [[ $count -gt 0 ]]; then
        local avg_tui avg_cli avg_hub
        avg_tui=$(echo "scale=2; $total_tui / $count" | bc -l 2>/dev/null || echo "N/A")
        avg_cli=$(echo "scale=2; $total_cli / $count" | bc -l 2>/dev/null || echo "N/A")
        avg_hub=$(echo "scale=2; $total_hub / $count" | bc -l 2>/dev/null || echo "N/A")

        printf "${BOLD}%-30s %-8s %-8s %-8s${NC}\n" "AVERAGE" "$avg_tui" "$avg_cli" "$avg_hub"
    fi

    echo
    echo -e "${BOLD}Target: 4.0+ across all scenarios${NC}"

    # Identify areas needing improvement
    echo
    echo -e "${BOLD}Areas Needing Improvement (< 4.0):${NC}"
    for f in "$UXBOW_DIR"/scenarios/*.yaml; do
        [[ -f "$f" ]] || continue

        local id
        id=$(grep '^id:' "$f" | head -1 | awk '{print $2}')

        for ep in tui cli hub; do
            local score
            score=$(parse_scenario "$f" "$ep")
            if (( $(echo "$score < 4.0" | bc -l 2>/dev/null || echo 0) )); then
                echo -e "  ${RED}$id${NC} ($ep): $score"
            fi
        done
    done
}

# Main
main() {
    local action=""
    local target=""
    local verbose=false

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --scenario=*)
                action="scenario"
                target="${1#*=}"
                ;;
            --persona=*)
                action="persona"
                target="${1#*=}"
                ;;
            --matrix)
                action="matrix"
                ;;
            --report)
                action="report"
                ;;
            --list)
                action="list"
                ;;
            --list-personas)
                action="list-personas"
                ;;
            --baseline)
                action="baseline"
                ;;
            --verbose|-v)
                verbose=true
                ;;
            --help|-h)
                usage
                exit 0
                ;;
            *)
                echo -e "${RED}Unknown option: $1${NC}"
                usage
                exit 1
                ;;
        esac
        shift
    done

    # Header
    echo
    echo -e "${BOLD}${CYAN}╔═══════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BOLD}${CYAN}║   UX-BOW: User Experience Benchmark                       ║${NC}"
    echo -e "${BOLD}${CYAN}║   For Observability Workflows                             ║${NC}"
    echo -e "${BOLD}${CYAN}╚═══════════════════════════════════════════════════════════╝${NC}"
    echo

    case "$action" in
        list)
            list_scenarios
            ;;
        list-personas)
            list_personas
            ;;
        scenario)
            run_scenario "$target"
            ;;
        persona)
            run_persona "$target"
            ;;
        matrix)
            echo "Running full test matrix..."
            for f in "$UXBOW_DIR"/scenarios/*.yaml; do
                [[ -f "$f" ]] || continue
                local id
                id=$(grep '^id:' "$f" | head -1 | awk '{print $2}')
                run_scenario "$id"
                echo "---"
            done
            generate_report
            ;;
        report)
            generate_report
            ;;
        baseline)
            echo "Creating baseline..."
            generate_report > "$UXBOW_DIR/results/baseline-$(date +%Y-%m-%d).txt"
            echo -e "${GREEN}Baseline saved.${NC}"
            ;;
        *)
            usage
            ;;
    esac
}

main "$@"
