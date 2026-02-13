# The Map

> **Archive status:** Historical planning context (non-canonical for current releases).
> Use these maintained docs for current behavior and scope:
> - `docs/roadmap.md`
> - `docs/reference/import-docs-crosswalk.md`
> - `docs/reference/connected-tiers-and-views-product-guide.md`

**What does ConfigHub do?** We give you a Map of the Truth: your application, your sources of truth and your operational infrastructure.  So that you always know what is being deployed, comply with regulations, and can see how to fix any operational issues as soon as they arise.

---

## The Map Is the Central Concept

The Map shows you:

| What | Description |
|------|-------------|
| **Operational facts** | What's actually running in your clusters |
| **Config** | The fully rendered literal configuration that defines each resource |
| **Sources of truth** | Git repos, Helm charts, OCI artifacts — where config comes from |
| **Sources and targets** | The wider config story: which sources deploy to which clusters |
| **Sync-state** | Are they in sync? Drifted? Failed? Suspended? |

The Map answers questions that can be painful to answer today:

- What's running?
- Who owns it?
- Where's the source?
- Did it drift?
- What's unmanaged?

Providing a full story of what is running and why goes beyond clusters and apps.  You need to know the desired state too (Git, OCI, etc) and you need to know how the desired and running states relate to each other.  Tracing this causal chain and any intermediate operational facts is hard and requires a way to aggregate multiple views into one -- that's "the Map".  Note we are **building on existing GitOps tools as much as possible** where these help to understand and make changes and fix any breaks quickly.  

## The Tech

Why is there a problem here?  Existing GitOps tools and UIs provide some informtion and operability.  At the level of one component deployed to 1-N identical runtimes, existing tools can give a complete answer.  But as your application deployments become more complicated it can get harder and more mysterious to make changes to them.  This is because doing so requires an understanding multiple **states** ie. in Git, runtime components, GitOps processes, etc, which are obscured in the wider 'config sprawl' of templates, umbrella charts, overlays and other files. 

An emerging technique to make GitOps artefacts and states less ambiguous and more transparent to operational processes is the **rendered manifest pattern** in which only flat YAML or JSON is stored in Git and rigoruous indexing keeps data organised.  This model is still limited by the tool-chain having been designed primarily for development and not for operations.  Our Map can be seen as taking the 'literal YAML' model to the next level of operability, full automation and compliance with (we hope) compelling business economics.

Our approach is an implementation of **configuration as data** in which the map acts the source of truth for operations on config data, and connects with sources (eg Git) and targets (eg Kubernetes) to coordinate GitOps workflows.  Unlike vertically integrated "config as code" or "infra as code" technologies, all config data in ConfigHub is API-accessible to any permitted "worker" (person, program or agent) and we provide a **configuration worker SDK** to help anyone and everyone do that.

Further notes on 'the problem' may be found in [**this introduction**](../../INTRODUCTION.md) and [**internal pitch**](../../INTERNAL-PITCH-WHY-WE-NEED-THIS.md).  For an analysis of **what GitOps does not make explicit** that we can help with, please see this [**feature matrix document**](confighub-feature-matrix.md).


---

We shall now do a quick walk-through of how to use it, followed by a few chapters on the model.

## The Map Evolves With Your Journey

For many people reading this, you probably have a cluster with apps deployed using Flux, Argo, Helm or similar.  What's the next step?

| Stage | How You See the Map | What's Included |
|-------|---------------------|-----------------|
| **Standalone** | `cub-agent map` | Cluster facts, ownership, drift — read-only |
| **Discovery** | `cub-agent import --dry-run` | Proposed Hub + App Space structure |
| **Connected** | Full ConfigHub | Hub + App Space + Units + Labels + Sources + Targets |

### Stage 1: Standalone (Read-Only)

The `cub-agent map` command gives you a live Map of your cluster:

```
$ cub-agent map

CLUSTER     NAMESPACE    KIND          NAME           OWNER     STATUS
prod-east   payments     Deployment    payment-api    Flux      Synced
prod-east   payments     Deployment    payment-worker Flux      Drifted
prod-west   orders       Deployment    order-api      Argo      Synced
staging     default      Deployment    debug-pod      Native    Unknown
```

This is your first live Map — before connecting to ConfigHub.

