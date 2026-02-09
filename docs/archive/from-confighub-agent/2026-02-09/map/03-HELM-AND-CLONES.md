# Helm Charts, Clones, and Upstream Tracking

Practical patterns for managing variants without duplication.

---

## How Helm Charts Work

ConfigHub uses the **Rendered Manifests Pattern** — we render Helm charts to concrete YAML, not HelmRelease CRDs.

### Why Render?

Traditional Helm:
```
Chart values → HelmRelease → Helm runtime → Rendered manifests → Cluster
                                ↑
                    "What actually got deployed?"
                    "Run helm template to find out"
```

ConfigHub:
```
Chart values → cub helm install → Rendered manifests (stored) → Cluster
                                          ↑
                            "What actually got deployed?"
                            "Look at the Unit"
```

**What you see is what deploys.** No runtime rendering. No surprises.

### Installing a Helm Chart

```bash
# Render and store as a Unit
cub helm install redis bitnami/redis \
  --space payments-team \
  --namespace payments \
  --values values.yaml \
  --labels "app=redis,variant=prod,region=us-east"
```

This:
1. Pulls the Helm chart
2. Renders it with your values
3. Stores the **rendered output** as a Unit
4. Deploys the WET manifests to the cluster

### Upgrading a Helm Chart

```bash
# Update the base chart
cub helm upgrade redis bitnami/redis --version 18.7.0

# Preview what changed
cub unit diff redis --space payments-team

# Apply the upgrade
cub unit apply redis --space payments-team
```

---

## Clones and Upstream Tracking

The key to managing variants without duplication: **clone with upstream tracking**.

### The Problem

Traditional approach:
```
redis-values-prod-us-east.yaml
redis-values-prod-eu-west.yaml
redis-values-prod-asia-pac.yaml
redis-values-staging.yaml
redis-values-dev.yaml

# Someone updates one. Others drift. Chaos.
```

### The Solution: Clone + Upstream

```bash
# 1. Create base Unit from Helm chart
cub helm install redis bitnami/redis \
  --space payments-team \
  --labels "app=redis,variant=base"

# 2. Clone for each variant with upstream tracking
cub unit clone redis \
  --from-labels "app=redis,variant=base" \
  --to-labels "app=redis,variant=prod,region=us-east" \
  --upstream   # Track the base

cub unit clone redis \
  --from-labels "app=redis,variant=base" \
  --to-labels "app=redis,variant=prod,region=eu-west" \
  --upstream

cub unit clone redis \
  --from-labels "app=redis,variant=base" \
  --to-labels "app=redis,variant=staging" \
  --upstream
```

### What Upstream Tracking Gives You

```bash
# See what's different from upstream
cub unit diff redis --labels "variant=prod,region=us-east" --vs-upstream

# Pull updates from upstream (explicit, not automatic)
cub unit pull redis --labels "variant=prod,region=us-east"

# Pull updates to all prod variants
cub unit pull --query "Labels['app'] = 'redis' AND Labels['variant'] = 'prod'"
```

**Updates are explicit.** The base changes. You decide when each variant pulls those changes.

### Customizing Clones

Clones can differ from their upstream:

```bash
# Clone with overrides
cub unit clone redis \
  --from-labels "app=redis,variant=base" \
  --to-labels "app=redis,variant=prod,region=us-east" \
  --upstream \
  --set "spec.replicas=5" \
  --set "resources.requests.memory=2Gi"
```

The clone tracks upstream but keeps your customizations. When you pull:
- Upstream changes merge in
- Your customizations are preserved
- Conflicts are flagged for resolution

---

## Regions and Stages

Regions and stages are just labels. Here's how to think about them:

### Stage = Variant Label

Stages represent your deployment pipeline:

```
variant=dev      → Development (frequent changes, drift OK)
variant=staging  → Staging (pre-prod testing)
variant=prod     → Production (stable, drift reverted)
```

### Region = Region Label

Regions represent geographic deployment:

```
region=us-east   → US East Coast
region=us-west   → US West Coast
region=eu-west   → Europe West
region=asia-pac  → Asia Pacific
```

### Combining Them

A single App Space contains all combinations:

```
App Space: payments-team
├── Unit: redis (variant=dev)                           # Dev, one region
├── Unit: redis (variant=staging)                       # Staging, one region
├── Unit: redis (variant=prod, region=us-east)          # Prod US East
├── Unit: redis (variant=prod, region=us-west)          # Prod US West
├── Unit: redis (variant=prod, region=eu-west)          # Prod EU
└── Unit: redis (variant=prod, region=asia-pac)         # Prod Asia
```

