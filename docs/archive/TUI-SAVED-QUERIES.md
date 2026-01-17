# TUI Saved Queries

Saved queries are named, reusable query expressions that help you quickly filter resources.

## Overview

The ConfigHub Agent TUI includes **saved queries** — shortcuts for common filters:

```bash
# List all saved queries
cub-agent map queries

# Run a saved query by name
cub-agent map list -q unmanaged

# Combine queries with filters
cub-agent map list -q "unmanaged AND namespace=prod*"
```

## TUI Output

Run `./test/atk/map queries` or `./examples/demos/tui-queries-demo.sh` to see this with terminal colors:

**Color Legend:**
- 🟡 **Yellow/Orange** = Native (unmanaged) — no GitOps owner
- 🟢 **Green** = GitOps managed / ConfigHub
- 🔵 **Cyan** = Flux
- 🟣 **Purple** = Argo CD
- 🟠 **Orange** = Helm
- ⚫ **Dim** = Informational (deployments, services, prod)

```
╭──────────────────────────────────────────────────────────────────────╮
│  🔍 SAVED QUERIES                       run with: map list -q <name>  │
╰──────────────────────────────────────────────────────────────────────╯

┌─ BUILT-IN QUERIES ───────────────────────────────────────────────────┐
│                                                                      │
│  NAME          DESCRIPTION                                   MATCHES │
│  ────────────────────────────────────────────────────────────────────│
🟡 unmanaged     Resources with no GitOps owner                    47  🟡
🟢 gitops        Resources managed by GitOps (Flux or Argo)        23  🟢
🔵 flux          All Flux-managed resources                        15  🔵
🟣 argo          All Argo CD-managed resources                      8  🟣
🟠 helm-only     Helm-managed resources (no GitOps)                 5  🟠
🟢 confighub     Resources managed by ConfigHub                     0  🟢
⚫ deployments   All Deployments across namespaces                 12  ⚫
⚫ services      All Services across namespaces                    18  ⚫
⚫ prod          Resources in production namespaces                31  ⚫
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘

┌─ YOUR QUERIES ───────────────────────────────────────────────────────┐
│                                                                      │
│  NAME          DESCRIPTION                                   QUERY   │
│  ────────────────────────────────────────────────────────────────────│
│  my-team       Payment team resources            labels[team]=payments│
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘

┌─ USAGE ──────────────────────────────────────────────────────────────┐
│                                                                      │
│  Run a saved query:                                                  │
│    cub-agent map list -q unmanaged                                   │
│    cub-agent map list -q "unmanaged AND namespace=prod*"             │
│                                                                      │
│  Save a new query:                                                   │
│    cub-agent map queries save my-team "labels[team]=payments"        │
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘

────────────────────────────────────────────────────────────────────────
🔗 Want team-shared queries, alerts, and history?
   See: cub-agent map queries connect
────────────────────────────────────────────────────────────────────────
```

## Sample Query Output

Running `cub-agent map list -q unmanaged` shows resources with no GitOps owner:

```
  NAMESPACE              KIND           NAME                                     OWNER
  ─────────────────────────────────────────────────────────────────────────────────────
🟡 argocd                StatefulSet    argocd-application-controller            Native
🟡 argocd                Deployment     argocd-applicationset-controller         Native
🟡 argocd                Service        argocd-server                            Native
🟡 argocd                ConfigMap      argocd-cm                                Native
🟡 default               ConfigMap      kube-root-ca.crt                         Native
🟡 monitoring            Deployment     prometheus                               Native
🟡 monitoring            Service        prometheus                               Native
  ... (47 total)
```

## Built-in Queries

| Name | Query | Description |
|------|-------|-------------|
| `unmanaged` | `owner=Native` | Resources with no GitOps owner (kubectl apply, etc.) |
| `gitops` | `owner=Flux OR owner=Argo` | Resources managed by GitOps |
| `flux` | `owner=Flux` | All Flux-managed resources |
| `argo` | `owner=Argo` | All Argo CD-managed resources |
| `helm-only` | `owner=Helm` | Helm-managed resources (no GitOps reconciliation) |
| `confighub` | `owner=ConfigHub` | Resources managed by ConfigHub |
| `deployments` | `kind=Deployment` | All Deployments across namespaces |
| `services` | `kind=Service` | All Services across namespaces |
| `prod` | `namespace=prod* OR namespace=production*` | Resources in production namespaces |

## User Queries

Save your own queries to `~/.confighub/queries.yaml`:

```bash
# Save a new query
cub-agent map queries save my-team "labels[team]=payments" "Payment team resources"

# Use it
cub-agent map list -q my-team

# Delete it
cub-agent map queries delete my-team
```

The queries file format:

