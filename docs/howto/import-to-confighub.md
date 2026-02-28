# Canonical Import Path: Argo/Helm to ConfigHub

This is the canonical migration path for moving existing ArgoCD/Helm-managed workloads into ConfigHub.

For a comprehensive guide with assessment, planning, validation checklists, and rollback procedures, see the [Migration Playbook](migration-playbook.md).

## Ownership Split

The import process involves three roles with distinct responsibilities:

| Step | Who | What |
|------|-----|------|
| **Discover** | cub-scout | Scan cluster, detect ownership, propose App structure |
| **Import** | ConfigHub | Create Apps/Deployments, set up bridge workers, connect OCI pipeline |
| **Deploy** | Flux/ArgoCD | Pull rendered manifests from ConfigHub's OCI registry, apply to cluster |

cub-scout is read-only — it discovers and proposes. ConfigHub handles the actual import and lifecycle.

For cluster-only discovery (no Git required), see [Import from Live](import-from-live.md).

> **API note:** The `cub` CLI currently uses Space/Unit commands while APIs evolve
> toward App/Deployment language. See [Glossary](../reference/glossary.md) for the mapping.

## Scope

Scope boundary:
- This guide covers workload discovery (cub-scout) and import (ConfigHub).
- Helm/Kustomize rendering pipelines are Provisional scope.
- Do not treat this import flow as a rendering/templating workflow.

The path is intentionally:
- single-cluster first
- namespace-scoped
- additive first, destructive later

That keeps rollout and rollback simple.

## When to Use This

Use this when your workloads are currently managed by:
- Argo CD Applications (including App-of-Apps/ApplicationSet-generated apps)
- Helm releases
- Mixed Argo/Helm/native workloads in the same namespace

## Prerequisites

```bash
# Connected mode auth
cub auth login

# Verify context and cluster access
cub context get
kubectl get ns
```

## Canonical Path

### 1. Baseline Current State

```bash
cub-scout map list -q "owner=ArgoCD OR owner=Helm OR owner=Native"
cub-scout map workloads
```

Capture this before migration so you can compare after import.

### 2. Start With One Namespace

```bash
cub-scout import -n <namespace> --dry-run
```

Review:
- discovered workloads
- suggested App structure
- suggested component names and labels

If the proposal is wrong, stop and adjust naming/labels strategy first.

### 3. Execute Import

```bash
cub-scout import -n <namespace>
```

Or non-interactive:

```bash
cub-scout import -n <namespace> -y
```

### 4. Verify Import

```bash
cub unit list --space <suggested-space>
cub-scout map workloads
cub-scout tree ownership
```

Success criteria:
- expected workloads exist in ConfigHub
- workload ownership context is still coherent
- no unexpected resource drift

### 5. Keep Existing Deployer During Validation

Do not immediately remove Argo/Helm control. First validate that ConfigHub state is correct.

For Argo App-of-Apps/ApplicationSet setups:
- treat generated/child Applications as workload sources of truth
- treat parent orchestration objects as orchestration metadata, not imported

### 6. Cut Over by Policy, Not Accident

After validation, pick one controller-of-record per workload path.

Recommended order:
1. validate ConfigHub import result
2. update team policy for controller ownership
3. remove duplicate reconciliation paths only after policy is explicit

Avoid dual-control long term (two systems reconciling the same manifests).

### 7. Repeat Namespace by Namespace

```bash
# Example: expand to next namespace
cub-scout import -n <next-namespace> --dry-run
cub-scout import -n <next-namespace> -y
```

Scale out only after single-namespace validation passes.

## Rollback

If a namespace migration is not acceptable:

```bash
# Remove created state (space-scoped)
cub unit list --space <space>
cub unit delete <unit-slug> --space <space>
```

Then continue using your existing Argo/Helm flow while you revise mapping.

Rollback is safe: import creates ConfigHub state only — it never modifies the cluster.

## Notes

- `cub-scout import --json` is for proposal automation and GUI workflows.
- `cub-scout import --wizard` runs the interactive TUI wizard.
- This path prioritizes predictable migration over fast migration.

## Related Docs

- [Migration Playbook](migration-playbook.md) — Comprehensive guide with assessment, planning, validation, and rollback
- [ArgoCD Import Demo](../../examples/argo-import-confighub-demo/) — Three import tools compared on real ArgoCD cluster (kind)
- [Flux Import Demo](../../examples/flux-import-confighub-demo/) — Management + discovery on real Flux cluster with D2 pattern (kind)
- [Import from Live](import-from-live.md) — Cluster-only discovery (no Git required)
- [Import from Live Example](../../examples/import-from-live/) — Worked example with fixtures
- [Combined Git+Live Example](../../examples/combined-git-live/) — Git repo + cluster alignment
- [Fleet Import Example](../../examples/fleet-import/) — Multi-cluster aggregation
- [Business Outcomes](../../outcomes/README.md) — Why ConfigHub import matters
- [ConfigHub Documentation](https://docs.confighub.com) — Full ConfigHub guide
- [Import Docs Crosswalk](../reference/import-docs-crosswalk.md) — Archived import docs mapped to current docs
