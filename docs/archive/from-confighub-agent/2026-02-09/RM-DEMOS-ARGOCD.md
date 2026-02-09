# Rendered Manifest Pattern Demos — ArgoCD Edition

> **The goal isn't to show ConfigHub features. It's to make people feel: "I need this NOW."**

These demos are designed to create visceral "holy shit" moments — not feature walkthroughs.

---

## The Setup: Your Repo Structure Doesn't Matter

People have wildly different setups. That's fine. ConfigHub adapts to all of them:

| Your Setup | ConfigHub Sees |
|------------|----------------|
| Monorepo with 50 folders | 50 Units in one App Space |
| Multi-repo, one per team | Multiple App Spaces |
| Helm umbrella charts | Units with Helm values |
| Kustomize overlays | Units with overlay paths |
| App-of-Apps pattern | Generator Units → Instance Units |
| ApplicationSets | Template Units → Instance Units |
| Raw YAML + kubectl | Native-owned resources (detected) |
| **Mixed (all of the above)** | **All visible in one hierarchy** |

**The point:** Whatever your structure, you get fleet-level control.

---

## Demo 1: "The Monday Morning Panic" (~3 min)

### The Setup

> **8:47 AM Monday. PagerDuty fires. "Payment API errors spiking."**
>
> You have 47 clusters. ArgoCD shows green everywhere.
> Where do you even start?

### The Old Way (What Everyone Does Now)

```bash
# Check ArgoCD UI... cluster 1... looks fine
# Check ArgoCD UI... cluster 2... looks fine
# Check ArgoCD UI... cluster 3...
# ...
# (47 clusters later, 45 minutes wasted)
# Finally find: prod-eu-west-2 has wrong image tag
```

### The ConfigHub Way (30 Seconds)

```bash
# One command. All 47 clusters. Find the problem.
cub unit list --where "app=payment-api"
```

```
UNIT         CLUSTER          VERSION   PODS   STATUS
───────────────────────────────────────────────────────────────
payment-api  prod-us-east-1   v2.3.1    5/5    ✓ Synced
payment-api  prod-us-west-2   v2.3.1    5/5    ✓ Synced
payment-api  prod-eu-west-1   v2.3.1    5/5    ✓ Synced
payment-api  prod-eu-west-2   v2.3.0    3/5    ⚠ BEHIND    ← FOUND IT
payment-api  prod-ap-south-1  v2.3.1    5/5    ✓ Synced
... (42 more, all v2.3.1)

Summary: 46 current, 1 behind (prod-eu-west-2 @ v2.3.0)
```

```bash
# Why is it behind?
cub unit history payment-api --cluster prod-eu-west-2

# Shows: Last sync failed 3 hours ago
# Reason: OCI pull timeout (registry issue in eu-west-2)
# ArgoCD status: "Synced" (it synced the OLD version successfully)
```

**The "aha" moment:** ArgoCD said "Synced" because it synced *something*. It didn't know it was behind. ConfigHub knows.

### Key Visual

```
┌─────────────────────────────────────────────────────────────────┐
│  ArgoCD View (per cluster)     ConfigHub View (fleet)          │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  [cluster-1] ✓ Synced          47 clusters, 1 view:            │
│  [cluster-2] ✓ Synced                                          │
│  [cluster-3] ✓ Synced            payment-api                   │
│  ...                              ├── 46 × v2.3.1 ✓            │
│  [cluster-47] ✓ Synced            └── 1 × v2.3.0 ⚠ BEHIND     │
│                                                                 │
│  "Everything looks fine"        "prod-eu-west-2 is behind"     │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## Demo 2: "The 2AM kubectl" (~4 min)

### The Setup

> **Tuesday morning standup. "Why did prod-us-east have 8 replicas overnight?"**
>
> Someone scaled it manually. GitOps said "Synced."
> But who? When? Why? And is it still like that?

This is the dirty secret of GitOps: **kubectl still works.** People use it. At 2am. During incidents. Without PRs.

### The Investigation

```bash
# What's the current state? (ArgoCD says "Synced")
kubectl get deploy payment-api -n payments -o jsonpath='{.spec.replicas}'
# Output: 8

# But Git says...
cat overlays/prod-us-east/replicas-patch.yaml
# Output: replicas: 5

# ArgoCD says...
argocd app get payment-api-prod-us-east
# Status: Synced ✓
# Health: Healthy ✓
```

**Wait, what?** Git says 5, cluster has 8, ArgoCD says "Synced"?

(This happens because ArgoCD synced successfully *at some point*. Then someone kubectl scaled. ArgoCD doesn't re-check unless triggered.)

### The ConfigHub Way

```bash
cub unit diff payment-api --cluster prod-us-east
```

```
DRIFT DETECTED: payment-api @ prod-us-east

