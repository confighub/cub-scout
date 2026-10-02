#!/bin/bash
# check-no-personal-names.sh - Keep personal names out of committed files
#
# Prose in this repo says "the team", "a colleague", "the maintainer", or the
# issue number instead of naming a person, and sample identities use neutral
# addresses such as user@example.com. This script fails when any tracked file
# contains one of the swept first names as a whole word, or when a tracked
# path does, and prints the offending lines.
#
# Scope notes:
#   - Every tracked file is in scope. This script is the only exclusion,
#     because it must hold the pattern. Keep names out of its prose too.
#   - Binary files are scanned too; a hit prints as "Binary file ... matches".
#   - The match is whole-word and case-insensitive (git grep -w). A bare first
#     name matches; a longer identifier that merely contains one (for example
#     a GitHub handle) does not.
#
# Usage: ./scripts/check-no-personal-names.sh

set -euo pipefail

PATTERN='alexis|jesper|brian|charlie'
SELF="scripts/check-no-personal-names.sh"

cd "$(git rev-parse --show-toplevel)"

# Match bytes, so binary files are scanned the same way in every locale.
export LC_ALL=C

echo "Checking tracked files for personal names..."
echo ""

status=0
CONTENT_VIOLATIONS=$(git grep -nwiE "$PATTERN" -- . ":(exclude)$SELF") || status=$?
if [ "$status" -gt 1 ]; then
    echo "ERROR: git grep failed with status $status" >&2
    exit 2
fi

PATH_VIOLATIONS=$(git ls-files | grep -wiE "$PATTERN" || true)

FOUND_VIOLATIONS=0

if [ -n "$CONTENT_VIOLATIONS" ]; then
    echo "ERROR: Found personal names in tracked files:"
    echo "$CONTENT_VIOLATIONS"
    echo ""
    FOUND_VIOLATIONS=1
fi

if [ -n "$PATH_VIOLATIONS" ]; then
    echo "ERROR: Found personal names in tracked file paths:"
    echo "$PATH_VIOLATIONS"
    echo ""
    FOUND_VIOLATIONS=1
fi

if [ "$FOUND_VIOLATIONS" -eq 1 ]; then
    echo "Replace each name with neutral wording (the team / a colleague /"
    echo "the maintainer / the issue number), use neutral sample identities"
    echo "such as user@example.com, and rename files with git mv."
    exit 1
fi

echo "OK: no personal names in tracked files"
