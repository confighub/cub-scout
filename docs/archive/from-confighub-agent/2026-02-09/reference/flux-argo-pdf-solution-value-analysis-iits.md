# IITS Value Analysis: What Adds MOST Value?

> **Archive status:** Historical planning context (non-canonical for current releases).
> For current `cub-scout` behavior and command contracts, use:
> - `docs/roadmap.md`
> - `docs/reference/commands.md`
> - `docs/reference/import-docs-crosswalk.md`
>
> Scope note: examples here that use mutating `cub` workflows (`drift accept`, `mutate`, `promote`, `changeset`, worker lifecycle) describe ConfigHub platform behavior, not direct `cub-scout` command commitments.

**Date:** 2026-01-06
**Question:** Is Hub + App Space the right split? What adds MOST value to IITS and similar customers?

---

## The Actual IITS Pattern (from PDFs)

### Flux CD Pattern
```
stages-repo/
├── dev/
│   ├── flux-system/
│   │   ├── infrastructure/
│   │   │   ├── cert-manager.yaml    # Kustomization CR
│   │   │   └── ingress-nginx.yaml   # Kustomization CR
│   │   └── apps/
│   │       └── applications.yaml    # Kustomization CR
│   ├── config/
│   │   └── values.yaml              # Environment ConfigMap (cluster-vars)
│   └── applications/
│
base-repo/
├── infrastructure/
│   ├── cert-manager/                # HelmRepository + HelmRelease
│   └── ingress-nginx/
```

**Pain:** "What you see in Git isn't what deploys" — mental compilation of base + patches + variable substitutions.

### Argo CD Pattern
```
managed-service-catalog/helm/        # Umbrella Helm charts (platform team)
├── cert-manager/
├── external-secrets/
├── kyverno/
└── ...

customer-service-catalog/helm/       # Per-cluster values (extends umbrella)
├── pe-gitops/
│   ├── cert-manager/values.yaml
│   ├── external-secrets/values.yaml
│   └── ...
```

**Pain:** "Teams build their own central hub because they don't like the defaults, or the umbrella is missing features."

### The Hub-and-Spoke Topology
```
┌─────────────────────────────────────────────────────────────────────────┐
│ Hub Cluster (Controlplane)                                               │
│                                                                          │
│   ┌─────────────────┐     ┌─────────────────────────────────────────┐  │
│   │ Helm Umbrella   │────▶│ ApplicationSet                          │  │
│   │ Charts (Catalog)│     │   generators:                           │  │
│   └─────────────────┘     │     - clusters:                         │  │
│                           │         selector:                        │  │
│                           │           cert-manager: enabled          │  │
│                           └────────────────┬────────────────────────┘  │
└────────────────────────────────────────────┼────────────────────────────┘
                                             │
              ┌──────────────────────────────┼──────────────────────────────┐
              │                              │                              │
              ▼                              ▼                              ▼
      ┌───────────────┐            ┌───────────────┐            ┌───────────────┐
      │ Dataplane-0   │            │ Dataplane-1   │            │ Dataplane-N   │
      │ (SKE)         │            │ (Edge)        │            │ (Virtual)     │
      └───────────────┘            └───────────────┘            └───────────────┘
```

**Key insight:** Labels on clusters determine what gets deployed. `cert-manager: enabled` = gets cert-manager.

---

## The IITS Pain Points (Direct Quotes)

### From Flux CD Doc:
| Pain | Quote |
|------|-------|
| **Visibility** | "What you see in the Git repository isn't what actually gets deployed" |
| **Mental compilation** | "To understand what's actually running in production, you need to mentally compile all these layers or run flux build for each kustomization" |
| **Code review impossible** | "Reviewers can't easily see the impact of a change without building the manifests locally" |
| **Multi-dimensional debugging** | "The issue could be in the base configuration, in a patch that's not applying correctly, in a variable substitution that's missing or wrong, or in the dependency chain" |
| **Structure doesn't scale** | "Every new environment or region typically needs its own overlay directory with its own set of patches and variables" |
| **Silent breakage** | "Hundreds of patch files that can break silently when the base resources change structure" |

### From Argo CD Doc:
| Pain | Quote |
|------|-------|
| **Umbrella divergence** | "Many teams end up building their own 'central hub' because they do not like the defaults from the central team, or because the umbrella chart is missing features" |
| **Per-cluster sprawl** | `$valuesRepo/customer-service-catalog/helm/{{name}}/certmanager/values.yaml` — one values file per cluster per tool |
| **Hydration complexity** | "They 'hydrate' the resources in the pipeline... commit the generated manifests to Git" |

