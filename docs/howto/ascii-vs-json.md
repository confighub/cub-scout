# ASCII vs JSON: What Each Output Gives You

cub-scout produces two output formats for most commands. They contain the same facts but serve different purposes.

---

## Quick Summary

| Format | Purpose | Audience | Stable? |
|--------|---------|----------|---------|
| `--format ascii` (default) | Human-readable explanation | Operators, on-call, demos | Layout may evolve |
| `--format json` | Machine-readable data | Scripts, CI/CD, automation | Field names are stable |

**Rule of thumb:** Read ASCII when debugging. Parse JSON when automating.

---

## What ASCII Gives You

ASCII output is designed for human comprehension. It:

- Groups resources by owner, namespace, or status
- Adds headings, icons, and color for quick scanning
- Orders items by importance (critical issues first)
- Summarizes counts and highlights anomalies
- Tells the debugging story: what happened, why, and what to do

```bash
# Human-readable ownership map
./cub-scout map list

# Human-readable trace chain
./cub-scout trace deploy/web -n prod
```

ASCII is the default format. It is what you see in the terminal and TUI.

**What ASCII is NOT:** ASCII is not a data contract. The layout, grouping, and ordering may change between versions to improve readability. Do not parse ASCII with `grep` or `awk` for automation — use JSON instead.

---

## What JSON Gives You

JSON output is the canonical data contract. It:

- Contains every structural fact cub-scout knows
- Uses stable field names (documented per command)
- Is deterministic: same input produces the same output
- Is lossless: nothing is grouped, summarized, or hidden

```bash
# Machine-readable ownership map
./cub-scout map list --format json

# Machine-readable trace chain
./cub-scout trace deploy/web -n prod --format json

# Pipe to jq for filtering
./cub-scout map list --format json | jq '.resources[] | select(.owner == "Native")'
```

**What JSON is NOT:** JSON does not include narrative explanations, debugging guidance, or visual emphasis. For those, use ASCII.

---

## Same Facts, Different Views

Both formats derive from the same internal model. The relationship is:

```
ASCII = render(JSON facts) + narrative
```

Every fact in the ASCII output has a corresponding field in the JSON output. ASCII adds narrative (headings, grouping, emphasis) to make the facts readable.

### Example: Map List

**ASCII** groups by owner and adds status indicators:

```
Flux (3 resources)
  ✓ deploy/payment-api     prod     Synced
  ✓ deploy/auth-service    prod     Synced
  ⚠ deploy/notification    staging  Suspended

Native (1 resource)
  ? deploy/hotfix-cache    prod     Unknown
```

**JSON** lists every resource with explicit fields:

```json
{
  "resources": [
    {"kind": "Deployment", "name": "payment-api", "namespace": "prod", "owner": "Flux", "status": "Synced"},
    {"kind": "Deployment", "name": "auth-service", "namespace": "prod", "owner": "Flux", "status": "Synced"},
    {"kind": "Deployment", "name": "notification", "namespace": "staging", "owner": "Flux", "status": "Suspended"},
    {"kind": "Deployment", "name": "hotfix-cache", "namespace": "prod", "owner": "Native", "status": "Unknown"}
  ]
}
```

Same four resources, same ownership, same status — just different presentations.

---

## When to Use Which

| Scenario | Use |
|----------|-----|
| Investigating an incident | ASCII |
| Running in CI/CD pipeline | JSON |
| Demo or presentation | ASCII |
| Feeding data to another tool | JSON |
| Alerting on orphan count | JSON |
| Understanding a trace chain | ASCII |
| Comparing two clusters | JSON |
| Weekly team review | ASCII |

---

## Common Patterns

### Count orphans in CI

```bash
ORPHAN_COUNT=$(./cub-scout map list --format json \
  | jq '[.resources[] | select(.owner == "Native")] | length')

if [ "$ORPHAN_COUNT" -gt 0 ]; then
  echo "WARNING: $ORPHAN_COUNT orphan resources detected"
  exit 1
fi
```

### Extract trace chain as JSON

```bash
./cub-scout trace deploy/web -n prod --format json \
  | jq '.chain[] | {kind, name, ready, status}'
```

### Filter by owner in a script

```bash
./cub-scout map list --format json \
  | jq '.resources[] | select(.owner == "Flux") | .name'
```

---

## Markdown Format

Some commands also support `--format md` for Markdown output, useful for:

- Pasting into GitHub issues or PRs
- Generating reports
- Slack messages (via code blocks)

```bash
./cub-scout map list --format md
./cub-scout gitops status --format md
```

---

## Field Naming Conventions

JSON field names follow per-surface conventions:

| Surface | Convention | Example |
|---------|------------|---------|
| CLI commands (map, trace) | camelCase | `formatVersion`, `capturedAt` |
| Debug bundle metadata | camelCase | `cubScoutVersion`, `createdAt` |
| Versioned schemas (graph, catalog) | snake_case | `schema_version`, `join_mode` |

Field names within each surface are frozen — they will not change in future versions.

---

## See Also

- [Semantic Contract](../semantic-contract.md) — Technical specification (R1-R6 rules)
- [JSON Contracts](../reference/json-contracts.md) — Field naming and schema references
- [CLI Contract](../reference/cli-contract.md) — Command stability guarantees
- [Query Resources](query-resources.md) — Query syntax for filtering
