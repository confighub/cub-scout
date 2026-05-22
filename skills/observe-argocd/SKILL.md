---
name: observe-argocd
description: 'Use when the user wants to observe ARGO CD-managed resources specifically — the controller-specific observer skill for Argo. Natural phrasing: "is this resource managed by Argo CD?", "show me Argo applications in this cluster", "trace this Argo-owned deployment back to source", "which ApplicationSet generated this app?", "did Argo apply the latest commit?", "explain the Argo CSA migration default", "what tracking-id annotation does Argo write?". Load whenever intent involves Argo CD specifically — Applications, ApplicationSets, sync status, tracking-id annotation, the kubectl-client-side-apply migration default, or Argo-as-deliverer-for-ConfigHub. Do NOT load for: generic ownership detection (use scout-observe), comparing live state to git (use scout-compare), the ConfigHub authoring side (use cub skills), or general Argo CRD authoring (Argo is a separate tool — this skill is about *observing* Argo-managed resources, not configuring Argo itself).'
phase: cross-cutting
allowed-tools: Bash(./cub-scout doctor *) Bash(cub-scout doctor *) Bash(cub scout doctor *) Bash(./cub-scout explain *) Bash(cub-scout explain *) Bash(cub scout explain *) Bash(./cub-scout trace *) Bash(cub-scout trace *) Bash(cub scout trace *) Bash(./cub-scout map *) Bash(cub-scout map *) Bash(cub scout map *) Bash(kubectl get *) Bash(kubectl describe *) Bash(kubectl get --show-managed-fields *) Bash(argocd app get *) Bash(argocd appset get *) Bash(argocd app list *) Bash(argocd appset list *)
---

# observe-argocd

The controller-specific observer skill for **Argo CD**. Documents the exact labels / annotations / manager strings Argo writes, how cub-scout's ownership detector classifies Argo-managed resources, and the edge cases (CSA migration, tracking-id format, ApplicationSet generators).

This skill is verified against `pkg/agent/ownership.go` `detectArgoOwnership` and `pkg/agent/manager_strings.go` `ManagerArgoCD` — the constants are the single source of truth for what Argo writes.

## When to use

- "Is this resource managed by Argo CD?"
- "Show me the Argo Applications in this cluster"
- "Trace this Argo-owned deployment back to source"
- "Which ApplicationSet generated this Application?"
- "Did Argo apply the latest commit?"
- "Explain the Argo CSA migration / `kubectl-client-side-apply` writer"
- "What does the `argocd.argoproj.io/tracking-id` annotation mean?"

## Do not load for

- Generic ownership detection ("who manages this resource?") — [`scout-observe`](../scout-observe/SKILL.md) walks all detectors
- Comparing live state to ConfigHub or git — [`scout-compare`](../scout-compare/SKILL.md)
- Authoring / mutating Argo Applications — that's the Argo CLI directly, not cub-scout
- Flux-managed resources — [`observe-flux`](../observe-flux/SKILL.md)

## Detection signals (in `detectArgoOwnership`)

cub-scout classifies a resource as Argo-owned when ANY of these labels / annotations is present, in priority order:

| Signal | Source field | Confidence | Notes |
|--------|-------------|-----------|-------|
| `argocd.argoproj.io/instance` label | label | medium | Primary signal. Value is the Application name. |
| `argocd.argoproj.io/tracking-id` annotation | annotation | medium | Alternative. Value is `<app-name>:<group>/<kind>:<namespace>/<name>` — cub-scout parses the leading `<app-name>` portion. Malformed tracking IDs are tolerated (split-on-`:` with a graceful fallback). |
| `app.kubernetes.io/instance` label | label (fallback only) | medium | Only used when `argocd.argoproj.io/instance` is present but empty — Argo writes both, and the Argo-specific label is preferred. |

Resulting `Ownership`: `Type=argo`, `SubType=application`, `Name=<application-name>`.

## Manager strings (the writer identity in `metadata.managedFields`)

Argo writes through Server-Side Apply with two possible manager strings:

| Manager | Source | Meaning |
|---------|--------|---------|
| `argocd-controller` | upstream `common.ArgoCDSSAManager` | Modern Argo CD application-controller SSA writes. The expected and unambiguous case. |
| `kubectl-client-side-apply` | upstream `kubectl/FieldManagerClientSideApply` | Argo's **CSA migration default** — Argo writes with this string during the client-side-apply → server-side-apply migration. This is ALSO the bare `kubectl apply` default, so the string alone is ambiguous. |

### The CSA disambiguation

When `metadata.managedFields` carries `kubectl-client-side-apply` and there's no other Argo signal, the writer is ambiguous: it could be Argo CSA-migration or a human running `kubectl apply` (no `--server-side`).