---

## What IITS Actually Delivers

IITS (and similar platform consultancies) deliver:

1. **Assessment** — "Here's what you have, here's what's broken"
2. **Architecture** — "Here's how to structure your GitOps"
3. **Migration** — "Here's how to get from here to there"
4. **Governance** — "Here's how to prevent this from happening again"
5. **Operations** — "Here's how to do day-2 at scale"

---

## How ConfigHub Maps to IITS Pattern

### The IITS "Managed Service Catalog" = ConfigHub Hub

IITS Pattern:
```
managed-service-catalog/helm/
├── cert-manager/      # Umbrella chart with best practices
├── kyverno/           # Umbrella chart with policies
└── ...
```

ConfigHub Equivalent:
```yaml
Hub: platform-catalog
  # Platform team's rendered units (not templates!)
  units:
    - name: cert-manager-base
      labels: { type: platform, component: cert-manager }
      content: |  # WET - rendered from umbrella chart
        apiVersion: apps/v1
        kind: Deployment
        metadata:
          name: cert-manager
        spec:
          replicas: 2
          ...

  # Constraints that ALL teams must follow
  constraints:
    - name: must-have-limits
      match: { kind: Deployment }
      require:
        path: spec.template.spec.containers[*].resources.limits
        exists: true
```

**Key difference:** IITS umbrella chart is a *template* that teams extend with values. ConfigHub Hub is *rendered WET* that teams *clone and modify*.

### The IITS "Customer Service Catalog" = ConfigHub App Space

IITS Pattern:
```
customer-service-catalog/helm/
├── pe-gitops/
│   ├── cert-manager/values.yaml    # Cluster-specific values
│   └── kyverno/values.yaml
├── edge-cluster/
│   └── cert-manager/values.yaml
```

ConfigHub Equivalent:
```yaml
App Space: pe-gitops-team
  units:
    # Cloned from Hub's cert-manager-base, with overrides
    - name: cert-manager
      labels: { type: platform, component: cert-manager, cluster: pe-gitops }
      upstream: cert-manager-base    # Tracks where it came from
      content: |  # WET - team's customized version
        apiVersion: apps/v1
        kind: Deployment
        metadata:
          name: cert-manager
        spec:
          replicas: 3    # Team chose 3 replicas
          ...
```

**Key difference:** IITS values.yaml can only change what umbrella exposes. ConfigHub clone can change *anything not constrained by Hub*.

### The IITS Labels = ConfigHub Labels

IITS Pattern:
```yaml
# ApplicationSet with cluster selector
generators:
  - clusters:
      selector:
        matchLabels:
          cert-manager: enabled
          edge-cluster: true
```

ConfigHub Equivalent:
```yaml
# Units with labels, queried the same way
units:
  - name: cert-manager
    labels:
      component: cert-manager
      cluster-type: edge
      region: eu-west
      variant: prod

# Query: "Deploy cert-manager to all edge clusters in EU"
cub unit list --where "Labels['component'] = 'cert-manager' AND Labels['cluster-type'] = 'edge' AND Labels['region'] = 'eu-west'"
```

**Same concept, more dimensions:** IITS uses labels on clusters. ConfigHub uses labels on units. Same query power, but ConfigHub can query the *content* too.

---

## Why Hub + App Space Beats Umbrella Charts

| IITS Umbrella Pattern | ConfigHub Hub + App Space |
|----------------------|--------------------------|
| Template + values = mystery output | WET manifests = exactly what deploys |
| Can only change exposed values | Can change anything not constrained |
| Fork umbrella when it doesn't fit | Clone and modify, keep upstream link |
| Central team controls all defaults | Hub constrains, App Space chooses |
| Per-cluster values files sprawl | Labels on units, one query |
| Patch files break silently | Clone is independent, pull explicitly |

### The Core Problem Solved

> "Teams build their own central hub because they don't like the defaults, or the umbrella is missing features."

**Why this happens with umbrellas:**
- Team needs to change something
- Umbrella doesn't expose it in values.yaml
- Team forks umbrella
- Now there are two "central" configs
- Repeat until chaos

**Why this doesn't happen with Hub + App Space:**
- Team clones from Hub
- Team can change *anything* in their clone
- Hub constraints still apply (security, compliance)
- Team's clone tracks upstream for updates
- No fork needed — explicit customization

---

## The IITS Customer Journey

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ Day 0: Assessment                                                            │
│ "What do we have? Who owns what? What's broken?"                            │
│                                                                              │
│ → Agent discovers cluster state                                              │
│ → Map shows ownership (Flux/Argo/Helm/Native)                               │
│ → Risk scanner finds anti-patterns                                          │
│                                                                              │
│ VALUE: Immediate. No ConfigHub account needed.                              │
└─────────────────────────────────────────────────────────────────────────────┘
                                    ↓
