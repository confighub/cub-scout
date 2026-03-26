# First Import: Connect and Import in 10 Minutes

> **1.x Connected** — This guide requires a ConfigHub account.
> For standalone features (no account needed), see [First Map](first-map.md).
>
> **Ownership:** Connected commands (`cub auth`, `cub unit`, `cub target`, `cub gitops`) come from
> the [ConfigHub SDK](https://github.com/confighub/sdk) (`cmd/cub`). cub-scout owns discovery and
> explanation. See [Interface Boundaries](../concepts/why-connected-mode.md#interface-boundaries-authoritative).

**Time:** 10 minutes
**Goal:** Connect to ConfigHub, import one namespace, see what you unlock

**Prerequisite:** You've already explored standalone (`cub-scout map`, `trace`, `scan`).
If not, start with [First Map](first-map.md).

---

## What, Why, When

**What:** Import registers your running workloads with ConfigHub. ConfigHub becomes
the system of record for what should be running, while cub-scout continues
reporting what actually is.

**Why:** Standalone answers "what's here now." Connected answers "what should be here,
what changed over time, and how does this compare across environments."

**When:** After you've explored standalone and want durable history, fleet queries,
or managed lifecycle. There's no rush — standalone works indefinitely.

---

## Step 1: Connect (~2 min)

```bash
cub auth login
```

Your browser opens. Sign in (or create a free account). When you see "Login successful", return to the terminal.

Verify the connection:

```bash
cub-scout status
```

```
ConfigHub:  ● Connected (you@company.com)
Cluster:    prod-east
Context:    kind-demo
```

The TUI also shows your mode — press `cub-scout map` and look at the header:

```
┌─ CUB-SCOUT MAP ──────────────────────────────── ● Connected ─┐
```

---

## Step 2: Discover (~3 min)

Pick a namespace you know well. Preview what cub-scout will propose:

```bash
./cub-scout import --dry-run -n payments-prod
```

```
Discovered 4 workloads in payments-prod:

  App: payments-team

  Deployments:
    payment-api      Deployment   owner=ArgoCD   variant=prod
    order-svc        Deployment   owner=ArgoCD   variant=prod
    redis            StatefulSet  owner=Helm     variant=prod

  Labels: app=payments, variant=prod

  Skipped: debug-config (Native/unmanaged — import separately if desired)

No changes made. Use without --dry-run to import.
```

> **API note:** The `cub` CLI currently uses `--space` and `unit` commands while the
> API evolves. Read them as: Space = App, Unit = Deployment.

**What to check:**
- Do the component names make sense to your team?
- Is the variant (`prod`) correctly inferred?
- Are the right workloads included (and unmanaged ones skipped)?

For machine-readable output: `./cub-scout import --dry-run -n payments-prod --json`

---

## Step 3: Import (~1 min)

If the proposal looks right:

```bash
./cub-scout import -n payments-prod -y
```

```
Imported 3 workloads to ConfigHub:
  App: payments-team
  Deployments: payment-api, order-svc, redis

Your existing deployer (ArgoCD, Helm) is still running.
ConfigHub is now aware of these workloads.
```

**Nothing changed in your cluster.** Your ArgoCD Applications and Helm releases
continue reconciling as before. The import only told ConfigHub about your workloads.

---

## Step 4: Verify (~2 min)

Check that ConfigHub received the import:

```bash
cub unit list --space payments-team
```

```
NAME           VARIANT   LABELS                          TARGET
payment-api    prod      app=payments, variant=prod      prod-east
order-svc      prod      app=payments, variant=prod      prod-east
redis          prod      app=payments, variant=prod      prod-east
```

Trace a workload to see ConfigHub in the ownership chain:

```bash
./cub-scout trace deploy/payment-api -n payments-prod
```

```
deploy/payment-api
  namespace: payments-prod
  owner: ArgoCD (Application/myapp-prod)
  confighub: payments-team/payment-api (variant=prod)
  source: git@github.com:acme/deploy.git @ envs/prod/payments
```

The ownership chain now includes ConfigHub context alongside the original ArgoCD ownership.

---

## What You Just Unlocked

With your workloads registered in ConfigHub, you can now:

**Connect a Git source.** ConfigHub renders your Git templates into OCI artifacts.
Flux or Argo pulls from OCI. One pipeline, one audit trail from commit to cluster.

```
Git → ConfigHub renders → OCI artifact → Flux/Argo → cluster
```

**Query across clusters.** Once you import multiple clusters:

```bash
cub unit list --where "Labels.app='payments'"
# See payment-api across dev, staging, prod — all clusters
```

**Track revision history.** Every change to a component is versioned in ConfigHub.
Compare any two revisions to see exactly what changed and when.

**Audit break-glass changes.** When someone hotfixes production outside the normal
pipeline, ConfigHub tracks the accept/reject decision with who, what, and why.

For the full story: [Why Connected Mode](../concepts/why-connected-mode.md)

---

## Rollback

Changed your mind? Rollback is safe:

```bash
cub unit delete payment-api --space payments-team
cub unit delete order-svc --space payments-team
cub unit delete redis --space payments-team
```

This removes ConfigHub's awareness. Your cluster and deployers are untouched.

---

## Next Steps

- **Import more namespaces** — Follow the [Migration Playbook](../howto/migration-playbook.md) for a phased approach
- **Understand discovery** — [Import from Live](../howto/import-from-live.md) explains variant inference and proposal logic
- **See worked examples** — [examples/import-from-live/](../../examples/import-from-live/) has fixture manifests and expected output
- **Learn the model** — [Glossary](../reference/glossary.md) explains App, Deployment, Target, and the API mapping