**What you see:**
- Every resource in your cluster
- Who owns it (Flux, Argo, Helm, ConfigHub, or Native/unmanaged)
- Sync status (Synced, Drifted, Failed, Suspended)
- The "Native" bucket — resources nobody owns (security risk, rebuild risk)

### Stage 2: Discovery

The agent can suggest how to organize what it found:

```bash
cub-agent import --namespace payments-prod --model hub-appspace --dry-run
```

```
Suggested structure (Hub/App Space model):
  App Space: payments-team
    - app=payment-api
        Unit: payment-api-prod (variant=prod)
    - app=payment-worker
        Unit: payment-worker-prod (variant=prod)
```

This is a *proposed* Map — showing how your cluster could be organized into Hub + App Space + Units.

### Stage 3: Connected

When you connect to ConfigHub:

1. The proposed structure becomes real (Hub, App Space, Units with labels)
2. Sources are registered (Git repos, Helm repos, OCI)
3. Targets are connected (your clusters)
4. Workers sync state between sources and targets
5. The Map shows the complete picture across your fleet

```
ConfigHub Fleet View (Hub/App Space Model)
Hierarchy: Application -> Variant -> Target

  payment-api
  ├── variant: dev
  │   └── payments-team @ rev 92
  ├── variant: staging
  │   └── payments-team @ rev 130
  └── variant: prod
      └── payments-team @ rev 127 (us-east)
      └── payments-team @ rev 127 (eu-west)
      └── payments-team @ rev 87 (asia) <- behind!
```

---

## What the Map Contains

### At Every Stage

| Element | Description |
|---------|-------------|
| **Resources** | Kubernetes resources (Deployments, Services, etc.) |
| **Ownership** | Who manages each resource (Flux, Argo, Helm, Native) |
| **Status** | Synced, Drifted, Failed, Suspended |
| **Relationships** | What depends on what |

### When Connected

| Element | Description |
|---------|-------------|
| **Hub** | Platform governance — constraints, policies, base configs |
| **App Space** | Team workspace — contains all variants via labels |
| **Units** | Single deployables with labels (app, variant, region) |
| **Sources** | Git repos, Helm repos, OCI artifacts |
| **Targets** | Clusters where config is deployed |
| **Workers** | Execution agents that sync sources to targets |

---

## The Map Command

```bash
# Interactive TUI dashboard
cub-agent map

# Plain text for scripting
cub-agent map list

# Filter by query
cub-agent map list -q "owner=Native"        # Find orphans
cub-agent map list -q "status=Drifted"      # Find drift
cub-agent map list -q "namespace=prod*"     # Filter by namespace

# Fleet view (when connected)
cub-agent map fleet --space payments-team
```

**Map subcommands:**

| Command | Description |
|---------|-------------|
| `./test/atk/map status` | Status overview |
| `./test/atk/map problems` | Problems only |
| `./test/atk/map pipelines` | Pipelines (source -> deployer -> resources) |
| `./test/atk/map workloads` | Workloads by owner |
| `./test/atk/map drift` | Drift detection |
| `./test/atk/map sprawl` | Sprawl analysis |

---

## Why the Map Matters

Your GitOps tool shows what *it* manages. The Map shows *everything* — including what nobody manages.

That "Native" bucket includes:
- Resources created by `kubectl` at 2am
- Things that won't rebuild if the cluster dies
- Security risks from untracked changes
- The stuff your GitOps dashboard can't show you

The Map connects your three sources of truth (Git, ConfigHub, Cluster) and shows when they disagree.

---

## See Also

- [02-HUB-APPSPACE-MODEL.md](02-HUB-APPSPACE-MODEL.md) — Hub, App Space, Unit definitions
- [05-THREE-SOURCES-OF-TRUTH.md](05-THREE-SOURCES-OF-TRUTH.md) — Git, ConfigHub, Cluster
- [04-MAP-USER-JOURNEY-TO-FULL-CONFIGHUB.md](04-MAP-USER-JOURNEY-TO-FULL-CONFIGHUB.md) — The full adoption path

---

**Next:** [02-HUB-APPSPACE-MODEL.md](02-HUB-APPSPACE-MODEL.md) — Core definitions: Hub, App Space, Unit, App = a name
