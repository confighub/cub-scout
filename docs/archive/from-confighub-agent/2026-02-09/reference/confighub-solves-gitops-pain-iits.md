# How ConfigHub Solves Enterprise GitOps Pain

**Purpose:** Solution analysis for IITS use cases — maps pain points from real-world Argo CD and Flux CD fleet patterns to ConfigHub solutions.

**Status:** Current
**Last Updated:** 2026-01-06

**Key correction:** Three-level hierarchy (Hub → App Space → Unit). App and Variant are labels on Units, not container objects.

**Analysis of:**
- `usecase-argocd-fleet-iits.pdf` — Argo CD fleet patterns from IITS consulting
- `usecase-fluxcd-fleet-iits.pdf` — Flux CD fleet patterns from IITS consulting

> **Canonical reference:** See [HUB-APPSPACE-MODEL.md](../historical/2026-01-07-before-reorg/map/HUB-APPSPACE-MODEL.md) for the complete model. CLI examples in this document are **proposed**.

---

## The Mental Model

> **Git says WHAT. ConfigHub says HOW. Cluster says NOW.**

---

## The Problems They Describe

### From the Argo CD Document

| Problem | Quote |
|---------|-------|
| **Umbrella chart divergence** | "Many teams end up building their own 'central hub' because they do not like the defaults from the central team, or because the umbrella chart is missing features" |
| **Per-cluster values sprawl** | `$valuesRepo/customer-service-catalog/helm/{{name}}/certmanager/values.yaml` — one values file per cluster per tool |
| **Central hub doesn't stick** | Teams want central config but then diverge because constraints are too rigid or too loose |
| **Hydration complexity** | "They 'hydrate' the resources in the pipeline... commit the generated manifests to Git" |

### From the Flux CD Document

| Problem | Quote |
|---------|-------|
| **Visibility** | "What you see in the Git repository isn't what actually gets deployed" |
| **Mental compilation** | "To understand what's actually running in production, you need to mentally compile all these layers or run flux build for each kustomization" |
| **Code review impossible** | "Reviewers can't easily see the impact of a change without building the manifests locally" |
| **Multi-dimensional debugging** | "The issue could be in the base configuration, in a patch that's not applying correctly, in a variable substitution that's missing or wrong, or in the dependency chain" |
| **Structure doesn't scale** | "Every new environment or region typically needs its own overlay directory with its own set of patches and variables" |
| **Silent breakage** | "Hundreds of patch files that can break silently when the base resources change structure" |

---

## How ConfigHub Solves Each Problem

### Problem 1: "What you see isn't what deploys"

**Traditional GitOps:**
```
Git (DRY)
├── base/cert-manager/          ← What you see
├── overlays/dev/patches/       ← + this
├── cluster-vars ConfigMap      ← + this
└── postBuild substitutions     ← + this
                                   = ??? (run flux build to find out)
```

**ConfigHub:**
```
Git (DRY)                        ConfigHub Unit (WET)
├── base/cert-manager/     →     └── Unit: cert-manager
├── overlays/dev/                    (app=cert-manager, variant=dev)
└── values.yaml                      └── EXACTLY what deploys
         │                           └── No mental compilation
         └── render (once) ───────────────────┘
```

**The Unit stores WET manifests.** What you see in ConfigHub is exactly what deploys. No layers. No patches to mentally apply. No variable substitution to guess.

---

### Problem 2: "Umbrella charts diverge — teams build their own"

**Traditional pattern:**
```
Central Team's Umbrella Chart
├── cert-manager (with "best practices")
├── ingress-nginx
└── kyverno

Team A: "I don't like these defaults" → builds own umbrella
Team B: "Missing features I need" → builds own umbrella
Team C: "Can't patch what I need" → builds own umbrella

Result: 4 different cert-manager configurations, no consistency
```

**ConfigHub — Hub sets constraints, App Space makes choices:**
```
Hub: platform-standards (CONSTRAINTS)
│
│   MUST: All cert-manager installs use approved ClusterIssuer
│   MUST: Resource limits on all pods
│   CAN'T: No self-signed certs in prod
│   CAN: Teams may choose replica count
│   CAN: Teams may add custom annotations
│
├── App Space: team-a (CHOICES within constraints)
│     ├── Unit: cert-manager (app=cert-manager, variant=prod)
│     │         replicas: 3, custom annotations
│     └── Action: notify-on-deploy
│
├── App Space: team-b (CHOICES within constraints)
│     └── Unit: cert-manager (app=cert-manager, variant=prod)
│               replicas: 2, different annotations
│
└── App Space: team-c (CHOICES within constraints)
      └── Unit: cert-manager (app=cert-manager, variant=prod)
                replicas: 5, their annotations
```

