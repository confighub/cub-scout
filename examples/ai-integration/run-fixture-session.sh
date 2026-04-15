#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
TESTDATA_DIR="$SCRIPT_DIR/testdata"
OUTPUT_DIR="$SCRIPT_DIR/sample-output"
BINARY="${CUB_SCOUT_BINARY:-$REPO_ROOT/cub-scout}"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --output-dir)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        --help|-h)
            echo "Usage: run-fixture-session.sh [--output-dir <path>]"
            exit 0
            ;;
        *)
            echo "Unknown argument: $1" >&2
            exit 1
            ;;
    esac
done

mkdir -p "$OUTPUT_DIR"

if [[ ! -x "$BINARY" ]]; then
    echo "Building cub-scout binary at $BINARY"
    (
        cd "$REPO_ROOT"
        go build -o "$BINARY" ./cmd/cub-scout
    )
fi

echo "Running fixture-backed AI integration session"
echo "Output directory: $OUTPUT_DIR"

TRACE_OUT="$OUTPUT_DIR/01-debug-deployment.txt"
HISTORY_OUT="$OUTPUT_DIR/02-change-history.txt"
SCAN_OUT="$OUTPUT_DIR/03-scan-safety.json"
ORPHANS_OUT="$OUTPUT_DIR/04-unmanaged-resources.txt"

# Fixture replay must stay deterministic even when the caller happens to be
# logged in to ConfigHub locally.
FIXTURE_ENV=(CUB_SCOUT_OFFLINE=true)

env "${FIXTURE_ENV[@]}" CUB_SCOUT_TEST_TRACE_JSON="$TESTDATA_DIR/trace_argo_source_signals.json" \
"$BINARY" trace deployment/checkout -n checkout > "$TRACE_OUT"

echo "Wrote $TRACE_OUT"

env "${FIXTURE_ENV[@]}" CUB_SCOUT_TEST_HISTORY_JSON="$TESTDATA_DIR/history_changesets.json" \
"$BINARY" history deployment/checkout -n checkout --since 24h > "$HISTORY_OUT"

echo "Wrote $HISTORY_OUT"

env "${FIXTURE_ENV[@]}" "$BINARY" scan --file "$TESTDATA_DIR/misconfigured-deployment.yaml" --json > "$SCAN_OUT"
echo "Wrote $SCAN_OUT (exit 0: scan succeeded, findings present but no --fail-on threshold)"

env "${FIXTURE_ENV[@]}" CUB_SCOUT_TEST_MAP_ENTRIES_JSON="$TESTDATA_DIR/map_orphans.json" \
"$BINARY" map orphans > "$ORPHANS_OUT"

echo "Wrote $ORPHANS_OUT"

# Quick assertions so demo failures are obvious.
grep -q "OutOfSync" "$TRACE_OUT"
grep -q "Change History" "$HISTORY_OUT"
grep -q '"ccve_id": "CCVE-2025-' "$SCAN_OUT"
grep -q "debug-nginx" "$ORPHANS_OUT"

echo "Fixture session completed successfully"
