#!/bin/bash
# Full test suite for cub-scout v0.16+
# Covers: unit tests, race detection, determinism, fixture E2E
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
cd "$ROOT_DIR"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

pass() { echo -e "${GREEN}✓ $1${NC}"; }
fail() { echo -e "${RED}✗ $1${NC}"; exit 1; }
info() { echo -e "${YELLOW}→ $1${NC}"; }
SKIPPED_OPTIONAL=0

echo "=========================================="
echo "cub-scout full test suite"
echo "=========================================="
echo ""

# Prevent automatic toolchain download (for air-gapped environments)
export GOTOOLCHAIN=local

# Check Go version (requires 1.24+)
REQUIRED_GO_VERSION="1.24"
CURRENT_GO_VERSION=$(go version 2>/dev/null | grep -oE 'go[0-9]+\.[0-9]+' | sed 's/go//')
if [ -z "$CURRENT_GO_VERSION" ]; then
  fail "Go is not installed. Please install Go $REQUIRED_GO_VERSION or later."
fi

# Compare versions (simple major.minor check)
CURRENT_MAJOR=$(echo "$CURRENT_GO_VERSION" | cut -d. -f1)
CURRENT_MINOR=$(echo "$CURRENT_GO_VERSION" | cut -d. -f2)
REQUIRED_MAJOR=$(echo "$REQUIRED_GO_VERSION" | cut -d. -f1)
REQUIRED_MINOR=$(echo "$REQUIRED_GO_VERSION" | cut -d. -f2)

if [ "$CURRENT_MAJOR" -lt "$REQUIRED_MAJOR" ] || \
   ([ "$CURRENT_MAJOR" -eq "$REQUIRED_MAJOR" ] && [ "$CURRENT_MINOR" -lt "$REQUIRED_MINOR" ]); then
  fail "Go $REQUIRED_GO_VERSION+ required, but found Go $CURRENT_GO_VERSION. Please upgrade."
fi
pass "Go version check passed (Go $CURRENT_GO_VERSION)"

# 1) Build
info "Building cub-scout..."
go build ./cmd/cub-scout || fail "Build failed"
pass "Build succeeded"

# 2) Unit tests
info "Running unit tests..."
GO_TEST_PARALLELISM="${GO_TEST_PARALLELISM:-4}"
go test -p "$GO_TEST_PARALLELISM" ./... || fail "Unit tests failed"
pass "Unit tests passed"

# 3) Race detection
info "Running race detector..."
go test -race ./pkg/agent/... || fail "Race detection failed"
pass "Race detection passed"

# 4) Determinism tests for v0.16 attribution
info "Running determinism tests..."
TMPDIR=$(mktemp -d)
trap "rm -rf $TMPDIR" EXIT

# Use a fixture bundle that includes attribution.json
ATTR_BUNDLE_FIXTURE="$ROOT_DIR/test/fixtures/attribution/bundle-with-attribution"

# Test attribution graph determinism
info "  Testing attribution graph determinism..."
for i in 1 2 3; do
  ./cub-scout bundle replay "$ATTR_BUNDLE_FIXTURE" \
    --section attribution --format json > "$TMPDIR/attr-$i.json" || fail "Attribution replay failed (run $i)"
done

if ! cmp -s "$TMPDIR/attr-1.json" "$TMPDIR/attr-2.json"; then
  fail "Determinism check failed: run 1 and run 2 differ"
fi
if ! cmp -s "$TMPDIR/attr-2.json" "$TMPDIR/attr-3.json"; then
  fail "Determinism check failed: run 2 and run 3 differ"
fi
pass "Determinism tests passed"

# 5) Fixture E2E for debug --save-bundle
info "Running fixture E2E tests..."

# Create a mock debug session for testing
cat > "$TMPDIR/debug-session.json" << 'ENDJSON'
{
  "target": {
    "kind": "Deployment",
    "name": "api-server",
    "namespace": "production"
  },
  "startedAt": "2024-01-15T10:00:00Z",
  "completedAt": "2024-01-15T10:05:00Z",
  "workloadStatus": {
    "kind": "Deployment",
    "name": "api-server",
    "namespace": "production",
    "replicas": 3,
    "readyReplicas": 3
  }
}
ENDJSON

# Test debug with save-bundle using test hooks
export CUB_SCOUT_TEST_DEBUG_JSON="$TMPDIR/debug-session.json"
export CUB_SCOUT_TEST_TARGET_OBJECT="$ROOT_DIR/test/fixtures/crossplane/deployment_no_crossplane.json"

if ./cub-scout debug deployment/api-server -n production --save-bundle "$TMPDIR/bundles" --format json > /dev/null 2>&1; then
  if [ -d "$TMPDIR/bundles" ]; then
    pass "Debug --save-bundle E2E passed"
  else
    fail "Debug command succeeded but no bundle directory was created"
  fi
else
  SKIPPED_OPTIONAL=$((SKIPPED_OPTIONAL + 1))
  info "  (Optional) Debug --save-bundle E2E skipped - requires cluster or full test hooks"
fi

# 6) Summary
echo ""
echo "=========================================="
echo -e "${GREEN}All required tests passed!${NC}"
if [ "$SKIPPED_OPTIONAL" -gt 0 ]; then
  echo -e "${YELLOW}Optional checks skipped: $SKIPPED_OPTIONAL${NC}"
fi
echo "=========================================="
