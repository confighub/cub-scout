# Map User Journey to Full ConfigHub

The path from read-only discovery to the full ConfigHub platform.

---

## Try It Now

```bash
# 30 seconds to value:
./cub-agent map                                    # See what's running
./cub-agent import --namespace my-app --dry-run    # See proposed structure
```

No account required. See your cluster organized by ownership and app structure.

> **Full import guide:** [import-to-confighub.md](https://github.com/confighubai/confighub-agent/search?q=import-to-confighub.md)
> **View tiers:** [VIEW-TIERS.md](../VIEW-TIERS.md) — OSS/Connected/Fleet with mockups

---

## The Journey Overview

| Stage | What You Run | What You Get | Commitment |
|-------|--------------|--------------|------------|
| **1. Standalone** | `cub-agent map` | See what's running, who owns it | None |
| **2. Discovery** | `cub-agent import --dry-run` | Proposed Hub/App Space structure | None |
| **3. Connected** | `cub-agent import` | Live units/apps in ConfigHub | Low |
| **4. Worker** | `cub worker run` | Can sync changes to targets | Medium |
| **5. Full Platform** | ConfigHub UI/CLI | Actions, queries, changesets | Higher |

Each stage adds value. You can stop at any stage.

---

## Stage 1: Standalone (Read-Only)

**Command:**
```bash
cub-agent map
```

**What happens:**
- Agent watches your cluster (read-only)
- Detects ownership (Flux, Argo, Helm, Native)
- Shows sync status (Synced, Drifted, Failed)
- No account required

**What you see:**
```
CLUSTER     NAMESPACE    KIND          NAME           OWNER     STATUS
prod-east   payments     Deployment    payment-api    Flux      Synced
prod-east   payments     Deployment    payment-worker Flux      Drifted
staging     default      Deployment    debug-pod      Native    Unknown
```

**Value:** Immediate visibility. "Native" reveals unmanaged resources.

**Exit cost:** Delete the agent.

---

## Stage 2: Discovery

**Command:**
```bash
cub-agent import --namespace payments-prod --model hub-appspace --dry-run
```

**What happens:**
- Agent analyzes discovered workloads
- Infers app/variant structure from namespace patterns and labels
- Proposes Hub/App Space organization
- Nothing created yet (dry-run)

**What you see:**
```
ConfigHub Import (Hub/App Space Model)
======================================

Discovered 3 workloads in namespace: payments-prod

Suggested structure:
  App Space: payments-team

    app=payment-api
      Unit: payment-api-prod (variant=prod)

    app=payment-worker
      Unit: payment-worker-prod (variant=prod)

    app=redis
      Unit: redis-prod (variant=prod)

Would create 3 unit(s) in space 'payments-team'
```

**Value:** See how your cluster maps to Hub/App Space model.

**Exit cost:** None. This is just a proposal.

---

## Stage 3: Connected (Import)

**Commands:**
```bash
# Authenticate
cub auth login

# Import for real
cub-agent import --namespace payments-prod --model hub-appspace --space payments-team
```

**What happens:**
- Units created in ConfigHub
- Labels assigned (app, variant)
- Workloads linked to Units
- Map now shows ConfigHub hierarchy

**What you see:**
```bash
cub-agent map fleet --space payments-team
```

```
ConfigHub Fleet View (Hub/App Space Model)
Hierarchy: Application -> Variant -> Target

  payment-api
  └── variant: prod
      └── payments-team @ rev 1

  payment-worker
  └── variant: prod
      └── payments-team @ rev 1
```

**Value:**
- App hierarchy (not just cluster/namespace/resource)
- Fleet queries across clusters
- Shared picture for the team

**Exit cost:** Low. Export Units as YAML anytime.

> **See also:** [import-to-confighub.md](https://github.com/confighubai/confighub-agent/search?q=import-to-confighub.md) — Full import guide

---

## Stage 4: Worker Connected

**Command:**
```bash
cub worker run --target prod-east
```

**What happens:**
- Worker connects cluster as a Target
- ConfigHub can deploy to this Target
- Source changes and propagate to Target thereby
- Drift detection compares Units to live state
- Reconciliation rules take effect

**Architecture:**
```
Hub
├── Sources (Git repos)
├── Targets (clusters)
├── Workers (execution agents)
│
└── App Spaces
    └── payments-team
        └── Units (deployed via Workers)
```

**Value:**
- Sources sync to targets automatically
- Drift detection and reconciliation
- Hub policies enforced

**Exit cost:** Medium. Would need to re-setup sync.

---

## Stage 5: Full Platform

**What you get:**

| Feature | Description |
|---------|-------------|
| **Saved Queries** | Define views, share with team |
| **Actions** | Automation workflows with label filters |
| **ChangeSets** | Grouped changes with approval |
| **Drift Operations** | Accept/revert with one command |
| **Fleet Queries** | Query across all clusters |
| **Sync-back** | Changes flow back to Git |

**Example operations:**
```bash
# Run a saved query
cub query @prod-drifted

# Accept drift (cluster change → Git PR)
cub drift accept payment-api --variant prod

# Bulk update across variants
cub mutate --query "Labels['app'] = 'redis'" \
  --set spec.template.spec.containers[0].image=redis:7.2

# Promote staging to prod
cub promote \
  --query "Labels['variant'] = 'staging'" \
  --to-variant prod
```

**Value:** Full operational control with audit trail.

**Exit cost:** Higher. Would need to re-implement automation.

---

## Adoption Ladder

| Phase | Commitment | Time to Value | Exit Cost |
|-------|------------|---------------|-----------|
| **1. Map** | None | 30 seconds | Delete agent |
| **2. Discovery** | None | 1 minute | Nothing to undo |
| **3. Import** | Low | 5 minutes | Export as YAML |
| **4. Worker** | Medium | 1 hour | Re-setup sync |
| **5. Full** | Higher | Days-weeks | Re-implement |

**Most users get massive value at stages 1-3 with near-zero lock-in.**

---

## Commands by Stage

### Stage 1: Standalone
```bash
cub-agent map                              # TUI dashboard
cub-agent map list                         # Plain text
cub-agent map list -q "owner=Native"       # Find orphans
cub-agent scan                             # risk detection
```

### Stage 2: Discovery
```bash
cub-agent import --dry-run                 # Default model
cub-agent import --model hub-appspace --dry-run  # Hub/App Space model
```

### Stage 3: Connected
```bash
cub auth login
cub-agent import --space my-team
cub-agent map fleet --space my-team
```

### Stage 4: Worker
```bash
cub worker run --target prod-east
cub target list
```

### Stage 5: Full
```bash
cub query "Labels['variant'] = 'prod'"
cub drift accept <unit>
cub changeset create --intent "Update redis"
```

---

## What Connects at Each Stage

| Stage | Agent | ConfigHub | Worker | Sources | Targets |
|-------|-------|-----------|--------|---------|---------|
| **1. Standalone** | Running | — | — | — | — |
| **2. Discovery** | Running | — | — | — | — |
| **3. Import** | Running | Connected | — | — | — |
| **4. Worker** | Running | Connected | Running | — | Connected |
| **5. Full** | Running | Connected | Running | Registered | Connected |

---

## See Also

- [01-MAP-CONCEPT.md](01-MAP-CONCEPT.md) — The Map explained
- [02-HUB-APPSPACE-MODEL.md](02-HUB-APPSPACE-MODEL.md) — Hub, App Space, Unit definitions
- [07-MODEL-MIGRATION.md](07-MODEL-MIGRATION.md) — Migrating from current spaces
- [09-EXTENSION-POINTS.md](09-EXTENSION-POINTS.md) — Future capabilities at each stage

---

**Next:** [05-THREE-SOURCES-OF-TRUTH.md](05-THREE-SOURCES-OF-TRUTH.md) — Git, ConfigHub, Cluster explained
