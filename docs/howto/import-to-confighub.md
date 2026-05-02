# Canonical Import Path: Argo/Flux/Helm to ConfigHub

> **Ownership:** The `cub` CLI is part of the [ConfigHub SDK](https://github.com/confighub/sdk) (`cmd/cub`).
> cub-scout owns discovery and explanation; `cub` owns connected lifecycle commands (`cub gitops import`, `cub auth`, etc.).
> See [Interface Boundaries](../concepts/why-connected-mode.md#interface-boundaries-authoritative).

This is the canonical migration path for moving existing ArgoCD, Flux,
or Helm-managed workloads into ConfigHub.

For the takeover flow that repoints an existing controller from Git to
ConfigHub OCI without redeploying, see
[Onboard Existing](onboard-existing.md). This document covers the
discovery-and-register path; onboarding adds the controller-source
repoint step.

For a comprehensive guide with assessment, planning, validation
checklists, and rollback procedures, see the
[Migration Playbook](migration-playbook.md).

## Ownership Split

The import process involves three roles with distinct responsibilities:

| Step | Who | What |
|------|-----|------|
| **Discover** | cub-scout | Scan cluster, detect ownership, propose Component / Variant / Target structure |
| **Delegate (when available)** | cub-scout + `cub gitops import` | Import Argo/Flux workloads via rendered GitOps path |
| **Register** | ConfigHub | Create Components, Base Variants, Deployable Variants, set up bridge workers, connect OCI pipeline |
| **Deploy** | Flux/Argo CD | Pull rendered manifests from ConfigHub's OCI registry, apply to cluster |

cub-scout is read-only in discovery mode (`--dry-run`). Non-dry-run
import creates ConfigHub state and may delegate Argo/Flux workloads to
`cub gitops import` when matching targets exist.

For cluster-only discovery (no Git required), see
[Import from Live](import-from-live.md).

For bundle-based import previews (no cluster discovery), see
[Import from Bundle Example](../../examples/import-from-bundle/).

## ConfigHub Model

This guide uses the Component / Variant / Target doctrine. In short:

| Concept | What it is |
|---------|-----------|
| **Component** | A logical piece of software (api, worker, payments). The family. |
| **Base Variant** | Non-deployable. Holds placeholders, the canonical render. |
| **Deployable Variant** | A Variant bound to a Target. What actually deploys. |
| **Target** | A Kubernetes cluster managed by ConfigHub. |
| **Connection** | Typed contract for cross-Component dependencies. (Imported as a draft in v1.) |

See [Glossary](../reference/glossary.md) for full definitions.

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

- Argo CD Applications (including App-of-Apps / ApplicationSet-generated apps)
- Flux Kustomizations or HelmReleases
- Helm releases (no GitOps)
- Mixed Argo/Flux/Helm/native workloads in the same namespace

Use [Onboard Existing](onboard-existing.md) instead when you want the
controller's source repointed at ConfigHub OCI as part of the same flow.
This document stops at "ConfigHub state created"; onboarding goes one
step further.

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
cub-scout map list -q "owner=ArgoCD OR owner=Flux OR owner=Helm OR owner=Native"
cub-scout map workloads
```

Capture this before migration so you can compare after import.

### 2. Start With One Namespace

```bash
cub-scout import -n <namespace> --dry-run
```

Review:

- discovered workloads
- proposed Component name
- proposed Base Variant + Deployable Variant structure
- inferred labels (`Component`, `Variant`)
- draft Connections (discovered Secrets, ConfigMaps, ServiceAccounts, etc.)

If the proposal is wrong, stop and adjust naming/labels strategy first.

### 3. Execute Import

```bash
cub-scout import -n <namespace>
```

Or non-interactive:

```bash
cub-scout import -n <namespace> --yes --connect
```

This creates the Component, Base Variant, and one Deployable Variant
per discovered environment. ConfigHub state only — no cluster changes.

### 4. Verify Import

```bash
cub space list -l Component=<component>
cub-scout map workloads
cub-scout tree ownership

# Connected three-way check (intent vs render vs observed)
cub-scout compare three-way --scope namespace/<namespace> --format ascii
```

Success criteria:

- expected workloads exist in ConfigHub under the new Component
- workload ownership context is still coherent
- no unexpected resource drift

If you need a repeatable proof harness rather than a generic checklist,
use the local AI-first demos:

- [ArgoCD Import Demo](../../examples/argo-import-confighub-demo/) with
  `./setup.sh` then `./verify.sh` proves cluster, ConfigHub, and cub-scout
  surfaces and currently shows a sample `cub-scout scan` finding.
- [Flux Import Demo](../../examples/flux-import-confighub-demo/) with
  `./setup.sh` then `./verify.sh` proves the same three surfaces and currently
  reports scan evidence using the same three-surface model; the exact scan
  outcome is environment-specific.

For the takeover flow (Pattern 1), see the worked examples at:

- [Onboard Existing ArgoCD](../../examples/onboard-existing-argocd/) *(planned, not shipping yet)*
- [Onboard Existing Flux](../../examples/onboard-existing-flux/) *(planned, not shipping yet)*

Those demo proof paths are environment-specific examples. Do not generalize
a single demo scan outcome into a universal import success criterion.

### 5. Keep Existing Deployer During Validation

Do not immediately remove Argo/Flux/Helm control. First validate that
ConfigHub state is correct.

For Argo App-of-Apps / ApplicationSet setups:

- treat generated/child Applications as workload sources of truth
- treat parent orchestration objects as orchestration metadata, not imported

### 6. Decide: Stop Here, or Continue with Takeover

This is the new fork.

- **Stop here** if you only want ConfigHub as a parallel source of truth
  during validation. Your existing controller keeps reconciling from Git.
  Cut over by policy when you're ready (see step 7).
- **Continue with takeover** if you want ConfigHub to become the controller's
  source via OCI. Use [Onboard Existing](onboard-existing.md) — it picks up
  from the imported state and performs the controller-source repoint.

### 7. Cut Over by Policy, Not Accident

If you stayed in import-only mode, after validation pick one
controller-of-record per workload path.

Recommended order:

1. validate ConfigHub import result
2. update team policy for controller ownership
3. remove duplicate reconciliation paths only after policy is explicit

Avoid dual-control long term (two systems reconciling the same manifests).

If you went via [Onboard Existing](onboard-existing.md), the controller
is already pointed at ConfigHub OCI and step 7 is implicit.

### 8. Repeat Namespace by Namespace

```bash
# Example: expand to next namespace
cub-scout import -n <next-namespace> --dry-run
cub-scout import -n <next-namespace> -y
```

Scale out only after single-namespace validation passes.

## Rollback

If a namespace migration is not acceptable:

```bash
# Remove created state (Component-scoped)
cub space list -l Component=<component>
cub space delete <space-slug>
```

Then continue using your existing Argo/Flux/Helm flow while you revise
mapping.

Rollback is safe: import creates ConfigHub state only — it does not
modify workload manifests in the cluster.

If you ran the takeover flow ([Onboard Existing](onboard-existing.md)),
rollback is different: you also need to repoint the controller back to
Git. See that document's rollback section.

## Notes

- `cub-scout import --json` is for proposal automation and GUI workflows
  (`evidence.source` shows `cluster` vs `bundle`).
- `workloads[].connected` in `--json` is computed from live cluster
  labels and a same-Component unit lookup, so reruns can stay connected
  even before labels are re-read.
- `cub-scout import --wizard` runs the interactive TUI wizard.
- `cub-scout import --from-bundle <path>` uses bundle facts instead of
  live cluster discovery.
- `cub-scout import` performs connected delegation for Argo/Flux when
  available, then imports leftovers.
- Snapshot-imported workloads are linked back with
  `confighub.com/UnitSlug=<unit-slug>` labels.
- Spaces created by import carry `Labels.Component=<name>` and
  `Labels.Variant=<env|base>` so Component- and Variant-scoped queries
  work.
- This path prioritizes predictable migration over fast migration.

## Repeatable Delegation Check

For authors and users validating import behavior after upgrades:

```bash
make test-import-delegation
# or:
./scripts/test-import-delegation.sh
```

## Related Docs

- [Onboard Existing](onboard-existing.md) — Pattern 1 takeover (controller source repoint)
- [Migration Playbook](migration-playbook.md) — Comprehensive guide with assessment, planning, validation, and rollback
- [ArgoCD Import Demo](../../examples/argo-import-confighub-demo/) — Three import tools compared on real ArgoCD cluster (kind)
- [Flux Import Demo](../../examples/flux-import-confighub-demo/) — Management + discovery on real Flux cluster (kind)
- [Import from Live](import-from-live.md) — Cluster-only discovery (no Git required)
- [Import from Live Example](../../examples/import-from-live/) — Worked example with fixtures
- [Import from Bundle Example](../../examples/import-from-bundle/) — Worked example with expected dry-run JSON output
- [Combined Git+Live Example](../../examples/combined-git-live/) — Git repo + cluster alignment
- [Fleet Import Example](../../examples/fleet-import/) — Multi-cluster aggregation
- [Business Outcomes](../outcomes/README.md) — Why ConfigHub import matters
- [ConfigHub Documentation](https://docs.confighub.com) — Full ConfigHub guide
- [Import Docs Crosswalk](../reference/import-docs-crosswalk.md) — Archived import docs mapped to current docs