cub-scout disambiguates via the **owner co-signal** (in `IsControllerManagerFor`):
- Argo-owned resource (label or tracking-id) + `kubectl-client-side-apply` → `controller-drift` (Argo CSA migration default)
- Native resource (no Argo signal) + `kubectl-client-side-apply` → `manual-edit` (a human ran `kubectl apply`)

Same string, opposite verdicts. This is why ownership detection must run before attribution classification — see `scout-attribute` for the full classification rule.

## ApplicationSet generators

When an Application was generated by an ApplicationSet (git generator, list generator, cluster generator, matrix generator), the parent Application label `argocd.argoproj.io/instance` still identifies it. cub-scout's git source tracer (`pkg/agent/argo_trace.go`) reads the Application spec and resolves the git path back to the generator's `directories[].path` pattern when applicable. ApplicationSet import is also supported by `cub-scout import argocd` — see [`scout-ingest`](../scout-ingest/SKILL.md).

The full ApplicationSet generator coverage:
- Git generator with `directories[].path` patterns including `!exclude`
- List generator with explicit Application enumeration
- Cluster generator with selector-based fan-out
- Matrix generator with nested git generators
- Full-path slugs so `apps/team-a/api` and `services/team-a/api` don't collide

## Worked example

```bash
$ kubectl get deploy api -n prod -o yaml | yq '.metadata.labels, .metadata.annotations, .metadata.managedFields[].manager'
labels:
  argocd.argoproj.io/instance: payments-api
  app: api
annotations:
  argocd.argoproj.io/tracking-id: "payments-api:apps/Deployment:prod/api"
managers:
  - argocd-controller
  - kubectl-edit          # uh oh
```

cub-scout reads this as:

```
$ cub-scout explain deploy/api -n prod
Resource: Deployment/api in prod
  Owner:    Argo CD application "payments-api" (label:argocd.argoproj.io/instance)
  Drift:    Detected
  Mutation cause: manual-edit (manager: kubectl-edit; controller co-signal: argocd-controller)
```

The classifier sees the Argo controller co-signal AND an interactive writer (`kubectl-edit`) → cause is `manual-edit` (someone bypassed Argo). See [`scout-attribute`](../scout-attribute/SKILL.md) for the full classification semantics.

## Edge cases

- **Argo CD as deliverer for ConfigHub units**: A ConfigHub-managed resource (label `confighub.com/UnitSlug`) delivered through Argo will have BOTH the ConfigHub label and the Argo signals. cub-scout treats this as `OwnerConfigHub` (ConfigHub detection runs before Argo detection); the Argo writer is then accepted as a valid controller co-signal for ConfigHub. See [`observe-confighub-managed`](../observe-confighub-managed/SKILL.md).
- **Empty `argocd.argoproj.io/instance` label**: cub-scout falls back to `app.kubernetes.io/instance` only when the Argo-specific label is present-but-empty. A pure `app.kubernetes.io/instance` label without any Argo signal does NOT classify as Argo-owned.
- **Malformed tracking-id**: `argocd.argoproj.io/tracking-id` without a `:` separator is tolerated; cub-scout uses the whole string as the Application name. Empty tracking-id is rejected.

## References

- Code: `pkg/agent/ownership.go` `detectArgoOwnership`, `pkg/agent/manager_strings.go` `ManagerArgoCD` + `controllerManagersForOwner(OwnerArgo, ...)`, `pkg/agent/argo_trace.go`
- Upstream:
  - Argo CD SSA manager constant: [`argoproj/argo-cd common.ArgoCDSSAManager`](https://github.com/argoproj/argo-cd)
  - Argo CD tracking-id format: ApplicationInstance label spec (`<app-name>:<group>/<kind>:<namespace>/<name>`)
- Related skills: [`scout-observe`](../scout-observe/SKILL.md), [`scout-attribute`](../scout-attribute/SKILL.md), [`scout-compare`](../scout-compare/SKILL.md), [`observe-confighub-managed`](../observe-confighub-managed/SKILL.md), [`observe-flux`](../observe-flux/SKILL.md)
- Examples: [`examples/argo-import-confighub-demo/`](../../examples/argo-import-confighub-demo/), [`examples/drift/mutation-cause-attribution/`](../../examples/drift/mutation-cause-attribution/)

## Constraints

- This skill is read-only by construction. `argocd app sync`, `argocd app patch`, `argocd app delete` are NOT in the allowed-tools — those are mutations and belong to the user driving Argo CLI directly.
- cub-scout's Argo support is **observation**, not authoring. Configuring Argo Applications themselves is out of scope; route to the Argo CLI.
- ApplicationSet *parsing* (for import-preview) lives in [`scout-ingest`](../scout-ingest/SKILL.md); this skill is about observing the *resulting* Application-managed workloads in the cluster.
