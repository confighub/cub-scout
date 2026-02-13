# Case Studies: IITS Fleet Patterns

> **Archive status:** Historical planning context (non-canonical for current releases).
> Use these maintained docs for current behavior and scope:
> - `docs/roadmap.md`
> - `docs/reference/import-docs-crosswalk.md`
> - `docs/reference/hub-appspace-examples.md`

Real-world enterprise GitOps problems from IITS consulting, mapped to ConfigHub solutions.

Based on:
- `usecase-argocd-fleet-iits.pdf` — Argo CD fleet patterns
- `usecase-fluxcd-fleet-iits.pdf` — Flux CD fleet patterns

---

## The Hub-and-Spoke Topology

Enterprise GitOps typically follows this pattern:

```
┌─────────────────────────────────────────────────────────────────┐
│ Hub Cluster (Controlplane)                                       │
│                                                                  │
│   ┌─────────────────┐     ┌─────────────────────────────────┐  │
│   │ Helm Umbrella   │────▶│ ApplicationSet                  │  │
│   │ Charts (Catalog)│     │   generators:                   │  │
│   └─────────────────┘     │     - clusters:                 │  │
│                           │         selector:               │  │
│                           │           cert-manager: enabled │  │
│                           └───────────────┬─────────────────┘  │
└───────────────────────────────────────────┼─────────────────────┘
                                            │
              ┌─────────────────────────────┼─────────────────────┐
              ▼                             ▼                     ▼
      ┌───────────────┐           ┌───────────────┐      ┌───────────────┐
      │ Dataplane-0   │           │ Dataplane-1   │      │ Dataplane-N   │
      └───────────────┘           └───────────────┘      └───────────────┘
```

**Key insight:** Labels on clusters determine what gets deployed.

---

## The Problems (Direct Quotes)

### From Flux CD Users

| Problem | Quote |
|---------|-------|
| **Visibility** | "What you see in the Git repository isn't what actually gets deployed" |
| **Mental compilation** | "To understand what's actually running in production, you need to mentally compile all these layers or run flux build for each kustomization" |
| **Code review impossible** | "Reviewers can't easily see the impact of a change without building the manifests locally" |
| **Multi-dimensional debugging** | "The issue could be in the base configuration, in a patch that's not applying correctly, in a variable substitution that's missing or wrong, or in the dependency chain" |
| **Structure doesn't scale** | "Every new environment or region typically needs its own overlay directory with its own set of patches and variables" |
| **Silent breakage** | "Hundreds of patch files that can break silently when the base resources change structure" |

### From Argo CD Users

| Problem | Quote |
|---------|-------|
| **Umbrella divergence** | "Many teams end up building their own 'central hub' because they do not like the defaults from the central team, or because the umbrella chart is missing features" |
| **Per-cluster sprawl** | Values file per cluster per tool — explosion of config files |
| **Hydration complexity** | "They 'hydrate' the resources in the pipeline... commit the generated manifests to Git" |

---

## Problem 1: "What you see isn't what deploys"

### Traditional GitOps

```
Git (DRY)
├── base/cert-manager/          ← What you see
├── overlays/dev/patches/       ← + this
├── cluster-vars ConfigMap      ← + this
└── postBuild substitutions     ← + this
                                   = ??? (run flux build to find out)
```

### ConfigHub Solution

```
Git (DRY)                        ConfigHub Unit (WET)
├── base/cert-manager/     →     └── Unit: cert-manager
├── overlays/dev/                    (app=cert-manager, variant=dev)
└── values.yaml                      └── EXACTLY what deploys
         │                           └── No mental compilation
         └── render (once) ──────────────────┘
```

**The Unit stores WET manifests.** What you see is what deploys.

### Try It

```bash
# See what's actually running
cub-agent map

# Compare what's in Git vs what deployed
cub unit diff cert-manager --variant dev --vs-source
```

---

## Problem 2: Umbrella Charts Diverge

> Scope note (current `cub-scout`): Hub constraints/policy enforcement shown below are ConfigHub platform behavior.
> `cub-scout` may surface policy context but does not enforce policy.

### Traditional Pattern

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

### ConfigHub Solution

```yaml
Hub: platform-standards (CONSTRAINTS)

  MUST: All cert-manager installs use approved ClusterIssuer
  MUST: Resource limits on all pods
  CAN'T: No self-signed certs in prod
  CAN: Teams may choose replica count
  CAN: Teams may add custom annotations
```

```yaml
App Space: team-a (CHOICES within constraints)

  Unit: cert-manager (variant=prod)
    # Team's choices — Hub validates
    replicas: 3
    custom-annotation: "our-value"
    # Hub constraint: uses approved ClusterIssuer ✓
```