**Teams don't build their own umbrella charts** because:
- Hub constrains what matters (security, compliance)
- App Space allows choice where multiple approaches are valid
- No one needs to fork — they just configure their Units

---

### Problem 3: "Per-cluster values files sprawl"

**Argo CD pattern:**
```
customer-service-catalog/helm/
├── cluster-a/
│   ├── cert-manager/values.yaml
│   ├── ingress-nginx/values.yaml
│   └── kyverno/values.yaml
├── cluster-b/
│   ├── cert-manager/values.yaml
│   ├── ingress-nginx/values.yaml
│   └── kyverno/values.yaml
├── cluster-c/
│   └── ... (repeat for every cluster)
└── cluster-n/
    └── ... (N clusters × M tools = sprawl)
```

**ConfigHub:**
```
Hub: platform-standards
│   (org-wide constraints — defined ONCE)
│
└── App Space: platform-team
      │   (team choices — defined ONCE per team)
      │
      ├── Unit: cert-manager (app=cert-manager, variant=cluster-a)
      ├── Unit: cert-manager (app=cert-manager, variant=cluster-b)
      └── Unit: cert-manager (app=cert-manager, variant=cluster-c)
```

**The difference:**
- No separate values files per cluster — Units hold the rendered config
- No overlay directories to maintain
- No patches that break when base changes
- Query across all Units: `cub map --query "name = 'cert-manager'"`

---

### Problem 4: "Code review is impossible"

**Traditional:**
```
PR: "Update cert-manager replica count"

Reviewer thinks:
- What's the base value?
- What patches apply?
- What variables substitute?
- Which clusters are affected?
- Let me run flux build locally...
- Wait, which cluster-vars ConfigMap?
- Actually I need to check 5 different files...
```

**ConfigHub:**
```
PR: "Update cert-manager replica count"

cub diff --space platform-team --query "name = 'cert-manager' AND Labels['variant'] = 'prod'"

  apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: cert-manager
  spec:
-   replicas: 2
+   replicas: 3
```

**Reviewer sees exactly the change.** No mental compilation. No local tooling. The diff is the diff.

---

### Problem 5: "Multi-dimensional debugging"

**Traditional — where's the bug?**
```
Production is broken. The issue could be in:

1. Base configuration?
2. Overlay patch not applying?
3. Variable substitution wrong?
4. Dependency chain (cert-manager before ingress)?
5. Kustomize vs Helm interaction?
6. Wrong branch/revision referenced?
```

**ConfigHub — single source of truth:**
```bash
# See exactly what's in the cluster
cub-agent map list --cluster prod --kind Deployment --namespace cert-manager

# Who owns this resource?
cub-agent map list --cluster prod --owner Flux
cub-agent map list --cluster prod --owner ArgoCD
cub-agent map list --cluster prod --owner Helm

# View the diff between Desired (Unit) and Live (Cluster)
cub unit get cert-manager --space platform-team --query "Labels['variant'] = 'prod'" --diff

Desired (Unit):                 Live (Cluster):
replicas: 3                     replicas: 3        ✓
image: cert-manager:v1.14.0     image: v1.13.0     ✗ DRIFT

# Who changed it? When?
cub-agent map history <resource-id>
```

**One place to look.** Desired vs Live. The Unit is the source of truth. Drift is explicit.

---

### Problem 5.5: "Multi-deployer chaos"

**Real-world clusters have multiple deployers:**
```
Cluster: prod-east
├── cert-manager (Helm)
├── ingress-nginx (Argo CD)
├── app-frontend (Flux)
├── app-backend (Flux)
├── monitoring (Terraform)
└── emergency-hotfix (kubectl)
```

**Traditional:** Each tool has its own view. No unified picture.

**ConfigHub Agent detects ownership automatically:**

```bash
$ cub-agent map list --cluster prod-east

KIND        NAME            NAMESPACE       OWNER       STATUS
Deployment  cert-manager    cert-manager    Helm        Synced
Deployment  nginx-ingress   ingress         ArgoCD      Synced
Deployment  frontend        app             Flux        Synced
Deployment  backend         app             Flux        Drifted
Deployment  prometheus      monitoring      Terraform   Synced
Deployment  hotfix-patch    default         Native      Unmanaged
```

