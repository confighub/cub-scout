#!/usr/bin/env bash
# connect-and-compare demo (fixture-first, no live cluster required)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

OUTPUT_DIR="$SCRIPT_DIR/sample-output"
VERIFY=false

usage() {
    cat <<USAGE
Usage: $(basename "$0") [--output-dir <path>] [--verify]

Options:
  --output-dir <path>  Write generated snapshots to this directory
                       (default: examples/connect-and-compare/sample-output)
  --verify             Compare generated snapshots with expected-output/
  -h, --help           Show this help
USAGE
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --output-dir)
            [[ $# -ge 2 ]] || { echo "missing value for --output-dir" >&2; usage; exit 2; }
            OUTPUT_DIR="$2"
            shift 2
            ;;
        --output-dir=*)
            OUTPUT_DIR="${1#*=}"
            shift
            ;;
        --verify)
            VERIFY=true
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            echo "unknown argument: $1" >&2
            usage
            exit 2
            ;;
    esac
done

if [[ -n "${CUB_SCOUT_BIN:-}" ]]; then
    CUB="$CUB_SCOUT_BIN"
elif [[ -x "$REPO_ROOT/cub-scout" ]]; then
    CUB="$REPO_ROOT/cub-scout"
else
    echo "building cub-scout binary..."
    (cd "$REPO_ROOT" && go build -o cub-scout ./cmd/cub-scout)
    CUB="$REPO_ROOT/cub-scout"
fi

if [[ ! -x "$CUB" ]]; then
    echo "cub-scout binary is not executable: $CUB" >&2
    exit 1
fi

DOCTOR_FIXTURE="$SCRIPT_DIR/testdata/doctor_input.json"
HISTORY_FIXTURE="$SCRIPT_DIR/testdata/history_changesets.json"
GIT_FIXTURE="$REPO_ROOT/examples/combined-git-live/git-repo"
BUNDLE_FIXTURE="$REPO_ROOT/examples/workflows/fleet-demo/bundles/dev"
EXPECTED_DIR="$SCRIPT_DIR/expected-output"

mkdir -p "$OUTPUT_DIR"

echo "[1/4] Standalone snapshot: doctor"
CUB_SCOUT_TEST_DOCTOR_INPUT_JSON="$DOCTOR_FIXTURE" "$CUB" doctor --format ascii > "$OUTPUT_DIR/01-doctor.txt"

echo "[2/4] Connect step (operator action)"
cat > "$OUTPUT_DIR/02-connect.txt" <<'STEP2'
$ cub auth login
Connected mode established (fixture replay path for demo).
STEP2

echo "[3/4] Compare step: Git intent vs bundle snapshot"
"$CUB" combined --git-path "$GIT_FIXTURE" --bundle "$BUNDLE_FIXTURE" --json > "$OUTPUT_DIR/03-compare.json"

echo "[4/4] History step: ConfigHub ChangeSet timeline"
CUB_SCOUT_TEST_HISTORY_JSON="$HISTORY_FIXTURE" "$CUB" history deploy/checkout -n prod --since 3650d --format ascii > "$OUTPUT_DIR/04-history.txt"

if [[ "$VERIFY" == "true" ]]; then
    diff -u "$EXPECTED_DIR/01-doctor.txt" "$OUTPUT_DIR/01-doctor.txt"
    diff -u "$EXPECTED_DIR/03-compare.json" "$OUTPUT_DIR/03-compare.json"
    diff -u "$EXPECTED_DIR/04-history.txt" "$OUTPUT_DIR/04-history.txt"
    echo "All snapshots match expected output"
fi

echo "Demo artifacts written to: $OUTPUT_DIR"