**Hub prevents bad outcomes. Teams have autonomy within guardrails.**

### Try It

```bash
# See Hub constraints
cub hub show constraints

# Validate a unit against Hub
cub unit validate cert-manager --against-hub
```

---

## Problem 3: Per-Cluster Values Sprawl

### Traditional Pattern

```
customer-service-catalog/helm/
├── cluster-1/cert-manager/values.yaml
├── cluster-1/ingress-nginx/values.yaml
├── cluster-2/cert-manager/values.yaml
├── cluster-2/ingress-nginx/values.yaml
├── cluster-3/cert-manager/values.yaml
...
# 50 clusters × N apps = explosion
```

### ConfigHub Solution

Labels replace folders:

```
App Space: platform-team
├── Unit: cert-manager (variant=prod, region=us-east)
├── Unit: cert-manager (variant=prod, region=us-west)
├── Unit: cert-manager (variant=prod, region=eu)
└── Unit: cert-manager (variant=staging, region=us-east)
```

Query instead of navigate:

```bash
# All prod cert-manager instances
cub query "Labels['app'] = 'cert-manager' AND Labels['variant'] = 'prod'"

# Cert-manager in EU
cub query "Labels['app'] = 'cert-manager' AND Labels['region'] = 'eu'"
```

### Try It

```bash
# Import with auto-labeling
cub-agent import --model hub-appspace --dry-run

# Query across fleet
cub-agent map list -q "app=cert-manager"
```

---

## Problem 4: Silent Patch Breakage

### Traditional Pattern

```yaml
# patches/production/deployment-patch.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
spec:
  template:
    spec:
      containers:
      - name: app
        resources:
          limits:
            memory: 1Gi  # Patch to increase memory
```

**Problem:** If base changes container name from `app` to `main`, patch silently fails.

### ConfigHub Solution

**Structural validation at import time:**

```
$ cub import --from git@github.com:org/app.git

ERROR: Patch target not found
  Patch: patches/production/deployment-patch.yaml
  Target: spec.template.spec.containers[name=app]
  Available: spec.template.spec.containers[name=main]

Hint: Base changed container name from 'app' to 'main'
```

**Hub catches the mismatch before it reaches the cluster.**

---

## Problem 5: Trigger/Action Divergence

> Scope note (current `cub-scout`): actions/triggers here are platform workflow engine examples,
> not direct `cub-scout` feature commitments.

### Traditional Pattern

```yaml
# dev-space/triggers.yaml
triggers:
  - on-commit: auto-deploy

# staging-space/triggers.yaml
triggers:
  - on-commit: auto-deploy
  - on-deploy: run-tests

# prod-space/triggers.yaml
triggers:
  - on-approval: deploy
  - on-deploy: run-smoke-tests
  - on-deploy: notify-slack
```

**Problem:** Three places to maintain. They diverge over time.

### ConfigHub Solution

One App Space, label filters:

```yaml
App Space: my-team
actions:
  - name: deploy
    filter: "Labels['variant'] = 'dev'"
    trigger: on-commit

  - name: deploy
    filter: "Labels['variant'] = 'staging'"
    trigger: on-commit
    post: run-tests

  - name: deploy
    filter: "Labels['variant'] = 'prod'"
    trigger: on-approval
    post: [run-smoke-tests, notify-slack]
```

**Actions defined once. Label filters determine behavior.**

---

## Problem 6: No Fleet-Wide Queries

> Scope note: saved-query lifecycle examples below describe platform query features.
> Use `cub-scout` command docs for currently shipped local/connected query surfaces.

### Traditional Pattern

```bash
# "Which clusters have cert-manager v1.12?"
for cluster in $(get-all-clusters); do
  kubectl --context=$cluster get helmrelease cert-manager -o jsonpath='{.spec.chart.spec.version}'
done
# Slow, error-prone, no saved queries
```

### ConfigHub Solution

```bash
# Fleet query
cub query "Labels['app'] = 'cert-manager'" --show-versions

# Save the query
cub query save "cert-manager-versions" \
  --query "Labels['app'] = 'cert-manager'" \
  --columns "variant,region,version"

# Run saved query
cub query @cert-manager-versions
```

---

## Summary: Problems → Solutions

![IITS Problems → Hub/App Space Solutions](images/iits-problems-solutions.png)