```yaml
# ~/.confighub/queries.yaml
queries:
  - name: my-team
    description: Payment team resources
    query: labels[team]=payments
  - name: critical-apps
    description: Production critical workloads
    query: "namespace=prod* AND labels[tier]=critical"
```

## Combining Queries

Saved query names are resolved before the query runs, so you can combine them:

```bash
# Find unmanaged resources in production
cub-agent map list -q "unmanaged AND namespace=prod*"

# Find GitOps resources that aren't Flux
cub-agent map list -q "gitops AND owner!=Flux"

# Find deployments managed by Argo
cub-agent map list -q "deployments AND argo"
```

## Commands Reference

| Command | Description |
|---------|-------------|
| `cub-agent map queries` | List all saved queries |
| `cub-agent map queries --json` | List queries as JSON |
| `cub-agent map queries save <name> <query> [desc]` | Save a user query |
| `cub-agent map queries delete <name>` | Delete a user query |
| `cub-agent map queries connect` | Get started with ConfigHub |
| `./test/atk/map queries` | TUI view with live match counts |

## ConfigHub Integration

Want team-shared queries, alerts when results change, and history over time?

```bash
cub-agent map queries connect
```

This shows how to connect to ConfigHub:

```
🔗 CONNECT TO CONFIGHUB
═══════════════════════

ConfigHub gives you:
  • Saved queries shared with your team
  • Alerts when query results change
  • History and trends over time
  • Fleet-wide queries across all clusters

GET STARTED
───────────
  1. Sign up or log in:  https://confighub.com
  2. Import workloads:   cub-agent import --namespace <ns>

  Full guide: docs/IMPORTING-WORKLOADS.md
```

### Connection Journey

The ConfigHub adoption journey (see [04-MAP-USER-JOURNEY-TO-FULL-CONFIGHUB.md](planning/map/04-MAP-USER-JOURNEY-TO-FULL-CONFIGHUB.md)):

| Stage | Command | What You Get |
|-------|---------|--------------|
| **1. Standalone** | `cub-agent map` | See what's running, who owns it |
| **2. Discovery** | `cub-agent import --dry-run` | Proposed structure |
| **3. Connected** | `cub auth login` + `cub-agent import` | Units in ConfigHub |
| **4. Worker** | `cub worker run` | Sync to targets |
| **5. Full** | ConfigHub UI | Actions, queries, changesets |

**Step 1: Install cub CLI** (if not installed)
```
STATUS
──────
  ✗ cub CLI not installed

Install the cub CLI:
  brew install confighubai/tap/cub
  # or
  curl -fsSL https://get.confighub.com | sh
```

**Step 2: Authenticate**
```
YOUR STATUS
───────────
  ✗ Not authenticated

NEXT STEP
─────────
  Run: cub auth login

  This opens your browser to authenticate with ConfigHub.
```

**Step 3: Import workloads** (creates units in ConfigHub)
```
YOUR STATUS
───────────
🟢 ✓ Authenticated (org: my-company)
  ✗ No units

NEXT STEP
─────────
  Run: cub-agent import --namespace myapp

  Import your workloads as ConfigHub Units to track them.
```

**Step 4: Set up workers** (enables sync to clusters)
```
YOUR STATUS
───────────
🟢 ✓ Authenticated (org: my-company)
🟢 ✓ Units imported
  ✗ No workers

NEXT STEP
─────────
  Run: cub worker run my-cluster

  Workers connect your cluster to ConfigHub for syncing.
```

**Step 5: Create targets** (deployment destinations)
```
YOUR STATUS
───────────
🟢 ✓ Authenticated (org: my-company)
🟢 ✓ Units imported
🟢 ✓ Workers configured
  ✗ No targets

NEXT STEP
─────────
  Run: cub target create my-target

  Targets are deployment destinations for your configs.
```

**All set!**
```
YOUR STATUS
───────────
🟢 ✓ Authenticated (org: my-company)
🟢 ✓ Units imported
🟢 ✓ Workers configured
🟢 ✓ Targets configured

🎉 ALL SET!
───────────
  Your saved queries will sync to ConfigHub.

  Open ConfigHub: https://confighub.com

  Or view fleet status: cub-agent map fleet
```

See [IMPORTING-WORKLOADS.md](IMPORTING-WORKLOADS.md) for the full import guide.

## Demo

Run the interactive demo:

```bash
./examples/demos/tui-queries-demo.sh
```

This shows the queries TUI with colored output and explains each feature.

## See Also

- [CLI-REFERENCE.md](CLI-REFERENCE.md) — Full CLI reference
- [IMPORTING-WORKLOADS.md](IMPORTING-WORKLOADS.md) — Import workloads into ConfigHub
- [examples/demos/](../examples/demos/) — Interactive demos
