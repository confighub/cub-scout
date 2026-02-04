#!/usr/bin/env bash
# Fleet Demo — v0.18 (#105)
#
# This script demonstrates comparing environments using catalogs.
#
# "Fleet" here means: an explicit set of bundles with labels.
# No inference. No magic. Just structured comparison.
#
# The workflow:
#   1. Create a catalog
#   2. Add bundles with environment labels
#   3. List the catalog to see the overview
#   4. Compare environments with bundle diff
#
# All operations are file-based and deterministic.
#
# Usage:
#   ./examples/workflows/fleet-demo/fleet-demo.sh
#
# Requirements:
#   - cub-scout binary built (go build ./cmd/cub-scout)
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$ROOT"

# Ensure binary exists
if [[ ! -x "./cub-scout" ]]; then
  echo "Building cub-scout..."
  go build ./cmd/cub-scout
fi

# Use the pre-built bundles in the demo directory
DEMO_DIR="$ROOT/examples/workflows/fleet-demo"
BUNDLES_DIR="$DEMO_DIR/bundles"

# Create a temporary workspace for the catalog
WORKSPACE="$(mktemp -d)"
trap 'rm -rf "$WORKSPACE"' EXIT

CATALOG_DIR="$WORKSPACE/env-catalog"

echo "=============================================="
echo "  Fleet Demo: Comparing Environments"
echo "=============================================="
echo ""

# -----------------------------------------------------------------------------
# Step 1: Create a catalog
# -----------------------------------------------------------------------------
echo "Step 1: Create a catalog"
echo "------------------------"
echo ""

./cub-scout catalog init "$CATALOG_DIR"
echo ""

# -----------------------------------------------------------------------------
# Step 2: Add bundles with environment labels
# -----------------------------------------------------------------------------
echo "Step 2: Add bundles with environment labels"
echo "--------------------------------------------"
echo ""

# Add prod first (oldest), then staging, then dev (newest)
# This gives a natural promotion timeline: prod -> staging -> dev

./cub-scout catalog add "$CATALOG_DIR" "$BUNDLES_DIR/prod" \
  --id prod \
  --label env=production \
  --label tier=stable \
  --sequence 1
echo ""

./cub-scout catalog add "$CATALOG_DIR" "$BUNDLES_DIR/staging" \
  --id staging \
  --label env=staging \
  --label tier=release-candidate \
  --sequence 2
echo ""

./cub-scout catalog add "$CATALOG_DIR" "$BUNDLES_DIR/dev" \
  --id dev \
  --label env=development \
  --label tier=bleeding-edge \
  --sequence 3
echo ""

# -----------------------------------------------------------------------------
# Step 3: List the catalog
# -----------------------------------------------------------------------------
echo "Step 3: List the catalog"
echo "------------------------"
echo ""

./cub-scout catalog list "$CATALOG_DIR"
echo ""

# -----------------------------------------------------------------------------
# Step 4: Compare environments (prod vs staging)
# -----------------------------------------------------------------------------
echo "Step 4: Compare environments (prod vs staging)"
echo "-----------------------------------------------"
echo ""

./cub-scout bundle diff "$CATALOG_DIR/bundles/prod" "$CATALOG_DIR/bundles/staging"
echo ""

# -----------------------------------------------------------------------------
# Step 5: Compare environments (staging vs dev)
# -----------------------------------------------------------------------------
echo "Step 5: Compare environments (staging vs dev)"
echo "----------------------------------------------"
echo ""

./cub-scout bundle diff "$CATALOG_DIR/bundles/staging" "$CATALOG_DIR/bundles/dev"
echo ""

# -----------------------------------------------------------------------------
# Summary
# -----------------------------------------------------------------------------
echo "=============================================="
echo "  Demo Complete"
echo "=============================================="
echo ""
echo "This workflow demonstrated:"
echo "  1. Creating a catalog of environment bundles"
echo "  2. Adding bundles with explicit env labels (no inference)"
echo "  3. Listing to see the fleet overview"
echo "  4. Comparing environments to see what differs"
echo ""
echo "Key points:"
echo "  - 'Fleet' = explicit set of bundles with labels"
echo "  - Environment labels are metadata (env=production)"
echo "  - Ordering via --sequence, not timestamps"
echo "  - Comparison is offline, deterministic, portable"
echo ""
echo "Real-world usage:"
echo "  - CI captures bundles per environment"
echo "  - Catalog indexes them with labels"
echo "  - Developers compare to understand differences"
echo "  - No inference, no magic, just structured data"
echo ""