| IITS Problem                      | Pain                                                                         | Hub/App Space Solution                                                       |
|-----------------------------------|------------------------------------------------------------------------------|------------------------------------------------------------------------------|
| **Umbrella chart divergence**     | "Teams fork because they don't like defaults or umbrella is missing features" | Clone from Hub with full customization; upstream tracking shows what changed |
| **Per-cluster values sprawl**     | 50 clusters × N apps = explosion of values files                             | Labels (variant=prod, region=eu) replace folder hierarchy                    |
| **"What you see isn't what deploys"** | Mental compilation of Kustomize overlays, patches, generators            | WET manifests in Units — what you see IS what deploys                        |
| **Silent patch breakage**         | Overlay changes break silently, discovered in prod                           | Hub validates on import; drift detection catches mismatches                  |
| **Multi-tool chaos**              | Flux + Argo + Helm + kubectl in same cluster                                 | Agent detects all owners in one view; Hub can enforce deployer policy        |
| **Can't query fleet**             | "What version of redis across 50 clusters?"                                  | `cub map --query "image contains redis"` — one command                       |
| **Hotfix → Git hell**             | kubectl edit in prod, then manually sync back to Git                         | `cub drift accept` — one command, Git PR created automatically               |
| **Trigger/action divergence**     | "Actions differ across environments"                                         | Actions on App Space with label filters — one definition, filter by variant  |
| **No ownership boundaries**       | Platform vs app team responsibilities unclear                                | Hub = platform constraints; App Space = team choices within constraints      |
| **Structure doesn't scale**       | Nested Kustomizations become unmaintainable                                  | Flat Units with labels; query by any dimension                               |

> **IITS delivers:** Managed Service Catalog → Customer Service Catalog
>
> **ConfigHub maps to:** Hub (base units + constraints) → App Space (cloned units + choices)

---

## Use Cases: Real Questions Answered

> Command note: examples in this section mix historical `cub`/`cub-agent` flows.
> For current `cub-scout` commands and flags, use `docs/reference/commands.md`.

### "What's running across all my clusters?"

```bash
$ cub map

  ✓ ALL HEALTHY   prod-east

  Deployers  4/4 ✓
  Workloads  17/17 ✓

  OWNERSHIP
  ────────────────────────────────────────────────
  Flux(8) Argo(4) ConfigHub(3) Helm(2) Native(0)
  ████████████████████░░░░░░░░
```

### "Who owns each workload?"

```bash
$ cub map workloads

STATUS  NAMESPACE     NAME            OWNER      MANAGED-BY        IMAGE
────────────────────────────────────────────────────────────────────────
✓     payments      payment-api     ConfigHub  payment-api-prod  api:2.4.1
✓     orders        order-service   Flux       orders-flux       order:1.8.0
✓     monitoring    prometheus      Argo       prometheus-app    prom:2.47
```

### "Which clusters are behind on this app?"

```bash
$ cub query "Labels['app'] = 'payment-api'"

App Space: payments-team

  payment-api (variant=prod)
  ├── ✓ us-east @ rev 127
  ├── ✓ us-west @ rev 127
  └── ⚠ eu-west @ rev 124    ← behind!

  payment-api (variant=staging)
  └── ✓ staging @ rev 130
```

### "What's broken right now?"

```bash
$ cub map problems

✗ HelmRelease/redis-cache in flux-system: SourceNotReady
⏸ Kustomization/monitoring-stack in flux-system: suspended
✗ Deployment/order-processor in orders: 0/2 ready
```

### "Find all prod resources managed by GitOps tools"

```bash
$ cub query "Labels['variant'] = 'prod' AND owner != 'Native'"

NAMESPACE     KIND        NAME              OWNER      VARIANT
─────────────────────────────────────────────────────────────
payments      Deployment  payment-api       ConfigHub  prod
payments      Deployment  payment-worker    ConfigHub  prod
orders        Deployment  order-api         Flux       prod
monitoring    Deployment  prometheus        Argo       prod
```

### "Find orphaned resources (kubectl'd at 2am)"

```bash
$ cub query "owner = 'Native' AND namespace != 'kube-system'"

NAMESPACE     KIND        NAME              OWNER
───────────────────────────────────────────────────
staging       Deployment  debug-pod         Native    ← who did this?
payments      ConfigMap   emergency-fix     Native
```

---

## The Global App Pattern

For organizations with truly global deployments, here's how to organize:

```
Hub: platform-standards
│
└── App Space: global-api-team
      │
      │  # Base (template for all regions)
      ├── Unit: api-gateway (variant=base)
      │
      │  # Americas
      ├── Unit: api-gateway (variant=prod, region=us-east)
      ├── Unit: api-gateway (variant=prod, region=us-west)
      ├── Unit: api-gateway (variant=prod, region=sa-east)
      │
      │  # Europe
      ├── Unit: api-gateway (variant=prod, region=eu-west)
      ├── Unit: api-gateway (variant=prod, region=eu-central)
      │
      │  # Asia-Pacific
      ├── Unit: api-gateway (variant=prod, region=asia-east)
      ├── Unit: api-gateway (variant=prod, region=asia-south)
      │
      │  # Non-prod (single instance each)
      ├── Unit: api-gateway (variant=staging)
      └── Unit: api-gateway (variant=dev)
```

