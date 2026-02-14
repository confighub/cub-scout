# How the New Model Addresses These Problems

> **Archive status:** Historical planning context (non-canonical for current releases).
> For current `cub-scout` behavior and command contracts, use:
> - `docs/roadmap.md`
> - `docs/reference/commands.md`
> - `docs/reference/import-docs-crosswalk.md`
>
> Scope note: examples here that use mutating `cub` workflows (`drift accept`, `mutate`, `promote`, `changeset`, worker lifecycle) describe ConfigHub platform behavior, not direct `cub-scout` command commitments.

**Purpose:** Maps real user pain points to solutions in the Hub/App Space model. Validates that the model solves actual problems.

**Status:** Current
**Last Updated:** 2026-01-06

**Key correction:** Three-level hierarchy (Hub → App Space → Unit). App and Variant are labels on Units, not container objects.

> **Canonical reference:** See [HUB-APPSPACE-MODEL.md](HUB-APPSPACE-MODEL.md) for the complete model. CLI examples in this document are **proposed**.

---

## The Mental Model

> **Git says WHAT. ConfigHub says HOW. Cluster says NOW.**

| Layer | Role | Source of Truth For |
|-------|------|---------------------|
| **Git** | WHAT | Intent — what the author wants deployed |
| **ConfigHub** | HOW | Operations — policies, deployer choice, reconciliation, who wins |
| **Cluster** | NOW | Rapid response — break glass, hotfixes, proposals that flow back |

---

## The Problems (from Jesper #3336, Philipp, and internal feedback)

### Jesper's Dogfooding Pain Points

| # | Problem | Quote |
|---|---------|-------|
| 1 | **Deployment is infuriating** | "You can't redo a bad operation without waiting for timeout, you don't know what's happening underneath. I use Claude with kubectl all the time." |
| 2 | **GitHub is still center of universe** | "The trigger for an action often still is a git push or PR merge... config starts in github and then evolves into ConfigHub." |
| 3 | **Organization is hard** | "Deciding how to organize in spaces can be difficult... dichotomy between 'space oriented' and 'unit oriented' organization." |
| 4 | **Triggers not worth the squeeze** | "It's very hard to create an intuitive UX when you have many functions that do almost the same thing. Even if you carefully read the docs, it still becomes trial and error." |
| 5 | **Variant propagation unsolved** | "Propagating some changes while NOT propagating other changes between variants is IMO an unsolved or poorly solved problem in git today." |
| 6 | **Bulk changes need intent** | "Performing bulk changes that are grouped by intent is an unsolved problem... In ConfigHub these can be performed as a 'transaction' with documented reason, metadata, audit trail." |

### Philipp's Core Fear

> "Isn't this same thing doomed to happen with units and functions in a space? Some folks do something ad-hoc on the unit level. Then there will eventually be a bunch of triggers on the space level that need to be kept aligned. Eventually the software changes and the triggers in per environment spaces diverge and I end up with an error that none of the lower environments caught and that takes down my production environment."

### Brian's Warning

> "I don't believe that simply performing ETL on config and creating a config data warehouse a la rendered manifest pattern has enough value to be a product."

### Jesper's Value Articulation

> "ConfigHub is source of truth for the config data that tracks an actual live resource... It is an operational system of record for what was, is and will be in the real system, and how it evolved."

---

## The New Model (Three Levels)

```
┌─────────────────────────────────────────────────────────────────────┐
│                              HUB                                    │
│                    (Governance — organization-wide)                 │
│                                                                     │
│   Base Space (implicit) — catalog of base units                     │
│   Sources — Git repos, Helm repos (feeds into Units)                │
│   Workers — execution agents                                        │
│   Targets — clusters/environments                                   │
│   Policies, constraints — INHERITED by all App Spaces               │
│   Saved Queries (org-wide)                                          │
└─────────────────────────────────────────────────────────────────────┘
                                 │
                          inherits from
                                 │
                                 ↓
┌─────────────────────────────────────────────────────────────────────┐
│                           APP SPACE                                 │
│                    (Team workspace — NOT environment)               │
│                                                                     │
│   Owner: Team                                                       │
│   Contains:                                                         │
│   • Units (with labels: app=X, variant=Y)                           │
│   • Actions (automation workflows)                                  │
│   • Saved Queries                                                   │
│   • (future) AI Agents                                              │
│                                                                     │
│   THE POLICY CHOICE POINT — where teams decide HOW:                 │
│   • Deployer (Flux, Argo, ConfigHub via Worker)                     │
│   • Reconciliation (who wins: Source vs Desired vs Live)            │
│   • Automation (auto vs manual approval)                            │
│   • Sync-back (always, pr-required, audit-only, never)              │
│   • Drift handling (revert, accept, alert, ignore)                  │
│                                                                     │
│   Hub sets CONSTRAINTS. App Space makes CHOICES within them.        │
└─────────────────────────────────────────────────────────────────────┘
                                 │
                              contains
                                 │
                                 ↓
┌─────────────────────────────────────────────────────────────────────┐
│                              UNIT                                   │
│                    (Single resource — WET manifest)                 │
│                                                                     │
│   Labels: app=synkro, variant=prod, owner=philipp                   │
│   Contains: WET YAML, lifecycle state, drift status                 │
│                                                                     │
│   Query by labels:                                                  │
│   • cub map --query "Labels['variant'] = 'prod'"                    │
│   • cub map --query "Labels['app'] = 'synkro'"                      │
└─────────────────────────────────────────────────────────────────────┘
```