┌─────────────────────────────────────────────────────────────────────────────┐
│ Day 1: Import                                                                │
│ "Let's bring your existing configs into ConfigHub"                          │
│                                                                              │
│ → cub helm install renders existing charts to Units                         │
│ → Import creates App Space with team's units                                │
│ → Labels organize: app, variant, region                                     │
│                                                                              │
│ VALUE: Quick. Shows "here's what you have, queryable."                      │
└─────────────────────────────────────────────────────────────────────────────┘
                                    ↓
┌─────────────────────────────────────────────────────────────────────────────┐
│ Day 7: Structure                                                             │
│ "Here's how to manage dev/staging/prod without copy-paste"                  │
│                                                                              │
│ → Clone base → variants with upstream tracking                              │
│ → Labels for multi-dimensional organization                                 │
│ → Saved queries for common views                                            │
│                                                                              │
│ VALUE: Solves "structure doesn't scale" problem                             │
└─────────────────────────────────────────────────────────────────────────────┘
                                    ↓
┌─────────────────────────────────────────────────────────────────────────────┐
│ Day 14: Governance                                                           │
│ "Here's how to enforce standards without blocking teams"                    │
│                                                                              │
│ → Hub constraints: MUST have limits, CAN'T use :latest                      │
│ → App Space choices: deployer, drift policy, actions                        │
│ → Validation on import/deploy                                               │
│                                                                              │
│ VALUE: Solves "central hub doesn't stick" problem                           │
└─────────────────────────────────────────────────────────────────────────────┘
                                    ↓
┌─────────────────────────────────────────────────────────────────────────────┐
│ Day 30+: Operations                                                          │
│ "Here's how to do fleet-wide updates, CVE response, etc."                   │
│                                                                              │
│ → Query + Mutate for bulk operations                                        │
│ → Actions for automation                                                    │
│ → Bidirectional sync for break-glass                                        │
│                                                                              │
│ VALUE: Ongoing operational efficiency                                       │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Where Does Hub vs App Space Fit?

| Phase | Hub | App Space | Agent |
|-------|-----|-----------|-------|
| Assessment | ❌ Not needed | ❌ Not needed | ✅ Primary |
| Import | ❌ Optional | ✅ Created | ✅ Discovers |
| Structure | ❌ Optional | ✅ Labels, Queries | ❌ Not needed |
| Governance | ✅ Constraints | ✅ Choices | ❌ Not needed |
| Operations | ✅ Org-wide policies | ✅ Team workflows | ✅ Drift detection |

**Key insight:** Hub is needed for governance, but **not required early**. The customer journey starts with Agent + App Space.

---

## The Core IITS Problem: "Central Hub Doesn't Stick"

From the IITS docs:
> "Teams want central config but then diverge because constraints are too rigid or too loose"

This is **THE** problem Hub + App Space solves:

| Too Rigid | Too Loose | Hub + App Space |
|-----------|-----------|-----------------|
| Central chart, no customization | Every team does own thing | Hub constrains what matters |
| Teams fork because can't patch | No consistency across teams | App Space allows choices |
| One size fits none | N different implementations | Governed autonomy |

**The split:**
- **Hub = Platform Contract** — What the platform team guarantees/requires
- **App Space = Team Workspace** — Where teams work within the contract

---

## What Should Hub Contain?

Think of Hub as the "Platform Contract":

```yaml
Hub: platform-contract
  # 1. APPROVED TOOLS
  deployers:
    allowed: [flux, argo, bridge]
    prohibited: [terraform, kubectl-direct]

  # 2. REQUIRED PROPERTIES (validated on import/deploy)
  constraints:
    - name: resource-limits
      match: { kind: Deployment }
      require:
        path: spec.template.spec.containers[*].resources.limits
        exists: true

    - name: no-latest-in-prod
      match: { kind: Deployment, Labels['variant']: 'prod' }
      deny:
        path: spec.template.spec.containers[*].image
        pattern: ".*:latest$"

  # 3. SHARED INFRASTRUCTURE (platform-owned units)
  units:
    - name: cert-manager
      labels: { type: platform, variant: prod }
      owner: platform-team

    - name: external-secrets
      labels: { type: platform, variant: prod }
      owner: platform-team

  # 4. ORG-WIDE QUERIES
  saved_queries:
    - name: all-prod
      query: "Labels['variant'] = 'prod'"
    - name: all-drifted
      query: "drift = true"
    - name: missing-limits
      query: "spec.template.spec.containers[*].resources.limits NOT exists"
```

