#!/usr/bin/env bash
# Record the fixture-driven connect-and-compare flow with asciinema.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CAST_PATH="$SCRIPT_DIR/connect-and-compare.cast"

if ! command -v asciinema >/dev/null 2>&1; then
    echo "asciinema not found. Install with: brew install asciinema" >&2
    exit 1
fi

asciinema rec "$CAST_PATH" -c "bash '$SCRIPT_DIR/demo.sh' --verify"
echo "Recording saved to: $CAST_PATH"