**Critical distinction:** App Space is a **team workspace**, not an environment. Environments are expressed as **labels on Units** (e.g., `variant=prod`).

---

## Problem → Solution Mapping

### Problem 1: "Deployment is infuriating, can't see what's happening"

**Jesper:** "You don't know what's happening underneath. I use Claude with kubectl all the time."

**Solution: Map + Agent**

The Map is the visibility layer. The Agent (read-only) watches clusters and feeds the Map continuously.

```bash
# What's in the fleet right now?
cub-agent map list

# Filter by cluster, namespace, kind
cub-agent map list --cluster prod-east --namespace default --kind Deployment

# Who owns what? (Flux, Argo, Helm, Terraform, ConfigHub, Native)
cub-agent map list --owner Flux

# See relationships (Pod → ReplicaSet → Deployment)
cub-agent map relations <resource-id>

# View change history
cub-agent map history <resource-id>

# Show drift between desired and live
cub unit get apiserver --variant demo --diff
```

**Key distinction:**
- **cub-agent** = read-only discovery, zero risk, standalone
- **Worker** = execution engine, only needed for operations

You get visibility without committing to the write path. If Worker fails, cub-agent still tells you what's happening.

---

### Problem 2: "GitHub is still center of universe"

**Jesper:** "Config starts in github and then evolves into ConfigHub... developing the concept of 'config source' symmetrical to 'config target'."

**Solution: Git (DRY) → render → Unit (WET) → optional sync-back to Git**

```
Git (DRY)                           Git (WET, optional)
├── _base/apiserver/                deploy/rendered/
├── _base/frontend/            ←    ├── demo/apiserver.yaml
├── demo/kustomization.yaml         └── prod/apiserver.yaml
└── prod/kustomization.yaml              ↑
         │                               │ sync-back
         ↓ render                        │
         ↓                               │
    ConfigHub App Space ─────────────────┘
    ├── Unit: apiserver (app=synkro, variant=demo)
    ├── Unit: frontend  (app=synkro, variant=demo)
    ├── Unit: apiserver (app=synkro, variant=prod)
    └── Unit: frontend  (app=synkro, variant=prod)
```

| Layer | Source of Truth For | Who Cares |
|-------|---------------------|-----------|
| Git (DRY) | Authoring — templates, Kustomizations | Developers |
| Unit (WET) | Operational — what gets deployed | SREs, Ops |
| Git (WET) | Audit — archive, compliance | Security, Compliance |

**GitHub Actions can trigger the render:**
```yaml
on: push
jobs:
  render:
    steps:
      - run: kustomize build deploy/manifests/demo/ | cub import --space my-team --labels "app=synkro,variant=demo"
```

Git push → render → ConfigHub Variant. GitHub is still the trigger. ConfigHub is where operational truth lives.

---

### Problem 3: "Organization is hard — space vs unit dichotomy"

**Jesper:** "Dichotomy between 'space oriented' and 'unit oriented' organization... creating a double usage of space as both higher level organizing construct and also 'environment'."

**Solution: Three levels with labels — Hub → App Space → Unit (with app/variant labels)**

The confusion came from overloading "Space" to mean both:
1. Organizational grouping (team, project)
2. Environment (dev, prod)

**New model separates these:**

| Concept | Purpose | How It Works |
|---------|---------|--------------|
| **Hub** | Governance — policies that apply across teams | Contains base units, sources, workers, targets |
| **App Space** | Team workspace — where a team works | Contains units, queries, actions |
| **Unit** | Single deployable resource | Has labels: `app=X`, `variant=Y` |

