# Importing Workloads into ConfigHub

**Status: Working** — Import your cluster workloads into ConfigHub in 30 seconds.

---

## Quick Start

```bash
# 1. See what you have (no account needed)
./cub-agent map

# 2. Preview what will be imported
./cub-agent import --dry-run

# 3. Import (requires ConfigHub account)
cub auth login
./cub-agent import
```

That's it. One command discovers, suggests, and creates everything.

---

## What Import Does

Import scans your cluster and creates ConfigHub **Units** for each workload.

**Before import:**
- Cluster has Deployments, StatefulSets, etc.
- You see resources via kubectl
- No app-level grouping

**After import:**
- Same resources, now tracked as Units in ConfigHub
- App hierarchy: Org → Space → Unit
- Fleet queries work across all imported clusters
- Shared picture for your team

---

## Command Options

| Command | What it does |
|---------|--------------|
| `./cub-agent import` | Import all namespaces |
| `./cub-agent import -n argocd` | Import one namespace |
| `./cub-agent import --dry-run` | Preview without changes |
| `./cub-agent import -y` | Skip confirmation |
| `./cub-agent import --json` | JSON output for GUI |
| `./cub-agent import --no-log` | Disable logging |

---

## Example Output

```bash
$ ./cub-agent import --dry-run
```

```
┌─────────────────────────────────────────────────────────────┐
│ DISCOVERED                                                  │
└─────────────────────────────────────────────────────────────┘
  argocd (7 workloads)
  payments-prod (3 workloads)
  payments-staging (3 workloads)

┌─────────────────────────────────────────────────────────────┐
│ WILL CREATE                                                 │
└─────────────────────────────────────────────────────────────┘
  App Space: payments-team

  • payment-api-prod
    labels: app=payment-api, variant=prod, team=payments
    workloads: 1

  • payment-api-staging
    labels: app=payment-api, variant=staging, team=payments
    workloads: 1

  Total: 6 units

(dry-run mode - no changes made)
Run without --dry-run to import.
```

---

## What Gets Created

| ConfigHub Concept | How it's determined |
|-------------------|---------------------|
| **App Space** | Auto-inferred from namespace patterns |
| **Unit slug** | From workload name + variant |
| **Labels** | Extracted from K8s labels and namespace patterns |

### Labels Added to Units

| Label | Source |
|-------|--------|
| `app` | `app.kubernetes.io/name` or workload name |
| `variant` | Namespace suffix (`-prod`, `-staging`) or `default` |
| `team` | `app.kubernetes.io/part-of` or namespace |
| `tier` | `app.kubernetes.io/component` |

---

## After Import

### View Your Units

```bash
# List units in ConfigHub
cub unit list --space my-team

# View with cub-agent map
./cub-agent map
```

### Query Across Units

```bash
# All prod variants
cub unit list --where "Labels.variant='prod'"

# All payment-api instances
cub unit list --where "Labels.app='payment-api'"
```

---

## GUI Integration

For GUI or scripted workflows, use JSON output:

```bash
# Generate proposal
./cub-agent import --json > proposal.json

# (GUI displays, user edits)

# Apply edited proposal
cat proposal.json | ./cub-agent apply -
```

---

## Logging & Session State

### Local Logs

Import creates a log file by default for debugging and audit trails.

```
Log: ~/.confighub/logs/import-2026-01-08-210327.log
```

**Log contents:**
- Start time and parameters
- Discovered namespaces and workloads
- Proposed App Space and Units
- Result (created/failed counts)

**Disable logging:**
```bash
./cub-agent import --no-log
```

### Session Persistence (Coming Soon)

When you exit mid-import, your progress is saved:

```
Session: ~/.confighub/sessions/import-latest.json
```

Use interactive wizard for guided import:
```bash
./cub-agent import --wizard      # Interactive TUI wizard (recommended)
./cub-agent import --dry-run     # Preview without making changes
```

### Cloud Audit Trail (Connected Mode)

**When connected to ConfigHub**, logs and session state sync to the cloud automatically.

| What Syncs | When | Who Can See |
|------------|------|-------------|
| Session progress | Every step | Your team (configurable) |
| Full logs | On complete/error | Org admins |

**Benefits:**
- **Cross-device resume** — Start import on laptop, finish on desktop
- **Team visibility** — See what colleagues are working on
- **Audit trail** — Full history of who imported what, when
- **Troubleshooting** — Support can see your logs (with permission)

View import history in ConfigHub GUI:
```
ConfigHub → Activity → Import Sessions
```

Note: JSON mode (`--json`) outputs to stdout for scripting.

---

## Troubleshooting

### "No workloads found"

Check your namespace has Deployments, StatefulSets, or DaemonSets:
```bash
kubectl get deploy,sts,ds -n my-namespace
```

### Want to import a specific namespace?

```bash
./cub-agent import -n my-namespace
```

### App Space name not right?

The App Space name is auto-inferred. After import, you can rename it in ConfigHub.

---

## See Also

- [README.md](../README.md) — Quick start
- [CLI-REFERENCE.md](CLI-REFERENCE.md) — Full CLI reference
