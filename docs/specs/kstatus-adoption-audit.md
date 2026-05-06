# kstatus adoption — audit

**Status:** Filed as [#394](https://github.com/confighub/cub-scout/issues/394). Not implemented.
**Background:** Inspired by [ahmetb/kubectl-tree](https://github.com/ahmetb/kubectl-tree)'s
use of `sigs.k8s.io/cli-utils/pkg/kstatus/status` for deterministic
status derivation. See discussion thread on the import session
2026-05-06.

---

## What kstatus is

`sigs.k8s.io/cli-utils/pkg/kstatus/status` is the Kubernetes SIG-CLI
reference library for deriving a resource's deployment status from its
`.spec` / `.status` block. It returns one of:

- `Current` — resource is reconciled and healthy
- `InProgress` — controller is still working
- `Failed` — terminal failure
- `Terminating` — being deleted
- `NotFound` — does not exist
- `Unknown` — cannot determine

It handles built-in workload kinds (Deployment, StatefulSet, DaemonSet,
Job, CronJob, Pod, Service, ReplicaSet, …) natively and falls back to a
generic `status.conditions[]` reader for CRDs. The same library backs
Argo CD's health computation and Flux's readiness checks via shared
upstream code paths, so adopting it aligns cub-scout's view with what
operators see in their existing tools.

Already in cub-scout's transitive deps via `client-go` neighbours; the
direct import would add `sigs.k8s.io/cli-utils` to `go.mod` (single
small dep, no big graph).

---

## Audit: where status is derived in cub-scout today

The grep pass on 2026-05-06 found status / readiness / health-deriving
code at the following sites. They are not all the same kind of work,
but they collectively describe the surface that adoption would touch.

### Workload-level Ready (3 kinds × 2 callers = 6 duplicated sites)

`cmd/cub-scout/import.go:1214,1252,1290` and
`cmd/cub-scout/import_argocd.go:754,779,891` each compute Ready
manually for Deployment, StatefulSet, and DaemonSet:

```go
Ready: d.Status.ReadyReplicas == *d.Spec.Replicas
Ready: s.Status.ReadyReplicas == *s.Spec.Replicas
Ready: d.Status.NumberReady == d.Status.DesiredNumberScheduled
```

These six sites are mechanically identical between `import.go` and
`import_argocd.go`. kstatus replaces the whole pattern with one call,
and removes the per-kind branch (Pod and Job are also handled correctly,
which the manual code skips).

### Generic condition-based Ready (1 helper, 5+ callers)

`pkg/agent/state_scanner.go:1426 isReadyOrUnknown(item)` — checks
`status.conditions[]` for a `type=Ready` entry. Returns true on
`True` or `Unknown`. Used at lines 239, 319, 1082, 1333 in the same
file (silent-failure scan, workload scan, etc.).

This is the same job kstatus does, with the difference that kstatus
also considers `type=Stalled`, `type=Reconciling`, and built-in
kind-specific status fields. Replacement is a one-for-one swap with a
small behaviour broadening.

### Stalled / failed condition checks (2 sites, time-based)

`pkg/agent/state_scanner.go:438, 538`:

```go
if (condType == "Ready" && status == "False") ||
   (condType == "Stalled" && status == "True") { ... }
```

These are time-based stuck-detection rules, so kstatus is the *input*
("is this Failed?") not the whole feature. The time-window logic stays
local; the condition reading itself becomes a kstatus call.

### Colour-mapping by status string (2 sites)

`cmd/cub-scout/color.go:125, 217`:

```go
case lower == "healthy" || lower == "ready" || lower == "true" ||
     lower == "running" || lower == "succeeded": ...
```

These are downstream of whatever produces the status string in the
first place. If the producers move to kstatus, the strings normalise
to `Current` / `InProgress` / `Failed` / `Terminating` / `Unknown`,
and these branches collapse to a small canonical set.

### GitOps-controller propagation (3 sites, controller-specific)

`cmd/cub-scout/gitops.go:276, 307, 316` propagate `details.Ready` from
controller-specific status (Flux Kustomization, Argo Application,
Helm Release). These are *controller-level* readiness, not workload
readiness. kstatus can still derive them — Argo Application and Flux
Kustomization both expose standard `status.conditions[]` shapes — but
the propagation logic is doing more than status reading (it's wiring
the controller status into cub-scout's GSF graph). Out of scope for
the swap; flag for a follow-on pass.

### Bridge Worker condition strings (multiple sites in hierarchy.go)

`hierarchy.go` has many `Condition == "Ready"` and
`Condition == "Disconnected"` checks for ConfigHub Bridge Workers.
These are not Kubernetes `.status.conditions[]` — they are
cub-scout-internal model fields populated from ConfigHub's API. Out
of scope for kstatus.

### Trace / explain controller-specific paths

`pkg/agent/flux_trace.go` and `pkg/agent/argo_trace.go` have their own
status reasoning. Not yet surveyed in detail. Likely some of it
overlaps with what kstatus provides; some of it is controller-specific
provenance that kstatus would not replace.

---

## Estimated swap size

- **Mechanical replacement**: 6 sites in `import.go` /
  `import_argocd.go`, ~20 lines each → small.
- **Helper consolidation**: `isReadyOrUnknown` in
  `state_scanner.go` becomes a one-liner that calls kstatus → small.
- **Colour mapping**: shrinks once status-string producers are
  uniform → small, downstream.
- **GitOps controller propagation**: out of scope for the initial
  swap; revisit after the first pass lands → separate issue.
- **Trace / explain paths**: out of scope; revisit → separate issue.

The first-pass scope is therefore: workload-level Ready computation in
the import path, plus the single condition-reading helper in the
state scanner. That is the cleanest "drop in kstatus" PR with the
smallest blast radius. It does not touch GSF semantics or the Bridge
Worker model.

A second pass can extend kstatus to the GitOps controller propagation
once the first pass has shaken out CRD-conformance edge cases (CRDs
that don't follow the standard `conditions[]` shape, which kstatus
handles via `--condition-types`-style configurability).

---

## Trade-offs and risks

**Behavioural delta.** kstatus considers `Stalled=True` to be
`Failed`, where cub-scout's `isReadyOrUnknown` only checks `Ready`.
Anything that was `Ready=Unknown, Stalled=True` will now report
`Failed` rather than "ready or unknown." This is almost certainly
the right behaviour, but it is a behaviour change and should be
called out in the PR description.

**CRD compatibility.** Some custom resources do not expose a
standard `status.conditions[]`. kstatus returns `Unknown` for them.
Today, cub-scout's manual `isReadyOrUnknown` also returns "unknown"
in that case — same outcome. No regression expected.

**`--condition-types` adoption.** kubectl-tree exposes a
`--condition-types` flag that lets users override which condition
types count toward "Ready." Worth considering as a CLI flag on
`map`, `tree`, and `scan`; not load-bearing for the swap itself.

**Dependency footprint.** `sigs.k8s.io/cli-utils` is the
dependency. It pulls in `cli-runtime` and `kustomize/kyaml` as
transitive deps. Cub-scout already has `kustomize/kyaml` via Argo
Application parsing. `cli-runtime` is new. Net add: small.

**Aligning with operator expectations.** Argo CD and Flux UIs derive
health roughly the same way. Operators familiar with those tools see
the same Current/InProgress/Failed labels in cub-scout. This is the
strongest argument: cub-scout stops having a private dialect of
"ready."

---

## Issue draft

Title:
> Adopt sigs.k8s.io/cli-utils/pkg/kstatus for status derivation

Body:

```markdown
## Background

Cub-scout currently derives workload readiness in 6+ places using
manual `ReadyReplicas == Spec.Replicas` comparisons (per kind), and
in `pkg/agent/state_scanner.go` using a custom condition-reading
helper. The result is duplication, kind-by-kind branching, and a
private dialect of "ready" that doesn't match what operators see in
Argo CD or Flux.

`sigs.k8s.io/cli-utils/pkg/kstatus/status` is the SIG-CLI reference
library that derives a resource's status (`Current` / `InProgress` /
`Failed` / `Terminating` / `NotFound` / `Unknown`) deterministically
from its `.spec` and `.status` blocks. It handles all built-in
workload kinds natively and falls back to standard `conditions[]`
reading for CRDs. It backs Argo CD and Flux health computation
upstream.

## Goal

First pass: replace cub-scout's manual workload-Ready code in the
import path and the `isReadyOrUnknown` helper in the state scanner
with kstatus calls. Out of scope: GitOps controller propagation
(`gitops.go`), Bridge Worker condition strings (`hierarchy.go`),
trace/explain controller-specific paths (`flux_trace.go`,
`argo_trace.go`) — those are tracked separately.

## Audit

See [docs/specs/kstatus-adoption-audit.md](docs/specs/kstatus-adoption-audit.md)
for the full site-by-site list, behavioural-delta notes, and
risk register.

## Acceptance

- `cmd/cub-scout/import.go` and `cmd/cub-scout/import_argocd.go`
  use kstatus for workload Ready instead of per-kind manual
  comparisons.
- `pkg/agent/state_scanner.go:isReadyOrUnknown` calls kstatus.
- Existing tests pass. Where behaviour changes (e.g. a
  `Stalled=True` resource newly reporting Failed), update goldens
  with a comment explaining the broadening.
- New behavioural delta documented in `docs/reference/health-failure-states.md`.
- A CRD-conformance smoke test against a Crossplane composition
  resource is added to verify the fallback path on a non-standard
  CRD.

## Out of scope (separate issues)

- kstatus adoption in GitOps controller propagation (`gitops.go`).
- kstatus adoption in trace/explain (`flux_trace.go`, `argo_trace.go`).
- A user-facing `--condition-types` flag on `map`, `tree`, and `scan`.

## Refs

- [kubectl-tree](https://github.com/ahmetb/kubectl-tree) — prior art using kstatus
- [docs/specs/kstatus-adoption-audit.md](docs/specs/kstatus-adoption-audit.md) — audit
- [docs/concepts/architecture.md](../concepts/architecture.md) — parse-don't-guess principle
```

---

## Status of this document

Filed as [#394](https://github.com/confighub/cub-scout/issues/394) on
2026-05-06. This document remains the long-form audit; the issue body
is the short-form summary derived from it.