**App and Variant are labels, not containers.** Query units by label to see "the application" or "the environment."

**Example:**

```
Hub: platform-standards
│
└── App Space: philipp-localdev        ← Team workspace (NOT environment)
      │
      ├── Unit: apiserver (app=synkro, variant=demo)
      ├── Unit: frontend  (app=synkro, variant=demo)
      ├── Unit: apiserver (app=synkro, variant=tilt)
      ├── Unit: frontend  (app=synkro, variant=tilt)
      └── ...

# Query "the synkro app":
cub query "Labels['app'] = 'synkro'"

# Query "demo environment":
cub query "Labels['variant'] = 'demo'"
```

**NOT this (wrong):**
```
Hub
├── App Space: demo     ← WRONG: environment as App Space
└── App Space: tilt     ← WRONG: environment as App Space
```

---

### Problem 4: "Triggers not worth the squeeze"

**Jesper:** "It's very hard to create an intuitive UX when you have many functions that do almost the same thing... it required 6 triggers on a unit with just a few resources."

**Philipp:** "How would I keep these in-sync if something changes in my app?"

**Solution: Actions on App Space with label filters**

The problem: if triggers are configured per-environment, they diverge (N environments × M triggers = sprawl).

The fix: **Actions belong to the App Space and use label filters to select Units.**

```yaml
# Action in App Space: philipp-localdev
apiVersion: confighub.com/v1
kind: Action
metadata:
  name: set-namespace-on-import
  space: philipp-localdev
spec:
  on:
    event: unit.created
    # No filter = applies to all Units in this App Space
  
  runs:
    steps:
      - uses: confighub/set-value@v1
        with:
          path: metadata.namespace
          value: "synkro-system-${{ unit.labels.variant }}"  # resolved from label
```

Action defined once on App Space. The `variant` label on each Unit determines the namespace.

**Philipp's concern answered:**

> "Triggers diverge across environments"

They can't — Actions live on the App Space, not per-Unit. All Units in the App Space have the same Actions applied. Use label filters for environment-specific behavior.

---

### Problem 5: "Variant propagation is unsolved"

**Jesper:** "Propagating some changes while NOT propagating other changes between variants is IMO an unsolved or poorly solved problem in git today."

**Solution: WET configs in Units + selective propagation via Map queries**

Each Unit has independent WET config. No "merge hell" because there's nothing to merge — each is a complete, rendered manifest.

**To propagate a change:**

```bash
# Find all units with old redis across all environments
cub map --query "spec.containers[*].image contains 'redis:7.0'"

# See them
UNIT          APP       VARIANT   IMAGE
redis-cache   synkro    demo      redis:7.0.0
redis-cache   synkro    tilt      redis:7.0.0
redis-cache   synkro    prod      redis:7.2.1   ← prod already updated

# Update only demo and tilt (by label filter)
cub mutate --space philipp-localdev \
    --query "Labels['app'] = 'synkro' AND Labels['variant'] in ('demo', 'tilt')" \
    --set "spec.template.spec.containers[0].image=redis:7.2.1"
```

**Intended differences stay different.** You query, see the state, choose what to propagate. No accidental override.

---

### Problem 6: "Bulk changes need intent"

**Jesper:** "Performing bulk changes that are grouped by intent is an unsolved problem. For example, update all redis to v2 because of a CVE... in ConfigHub these can be performed as a 'transaction' with documented reason, metadata (links to tickets), audit trail."

**Solution: Changesets**

```bash
# Create a changeset with intent
cub changeset create --name "CVE-2025-001-redis" \
    --description "Update redis to 7.2.1 for CVE-2025-001" \
    --ticket "JIRA-1234"

# Add mutations to it
cub unit mutate --changeset CVE-2025-001-redis \
    --query "image contains redis" \
    --where "Labels.version < '7.2.0'" \
    --set "spec.template.spec.containers[0].image=redis:7.2.1"

# Review what's in the changeset
cub changeset diff CVE-2025-001-redis

# Apply as atomic unit
cub changeset apply CVE-2025-001-redis
```

**Audit trail shows:**
- What changed
- Why (CVE-2025-001)
- Who approved
- Link to ticket
- All affected units across all variants

Git commits are "all or nothing for a repo." Changesets are scoped to intent.

---

### Problem 7: Philipp's "triggers diverge" fear — FULLY ANSWERED

