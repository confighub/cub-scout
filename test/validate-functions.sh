#!/bin/bash
# validate-functions.sh - Validate CCVE remedy function definitions
#
# Checks:
# 1. YAML syntax is valid
# 2. Required fields are present (id, ccves, description, input.types, transform, example.before, example.after)
# 3. Transform operations are valid types
# 4. Example before/after are valid YAML

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
FUNCTIONS_DIR="${SCRIPT_DIR}/../cve/functions"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

VALID_OPERATIONS="set_field regex_replace add_annotation add_label delete_field fix_reference"

errors=0
warnings=0
validated=0

log_error() {
    echo -e "${RED}ERROR${NC}: $1"
    ((errors++))
}

log_warning() {
    echo -e "${YELLOW}WARNING${NC}: $1"
    ((warnings++))
}

log_success() {
    echo -e "${GREEN}OK${NC}: $1"
}

validate_function() {
    local file="$1"
    local filename=$(basename "$file")
    local has_error=0

    # Skip README
    if [[ "$filename" == "README.md" ]]; then
        return 0
    fi

    # Check YAML syntax
    if ! yq '.' "$file" > /dev/null 2>&1; then
        log_error "$filename: Invalid YAML syntax"
        return 1
    fi

    # Check required fields
    local id=$(yq -r '.id' "$file")
    if [[ -z "$id" || "$id" == "null" ]]; then
        log_error "$filename: Missing required field 'id'"
        has_error=1
    fi

    local ccves=$(yq -r '.ccves' "$file")
    if [[ -z "$ccves" || "$ccves" == "null" ]]; then
        # Check for old 'ccve' field
        local ccve=$(yq -r '.ccve' "$file")
        if [[ -n "$ccve" && "$ccve" != "null" ]]; then
            log_warning "$filename: Uses deprecated 'ccve' field, should be 'ccves' array"
        else
            log_error "$filename: Missing required field 'ccves'"
            has_error=1
        fi
    fi

    local description=$(yq -r '.description' "$file")
    if [[ -z "$description" || "$description" == "null" ]]; then
        log_error "$filename: Missing required field 'description'"
        has_error=1
    fi

    local input_types=$(yq -r '.input.types' "$file")
    if [[ -z "$input_types" || "$input_types" == "null" ]]; then
        log_error "$filename: Missing required field 'input.types'"
        has_error=1
    fi

    local transform=$(yq -r '.transform' "$file")
    if [[ -z "$transform" || "$transform" == "null" ]]; then
        log_error "$filename: Missing required field 'transform'"
        has_error=1
    fi

    # Check transform operations
    local transform_count=$(yq -r '.transform | length' "$file" 2>/dev/null || echo "0")
    for ((i=0; i<transform_count; i++)); do
        local path=$(yq -r ".transform[$i].path" "$file")
        local operation=$(yq -r ".transform[$i].operation" "$file")

        if [[ -z "$path" || "$path" == "null" ]]; then
            log_error "$filename: Transform[$i] missing 'path'"
            has_error=1
        fi

        if [[ -z "$operation" || "$operation" == "null" ]]; then
            log_warning "$filename: Transform[$i] missing 'operation', assuming set_field"
        elif ! echo "$VALID_OPERATIONS" | grep -qw "$operation"; then
            log_error "$filename: Transform[$i] has invalid operation '$operation'"
            has_error=1
        fi
    done

    # Check example.before and example.after
    local before=$(yq -r '.example.before' "$file")
    if [[ -z "$before" || "$before" == "null" ]]; then
        log_error "$filename: Missing required field 'example.before'"
        has_error=1
    else
        # Validate before is valid YAML
        if ! echo "$before" | yq '.' > /dev/null 2>&1; then
            log_error "$filename: 'example.before' is not valid YAML"
            has_error=1
        fi
    fi

    local after=$(yq -r '.example.after' "$file")
    if [[ -z "$after" || "$after" == "null" ]]; then
        log_error "$filename: Missing required field 'example.after'"
        has_error=1
    else
        # Validate after is valid YAML
        if ! echo "$after" | yq '.' > /dev/null 2>&1; then
            log_error "$filename: 'example.after' is not valid YAML"
            has_error=1
        fi
    fi

    if [[ $has_error -eq 0 ]]; then
        log_success "$filename: Valid"
        ((validated++))
    fi

    return $has_error
}

echo "Validating CCVE Remedy Functions"
echo "================================="
echo ""

# Check yq is installed
if ! command -v yq &> /dev/null; then
    echo "Error: yq is required but not installed."
    echo "Install with: brew install yq"
    exit 1
fi

# Validate all function files
for file in "$FUNCTIONS_DIR"/*.yaml; do
    if [[ -f "$file" ]]; then
        validate_function "$file" || true
    fi
done

echo ""
echo "================================="
echo "Summary:"
echo "  Validated: $validated"
echo "  Warnings:  $warnings"
echo "  Errors:    $errors"

if [[ $errors -gt 0 ]]; then
    echo ""
    echo -e "${RED}Validation failed with $errors error(s)${NC}"
    exit 1
else
    echo ""
    echo -e "${GREEN}All functions validated successfully!${NC}"
    exit 0
fi