### Per-Label Reconciliation

The App Space can define different behavior by label:

```yaml
App Space: payments-team
  reconciliation:
    # Dev: anything goes
    - match: { variant: dev }
      drift: accept
      approval: none

    # Staging: revert drift, auto-deploy
    - match: { variant: staging }
      drift: revert
      approval: none

    # Prod US: revert drift, require approval
    - match: { variant: prod, region: us-east }
      drift: revert
      approval: required
      approvers: [platform-team, payments-lead]

    # Prod EU: same as US, different approvers
    - match: { variant: prod, region: eu-west }
      drift: revert
      approval: required
      approvers: [platform-team-eu, payments-lead]
```

---

## Complete Example: Payment API Across Regions

Let's walk through setting up a payment API across multiple regions and stages.

### Step 1: Platform Team Creates Hub

```yaml
# Created by platform team
Hub: acme-platform
  constraints:
    - name: require-resource-limits
      match: { kind: Deployment }
      require:
        path: spec.template.spec.containers[*].resources.limits
        exists: true

    - name: no-latest-in-prod
      match: { kind: Deployment }
      when: Labels['variant'] = 'prod'
      deny:
        path: spec.template.spec.containers[*].image
        pattern: ":latest$"

  allowed_deployers: [flux, argo]
```

### Step 2: Payments Team Creates App Space

```bash
cub space create payments-team \
  --owner payments@acme.com \
  --deployer flux
```

### Step 3: Install Base Application

```bash
# Install from Helm chart as base
cub helm install payment-api ./charts/payment-api \
  --space payments-team \
  --labels "app=payment-api,variant=base" \
  --values base-values.yaml
```

### Step 4: Clone for Each Environment/Region

```bash
# Dev (single instance)
cub unit clone payment-api \
  --from-labels "variant=base" \
  --to-labels "variant=dev" \
  --upstream \
  --set "spec.replicas=1"

# Staging (single instance, more resources)
cub unit clone payment-api \
  --from-labels "variant=base" \
  --to-labels "variant=staging" \
  --upstream \
  --set "spec.replicas=2"

# Prod US East (full resources)
cub unit clone payment-api \
  --from-labels "variant=base" \
  --to-labels "variant=prod,region=us-east" \
  --upstream \
  --set "spec.replicas=5" \
  --set "resources.requests.memory=2Gi"

# Prod EU West (same as US, EU-specific config)
cub unit clone payment-api \
  --from-labels "variant=base" \
  --to-labels "variant=prod,region=eu-west" \
  --upstream \
  --set "spec.replicas=5" \
  --set "env.REGION=eu-west"
```

### Step 5: Configure Reconciliation Rules

```bash
cub space configure payments-team --reconciliation - <<EOF
rules:
  - match: { variant: dev }
    drift: accept
    approval: none

  - match: { variant: staging }
    drift: revert
    approval: none

  - match: { variant: prod }
    drift: revert
    approval: required
    approvers: [payments-lead]
EOF
```

### Step 6: Day-to-Day Operations

```bash
# See all payment-api variants
cub query "Labels['app'] = 'payment-api'" --space payments-team

# Update base chart
cub helm upgrade payment-api ./charts/payment-api --version 2.0.0

# Preview changes to prod variants
cub unit diff --query "Labels['app']='payment-api' AND Labels['variant']='prod'" --vs-upstream

# Pull to staging first
cub unit pull --query "Labels['app']='payment-api' AND Labels['variant']='staging'"

# After validation, pull to all prod
cub unit pull --query "Labels['app']='payment-api' AND Labels['variant']='prod'"
```

---

## See Also

- [02-HUB-APPSPACE-MODEL.md](02-HUB-APPSPACE-MODEL.md) — Core definitions
- [06-MERGES-AND-WRITE-FLOWS.md](06-MERGES-AND-WRITE-FLOWS.md) — Reconciliation strategies
- [07-MODEL-MIGRATION.md](07-MODEL-MIGRATION.md) — Migrating to Hub/App Space model

---

**Next:** [04-MAP-USER-JOURNEY-TO-FULL-CONFIGHUB.md](04-MAP-USER-JOURNEY-TO-FULL-CONFIGHUB.md) — Adoption path from standalone to full platform