**Query patterns:**

```bash
# All prod instances globally
cub query "Labels['app'] = 'api-gateway' AND Labels['variant'] = 'prod'"

# All Americas instances
cub query "Labels['region'] LIKE 'us-%' OR Labels['region'] LIKE 'sa-%'"

# All EU prod instances
cub query "Labels['variant'] = 'prod' AND Labels['region'] LIKE 'eu-%'"

# Specific region
cub query "Labels['region'] = 'asia-east'"
```

---

## The IITS Hub-and-Spoke Pattern

For enterprises with central platform teams and multiple application teams:

```
Hub: managed-service-catalog
│   "Platform team's golden configs"
│
│   Constraints:
│   - All containers must have resource limits
│   - All secrets via External Secrets Operator
│   - Approved base images only
│
├── App Space: payments-team
│     │
│     │  Cloned from Hub + customized
│     ├── Unit: cert-manager (variant=prod)     ← cloned from Hub
│     ├── Unit: ingress-nginx (variant=prod)    ← cloned from Hub
│     ├── Unit: payment-api (variant=prod)      ← team's own
│     └── Unit: payment-api (variant=dev)
│
├── App Space: orders-team
│     │
│     ├── Unit: cert-manager (variant=prod)     ← cloned from Hub
│     ├── Unit: ingress-nginx (variant=prod)    ← cloned from Hub
│     └── Unit: order-service (variant=prod)    ← team's own
│
└── App Space: data-team
      │
      ├── Unit: cert-manager (variant=prod)     ← cloned from Hub
      └── Unit: data-pipeline (variant=prod)    ← team's own
```

**Why this works:**

1. **Platform team** maintains Hub with base configs
2. **Teams clone** cert-manager, ingress-nginx, etc. from Hub
3. **Teams can't bypass** Hub constraints (security, compliance)
4. **Teams can customize** within constraints (replica count, resources)
5. **Updates flow** from Hub → clones via explicit `cub unit pull`

**No umbrella chart forks.** Teams clone, customize, and pull updates explicitly.

---

## Demo: Try It Yourself

> Historical note: `./test/atk/*` scripts and some example repos are legacy demo scaffolding.
> Treat these as reference context, not current release-contract automation.

### Quick Demo (30 seconds)

```bash
# Clone and run
git clone https://github.com/confighubai/confighub-agent.git
cd confighub-agent
./test/atk/demo quick
```

### IITS-Style Demo

```bash
# Set up test cluster with Flux + Argo
./test/atk/setup-cluster

# See the map
./test/atk/map

# Scan for risks
./test/atk/scan

# See how import would organize it
cub-agent import --model hub-appspace --dry-run
```

### Working Examples

| Example | Repo | Pattern |
|---------|------|---------|
| ArgoCD integration | `confighubai/examples-internal/argocd` | Hub-and-spoke |
| FluxCD integration | `confighubai/examples-internal/fluxcd` | Kustomize-based |
| Multi-cluster global app | `confighub/examples/global-app` | Multi-region labels |

See [examples/](../../archive/test/atk/examples) for runnable demos.

---

## See Also

- [01-MAP-CONCEPT.md](01-MAP-CONCEPT.md) — The Map explained
- [02-HUB-APPSPACE-MODEL.md](02-HUB-APPSPACE-MODEL.md) — Hub constraints, App Space choices
- [05-THREE-SOURCES-OF-TRUTH.md](05-THREE-SOURCES-OF-TRUTH.md) — Git, ConfigHub, Cluster
- [07-MODEL-MIGRATION.md](07-MODEL-MIGRATION.md) — Migrating existing setups

**Original analysis docs (for deep dives):**
- [confighub-solves-gitops-pain-iits.md](https://github.com/confighubai/confighub-agent/search?q=confighub-solves-gitops-pain-iits.md)
- [flux-argo-pdf-solution-value-analysis-iits.md](https://github.com/confighubai/confighub-agent/search?q=flux-argo-pdf-solution-value-analysis-iits.md)
- [how-maps-design-helps-artem-25-questions-iits.md](https://github.com/confighubai/confighub-agent/search?q=how-maps-design-helps-artem-25-questions-iits.md)

---

**Next:** [09-EXTENSION-POINTS.md](09-EXTENSION-POINTS.md) — Future Hub and App Space capabilities