┌──────────────────────────────────────────────────────────────────┐
│  ConfigHub (Desired)              Cluster (Live)                 │
├──────────────────────────────────────────────────────────────────┤
│  spec.replicas: 5                 spec.replicas: 8               │
│                                                                  │
│  DRIFT SOURCE                                                    │
│  ─────────────                                                   │
│  Changed: 2026-01-12 02:47:23 UTC                               │
│  By: kubectl (user: oncall-sarah@acme.com)                      │
│  Context: Incident INC-4521 (payment latency spike)             │
│                                                                  │
│  Annotation found on resource:                                   │
│    kubectl.kubernetes.io/last-applied-configuration             │
│    kubernetes.io/change-cause: "emergency scale for INC-4521"   │
└──────────────────────────────────────────────────────────────────┘

OPTIONS:
  cub unit revert payment-api --cluster prod-us-east    # Force back to 5
  cub unit accept payment-api --cluster prod-us-east    # Accept 8 as new desired
  cub unit ignore payment-api --cluster prod-us-east    # Mark as expected drift
```

```bash
# Check: was this a one-off or did it happen elsewhere?
cub unit list --where "drift=true"
```

```
DRIFTED UNITS (3)
───────────────────────────────────────────────────────────────────
payment-api     prod-us-east   replicas: 5→8    02:47 UTC  INC-4521
redis-cache     prod-us-east   replicas: 3→6    02:52 UTC  INC-4521
order-api       prod-us-east   replicas: 3→5    02:55 UTC  INC-4521

All 3 drifts are from the same incident.
Remediation: cub unit revert --where "drift.cause=INC-4521"
```

**The "aha" moments:**
1. ConfigHub caught drift that ArgoCD missed
2. It shows WHO made the change and WHY
3. It finds ALL related drift across the fleet
4. You can bulk revert or bulk accept

### The Remediation (Make Changes THROUGH ConfigHub)

```bash
# Option A: Revert all incident-related drift
cub unit revert --where "drift.cause=INC-4521"

# Option B: Accept the new values (incident response was correct)
cub unit accept --where "drift.cause=INC-4521" \
  --reason "INC-4521 scale-up should be permanent, updating desired state"
# This updates ConfigHub's desired state AND creates a PR to sync back to Git
```

```
Accepting drift for 3 units...

Creating ChangeSet CS-892:
  payment-api (prod-us-east): replicas 5→8
  redis-cache (prod-us-east): replicas 3→6
  order-api (prod-us-east): replicas 3→5

Syncing back to Git...
  Created PR #1247: "Accept INC-4521 scale changes"
  URL: https://github.com/acme/configs/pull/1247

Desired state updated. Drift resolved.
```

**The key insight:** ConfigHub doesn't just *detect* drift — it lets you *resolve* it properly, with audit trail and Git sync-back.

---

## Demo 3: "The Critical Security Patch" (~4 min)

### The Setup

> **Friday 4pm. Slack explodes. "CVE-2026-1234 — critical vulnerability in base image."**
>
> You have 847 microservices across 47 clusters.
> How long to patch everything?

### The Old Way (Pain Everyone Knows)

```bash
# For each of 847 services:
#   1. Find the repo
#   2. Update the base image
#   3. Create PR
#   4. Wait for CI
#   5. Get approval
#   6. Merge
#   7. Wait for ArgoCD to sync
#   8. Verify deployment
#
# Estimated time: 847 PRs × 15 min each = 212 hours
# Reality: "We'll do it next sprint"
```

### The ConfigHub Way (15 Minutes)

```bash
# Step 1: How bad is it? (30 seconds)
cub unit list --where "image.base=alpine:3.18*"
```

```
AFFECTED UNITS: 847

By Team:
  payments-team:     127 units
  orders-team:       89 units
  inventory-team:    234 units
  platform-team:     397 units

By Environment:
  production:        312 units (47 clusters)
  staging:           285 units (12 clusters)
  development:       250 units (3 clusters)

Oldest image: alpine:3.18.0 (deployed 2025-09-14)
Newest image: alpine:3.18.4 (deployed 2026-01-10)
```

```bash
# Step 2: What's the fix? (Preview without applying)
cub unit update \
  --where "image.base=alpine:3.18*" \
  --set image.base=alpine:3.19.1 \
  --dry-run
