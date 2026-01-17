# Journey: Import Workloads

**Time:** 5-10 minutes
**Goal:** Import your cluster workloads into ConfigHub as Units

**Prerequisites:** Complete [JOURNEY-FIRST-SETUP.md](JOURNEY-FIRST-SETUP.md) first.

---

## The Import Flow

```
1. Scan cluster       → See what's running
2. Review proposal    → What will be created
3. Confirm            → Create Units in ConfigHub
4. Start worker       → Connect cluster to ConfigHub
5. Verify             → See hierarchy in GUI
```

---

## Step 1: Preview What You Have

First, see what's in your cluster:

```bash
./cub-scout map
```

Note the namespaces with workloads. Pick one to import.

---

## Step 2: Dry Run

Preview what import will create:

```bash
./cub-scout import -n my-namespace --dry-run
```

**Expected output:**

```
┌─ DISCOVERED ───────────────────────────────────────────────────┐
│                                                                │
│  my-namespace (5 workloads)                                    │
│    • api-server (Deployment)                  [Flux]           │
│    • worker (Deployment)                      [Flux]           │
│    • redis (StatefulSet)                      [Helm]           │
│    • postgres (StatefulSet)                   [Helm]           │
│    • cronjob-cleanup (CronJob)                [Native]         │
│                                                                │
└────────────────────────────────────────────────────────────────┘

┌─ WILL CREATE ──────────────────────────────────────────────────┐
│                                                                │
│  App Space: my-namespace-team                                  │
│                                                                │
│  Units:                                                        │
│    • api-server                                                │
│      labels: app=api-server, variant=default, owner=Flux       │
│                                                                │
│    • worker                                                    │
│      labels: app=worker, variant=default, owner=Flux           │
│                                                                │
│    • redis                                                     │
│      labels: app=redis, variant=default, owner=Helm            │
│                                                                │
│    • postgres                                                  │
│      labels: app=postgres, variant=default, owner=Helm         │
│                                                                │
│    • cronjob-cleanup                                           │
│      labels: app=cronjob-cleanup, variant=default, owner=Native│
│                                                                │
└────────────────────────────────────────────────────────────────┘

Dry run: no changes made
```

**What the labels mean:**
- `app` — Application name (from k8s labels or workload name)
- `variant` — Environment variant (inferred from namespace suffix)
- `owner` — GitOps tool managing this resource

---

## Step 3: Run Import

When you're happy with the preview:

```bash
./cub-scout import -n my-namespace
```

**Expected output:**

```
┌─ IMPORTING ────────────────────────────────────────────────────┐
│                                                                │
│  Creating App Space: my-namespace-team                         │
│  Creating Unit: api-server                            ✓        │
│  Creating Unit: worker                                ✓        │
│  Creating Unit: redis                                 ✓        │
│  Creating Unit: postgres                              ✓        │
│  Creating Unit: cronjob-cleanup                       ✓        │
│                                                                │
└────────────────────────────────────────────────────────────────┘

✓ Imported 5 units to App Space: my-namespace-team

Log: ~/.confighub/logs/import-2026-01-09-150322.log
```

---

## Step 4: Verify in ConfigHub

List your new Units:

```bash
cub unit list --space my-namespace-team
```

**Expected output:**

```
SLUG             STATUS    TARGET              SYNC
api-server       Ready     (no target)         -
worker           Ready     (no target)         -
redis            Ready     (no target)         -
postgres         Ready     (no target)         -
cronjob-cleanup  Ready     (no target)         -
```

Units are created but not yet connected to a cluster. Next: start a worker.

---

## Step 5: Start Worker

The worker connects ConfigHub to your cluster:

```bash
cub context set space my-namespace-team
cub worker run dev
```

**Expected output:**

```
Starting worker: dev
Connecting to ConfigHub...
✓ Connected

Registering targets...
✓ Target: kind-my-cluster registered

Worker running. Press Ctrl+C to stop.
```

Leave this running in a terminal.

---

## Step 6: Verify Connection

In another terminal, check the connection:

