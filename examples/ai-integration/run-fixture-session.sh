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

CUB_SCOUT_TEST_TRACE_JSON="$TESTDATA_DIR/trace_argo_source_signals.json" \
"$BINARY" trace deployment/checkout -n checkout > "$TRACE_OUT"

echo "Wrote $TRACE_OUT"

CUB_SCOUT_TEST_HISTORY_JSON="$TESTDATA_DIR/history_changesets.json" \
"$BINARY" history deployment/checkout -n checkout --since 24h > "$HISTORY_OUT"

echo "Wrote $HISTORY_OUT"

set +e
"$BINARY" scan --file "$TESTDATA_DIR/misconfigured-deployment.yaml" --json > "$SCAN_OUT"
scan_rc=$?
set -e
if [[ "$scan_rc" -ne 1 ]]; then
    echo "Expected scan --file exit code 1 (findings), got $scan_rc" >&2
    exit 1
fi
echo "Wrote $SCAN_OUT (expected exit code 1 due to findings)"

CUB_SCOUT_TEST_MAP_ENTRIES_JSON="$TESTDATA_DIR/map_orphans.json" \
"$BINARY" map orphans > "$ORPHANS_OUT"

echo "Wrote $ORPHANS_OUT"

# Quick assertions so demo failures are obvious.
grep -q "OutOfSync" "$TRACE_OUT"
grep -q "Change History" "$HISTORY_OUT"
grep -q "CCVE-2025-0244" "$SCAN_OUT"
grep -q "debug-nginx" "$ORPHANS_OUT"

echo "Fixture session completed successfully"
