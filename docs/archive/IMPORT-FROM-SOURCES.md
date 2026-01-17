# Import Architecture: TUI vs GUI

**Architecture** — What the TUI handles (single cluster, LIVE data) vs what the GUI handles (fleet, Git integration).

---

## The Flow: TUI → ConfigHub → GUI

```
TUI (single cluster)
    │
    │  import + suggest → create Units
    │
    ▼
ConfigHub (connected)
    │
    │  Hub owns Workers → connect to Targets
    │  App Spaces select which Worker for deploy
    │
    ▼
GUI (fleet view)
    │
    ├─ Run import with suggestions on each cluster via workers
    ├─ Aggregate results
    ├─ Adjust names/labels (not locked in)
    └─ Enhance from Git
```

**TUI is complete.** It handles single-cluster import from LIVE data.

**GUI builds on TUI.** It runs the same import/suggest logic on each cluster via workers, then adds Git intelligence.

**Key insight:** Once connected, names and labels can be adjusted. The initial import suggests structure, but you're not locked in.

---

## The Mapping Rule

| What in Git/Cluster | Maps To in ConfigHub |
|---------------------|----------------------|
| Git repo URL | **Source** (with pattern metadata) |
| `base/` folders | **Base Unit** in Hub Catalog |
| Each deployed Application/Kustomization | **Unit** in App Space |
| Overlays/variants | **Labels** on Unit (`variant=prod`) |
| Tenant folders | **App Space** per tenant |
| Orchestration parents (App-of-Apps root) | **Nothing** — deployer mechanism, not config |

**Key insight:** Deployer orchestration (App-of-Apps parent, ApplicationSet generator) is NOT modeled as Units. ConfigHub has its own orchestration via Workers → Targets.

---

## What GUI Adds

### 1. Fleet Aggregation

TUI sees one cluster. GUI sees all.

```
Cluster: k8s-east    → payment-api (variant=prod)
Cluster: k8s-west    → payment-api (variant=prod)
Cluster: k8s-staging → payment-api (variant=staging)
```

**How:** Run import on each cluster via worker. Aggregate results.

### 2. Cross-Cluster Matching

Same app deployed to multiple clusters. GUI correlates them.

```
App Space: checkout-team
  Unit: payment-api
    ├─ k8s-staging (variant=staging)
    ├─ k8s-east (variant=prod)
    └─ k8s-west (variant=prod)
```

**How:** Match by app label + namespace pattern across worker results.

### 3. Git Enhancement

LIVE shows what IS. Git shows what SHOULD BE.

| From LIVE (via workers) | From Git |
|------------------------|----------|
| Deployed variants | All variants (including undeployed) |
| Current state | Pending changes |
| Who manages it | Who changed it (audit trail) |

### 4. Base Template Detection

Shared configs in `apps/base/` are templates, never deployed directly.

```
apps/
  base/
    podinfo/       → Base Unit in Hub Catalog (NOT deployed)
  staging/
    podinfo/       → Unit [variant=staging] (references base)
  production/
    podinfo/       → Unit [variant=prod] (references base)
```

| Source | What It Sees |
|--------|--------------|
| **LIVE** | Can't see `base/` — it's not deployed |
| **Git** | Parse repo, identify bases, link overlays to bases |

---

## Implementation: Who Does What

| Capability | TUI | Worker | GUI |
|------------|-----|--------|-----|
| Scan cluster | ✅ | ✅ (same code) | — |
| Detect ownership | ✅ | ✅ | — |
| Infer variant | ✅ | ✅ | — |
| Suggest structure | ✅ | ✅ | — |
| Create Units | ✅ | — | ✅ |
| Aggregate fleet | — | — | ✅ |
| Parse Git | — | — | ✅ |
| Match cross-cluster | — | — | ✅ |

**Key insight:** Worker runs the same import/suggest logic as TUI. GUI orchestrates and enhances.

---

## GUI Backend Requirements

### 1. Worker Import Endpoint

Workers need to expose import results:

```
Worker receives: "run import on namespace X"
Worker returns: suggested structure (App Spaces, Units, labels)
```

This is the same logic as `cub-agent import --dry-run --json`.

### 2. Fleet Aggregation API

Collect import results from all workers, merge into unified view.

### 3. Git Provider Integration

For enhancement layer:
- Connect to GitHub/GitLab
- Parse repo structure
- Identify base templates and all variants
- Track pending changes (Git HEAD vs deployed revision)

---

## Summary

| Layer | Source | What It Shows |
|-------|--------|---------------|
| TUI | LIVE (1 cluster) | What's deployed here |
| Workers | LIVE (N clusters) | What's deployed everywhere |
| Git | Source | What should exist + history |

**TUI is the building block. GUI scales it across fleet and adds Git.**
