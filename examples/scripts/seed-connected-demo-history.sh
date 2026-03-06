#!/usr/bin/env bash
# seed-connected-demo-history.sh - Demo-only synthetic ChangeSet seeding.
#
# IMPORTANT:
# - Synthetic demo data only. Never run in production spaces.
# - Requires explicit --allow-synthetic.
# - Defaults to dry-run; pass --apply to execute.

set -euo pipefail

SPACE=""
ALLOW_SYNTHETIC=false
APPLY=false
ALLOW_CI=false
PREFIX="demo"

usage() {
    cat <<USAGE
Usage: $(basename "$0") --space <slug> --allow-synthetic [--apply] [--allow-ci] [--prefix <value>]

Options:
  --space <slug>         Target ConfigHub space (required)
  --allow-synthetic      Required safety gate acknowledging synthetic demo data
  --apply                Execute changeset creation (default: dry-run only)
  --allow-ci             Allow --apply when CI environment variable is set
  --prefix <value>       Prefix for seeded ChangeSet slugs (default: demo)
  -h, --help             Show this help
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
        --allow-synthetic)
            ALLOW_SYNTHETIC=true
            shift
            ;;
        --apply)
            APPLY=true
            shift
            ;;
        --allow-ci)
            ALLOW_CI=true
            shift
            ;;
        --prefix)
            [[ $# -ge 2 ]] || { echo "missing value for --prefix" >&2; usage; exit 2; }
            PREFIX="$2"
            shift 2
            ;;
        --prefix=*)
            PREFIX="${1#*=}"
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

if [[ -z "$SPACE" ]]; then
    echo "--space is required" >&2
    usage
    exit 2
fi

if [[ "$ALLOW_SYNTHETIC" != "true" ]]; then
    echo "Refusing to seed synthetic data without --allow-synthetic" >&2
    exit 1
fi

if [[ "$APPLY" == "true" && -n "${CI:-}" && "$ALLOW_CI" != "true" ]]; then
    echo "Refusing --apply in CI without --allow-ci" >&2
    exit 1
fi

if [[ "$APPLY" == "true" ]] && ! command -v cub >/dev/null 2>&1; then
    echo "cub CLI not found in PATH" >&2
    exit 1
fi

# slug|description
seed_items=(
    "${PREFIX}-ci-rollout-api-v1-4-3|ci-bot rolled api from v1.4.2 to v1.4.3"
    "${PREFIX}-scale-worker-debug|sarah scaled worker replicas from 3 to 1 for incident triage"
    "${PREFIX}-db-pool-tuning|platform tuned database pool config for checkout latency"
)

run_create() {
    local slug="$1"
    local description="$2"

    local cmd=(
        cub changeset create
        --space "$SPACE"
        "$slug"
        --description "$description"
        --label demo=true
        --label synthetic=true
        --label source=cub-scout-demo-seed
        --label scenario=connected-history
        --allow-exists
        --quiet
    )

    if [[ "$APPLY" == "true" ]]; then
        "${cmd[@]}"
        echo "Created: $slug"
    else
        printf 'DRY RUN: '
        printf '%q ' "${cmd[@]}"
        printf '\n'
    fi
}

echo "Seeding connected demo history"
echo "space=$SPACE mode=$([[ "$APPLY" == "true" ]] && echo apply || echo dry-run) synthetic=true"

for item in "${seed_items[@]}"; do
    slug="${item%%|*}"
    description="${item#*|}"
    run_create "$slug" "$description"
done

if [[ "$APPLY" != "true" ]]; then
    echo "No changes applied. Re-run with --apply to create synthetic ChangeSets."
    echo "Synthetic markers: demo=true, synthetic=true, source=cub-scout-demo-seed"
fi