---

## What Should App Space Contain?

Think of App Space as the "Team Workspace":

```yaml
App Space: payments-team
  inherits: platform-contract   # Hub constraints apply

  # 1. TEAM CHOICES (within Hub constraints)
  reconciliation:
    rules:
      - match: { Labels['variant']: 'prod' }
        deployer: flux
        drift: revert
        sync_back: pr-required
      - match: { Labels['variant']: 'dev' }
        deployer: argo
        drift: accept

  # 2. TEAM UNITS (multi-dimensional via labels)
  units:
    - name: payment-api
      labels: { app: payment-api, variant: prod, region: eu }
      upstream: payment-api-base
    - name: payment-api
      labels: { app: payment-api, variant: staging, region: eu }
      upstream: payment-api-base
    # ... all variants in ONE space

  # 3. TEAM QUERIES
  saved_queries:
    - name: my-prod
      query: "Labels['variant'] = 'prod'"
    - name: my-drifted
      query: "drift = true AND Labels['app'] IN ('payment-api', 'payment-worker')"

  # 4. TEAM ACTIONS
  actions:
    - name: deploy-preview
      on: unit.changed
      filter: Labels['variant'] = 'staging'
    - name: alert-prod-drift
      on: unit.drift.detected
      filter: Labels['variant'] = 'prod'
```

---

## The MOST Value for IITS

Ranking by impact:

### 1. **Visibility (Highest Impact)**

> "What you see in Git isn't what actually gets deployed"

**Solution:** Rendered Manifests Pattern + Map

```bash
# See exactly what deploys, no mental compilation
cub unit get payment-api --variant prod

# Query across everything
cub unit list --where "Labels['variant'] = 'prod'"
```

**Why highest:** This is Day 0 value. Every customer has this problem.

### 2. **Clone + Upstream (High Impact)**

> "Every new environment needs its own overlay directory"

**Solution:** Clone with upstream tracking

```bash
# Clone, don't copy-paste
cub unit create payment-api-eu --upstream-unit payment-api-base --labels "region=eu"

# Pull upstream changes
cub unit update --where "upstream = 'payment-api-base'" --upgrade --preview
```

**Why high:** Solves the "structure doesn't scale" problem directly.

### 3. **Hub Constraints (Medium-High Impact)**

> "Teams diverge because constraints are too rigid or too loose"

**Solution:** Hub = what matters, App Space = what's flexible

```yaml
# Hub says WHAT must be true
Hub:
  constraints:
    - must-have-limits
    - must-use-approved-images

# App Space says HOW
App Space:
  reconciliation:
    deployer: flux  # Team's choice
```

**Why medium-high:** Takes time to define, but prevents future problems.

### 4. **Multi-Dimensional Labels (Medium Impact)**

> "Per-cluster values files sprawl"

**Solution:** Labels instead of folders

```bash
# One query, all dimensions
cub unit list --where "Labels['app'] = 'redis' AND Labels['variant'] = 'prod'"

# Bulk update across regions
cub mutate --where "Labels['app'] = 'redis'" --set "image=redis:7.2.4"
```

**Why medium:** Requires mindset shift from folders to labels.

### 5. **Actions + Automation (Lower Initial Impact)**

> "Triggers diverge across environments"

**Solution:** Actions with label filters (not per-space)

```yaml
# One action, filter by labels
- name: auto-heal-non-prod
  filter: Labels['variant'] != 'prod'
```

**Why lower initially:** Requires understanding the system first. Day 30+ value.

---

## Recommendation: Phased Approach

### Phase 1: Agent + App Space (No Hub Required)

```bash
# Day 0: Discover
./test/atk/map
./test/atk/scan

# Day 1: Import
cub helm install redis bitnami/redis --space my-team
cub helm install nginx bitnami/nginx --space my-team

# Day 7: Structure with labels
cub unit create redis-prod --upstream-unit redis --labels "variant=prod"
cub unit create redis-staging --upstream-unit redis --labels "variant=staging"
```

**Value:** Visibility + Structure without governance overhead.

### Phase 2: Add Hub for Governance

```bash
# Day 14: Create Hub with constraints
cub hub create platform-contract
cub hub add-constraint platform-contract --must-have-limits
cub hub add-constraint platform-contract --no-latest-prod

# Attach App Space to Hub
cub space update my-team --hub platform-contract

# Now imports/changes are validated
cub helm install something bad/chart --space my-team
# Error: Hub constraint violated: must-have-limits
```

