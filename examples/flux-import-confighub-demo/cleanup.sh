#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SPACE="flux-import-demo"
WORKER_STATE_DIR="${TMPDIR:-/tmp}/cub-scout-demo-workers"
WORKER_LIFECYCLE_SCRIPT="$SCRIPT_DIR/../scripts/demo-worker-lifecycle.sh"

usage() {
    cat <<'USAGE'
Usage: ./cleanup.sh [--space <slug>]

Options:
  --space <slug>   ConfigHub space slug used for the live worker pid path
                   default: flux-import-demo
  -h, --help       Show this help

Notes:
  - cleanup.sh stops the detached discovery worker if its pid file exists.
  - cleanup.sh deletes the local kind cluster.
  - cleanup.sh does not remove ConfigHub units or ChangeSets.
USAGE
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --space)
            [[ $# -ge 2 ]] || { echo "missing value for --space" >&2; usage; exit 2; }
            SPACE="$2"
            shift 2
            ;;
        --space=*)
            SPACE="${1#*=}"
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

PID_FILE="$WORKER_STATE_DIR/${SPACE}-discovery-worker.pid"

if [[ -f "$PID_FILE" ]]; then
    bash "$WORKER_LIFECYCLE_SCRIPT" stop --pid-file "$PID_FILE" >/dev/null 2>&1 || true
    echo "stopped discovery worker for space '$SPACE'"
else
    echo "no discovery worker pid file found for space '$SPACE'"
fi

kind delete cluster --name flux-import-demo >/dev/null 2>&1 || true
echo "deleted kind cluster 'flux-import-demo'"
