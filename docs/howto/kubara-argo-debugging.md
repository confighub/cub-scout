# Using cub-scout with Kubara (Argo + ApplicationSet)

Guide for Kubara operators using cub-scout to debug and inspect
Argo CD + ApplicationSet + Cluster Generator platform environments.

## What Kubara Looks Like to cub-scout

Kubara generates an Argo-first, label-driven, multi-cluster framework:

```
kubara init → config.yaml → ApplicationSets → Applications → Workloads
```

cub-scout sees everything from **ApplicationSets → Workloads** onward.
It detects Argo ownership, traces lineage, and reports health.

## What cub-scout Can Answer Today

| Question | Command | Evidence |
|----------|---------|----------|
| What's running and who owns it? | `./cub-scout map list` | Ownership table (Argo, Helm, Native) |
| What does this ApplicationSet generate? | `./cub-scout tree ownership -n argocd` | App-of-Apps / ApplicationSet hierarchy |
| Where did this Deployment come from? | `./cub-scout trace deploy/NAME -n NS` | Application → Deployment → Pod chain |
| What does the Git repo structure look like? | `./cub-scout tree git --path REPO` | Git tree with Kustomize/Helm detection |
| Are there known risks or misconfigs? | `./cub-scout scan --state` | 46+ pattern checks on live state |
| What patterns exist in my Git repo? | `./cub-scout patterns detect --git-root REPO` | Helm charts, Kustomize overlays, values |
| Is GitOps healthy? | `./cub-scout gitops status` | Controller health, reconciliation status |

## Recommended First Commands

### 1. Cluster overview

```bash
# See ownership classification
./cub-scout map list

# See only Argo-managed resources
./cub-scout map list | grep ArgoCD

# See unmanaged resources (potential shadow IT)
./cub-scout map orphans
```

### 2. ApplicationSet lineage

```bash
# Tree view of Argo hierarchy in argocd namespace
./cub-scout tree ownership -n argocd

# Trace a specific generated Application
./cub-scout trace application/myapp-prod -n argocd
```

### 3. Workload provenance

```bash
# Full trace: Application → Deployment → ReplicaSet → Pod
./cub-scout trace deploy/api -n myapp-prod

# With diff (live vs desired)
./cub-scout trace deploy/api -n myapp-prod --diff
```

### 4. Git repo inspection

```bash
# What's in the repo?
./cub-scout tree git --path /path/to/deploy-repo

# Detect patterns (Helm umbrella, Kustomize overlays)
./cub-scout patterns detect --git-root /path/to/deploy-repo
```

### 5. Health and risk scanning

```bash
# Live cluster scan
./cub-scout scan --state

# Scan exported manifests (no cluster needed)
./cub-scout scan --file manifests/

# GitOps pipeline health
./cub-scout gitops status
```

## What cub-scout Cannot Answer (Yet)

| Question | Why | Workaround |
|----------|-----|------------|
| What was in `kubara config.yaml`? | Pre-generation provenance is not preserved in cluster labels | Add `kubara.io/config-hash` annotation to generated resources |
| What `.env` values were used? | Same — generation-time context is lost | Preserve in annotations or ConfigMap |
| What Cluster Generator matched? | ApplicationSet controller internals are not exposed as labels | Check ApplicationSet status field directly |
| Cross-cluster fleet comparison | cub-scout is single-cluster | Use `cub` CLI for fleet queries after importing to ConfigHub |

### Making Pre-Generation Provenance Visible

If you want cub-scout to trace all the way back to `kubara init`:

1. Add annotations during generation:
   ```yaml
   metadata:
     annotations:
       kubara.io/config-version: "v2.3"
       kubara.io/generated-at: "2026-03-15T10:00:00Z"
   ```

2. cub-scout will display these in `trace` and `explain` output.

3. For full lineage, use a custom ownership detector:
   ```yaml
   # ~/.cub-scout/custom-detectors.yaml
   detectors:
     - name: kubara
       labels:
         - kubara.io/managed-by
   ```

## Connected Mode: Fleet Handoff

The commands above are the **diagnostic path** — read-only inspection
of an existing Kubara/Argo install. For the **production onboarding
path** (registering a Kubara workload as a ConfigHub Component and
repointing Argo from Git to ConfigHub OCI), see
[Onboard Existing](onboard-existing.md). The diagnostic commands here
remain useful afterward for trace, drift, and explain.

### Discovery-only (no controller changes)

After inspecting standalone, import into ConfigHub for fleet-level queries:

```bash
# Discover and propose Component structure
./cub-scout import --dry-run -n myapp-prod

# Import (creates ConfigHub state, does not change cluster)
./cub-scout import -n myapp-prod

# Then use cub CLI for fleet queries
cub space list -l Component=myapp
```

### Onboarding (Pattern 1 takeover)

When you want Argo to reconcile from ConfigHub OCI instead of Git:

```bash
# Dry-run: shows the render diff and the proposed Component/Variant/Target
./cub-scout onboard --controller argo -n myapp-prod --dry-run

# Take over (controller source repoint)
./cub-scout onboard --controller argo -n myapp-prod
```

> **Status:** `cub-scout onboard` is planned, not shipping yet — see
> [docs/specs/pattern-1-takeover-v1.md](../specs/pattern-1-takeover-v1.md)
> and [docs/roadmap.md](../roadmap.md).

> **Ownership:** `cub` commands come from the [ConfigHub SDK](https://github.com/confighub/sdk).
> cub-scout discovers and orchestrates; `cub` handles connected lifecycle.

## Positioning

cub-scout **complements** Argo CD and Kubara. It does not replace them:

- Argo CD reconciles desired state to clusters
- Kubara generates the Argo configuration from a higher-level model
- cub-scout **observes** what Argo deployed and reports on it

cub-scout is read-only. It never modifies cluster state or interferes
with Argo reconciliation.

## See Also

- [Onboard Existing](onboard-existing.md) — Pattern 1 takeover (Argo → ConfigHub OCI)
- [Import to ConfigHub](import-to-confighub.md) — Discovery-only import
- [Argo App-of-Apps example](../../examples/apptique-examples/argo-app-of-apps/) — ApplicationSet hierarchy detection
- [Platform Example](../../examples/platform-example/) — Live Flux+orphans demo
- [Command Reference](../reference/commands.md) — Command usage and examples
- [Fleet Queries](fleet-queries.md) — Cross-environment queries with ConfigHub