**Philipp:** "Eventually the software changes and the triggers in per environment spaces diverge and I end up with an error that none of the lower environments caught and that takes down my production environment."

**Solution: Actions on App Space with label filters + Hub validation**

```
Hub: platform-standards
│   Policies: must have resource limits, no raw secrets, etc.
│
└── App Space: philipp-localdev
      │
      ├── Unit: apiserver (app=synkro, variant=demo)
      ├── Unit: apiserver (app=synkro, variant=prod)
      │
      ├── Action: validate-on-import        ← applies to ALL Units
      ├── Action: auto-heal-non-prod        ← filter: variant != prod
      └── Action: alert-prod-drift          ← filter: variant = prod
```

**Key protections:**

1. **Actions can't diverge** — they're on the App Space, with label filters
2. **Hub policies enforce consistency** — same validation for all Units
3. **Promotion is explicit** — `cub promote --query "app=synkro AND variant=demo" --to-variant prod`
4. **Hub policy gate** — if demo passes Hub policies, prod will too

**The failure Philipp fears:**

> "Error in production that lower environments didn't catch"

This happens when:
- Dev has different triggers than prod → **fixed: Actions on App Space, label filters**
- Dev has different policies than prod → **fixed: Hub policies apply to all**
- Prod has manual config drift → **fixed: drift detection + reconciliation**

---

## Summary: Before vs After

| Problem | Before (Current UX) | After (New Model) |
|---------|---------------------|-------------------|
| Can't see what's happening | Worker is the only window | Map + cub-agent = continuous visibility |
| GitHub is center of universe | Import is awkward disconnect | Git (DRY) → render → Unit (WET) → sync-back |
| Space vs Unit confusion | Overloaded "Space" concept | Hub → App Space → Unit (with labels) |
| Triggers per-environment sprawl | Configure triggers per Space | Actions on App Space with label filters |
| Variant propagation | Merge conflicts, accidental overrides | WET Units + Map queries for selective propagation |
| Bulk changes lack intent | Individual mutations, no audit grouping | Changesets with reason, metadata, ticket links |
| Triggers diverge across envs | Manual keeping in sync | Actions with label filters — cannot diverge |

---

## Jesper's "Cut to the Bone" Value

> "If we really 'cut to the bone', then here's how I see the value: It is challenging to perform changes across a complex topology of config."

The new model delivers:

1. **Formatting errors caught before deployment** — Hub policies validate on import
2. **Selective propagation works** — WET configs + Map queries
3. **Intended vs unintended differences visible** — `cub map diff --variant demo --variant prod`
4. **Bulk changes grouped by intent** — Changesets with audit trail

---

## What This Means for Philipp

His workflow:
```
Git (Kustomizations) → Terraform applies → Cluster
```

New model:
```
Git (Kustomizations)
    ↓ render (cub import --space philipp-localdev --labels "app=synkro,variant=demo")
ConfigHub App Space
├── Unit: apiserver (app=synkro, variant=demo)
├── Unit: frontend  (app=synkro, variant=demo)
└── ...
    ↓ Hub policy validation (automatic)
    ↓ optional sync-back / OCI push
Git (WET) or OCI Registry for audit
    ↓
Terraform/Flux/Argo deploys from Git (WET) or OCI
```

**His Kustomizations stay in Git.** ConfigHub stores the rendered output as Units with labels. Hub policies prevent the sprawl he fears. Actions on App Space apply to all Units — label filters determine environment-specific behavior. Sync-back keeps Git updated for audit and for his existing Terraform workflow.

His concern:
> "ConfigHub tries to pull all of this out of Git"

Answer: **Config isn't pulled out. It's transformed.** DRY → WET. And it can flow back to Git for audit and for tools that need Git (Flux, Argo, Terraform).

---

## Open Questions

1. **How does sync-back work for Philipp's Terraform workflow?**
   - ConfigHub syncs WET to Git → Terraform applies from Git
   - Or: ConfigHub pushes to OCI → new Terraform provider reads from OCI

2. **What about his secrets in Kustomization?**
   - Option A: ConfigHub stores rendered Secret (protected)
   - Option B: Kustomization uses ExternalSecret, ConfigHub stores that
   - Option C: Secrets excluded from ConfigHub, managed separately

3. **How does Hub validation happen on import?**
   - Automatic? Blocking?
   - Or just flagged for review?

4. **What's the CLI syntax for the hierarchy?**
   - `cub unit get apiserver --app synkro --variant demo`
   - Or: `cub unit get synkro/demo/apiserver`
   - Or: context-based (set current app/variant)