```bash
./test/atk/map-import --space my-namespace-team
```

**Expected output (fully connected):**

```
┌─ CONFIGHUB ────────────────────────────────────────────────────┐
│                                                                │
│  Org: your-org                                                 │
│  └─ Platform Hub: you@example.com                              │
│     └─ AppSpace: my-namespace-team                             │
│        ├─ Unit: api-server ✓                                   │
│        │  ├─ Status: Ready                                     │
│        │  └─ Target: kind-my-cluster                           │
│        ├─ Unit: worker ✓                                       │
│        └─ Unit: redis ✓                                        │
│                                                                │
└────────────────────────────────────────────────────────────────┘

┌─ CONNECTION ───────────────────────────────────────────────────┐
│                                                                │
│  ✓ Hub ──▶ dev ──▶ kind-my-cluster                            │
│                                                                │
│  ✓ Authenticated                                               │
│  ✓ 5 Units imported                                            │
│  ✓ Worker: dev (connected)                                     │
│  ✓ 1 Target registered                                         │
│                                                                │
│  ALL SET                                                       │
│                                                                │
└────────────────────────────────────────────────────────────────┘
```

---

## Import Multiple Namespaces

Import several namespaces at once:

```bash
./cub-scout import -n payments-prod,payments-staging,orders-prod
```

Or import all namespaces:

```bash
./cub-scout import
```

---

## Customize the Import

### Change App Space Name

```bash
./cub-scout import -n my-namespace --app-space custom-team-name
```

### Override Labels

```bash
./cub-scout import -n my-namespace --labels "team=platform,env=prod"
```

### Skip Certain Workloads

```bash
./cub-scout import -n my-namespace --exclude "cronjob-*"
```

---

## Resume Interrupted Import

Use the interactive wizard for a guided experience:

```bash
./cub-scout import --wizard    # Interactive TUI wizard (recommended)
./cub-scout import --dry-run   # Preview without making changes
```

When connected to ConfigHub, import activity is logged for audit trail.

---

## What Happens After Import

| Before Import | After Import |
|---------------|--------------|
| Resources in kubectl | Same resources + Units in ConfigHub |
| No app grouping | Hierarchy: Org → Hub → App Space → Unit |
| No cross-cluster view | Query across all imported clusters |
| Local visibility only | Team shares same picture |

---

## Next Steps

| Journey | What You'll Learn |
|---------|-------------------|
| [**JOURNEY-MAP.md**](JOURNEY-MAP.md) | Navigate the map, trace ownership |
| [**JOURNEY-SCAN.md**](JOURNEY-SCAN.md) | Find configuration issues |
| [**JOURNEY-QUERY.md**](JOURNEY-QUERY.md) | Query across fleet |

---

## Troubleshooting

### "No workloads found"

Check the namespace has Deployments/StatefulSets/DaemonSets:
```bash
kubectl get deploy,sts,ds -n my-namespace
```

### "Unit already exists"

Unit was previously imported. To re-import:
```bash
./cub-scout import -n my-namespace --force
```

### "Worker not connected"

Check worker is running:
```bash
cub worker list --space my-namespace-team
```

Start worker if needed:
```bash
cub worker run dev --space my-namespace-team
```

### Import failed mid-way

Check the log:
```bash
cat ~/.confighub/logs/import-*.log | tail -50
```

Interactive wizard (recommended):
```bash
./cub-scout import --wizard
```

---

**Previous:** [JOURNEY-FIRST-SETUP.md](JOURNEY-FIRST-SETUP.md) — Install and connect | **Next:** [JOURNEY-MAP.md](JOURNEY-MAP.md) — Navigate the map TUI

---

## See Also

- [IMPORTING-WORKLOADS.md](IMPORTING-WORKLOADS.md) — Full import reference
- [GLOSSARY-OF-CONCEPTS.md](GLOSSARY-OF-CONCEPTS.md) — Unit, App Space, Hub definitions
- [TUI-SESSION-STATE.md](TUI-SESSION-STATE.md) — Session persistence design