**Detection methods:**

| Owner | How Agent Detects |
|-------|-------------------|
| Flux | `kustomize.toolkit.fluxcd.io/*` labels |
| Argo CD | `argocd.argoproj.io/instance` label |
| Helm | `app.kubernetes.io/managed-by: Helm` |
| Terraform | `app.terraform.io/*` annotations |
| ConfigHub | `confighub.com/UnitSlug` label |
| Native/kubectl | OwnerReferences only |

**One view across all deployers.** You see drift regardless of who manages the resource.

---

### Problem 6: "Structure doesn't scale"

**Traditional — adding a new environment:**
```
To add staging-eu-west-2:

1. Create overlay directory: stages-repo/staging-eu-west-2/
2. Create flux-system/infrastructure/*.yaml (copy from similar env)
3. Create flux-system/apps/*.yaml (copy and modify)
4. Create config/values.yaml with all variables
5. Create patches for this environment's quirks
6. Update ApplicationSet selectors or add new ones
7. Hope you didn't miss anything
8. Hope patches still apply to current base
```

**ConfigHub — adding a new environment:**
```bash
# Import config (renders from your existing Kustomization/Helm)
kustomize build overlays/staging | cub import --space platform-team \
    --labels "app=cert-manager,variant=staging-eu-west-2"

# Done. Unit inherits:
# - Hub constraints (automatic)
# - App Space Actions (automatic)
# - App Space reconciliation rules (automatic)
```

**No new directories. No new patch files. No new variable ConfigMaps.**

---

### Problem 7: "Patches break silently"

**Traditional:**
```yaml
# Patch written 6 months ago
patches:
  - patch: |-
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: cert-manager
      spec:
        template:
          spec:
            containers:
            - name: cert-manager
              resources:
                limits:
                  memory: 512Mi
```

**6 months later:** Upstream chart renamed container from `cert-manager` to `controller`. Patch silently does nothing. No error. Production runs without memory limits.

**ConfigHub:**
```yaml
# Hub constraint
constraints:
  - name: require-memory-limits
    match: { kind: Deployment }
    require:
      path: spec.template.spec.containers[*].resources.limits.memory
      exists: true
```

**If the rendered config lacks memory limits, Hub rejects it.** Doesn't matter if upstream renamed things. The constraint checks the final WET manifest.

---

## The Hub + App Space Model — Why It Works

### Traditional "Central Hub" (fails)

```
Central Team creates umbrella charts
    ↓
Teams don't like defaults
    ↓
Teams fork or build their own
    ↓
Central Team has no visibility
    ↓
Divergence, inconsistency, security gaps
```

### ConfigHub Hub + App Space (works)

```
Hub: GOVERNANCE (what you must/can't do)
│
│   Base Space (implicit) — catalog of base units
│   Sources — Git repos, Helm repos
│   Workers — execution agents
│   Targets — clusters/environments
│   Policies/Constraints:
│   - Security requirements (enforced)
│   - Compliance requirements (enforced)
│   - Allowed deployers (Flux, Argo, Bridge)
│   - Allowed reconciliation modes
│
└── App Space: CHOICES (how you operate)
      │
      │   - Which deployer (team picks from allowed)
      │   - Reconciliation strategy (team picks)
      │   - Automation level (team picks)
      │   - Drift handling (team picks)
      │
      └── Unit: CONFIG (WET manifest)
            │
            ├── Labels: app=cert-manager, variant=prod
            └── The actual deployable resource
```

**Why this works:**

| Concern | Who Decides | Why It Doesn't Diverge |
|---------|-------------|------------------------|
| Security | Hub | Enforced on all Units — can't bypass |
| Compliance | Hub | Enforced on all Units — can't bypass |
| Deployer choice | App Space | Multiple valid options — team picks |
| Drift handling | App Space | Multiple valid options — team picks |
| Replica count | Unit | Environment-specific (via labels) — expected to differ |

**Teams don't fork because:**
1. Hub doesn't over-constrain (only what matters)
2. App Space gives real autonomy (choices, not just values)
3. Units hold WET config (no patch hell)
4. Everything is queryable (fleet visibility)

---

## Side-by-Side: Adding cert-manager to 10 Clusters

### Traditional (Argo CD ApplicationSet + Umbrella)

