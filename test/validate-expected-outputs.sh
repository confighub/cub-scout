#!/bin/bash
# Validate Expected Outputs
#
# Runs commands and verifies output matches expected patterns.
# Uses YAML files in test/expected-outputs/ as source of truth.
#
# Usage:
#   ./test/validate-expected-outputs.sh                    # All
#   ./test/validate-expected-outputs.sh --category=cli     # CLI only
#   ./test/validate-expected-outputs.sh --category=demos   # Demos only
#   ./test/validate-expected-outputs.sh --file=cli/map/standalone.yaml
#   ./test/validate-expected-outputs.sh --quick            # Skip slow tests
#   ./test/validate-expected-outputs.sh --connected        # Include connected
#
# Exit codes:
#   0 = All validations passed
#   1 = One or more validations failed

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
EXPECTED_DIR="$SCRIPT_DIR/expected-outputs"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

# Counters
PASSED=0
FAILED=0
SKIPPED=0

# Options
CATEGORY=""
SPECIFIC_FILE=""
QUICK=false
INCLUDE_CONNECTED=false
VERBOSE=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --category=*) CATEGORY="${1#*=}"; shift ;;
        --file=*) SPECIFIC_FILE="${1#*=}"; shift ;;
        --quick) QUICK=true; shift ;;
        --connected) INCLUDE_CONNECTED=true; shift ;;
        --verbose|-v) VERBOSE=true; shift ;;
        --help|-h)
            echo "Usage: validate-expected-outputs.sh [options]"
            echo ""
            echo "Options:"
            echo "  --category=X    Only validate category (cli, demos, examples)"
            echo "  --file=X        Only validate specific file"
            echo "  --quick         Skip slow validations"
            echo "  --connected     Include connected mode tests"
            echo "  --verbose       Show detailed output"
            exit 0
            ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

pass() {
    echo -e "${GREEN}✓${NC} $1"
    PASSED=$((PASSED + 1))
}

fail() {
    echo -e "${RED}✗${NC} $1"
    FAILED=$((FAILED + 1))
}

skip() {
    echo -e "${YELLOW}○${NC} $1 (skipped)"
    SKIPPED=$((SKIPPED + 1))
}

section() {
    echo ""
    echo -e "${BLUE}━━━ $1 ━━━${NC}"
}

verbose() {
    if $VERBOSE; then
        echo -e "  ${YELLOW}→${NC} $1"
    fi
}

# Check if yq is available
if ! command -v yq &>/dev/null; then
    echo "yq required but not installed. Install: brew install yq"
    exit 1
fi

# Check prerequisites for a command
check_prereqs() {
    local yaml_file="$1"
    local mode
    mode=$(yq -r '.mode // "standalone"' "$yaml_file")

    case "$mode" in
        standalone)
            # Just need cluster
            if ! kubectl cluster-info &>/dev/null 2>&1; then
                echo "no_cluster"
                return
            fi
            echo "ok"
            ;;
        connected)
            if ! $INCLUDE_CONNECTED; then
                echo "skip_connected"
                return
            fi
            # Need cub auth + workers
            if ! command -v cub &>/dev/null; then
                echo "no_cub"
                return
            fi
            if ! cub context get &>/dev/null 2>&1; then
                echo "no_auth"
                return
            fi
            # Check workers
            local worker_count
            worker_count=$(cub worker list --json 2>/dev/null | jq 'length' 2>/dev/null || echo "0")
            if [[ "$worker_count" -eq 0 ]]; then
                echo "no_workers"
                return
            fi
            echo "ok"
            ;;
        *)
            echo "ok"
            ;;
    esac
}

