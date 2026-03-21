#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WITH_WORKER=false
SEED_HISTORY=false
EXPLAIN=false
EXPLAIN_JSON=false
CONFIGHUB_URL_OVERRIDE=""

usage() {
    cat <<'USAGE'
Usage: ./setup.sh [--with-worker] [--seed-history] [--confighub-url=<url>] [--explain | --explain-json]

Options:
  --with-worker            Run the live ConfigHub import path (maps to ./demo.sh --live)
  --seed-history           Seed synthetic ChangeSet history after import (implies --with-worker)
  --confighub-url=<url>    Override the ConfigHub URL passed through to ./demo.sh
  --explain                Print the planned actions without mutating anything
  --explain-json           Print the planned actions as JSON without mutating anything
  -h, --help               Show this help

Notes:
  - setup.sh is the AI-first entry point for this example.
  - It keeps the cluster running so ./verify.sh can inspect it afterwards.
  - ./demo.sh remains the narrated human walkthrough.
USAGE
}

for arg in "$@"; do
    case "$arg" in
        --with-worker)
            WITH_WORKER=true
            ;;
        --seed-history)
            SEED_HISTORY=true
            WITH_WORKER=true
            ;;
        --confighub-url=*)
            CONFIGHUB_URL_OVERRIDE="${arg#*=}"
            ;;
        --explain)
            EXPLAIN=true
            ;;
        --explain-json)
            EXPLAIN_JSON=true
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            echo "unknown argument: $arg" >&2
            usage
            exit 2
            ;;
    esac
done

if [[ "$EXPLAIN" == "true" && "$EXPLAIN_JSON" == "true" ]]; then
    echo "choose only one of --explain or --explain-json" >&2
    exit 2
fi

DEMO_FLAGS=(--keep)
if [[ "$WITH_WORKER" == "true" ]]; then
    DEMO_FLAGS+=(--live)
fi
if [[ "$SEED_HISTORY" == "true" ]]; then
    DEMO_FLAGS+=(--seed-history)
fi
if [[ -n "$CONFIGHUB_URL_OVERRIDE" ]]; then
    DEMO_FLAGS+=("--confighub-url=$CONFIGHUB_URL_OVERRIDE")
fi

if [[ "$EXPLAIN_JSON" == "true" ]]; then
    cat <<JSON
{
  "example": "argo-import-confighub-demo",
  "entrypoint": "./setup.sh",
  "humanDemoEntrypoint": "./demo.sh",
  "mutates": false,
  "clusterName": "argo-import-demo",
  "configHubSpace": "argo-import-demo",
  "keepsClusterRunning": true,
  "withWorker": $WITH_WORKER,
  "seedHistory": $SEED_HISTORY,
  "executionCommand": "./demo.sh ${DEMO_FLAGS[*]}",
  "writes": [
    "kind cluster kind-argo-import-demo",
    "current kubeconfig context via kind",
    "repo-root cub-scout binary built by demo.sh",
    "temporary discovery worker pid/log under TMPDIR or /tmp when --with-worker is used",
    "ConfigHub space, targets, and units only when --with-worker is used"
  ],
  "evidenceSurfaces": [
    "cluster",
    "ConfigHub",
    "cub-scout"
  ]
}
JSON
    exit 0
fi

if [[ "$EXPLAIN" == "true" ]]; then
    cat <<TEXT
Example: argo-import-confighub-demo
Entry point: ./setup.sh
Human demo path: ./demo.sh
Mutates: no (this preview only)

Execution plan:
- run ./demo.sh with: ${DEMO_FLAGS[*]}
- keep the cluster running so ./verify.sh can inspect it afterwards
- use ./cleanup.sh for explicit teardown

What this writes when executed:
- a kind cluster named argo-import-demo
- current kubeconfig context via kind
- a repo-root cub-scout binary built by demo.sh
- temporary discovery worker pid/log under TMPDIR or /tmp when --with-worker is used
- ConfigHub space, targets, and units only when --with-worker is used

Evidence surfaces after setup:
- cluster evidence via kubectl against kind-argo-import-demo
- ConfigHub evidence via cub when --with-worker is used
- cub-scout evidence via gitops status and map list

Current boundary:
- post-import scan/finding evidence is not part of this setup/verify contract yet
TEXT
    exit 0
fi

exec "$SCRIPT_DIR/demo.sh" "${DEMO_FLAGS[@]}"