```
1. Create/update umbrella chart (if defaults don't fit)
2. Create values.yaml per cluster (10 files)
3. Update ApplicationSet selector labels
4. Commit all files
5. Wait for Argo to sync
6. Check each Application in UI (10 checks)
7. If something's wrong, debug across:
   - Umbrella chart
   - Per-cluster values
   - ApplicationSet template
   - Label selectors
```

### ConfigHub

```bash
# 1. Import once (renders from your Helm/Kustomize)
helm template cert-manager jetstack/cert-manager | cub import --space platform-team \
    --labels "app=cert-manager,variant=cluster-1"

# 2. Clone to other clusters (copies config, changes variant label)
for i in {2..10}; do
  cub clone --space platform-team \
    --from "Labels['app'] = 'cert-manager' AND Labels['variant'] = 'cluster-1'" \
    --to-variant "cluster-$i"
done

# 3. Adjust per-cluster if needed
cub mutate --space platform-team \
    --query "Labels['app'] = 'cert-manager' AND Labels['variant'] = 'cluster-5'" \
    --set "spec.replicas=5"

# 4. See everything
cub map --query "Labels['app'] = 'cert-manager'"

VARIANT      REPLICAS   IMAGE              STATUS
cluster-1    3          v1.14.0            Synced
cluster-2    3          v1.14.0            Synced
...
cluster-5    5          v1.14.0            Synced
...
cluster-10   3          v1.14.0            Synced
```

**No umbrella charts. No per-cluster values files. No ApplicationSet templating. Fleet-wide visibility in one query.**

---

## Fleet-Wide Queries (The Agent's Superpower)

Neither Argo CD nor Flux can answer these questions across clusters:

**"What version of redis is running across my 50 clusters?"**
```bash
$ cub-agent map list --kind Deployment --all-clusters | grep redis

CLUSTER      NAMESPACE    NAME           IMAGE              OWNER
prod-east    cache        redis          redis:7.2.1        Helm
prod-west    cache        redis          redis:7.2.1        Helm
staging      cache        redis          redis:7.0.0        Flux     # <- old!
dev-1        default      redis          redis:7.2.1        Native
dev-2        default      redis          redis:6.2.0        Native   # <- very old!
```

**"Which deployments are drifted right now?"**
```bash
$ cub-agent map list --status Drifted --all-clusters

CLUSTER      NAMESPACE    NAME           OWNER    DRIFT
prod-east    app          backend        Flux     replicas: 3→5
staging      monitoring   prometheus     Helm     image changed
dev-1        default      hotfix         Native   (unmanaged)
```

**"Who's using the vulnerable image?"**
```bash
$ cub-agent map list --all-clusters | grep "nginx:1.19"

CLUSTER      NAMESPACE    NAME           IMAGE              OWNER
prod-west    ingress      nginx          nginx:1.19.0       ArgoCD   # <- CVE!
staging      ingress      nginx          nginx:1.19.0       ArgoCD   # <- CVE!
```

**"Show me everything Flux manages across all clusters"**
```bash
$ cub-agent map list --owner Flux --all-clusters

CLUSTER      NAMESPACE    NAME           KIND          STATUS
prod-east    app          frontend       Deployment    Synced
prod-east    app          backend        Deployment    Drifted
prod-west    app          frontend       Deployment    Synced
staging      app          frontend       Deployment    Synced
...
```

**This is impossible with native Argo/Flux.** Each instance only knows its own cluster. ConfigHub's Agent aggregates everything into the Map.

---

## Summary

| Pain Point | Traditional GitOps | ConfigHub |
|------------|-------------------|-----------|
| "Can't see what deploys" | Run flux build / helm template locally | Unit shows WET config directly |
| "Teams fork umbrella charts" | Central team loses control | Hub constrains, App Space chooses |
| "Per-cluster values sprawl" | N clusters × M tools = files | Units hold config (labels for organization) |
| "Code review impossible" | Mental compilation required | Diff shows exact change |
| "Debugging is multi-dimensional" | Base? Patch? Variable? Dependency? | Desired vs Live, one place |
| "Adding envs doesn't scale" | New directories, patches, variables | `cub import --labels variant=X`, inherits everything |
| "Patches break silently" | No validation of final output | Hub validates WET manifest |

**The core insight:**

Traditional GitOps stores DRY config and hopes the rendering works. ConfigHub stores WET config and makes the rendering explicit. You always see what deploys.

> **Git says WHAT. ConfigHub says HOW. Cluster says NOW.**