# Validate a single command from YAML
validate_command() {
    local yaml_file="$1"
    local cmd_index="$2"
    local cmd_id
    local cmd_command
    local expected_exit

    cmd_id=$(yq -r ".commands[$cmd_index].id" "$yaml_file")
    cmd_command=$(yq -r ".commands[$cmd_index].command" "$yaml_file")
    expected_exit=$(yq -r ".commands[$cmd_index].expected.exit_code // 0" "$yaml_file")

    verbose "Running: $cmd_command"

    # Run command and capture output
    local output
    local actual_exit=0
    cd "$PROJECT_ROOT"
    output=$(eval "$cmd_command" 2>&1) || actual_exit=$?

    # Check exit code
    if [[ "$actual_exit" -ne "$expected_exit" ]]; then
        fail "$cmd_id: exit code $actual_exit (expected $expected_exit)"
        if $VERBOSE; then
            echo "    Output: ${output:0:200}..."
        fi
        return 1
    fi

    # Check 'contains' patterns
    local contains_count
    contains_count=$(yq -r '.commands['"$cmd_index"'].expected.contains | length' "$yaml_file")

    for ((i=0; i<contains_count; i++)); do
        local pattern
        pattern=$(yq -r ".commands[$cmd_index].expected.contains[$i]" "$yaml_file")

        # Handle pattern objects vs strings
        if [[ "$pattern" == *"pattern:"* ]]; then
            pattern=$(yq -r ".commands[$cmd_index].expected.contains[$i].pattern" "$yaml_file")
        fi

        if ! echo "$output" | grep -qE "$pattern"; then
            fail "$cmd_id: missing pattern '$pattern'"
            return 1
        fi
    done

    # Check 'not_contains' patterns
    local not_contains_count
    not_contains_count=$(yq -r '.commands['"$cmd_index"'].expected.not_contains | length' "$yaml_file")

    for ((i=0; i<not_contains_count; i++)); do
        local pattern
        pattern=$(yq -r ".commands[$cmd_index].expected.not_contains[$i]" "$yaml_file")

        if echo "$output" | grep -qiE "$pattern"; then
            fail "$cmd_id: contains forbidden pattern '$pattern'"
            if $VERBOSE; then
                echo "    Found: $(echo "$output" | grep -iE "$pattern" | head -1)"
            fi
            return 1
        fi
    done

    pass "$cmd_id"
    return 0
}

# Validate a single YAML file
validate_file() {
    local yaml_file="$1"
    local name
    name=$(yq -r '.name // "unknown"' "$yaml_file")

    verbose "Validating: $name"

    # Check prerequisites
    local prereq_status
    prereq_status=$(check_prereqs "$yaml_file")

    case "$prereq_status" in
        no_cluster)
            skip "$name (no cluster access)"
            return
            ;;
        skip_connected)
            skip "$name (connected mode, use --connected)"
            return
            ;;
        no_cub)
            skip "$name (cub CLI not installed)"
            return
            ;;
        no_auth)
            skip "$name (not authenticated to ConfigHub)"
            return
            ;;
        no_workers)
            skip "$name (no workers connected)"
            return
            ;;
    esac

    # Validate each command
    local cmd_count
    cmd_count=$(yq -r '.commands | length' "$yaml_file")

    for ((i=0; i<cmd_count; i++)); do
        validate_command "$yaml_file" "$i" || true
    done
}

# Main
echo ""
echo -e "${BLUE}╔════════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║           Expected Output Validation                           ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════════════════════════╝${NC}"

if [[ -n "$SPECIFIC_FILE" ]]; then
    # Validate single file
    if [[ -f "$EXPECTED_DIR/$SPECIFIC_FILE" ]]; then
        section "Validating: $SPECIFIC_FILE"
        validate_file "$EXPECTED_DIR/$SPECIFIC_FILE"
    else
        fail "File not found: $SPECIFIC_FILE"
    fi
else
    # Validate by category or all
    for category_dir in "$EXPECTED_DIR"/*/; do
        category_name=$(basename "$category_dir")

        # Filter by category if specified
        if [[ -n "$CATEGORY" && "$category_name" != "$CATEGORY" ]]; then
            continue
        fi

        section "Category: $category_name"

        # Find all YAML files in category
        find "$category_dir" -name "*.yaml" -type f | sort | while read -r yaml_file; do
            validate_file "$yaml_file"
        done
    done
fi

# Summary
section "Summary"

echo ""
echo -e "Passed:  ${GREEN}$PASSED${NC}"
echo -e "Failed:  ${RED}$FAILED${NC}"
echo -e "Skipped: ${YELLOW}$SKIPPED${NC}"
echo ""

if [[ $FAILED -gt 0 ]]; then
    echo -e "${RED}${BOLD}VALIDATION FAILED${NC}"
    echo ""
    echo "Fix the failures above or update expected outputs if behavior changed intentionally."
    exit 1
elif [[ $PASSED -eq 0 && $SKIPPED -gt 0 ]]; then
    echo -e "${YELLOW}${BOLD}ALL TESTS SKIPPED${NC}"
    echo ""
    echo "No tests ran. Check prerequisites:"
    echo "  - Cluster: kubectl cluster-info"
    echo "  - Connected: --connected flag + cub auth login"
    exit 0
else
    echo -e "${GREEN}${BOLD}VALIDATION PASSED${NC}"
    exit 0
fi
