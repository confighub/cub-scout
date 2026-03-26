# Migration Playbook: Argo/Helm to ConfigHub

A guided playbook for migrating existing Kubernetes workloads into ConfigHub.
Covers assessment, planning, execution, validation, and rollback.

> **Prerequisite reading:**
> - [Import from Live](import-from-live.md) — How cub-scout discovers workloads
> - [Import to ConfigHub](import-to-confighub.md) — The canonical 7-step import path
> - [Glossary](../reference/glossary.md) — ConfigHub terminology
>
> **Ownership:** Connected `cub` commands come from the [ConfigHub SDK](https://github.com/confighub/sdk).
> See [Interface Boundaries](../concepts/why-connected-mode.md#interface-boundaries-authoritative) for the cub-scout/cub split.

---

## ConfigHub Model

ConfigHub uses app-centric language:

| Concept | What It Is |
|---------|-----------|
| **App** | A logical application (e.g., payment-service) with components (api, worker, db) |
| **Deployment** | An App deployed to a specific Target (App × environment) |
| **Target** | A Kubernetes cluster managed by ConfigHub |

The operating boundary:
- **ConfigHub** stores and publishes explicit intended state + provenance
- **Flux/Argo** reconcile runtime (ConfigHub does not replace your deployer)
- **cub-scout** reports reality and drift (read-only observer)

OCI is the default transport: Git → ConfigHub renders → OCI artifact → Flux/Argo pulls → cluster.

> **API note:** The current `cub` CLI uses Space/Unit commands while APIs evolve.
> App/Deployment concepts map to Space/Unit under the hood.
> See [Glossary — Emerging Concepts](../reference/glossary.md#emerging-concepts-confighub-evolution) for details.

---

## Who This Is For

You have a running Kubernetes cluster with workloads managed by ArgoCD, Helm, Flux, or a mix of these. You want to bring those workloads under ConfigHub management without disrupting production.

This playbook assumes:
- You have `kubectl` access to the cluster
- You have `cub-scout` installed (v1.0+)
- You have a ConfigHub account (`cub auth login`)

---

## Decision: Should You Migrate?

ConfigHub adds value when you need:

| Need | Without ConfigHub | With ConfigHub |
|------|-------------------|----------------|
| **Understand what's running** | `cub-scout map` (local, single cluster) | Same — cub-scout works standalone |
| **Track config history** | Git log per repo, manual correlation | Unified revision history across Deployments |
| **Compare environments** | Manual kubectl/diff per namespace | Query across Deployments by label |
| **Multi-cluster visibility** | Repeat cub-scout per cluster | Fleet queries across all Targets |
| **Break-glass audit trail** | Hope someone documented the hotfix | Versioned accept/reject with context |

**Start with standalone cub-scout.** Migrate to ConfigHub when you need durable history, cross-cluster views, or managed lifecycle.

---

## Phase 1: Assess (Read-Only)

Time: 15–30 minutes. No changes to cluster or ConfigHub.

### 1.1 Scan Your Cluster

```bash
# Build cub-scout if needed
go build ./cmd/cub-scout

# Full cluster scan — what's running and who owns it?
./cub-scout map list

# Focus on a specific namespace
./cub-scout map list -n payments

# Ownership breakdown
./cub-scout map list -q "owner=ArgoCD"
./cub-scout map list -q "owner=Helm"
./cub-scout map list -q "owner=Native"
```

### 1.2 Identify Ownership Boundaries

Look for these patterns in the output:

| Pattern | What It Means | Migration Approach |
|---------|---------------|-------------------|
| Single owner per namespace | Clean boundary | Import namespace as one App |
| Mixed owners in one namespace | Shared namespace | Import by owner, not namespace |
| ArgoCD App-of-Apps parent | Orchestration layer | Import child apps; parent is metadata |
| Helm releases with no GitOps | Direct `helm install` | Import as-is; consider adding Git source later |
| Native/unmanaged workloads | No controller | Import as-is; ConfigHub becomes first controller |

### 1.3 Check for Blockers

Before proceeding, verify:

```bash
# Can you reach ConfigHub?
./cub-scout status

# Is your auth valid?
cub auth get-token
cub context get
```

If `cub-scout status` shows **Offline** or **Standalone**, you need to authenticate first:

```bash
cub auth login
```

---

## Phase 2: Plan Your Mapping

Time: 30–60 minutes. Design the App/Deployment structure before importing.

### 2.1 Map Namespaces to Apps

Think in terms of Apps (what your team calls "a service") and Deployments (where it runs):

```
Kubernetes workload group → ConfigHub App
Namespace × environment   → ConfigHub Deployment
Cluster                   → ConfigHub Target
```

This isn't always 1:1. Consider these patterns:

**Pattern A: One namespace = one App** (simplest)

```
Namespace: payments-prod
  → App: payments
    → Deployment: payments-prod (variant=prod)
      Components: payment-api, order-svc
```

**Pattern B: Multiple namespaces = one App** (environment split)

```
Namespace: payments-dev     → App: payments, Deployment: payments-dev (variant=dev)
Namespace: payments-staging → App: payments, Deployment: payments-staging (variant=staging)
Namespace: payments-prod    → App: payments, Deployment: payments-prod (variant=prod)
```

**Pattern C: Platform + App split** (infra separation)

```
Namespace: cert-manager      → App: platform-infra (platform-owned)
Namespace: ingress-nginx     → App: platform-infra (platform-owned)
Namespace: payments-prod     → App: payments (team-owned)
Namespace: orders-prod       → App: orders (team-owned)
```

### 2.2 Use cub-scout's Proposal

cub-scout can suggest the mapping for you:

```bash
# Dry-run import for one namespace
./cub-scout import -n payments-prod --dry-run

# See the structured proposal
./cub-scout import -n payments-prod --dry-run --json
```

Review the proposal:
- Are the suggested App names sensible?
- Do the component names match your team's vocabulary?
- Are variants correctly inferred (prod, staging, dev)?

If the proposal is wrong, don't import yet. Adjust your labels or naming strategy first.

> **API note:** The dry-run output currently uses Space/Unit terminology.
> Read "Space" as "where the App's Deployments live" and "Unit" as "a component of the App."

### 2.3 Document Your Decisions

Before executing, write down:

1. **Which namespaces** are in scope for this migration wave
2. **App names** you'll use (or let cub-scout suggest)
3. **Component naming convention** (e.g., `{service}-{variant}` or just `{service}`)
4. **Label strategy**: which labels map to `variant`, `region`, `team`
5. **What stays outside**: namespaces or workloads you'll migrate later

---

## Phase 3: Execute (One Namespace at a Time)

Time: 10–15 minutes per namespace.

### 3.1 Start With Your Simplest Namespace

Pick a namespace that has:
- Few workloads (2–5)
- Single ownership (all Argo, all Helm, or all Native)
- Non-critical environment (dev or staging)

```bash
# Final dry-run check
./cub-scout import -n payments-dev --dry-run

# Execute import
./cub-scout import -n payments-dev
```

Or non-interactive:

```bash
./cub-scout import -n payments-dev -y
```

### 3.2 Verify Immediately

After import, verify the result:

```bash
# Check the import created the expected state in ConfigHub
cub unit list --space payments-team

# Verify cub-scout sees the new ownership
./cub-scout map list -n payments-dev

# Check ownership chain
./cub-scout trace deploy/payment-api -n payments-dev
```

**Success criteria:**
- Expected workloads exist in ConfigHub
- Workload ownership context is coherent (no "unknown" or incorrect owners)
- No unexpected resource drift

### 3.3 Keep Your Existing Deployer Running

**Do not remove ArgoCD or Helm control yet.**

The import registers your workloads with ConfigHub. Your existing deployer (Argo, Helm, Flux) continues to reconcile. This is intentional — you validate ConfigHub's view before changing who controls what.

During this validation window:
- ConfigHub knows about your workloads (Deployments exist)
- Your deployer still manages the actual cluster state
- There is no conflict because cub-scout and ConfigHub are read-only observers

---

## Phase 4: Validate

Time: 1–3 days (or however long you need confidence).

### 4.1 Post-Import Checklist

Run through this checklist for each migrated namespace:

| Check | Command | Expected |
|-------|---------|----------|
| Workloads imported | `cub unit list --space <space>` | All expected workloads listed |
| Ownership visible | `./cub-scout map list -n <ns>` | ConfigHub ownership shown |
| Trace works | `./cub-scout trace deploy/<name> -n <ns>` | Full ownership chain |
| No false orphans | `./cub-scout map list -q "owner=Native" -n <ns>` | Only genuinely unmanaged workloads |
| Variants correct | `cub unit list --where "Labels.variant='prod'"` | Correct environment tagging |
| Status healthy | `./cub-scout status` | Connected, auth valid |

### 4.2 Handle Mixed Ownership

If a namespace has workloads from multiple owners:

```bash
# See the ownership breakdown
./cub-scout map list -n mixed-namespace -q "owner=ArgoCD"
./cub-scout map list -n mixed-namespace -q "owner=Helm"
./cub-scout map list -n mixed-namespace -q "owner=Native"
```

Import each owner group separately. In ConfigHub, they'll all be components of the same App but with different provenance.

The key rule: **one controller-of-record per workload**. After import, each workload should have exactly one system reconciling its state. Mixed ownership in the same namespace is fine; mixed control of the same workload is not.

### 4.3 Validate ArgoCD App-of-Apps

If you have App-of-Apps or ApplicationSet patterns:

```bash
# See the hierarchy
./cub-scout tree ownership -n argocd

# List ApplicationSet-generated apps
./cub-scout map list -q "owner=ArgoCD"
```

Import rules for Argo hierarchies:
- **Child Applications** (the ones that manage actual workloads) → import as App components
- **Parent Application** (App-of-Apps root) → treat as orchestration metadata, not imported
- **ApplicationSet** → treat as a generator/template, not imported

The parent/generator creates child apps; ConfigHub imports the children.

---

## Phase 5: Cut Over

Time: varies by team.

### 5.1 When to Cut Over

Cut over when:
- All post-import checks pass
- Team is comfortable with ConfigHub's view of their workloads
- You've decided on controller-of-record per workload

### 5.2 Cut-Over Order

1. **Update team policy** — document which system is controller-of-record
2. **Configure ConfigHub Targets** — point workers at the right clusters
3. **Remove duplicate reconciliation** — stop Argo/Helm from reconciling workloads that ConfigHub now manages

**Do not skip step 1.** Policy clarity prevents the "two systems reconciling the same manifest" problem.

### 5.3 What NOT to Do

- Do not delete ArgoCD Applications before ConfigHub workers are running
- Do not remove Helm releases while they're the only reconciliation source
- Do not cut over production and dev simultaneously — stagger by environment
- Do not assume "it worked in staging" means production is ready

---

## Phase 6: Expand

### 6.1 Next Namespace

After your first namespace is stable:

```bash
# Repeat for the next namespace
./cub-scout import -n payments-staging --dry-run
./cub-scout import -n payments-staging -y
```

### 6.2 Recommended Expansion Order

1. **Dev** (lowest risk, fastest feedback)
2. **Staging** (validates the pattern with more realistic workloads)
3. **Production non-critical** (internal tools, batch jobs)
4. **Production critical** (revenue-path services, last)

### 6.3 Fleet Expansion

Once multiple clusters are imported:

```bash
# Cross-cluster queries (connected mode)
cub unit list --where "Labels.app='payment-api'"

# Compare environments
cub unit list --where "Labels.variant='prod'" --where "Labels.variant='staging'"
```

---

## Rollback

If an import doesn't look right, roll back before cutting over:

```bash
# List what was imported
cub unit list --space <space>

# Remove specific components
cub unit delete <unit-slug> --space <space>

# Or remove the entire space
cub space delete <space-slug>
```

Your existing Argo/Helm/Flux deployer is still running. Removing ConfigHub state doesn't affect the cluster — it only removes ConfigHub's awareness of those workloads.

**Rollback is safe because:**
- Import creates ConfigHub state (Apps, Deployments)
- Import does not modify cluster state
- Your existing deployer was never stopped
- Removing ConfigHub state returns to pre-import behavior

---

## Sources of Intent

ConfigHub supports two sources of intent:

| Source | When to Use |
|--------|-------------|
| **Git import** (default) | You have Git repos with manifests, Kustomize overlays, or Helm charts. ConfigHub renders and publishes via OCI. |
| **Live import** | Git repos aren't organized or available. cub-scout discovers from the running cluster. |

Both are first-class. You can start with live import and add Git sources later.

The artifact publish target is OCI by default. The runtime reconciler remains Flux or Argo — ConfigHub does not replace your deployer, it feeds it rendered manifests.

---

## Common Patterns

### Helm-Only Cluster

```bash
# Discover Helm releases
./cub-scout map list -q "owner=Helm"

# Import all Helm workloads in a namespace
./cub-scout import -n <namespace> --dry-run
```

Helm releases become App components. The Helm release name typically becomes the component name.

### ArgoCD with ApplicationSets

```bash
# See what ApplicationSets generate
./cub-scout map list -q "owner=ArgoCD"

# Import generated Applications as App components
./cub-scout import -n <namespace> --dry-run
```

Each generated Application becomes an App component. The ApplicationSet itself is not imported.

### Flux Kustomizations

```bash
# See Flux-managed workloads
./cub-scout map list -q "owner=Flux"

# Import Flux workloads
./cub-scout import -n <namespace> --dry-run
```

Flux Kustomization paths inform the variant inference (e.g., `./overlays/prod` → `variant=prod`).

### Mixed Argo + Helm in Same Namespace

```bash
# See the mix
./cub-scout map list -n mixed-ns

# Import handles both — each workload retains its detected owner
./cub-scout import -n mixed-ns --dry-run
```

Both Argo-managed and Helm-managed workloads become components of the same App. The original owner is preserved as metadata.

---

## Troubleshooting

### "No workloads found"

```bash
# Check you have the right namespace
kubectl get deployments -n <namespace>

# Check cub-scout can see the cluster
./cub-scout map list
```

### "Auth expired"

```bash
# Re-authenticate
cub auth login

# Verify
./cub-scout status
```

### "Import proposal looks wrong"

Don't import. Instead:
1. Check labels: `kubectl get deploy -n <ns> --show-labels`
2. Check annotations: `kubectl get deploy -n <ns> -o json | jq '.items[].metadata.annotations'`
3. Adjust workload labels if ownership detection is incorrect
4. Re-run `./cub-scout import -n <ns> --dry-run` after fixing

### "Imported but ownership chain broken"

```bash
# Trace the specific workload
./cub-scout trace deploy/<name> -n <namespace>

# Check if the deployer is still reconciling
kubectl get deploy/<name> -n <namespace> -o json | jq '.metadata.labels'
```

---

## Related Docs

- [Import from Live](import-from-live.md) — Cluster-only discovery without Git
- [Import to ConfigHub](import-to-confighub.md) — Step-by-step import reference
- [Glossary](../reference/glossary.md) — ConfigHub terminology (including emerging App/Deployment model)
- [Ownership Detection](ownership-detection.md) — How cub-scout identifies owners
- [Trace Ownership](trace-ownership.md) — Following the ownership chain
