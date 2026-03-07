#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

if [[ ! -x "./cub-scout" ]]; then
  echo "Building ./cub-scout ..."
  go build ./cmd/cub-scout
fi

echo "== Connected summary window =="
./cub-scout summary list --since 24h || true

echo
echo "== Slack payload preview (dry-run) =="
./cub-scout summary slack --since 24h --dry-run || true

if [[ -n "${CUB_SCOUT_SLACK_WEBHOOK_URL:-}" ]]; then
  echo
  echo "== Posting digest to Slack =="
  ./cub-scout summary slack --since 24h --webhook-url "$CUB_SCOUT_SLACK_WEBHOOK_URL"
else
  echo
  echo "Skipping post: set CUB_SCOUT_SLACK_WEBHOOK_URL to enable webhook delivery."
fi