```

```
DRY RUN: Would update 847 units

Changes by team (requires their approval):
  payments-team:     127 units → ChangeSet for @payments-leads
  orders-team:       89 units  → ChangeSet for @orders-leads
  inventory-team:    234 units → ChangeSet for @inventory-leads
  platform-team:     397 units → ChangeSet for @platform-leads

Rollout strategy (based on policies):
  Phase 1: development (250 units) — auto-approve
  Phase 2: staging (285 units) — auto-approve after dev healthy
  Phase 3: production (312 units) — requires manual approval

Estimated time: 2-4 hours (phased rollout)
```

```bash
# Step 3: Do it. (One command)
cub unit update \
  --where "image.base=alpine:3.18*" \
  --set image.base=alpine:3.19.1 \
  --reason "CVE-2026-1234 critical security patch"
```

```
Creating ChangeSets...

CS-901: payments-team (127 units)     → Pending approval from @payments-leads
CS-902: orders-team (89 units)        → Pending approval from @orders-leads
CS-903: inventory-team (234 units)    → Pending approval from @inventory-leads
CS-904: platform-team (397 units)     → Pending approval from @platform-leads

Development phase auto-approved (250 units rendering...)
  ████████████████████████████████████████ 100%
  Pushing to OCI registries... done
  ArgoCD syncing... 250/250 synced

Staging phase starting in 10 minutes (waiting for dev health checks)...

Track progress: cub changeset status --where "reason~=CVE-2026-1234"
```

```bash
# Step 4: Watch the rollout
cub rollout status --where "reason~=CVE-2026-1234" --watch
```

```
CVE-2026-1234 PATCH ROLLOUT

Phase 1: Development ████████████████████ 250/250 ✓ Complete
Phase 2: Staging     ████████████░░░░░░░░ 187/285   Progressing
Phase 3: Production  ░░░░░░░░░░░░░░░░░░░░   0/312   Waiting for approval

Approvals:
  ✓ @payments-leads approved CS-901 (127 units)
  ✓ @orders-leads approved CS-902 (89 units)
  ⏳ @inventory-leads pending CS-903 (234 units)
  ⏳ @platform-leads pending CS-904 (397 units)

ETA to full rollout: 1h 45m (after approvals)
```

**The "aha" moments:**
1. **847 services patched with ONE command** (not 847 PRs)
2. **Respects team ownership** — each team approves their own units
3. **Phased rollout built-in** — dev → staging → prod with health gates
4. **Full audit trail** — every change tied to CVE-2026-1234
5. **ArgoCD does the actual deployment** — ConfigHub orchestrates

### The Key Visual

```
┌─────────────────────────────────────────────────────────────────────┐
│  Traditional GitOps              ConfigHub RM Pattern               │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  847 repos × PR workflow         1 command                          │
│  ─────────────────────           ─────────                          │
│                                                                     │
│  Week 1: 200 PRs created         Minute 1: Preview impact           │
│  Week 2: 400 PRs merged          Minute 2: Create ChangeSets        │
│  Week 3: Still 247 pending       Minute 15: Dev complete            │
│  Week 4: "Can we close these?"   Hour 2: Staging complete           │
│                                  Hour 4: Production complete        │
│                                                                     │
│  Audit: "Check 847 PR threads"   Audit: cub audit CVE-2026-1234    │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Your Repo Structure? Doesn't Matter.

These demos work regardless of how you've organized things:

### If You Have: Monorepo with Folders

```
your-configs/
├── apps/
│   ├── payment-api/
│   │   ├── base/
│   │   └── overlays/{dev,staging,prod}/
│   ├── order-api/
│   └── ... (200 more)
└── argocd/
    └── apps.yaml  # App-of-apps or ApplicationSet
```

**ConfigHub sees:** 200 Units in one App Space, one Git source

### If You Have: Multi-Repo per Team

```
payments-team/configs/     → App Space: payments
orders-team/configs/       → App Space: orders
platform-team/configs/     → App Space: platform
```

**ConfigHub sees:** 3 App Spaces, fleet queries work across all

### If You Have: Helm Umbrella Charts

```
platform-charts/
├── Chart.yaml
├── charts/
│   ├── redis/
│   ├── postgres/
│   └── kafka/
└── values-{dev,staging,prod}.yaml
```

**ConfigHub sees:** Units with Helm values, can update values across environments

### If You Have: ApplicationSets

```yaml
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: payment-api
spec:
  generators:
    - clusters: {}  # Deploy to all clusters
```

**ConfigHub sees:** Generator Unit → Instance Units, tracks what's generated where