**Value:** Governance without blocking teams.

### Phase 3: Scale Operations

```bash
# Day 30+: Fleet operations
cub mutate --where "Labels['app'] = 'redis'" --set "image=redis:7.2.4"
cub unit update --where "upstream = 'redis-base'" --upgrade

# Automation
cub action create auto-heal --space my-team --filter "variant != 'prod'"
```

**Value:** Operational efficiency at scale.

---

## Summary: The Right Split?

**Yes, Hub + App Space is the right split**, but with these refinements:

| Aspect | Current | Recommendation |
|--------|---------|----------------|
| Hub required? | Implied yes | **No — optional for Phase 1** |
| Hub contains units? | Unclear | **Yes — platform-owned infrastructure** |
| App Space one-per-team? | Yes | **Yes — all variants via labels** |
| Hub = governance only? | Yes | **Hub = governance + shared infra** |

**The key insight:** Hub is a "Platform Contract" that includes:
1. Constraints (governance)
2. Shared units (platform infrastructure)
3. Org-wide queries (visibility)

App Space is a "Team Workspace" that includes:
1. Choices (within constraints)
2. Team units (all their stuff)
3. Team queries (their views)
4. Team actions (their automation)

**Entry point matters:** Start with Agent + App Space. Add Hub when governance needed.

---

## What's Missing?

Potential gaps:

1. **Cross-team dependencies** — How does Team A depend on Team B's service?
2. **Promotion paths** — Dev → Staging → Prod across teams (not just within)
3. **Shared App Spaces** — Multiple teams contributing to same space?
4. **Hub hierarchy** — Can a Hub inherit from another Hub?

These might be Phase 4+ concerns.

---

## Mapping to What IITS Actually Sells

IITS (and similar consultancies) sell **engagements**, not tools. ConfigHub maps to their deliverables:

| IITS Deliverable | What They Do | ConfigHub Feature | How It Helps IITS |
|------------------|--------------|-------------------|-------------------|
| **Assessment** | Audit clusters, find problems | Agent + Risk scanner | Faster discovery, consistent findings |
| **Architecture Design** | Define GitOps structure | Hub + App Space model | Prescriptive pattern, not custom |
| **Migration** | Move to GitOps | `cub helm install` + Import | Render existing → Units |
| **Governance** | Define standards | Hub constraints | Constraints ship with engagement |
| **Operations Playbook** | Day-2 procedures | Actions + Queries | Automated, not documented |

### The Business Case for IITS

**Before ConfigHub:**
- Assessment = manual kubectl commands + spreadsheets
- Architecture = custom design per customer
- Migration = manual conversion to Kustomize/Helm
- Governance = policy docs that nobody follows
- Operations = runbooks that go stale

**With ConfigHub:**
- Assessment = `./test/atk/map && ./test/atk/scan` (5 min)
- Architecture = "Hub + App Space with these labels" (standardized)
- Migration = `cub helm install` for each component (automated)
- Governance = Hub constraints that enforce (not documents)
- Operations = Actions + Queries (executable, not docs)

**The multiplier:** IITS can deliver more engagements with same team because ConfigHub standardizes their approach.

---

## For IITS Demo

**30-second pitch:**
> "ConfigHub gives you visibility into what's actually running, lets you organize it with labels instead of folder sprawl, and adds governance that doesn't block your teams."

**Demo flow:**
1. Agent discovers cluster (30 sec)
2. Import existing Helm charts (30 sec)
3. Clone to create variants (30 sec)
4. Query across everything (30 sec)
5. Add Hub constraint, show validation (30 sec)

Total: 2.5 minutes from zero to governed GitOps.

---

## Bottom Line

**For IITS specifically:**

1. **Hub + App Space IS the right model** — it directly solves the "umbrella chart divergence" problem that IITS sees repeatedly

2. **Hub is optional initially** — don't gate adoption on governance. Start with Agent + App Space, add Hub when customer is ready for constraints

3. **Rendered Manifests Pattern is key** — this is what makes visibility possible. "What you see is what deploys" eliminates the mental compilation problem

4. **Labels > Folders** — this is a mindset shift but solves the "structure doesn't scale" problem permanently

5. **Clone + Upstream is unique** — neither Flux nor Argo has this. Teams customize without forking, pull updates explicitly

**The Hub + App Space split mirrors exactly what IITS delivers:**
- Hub = Platform Contract (what IITS designs for the customer)
- App Space = Team Workspace (where teams operate within the contract)

This isn't a coincidence — it's the correct abstraction for enterprise GitOps.
