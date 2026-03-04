#!/usr/bin/env bash
set -euo pipefail

if [ "${1:-}" = "--help" ] || [ "$#" -lt 5 ]; then
  cat <<'EOF'
Usage:
  ./scripts/create-ai-capability-issue.sh \
    "<title>" \
    "<user-goal>" \
    "<commands-attempted>" \
    "<observed-gap>" \
    "<expected-behavior>" \
    [repo]

Example:
  ./scripts/create-ai-capability-issue.sh \
    "Capability gap: Git patch flow after import" \
    "Import Argo workloads and generate Git patch proposals" \
    "./cub-scout import --dry-run -n demo; ./cub-scout import -n demo -y" \
    "Import completes, but no Git patch/upgrade follow-up is available" \
    "Offer optional Git-aware patch/upgrade workflow after import"

Defaults:
  repo = confighub/cub-scout
EOF
  exit 1
fi

title="$1"
user_goal="$2"
attempted="$3"
observed="$4"
expected="$5"
repo="${6:-confighub/cub-scout}"

tmp_body="$(mktemp)"
trap 'rm -f "$tmp_body"' EXIT

cat >"$tmp_body" <<EOF
## User Goal
$user_goal

## Commands Attempted
\`\`\`bash
$attempted
\`\`\`

## Observed Behavior
$observed

## Expected Behavior
$expected

## Why This Matters
Discovered during AI-assisted usage/demo. This gap blocks a natural "ask -> do -> extend" workflow.
EOF

gh issue create -R "$repo" --title "$title" --template ai-capability-gap.yml --body-file "$tmp_body"