### If You Have: Complete Chaos (Mixed Everything)

**ConfigHub sees:** All of it. Unified hierarchy. Query anything.

---

## File Layout: The Demo Kit

```
examples/rm-demos-argocd/
├── README.md                     # Demo guide (this doc)
│
├── scenarios/
│   ├── monday-panic/             # Demo 1: Find the problem
│   │   ├── setup.sh              # Create 47 mock clusters
│   │   ├── break-one.sh          # Introduce the version lag
│   │   └── demo.sh               # Run the demo
│   │
│   ├── 2am-kubectl/              # Demo 2: Drift detection
│   │   ├── setup.sh
│   │   ├── cause-drift.sh        # Simulate kubectl edits
│   │   └── demo.sh
│   │
│   └── security-patch/           # Demo 3: Fleet-wide update
│       ├── setup.sh
│       ├── generate-services.sh  # Create 847 mock services
│       └── demo.sh
│
├── repo-patterns/                # "Your structure" examples
│   ├── monorepo/
│   ├── multi-repo/
│   ├── helm-umbrella/
│   ├── applicationsets/
│   └── mixed-chaos/
│
└── confighub/                    # ConfigHub config for demos
    ├── hub.yaml
    └── spaces/
        ├── payments-team.yaml
        ├── orders-team.yaml
        └── platform-team.yaml
```

---

## Summary: The 3 Demos

| Demo | Duration | Pain Point | "Aha" Moment |
|------|----------|------------|--------------|
| **Monday Panic** | 3 min | 47 clusters, where's the problem? | "Found it in 30 seconds, not 45 minutes" |
| **2AM kubectl** | 4 min | Someone edited prod directly | "Full audit trail + bulk remediate" |
| **Security Patch** | 4 min | CVE affects 847 services | "One command, not 847 PRs" |

### The Visceral Reactions We Want

| Demo | Audience Says |
|------|--------------|
| **Monday Panic** | "I literally did this last week. It took me 2 hours." |
| **2AM kubectl** | "Wait, ArgoCD says Synced but it's actually drifted? That's terrifying." |
| **Security Patch** | "847 PRs would take us a month. This is... 15 minutes?" |

### Why ArgoCD Users Should Care

ConfigHub doesn't replace ArgoCD — it **completes** it.

| ArgoCD Does | ConfigHub Adds |
|-------------|----------------|
| Sync from source | Fleet-wide visibility |
| Deploy to cluster | Cross-cluster queries |
| Show app health | Drift detection (beyond sync) |
| Per-cluster UI | Unified control plane |
| Manual promotion | One-command rollouts |
| Git as source | Git + OCI + WET config |

**The pitch:** "Keep ArgoCD. Add ConfigHub. See everything. Control everything."

---

## Single-Cluster-First Principle

**Core insight:** If these demos work with one cluster, they work with N clusters.

Before running fleet demos, verify with single cluster:

```bash
# 1. Map the cluster
./cub-agent map

# 2. Detect ownership
./cub-agent map -q "owner=ArgoCD"

# 3. Import to ConfigHub
./cub-agent import -n <namespace>

# 4. Verify hierarchy
cub unit list --space <space>

# 5. Make a change
cub unit update <unit> --set image.tag=v2.0.0

# 6. Verify propagation
kubectl get deployment <name> -o jsonpath='{.spec.template.spec.containers[0].image}'
```

**If all 6 steps work on one cluster, the demo will work at scale.**

See [REPO-SKELETON-TAXONOMY.md](REPO-SKELETON-TAXONOMY.md) for the full taxonomy of repo structures and verification checklist.

---

## References

- [REPO-SKELETON-TAXONOMY.md](REPO-SKELETON-TAXONOMY.md) — Skeleton classification & single-cluster-first
- [IMPORT-GIT-REFERENCE-ARCHITECTURES.md](../IMPORT-GIT-REFERENCE-ARCHITECTURES.md) — Pattern → ConfigHub mapping
- [kostis-argocd-best-practices.md](reference/kostis-argocd-best-practices.md) — ArgoCD best practices (Kostis/Codefresh)
- [examples/rm-demos-argocd/](../../examples/rm-demos-argocd/) — Runnable demo kit

---

## Next Steps

1. [x] Create demo fixtures in `examples/rm-demos-argocd/`
2. [x] Write runnable demo scripts
3. [ ] Test with real Kind clusters + ArgoCD
4. [ ] Record video walkthroughs
5. [ ] Add to DEMO-SCRIPTS-TOP-10.md
