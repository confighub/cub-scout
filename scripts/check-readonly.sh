#!/bin/bash
# check-readonly.sh - Verify read-only policy is enforced
#
# This script checks that Kubernetes client write operations are only used
# in allowed files (remedy.go, import commands, test files).
#
# We look for patterns like:
#   - client.Create(ctx, ...)
#   - client.Update(ctx, ...)
#   - client.Delete(ctx, ...)
#   - client.Patch(ctx, ...)
#   - .Resource(...).Create(...)
#   - .Resource(...).Update(...)
#
# Usage: ./scripts/check-readonly.sh

set -e

ALLOWED_PATTERNS=(
    "remedy.go"           # Remedy command can write
    "import.go"           # Import command can write (ConfigHub sync)
    "import_wizard.go"    # Import wizard can write
    "import_argocd.go"    # ArgoCD import can write
    "_test.go"            # Tests can use any operations
    "mock"                # Mock implementations
    "fake"                # Fake implementations
)

# Build exclusion pattern
EXCLUDE=""
for pattern in "${ALLOWED_PATTERNS[@]}"; do
    EXCLUDE+=" --exclude=$pattern"
done

echo "Checking for Kubernetes write operations outside allowed files..."
echo ""

# Check for Kubernetes client .Create( calls (exclude os.Create, etc.)
# Look for patterns like: ).Create( or client.Create( or clientset.Create(
CREATE_VIOLATIONS=$(grep -rn '\(client\|clientset\|Resource\|Namespace\)\.[^.]*\.Create(' --include='*.go' $EXCLUDE cmd/ pkg/ 2>/dev/null || true)
CREATE_VIOLATIONS+=$(grep -rn '\.Resource([^)]*)\.\(Namespace([^)]*)\.\)\?Create(' --include='*.go' $EXCLUDE cmd/ pkg/ 2>/dev/null || true)

# Check for Kubernetes client .Update( calls (exclude spinner.Update, etc.)
UPDATE_VIOLATIONS=$(grep -rn '\(client\|clientset\|Resource\|Namespace\)\.[^.]*\.Update(' --include='*.go' $EXCLUDE cmd/ pkg/ 2>/dev/null || true)
UPDATE_VIOLATIONS+=$(grep -rn '\.Resource([^)]*)\.\(Namespace([^)]*)\.\)\?Update(' --include='*.go' $EXCLUDE cmd/ pkg/ 2>/dev/null || true)

# Check for Kubernetes client .Delete( calls
DELETE_VIOLATIONS=$(grep -rn '\(client\|clientset\|Resource\|Namespace\)\.[^.]*\.Delete(' --include='*.go' $EXCLUDE cmd/ pkg/ 2>/dev/null || true)
DELETE_VIOLATIONS+=$(grep -rn '\.Resource([^)]*)\.\(Namespace([^)]*)\.\)\?Delete(' --include='*.go' $EXCLUDE cmd/ pkg/ 2>/dev/null || true)

# Check for Kubernetes client .Patch( calls
PATCH_VIOLATIONS=$(grep -rn '\(client\|clientset\|Resource\|Namespace\)\.[^.]*\.Patch(' --include='*.go' $EXCLUDE cmd/ pkg/ 2>/dev/null || true)
PATCH_VIOLATIONS+=$(grep -rn '\.Resource([^)]*)\.\(Namespace([^)]*)\.\)\?Patch(' --include='*.go' $EXCLUDE cmd/ pkg/ 2>/dev/null || true)

FOUND_VIOLATIONS=0

if [ -n "$CREATE_VIOLATIONS" ]; then
    echo "ERROR: Found .Create() calls outside allowed files:"
    echo "$CREATE_VIOLATIONS"
    echo ""
    FOUND_VIOLATIONS=1
fi

if [ -n "$UPDATE_VIOLATIONS" ]; then
    echo "ERROR: Found .Update() calls outside allowed files:"
    echo "$UPDATE_VIOLATIONS"
    echo ""
    FOUND_VIOLATIONS=1
fi

if [ -n "$DELETE_VIOLATIONS" ]; then
    echo "ERROR: Found .Delete() calls outside allowed files:"
    echo "$DELETE_VIOLATIONS"
    echo ""
    FOUND_VIOLATIONS=1
fi

if [ -n "$PATCH_VIOLATIONS" ]; then
    echo "ERROR: Found .Patch() calls outside allowed files:"
    echo "$PATCH_VIOLATIONS"
    echo ""
    FOUND_VIOLATIONS=1
fi

if [ $FOUND_VIOLATIONS -eq 1 ]; then
    echo "FAILED: Read-only policy violation detected!"
    echo ""
    echo "If this is intentional, add the file to the ALLOWED_PATTERNS in this script."
    echo "See SECURITY.md for the read-only policy."
    exit 1
fi

echo "PASSED: No write operations found outside allowed files."
echo ""
echo "Allowed files: ${ALLOWED_PATTERNS[*]}"
