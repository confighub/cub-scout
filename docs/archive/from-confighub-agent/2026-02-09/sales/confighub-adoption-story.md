# ConfigHub: Zero-Friction Adoption

**Status:** Current
**Last Updated:** 2025-12-30

**Key insight:** Lead with the Map. People understand "see everything" instantly. They don't understand "Configuration as Data" until much later.

---

## The Three-State Model

| Location | What It Holds | Role |
|----------|---------------|------|
| **Git** | Intent | What you *want* to be true (journal of decisions) |
| **ConfigHub** | Operational state | What *should* be running (current desired state, queryable) |
| **Cluster** | Reality | What *is* running |

**Normal flow:**
```
Git (intent) → ConfigHub → Cluster
```

**Hotfix flow (bidirectional):**
```
Cluster (changed) → ConfigHub accepts → Git updated to match
```

ConfigHub is the **operational source of truth** because it knows both what Git says and what the cluster says, and can reconcile in either direction. Git remains the **audit trail** — when you `drift accept`, ConfigHub creates a PR documenting who changed what and when.

---

## Core Concepts

| Concept | Role |
|---------|------|
| **Hub** | Governance — policies, templates, constraints |
| **App Space** | Workspace — where teams manage their apps |
| **Map** | The graph — units, relations, state, history, activity |
| **Unit** | Node in the Map — individual managed resource |

---

## Components

| Component | Role | Permissions |
|-----------|------|-------------|
| **cub-agent** | Read-only observer — discovers workloads, feeds the Map | Read-only RBAC |
| **Worker** | Execution engine — applies changes, runs Functions | Read-write RBAC |

cub-agent is standalone and lightweight. Workers are part of the ConfigHub platform.

---

## Entry Point

```bash
kubectl apply -f https://confighub.com/agent.yaml
cub map
```

Works for everyone. No prerequisites. No configuration. No write permissions. No commitment.

**Time to value: 30 seconds.**

---

## What You Get Immediately

```
cub map

CLUSTER     NAMESPACE    KIND          NAME              OWNER
prod-east   default      Deployment    nginx             flux:helmrelease/nginx
prod-east   default      Deployment    mystery-app       unknown
prod-east   kube-system  Deployment    coredns           system
staging     default      Deployment    redis             helm:release/redis

312 units across 3 clusters. 4 unowned.
```

**The "unknown" units are the value.** That's the deployment someone did at 2am. That's the ConfigMap from a tutorial someone forgot to delete. That's the security hole.

---

## The Map

Map is the queryable graph of everything:

| View | Command | What You See |
|------|---------|--------------|
| **Default** | `cub map` | All units in scope |
| **Filtered** | `cub map --query "kind=Deployment"` | Subset of units |
| **Relations** | `cub map relations nginx` | What nginx depends on, what depends on nginx |
| **History** | `cub map history --since=1h` | What changed |
| **Activity** | `cub map activity` | Event stream, reconciliations, drift |

Status is a summary view of the Map:

```
cub status

3 clusters • 312 units • 3 drifted • 1 failing
```

---

## Questions You Can't Answer Today

| Question | Current Answer | With ConfigHub |
|----------|---------------|----------------|
| "Where is X deployed?" | Grep + kubectl + spreadsheet | `cub map --query "name=X"` |
| "What version is running in prod?" | kubectl per cluster | `cub map --query "namespace=prod"` |
| "Why is staging different from prod?" | Manual diff | `cub map diff staging prod` |
| "What changed in the last hour?" | Git log + hope it synced | `cub map history --since=1h` |
| "Are we drifted anywhere?" | Don't know until something breaks | `cub map --drifted` |
| "Where is log4j deployed?" | Hours of grep + kubectl | `cub map --query "image contains log4j"` |
| "What depends on this service?" | Tribal knowledge | `cub map relations my-service` |

---

## Layers (Additive)

| Layer | Requires | Gives You |
|-------|----------|-----------|
| **Inventory** | Agent | Map — "what's running" |
| **Ownership** | Nothing (auto-detected) | "Who manages what" |
| **Drift** | Nothing (compares desired to live) | "What's wrong" |
| **Provenance** | Git connection (optional) | "Where it came from" |
| **Organization** | Hub / App Space (optional) | "Who's responsible, what policies apply" |
| **Operations** | Worker (optional) | "Change things across fleet" |

Value starts at layer 1. Each layer is additive. Nothing is required.

---

## Auto-Discovery

Agent auto-discovers ownership:

