#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "Checking docs for known stale shipped-feature wording..."

patterns=(
  "planned for v1\\.4"
  "PLANNED - requires cub-scout v1\\.4\\+"
  "MCP gateway .* planned for v1\\.4"
  "^## 4\\. Webhooks \\(Planned\\)$"
)

failed=0
for pattern in "${patterns[@]}"; do
  if rg -n --glob '!docs/archive/**' --glob '!examples/integrations/**' "$pattern" docs examples README.md CLI-GUIDE.md >/tmp/cub_scout_doc_freshness_hits.txt 2>/dev/null; then
    echo ""
    echo "Stale wording matched pattern: $pattern"
    cat /tmp/cub_scout_doc_freshness_hits.txt
    failed=1
  fi
done

if [[ "$failed" -ne 0 ]]; then
  echo ""
  echo "Doc freshness check failed."
  exit 1
fi

echo "Doc freshness check passed."