| Method | Detected? | How |
|--------|-----------|-----|
| Flux Kustomization | ✓ | Watches CRD |
| Flux HelmRelease | ✓ | Watches CRD |
| Argo Application | ✓ | Watches CRD |
| Helm release | ✓ | Reads release secrets |
| Terraform | ✓ | Labels/annotations |
| Raw kubectl apply | ✓ | Resource exists, owner = unknown |

The "unknown" category is valuable — it shows what's unmanaged.

---

## Community Entry Points

| User | Reason to Install Agent | Immediate Value |
|------|-------------------------|-----------------|
| **FluxCD** | "What's drifted?" | Drift detection across fleet |
| **ArgoCD** | "What's deployed where?" | Query across Argo instances |
| **Helm** | "What versions are running?" | Fleet inventory without GitOps |
| **Terraform** | "What k8s resources did we create?" | Visibility into TF-managed workloads |
| **"We use kubectl"** | "What's actually in our clusters?" | Map shows everything, find orphans |
| **"We're migrating to GitOps"** | "What's managed vs not yet?" | Migration progress tracking |

---

## Progression (The Adoption Ladder)

| Phase | What | Lock-in | Who |
|-------|------|---------|-----|
| **1. Map** | `cub map` — see everything | None | Anyone curious (30 seconds) |
| **2. Control** | `cub drift accept/revert` — fix drift | Low | Teams with drift problems |
| **3. Organize** | Hub/App Space — structure | Medium | Platform teams |
| **4. Automate** | Functions/Actions — custom logic | Higher | Advanced users (optional) |

```
Phase 1: Agent only (read-only)
         └── cub map → inventory, relations, history, risks

Phase 2: Control drift
         └── cub drift accept/revert → bidirectional GitOps

Phase 3: Define Hub / App Space
         └── Structural organization, team boundaries, policies

Phase 4: Functions and Actions (optional)
         └── Custom validation, automated remediation
```

**Most users get massive value at Phase 1-2.** Phase 3-4 are optional.

### Addressing Common Objections

| Objection | Map-First Answer |
|-----------|------------------|
| "Two sinks (Git + ConfigHub)" | Git is for authoring. Map is for operating. Different purposes. |
| "Vendor lock-in" | Start read-only. Delete agent anytime. No commitment. |
| "Don't want to code" | 90% of operations need zero code. Functions are optional. |
| "More complex" | One `cub map` vs. 30 Argo dashboards. |

See [How Map Design Helps Artem Questions](https://github.com/confighubai/confighub-agent/search?q=how-maps-design-helps-artem-25-questions-iits.md) for detailed analysis.

See [Modern CI/CD Problems](use-case-modern-cicd.md) for how ConfigHub addresses anti-patterns from traditional CI/CD.

---

## CLI Reference

```bash
# Map (core interface)
cub map                                       # Everything in scope
cub map --cluster=prod-east                   # One cluster
cub map --owner=flux                          # Flux-managed only
cub map --owner=unknown                       # Find orphans
cub map --query "kind=Deployment"             # Filter by query
cub map --query "image contains nginx"
cub map --query "replicas < 3"
cub map --drifted                             # Only drifted units

# Relations
cub map relations nginx                       # Dependency graph
cub map relations --reverse nginx             # What depends on nginx

# History
cub map history                               # Recent changes
cub map history --since=1h                    # Time-bounded
cub map history nginx                         # One unit's history

# Activity
cub map activity                              # Event stream
cub map activity --type=drift                 # Just drift events

# Status (summary)
cub status                                    # Health overview

# Diff
cub map diff prod-east prod-west              # Cluster vs cluster
cub map diff staging prod --app=nginx         # Environment vs environment

# Operations (requires Worker)
cub apply --query="image contains log4j:2.14" --set="image=log4j:2.17"
cub rollback nginx --to=2h-ago
```

---

## What Triggers Worker Setup?

| Trigger | What They Need |
|---------|---------------|
| "Found unknown resources, want to assign ownership" | App Space + Worker |
| "See drift in 12 places, want to fix all at once" | Bulk operations via Worker |
| "Want to push changes across fleet" | Worker |
| "Want ConfigHub to sync to OCI for Flux/Argo" | Worker |

**cub-agent gives READ. Worker gives WRITE.**

---

## The Pitch

> "What's running in your clusters?"
> 
> If you can't answer that in one command, install the agent.

---

## Summary

1. **Install agent** — 30 seconds, read-only, zero risk
2. **cub map** — see everything
3. **Find orphans** — immediate value
4. **Query the Map** — answer questions you couldn't before
5. **Organize later** — Hub, App Space when you need them
6. **Set up Worker** — when you need to change things at scale

ConfigHub is the **Map** first, management layer second.
