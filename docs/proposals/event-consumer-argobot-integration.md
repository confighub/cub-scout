# Event-Consumer And Argobot Integration

Status: next-release planning. Tracked in
[#502](https://github.com/confighub/cub-scout/issues/502).

This note captures the current integration facts for ConfigHub's event-consumer
path and the open-source `argobot` consumer so cub-scout can integrate with the
right boundary.

## Verified Facts

- ConfigHub's event system is deployed to production.
- `argobot` is running in ConfigHubOps as an in-cluster event consumer.
- The dogfood delivery path now uses OCI bundle releases instead of Unit apply
  as the primary path.
- `argobot` listens for release publishing and force-syncs the corresponding
  Argo CD Application.
- Current `argobot` is an immediacy layer, not a live status reporter. It does
  not yet push Argo or Kubernetes health back to ConfigHub.
- `cub` supports `CUB_CONTEXT`, which should be used in multi-terminal and
  multi-agent work to avoid context bleed between ConfigHub sessions.

Sources:

- [confighub/argobot](https://github.com/confighub/argobot)
- [ConfigHub event-consumer docs PR](https://github.com/confighubai/docs/pull/122)
- [AI and software deployment blog post](https://confighub.com/blog/ai-and-software-deployment)

## Integration Boundary

The event log is trigger evidence, not delivery truth.

For the current production shape, cub-scout can use these evidence layers:

| Layer | Owner | What cub-scout may report |
|---|---|---|
| Release event | ConfigHub | A release-published fact, when connected evidence exposes it. |
| Event consumer | argobot | That an in-cluster consumer exists and appears healthy as a normal Kubernetes workload. |
| Delivery controller | Argo CD | Application sync, health, operation, and source evidence from Argo-owned status. |
| Runtime | Kubernetes | Workload generation, kstatus, pod symptoms, and Kubernetes Events. |
| Feedback write-back | Future ConfigHub API shape | External observed status with `observedAt`, freshness, and omissions once the REST shape exists. |

cub-scout should not claim that `argobot` proves delivery success. A successful
event reaction only means the immediacy layer attempted to nudge Argo; Argo and
Kubernetes still own sync and runtime truth.

## Cursor Safety

ConfigHub event-consumer cursors are server-held and keyed by the worker plus
subscription `Name`. Delivery is at-most-once: the server advances the cursor as
events are delivered, without a later client acknowledgment.

Therefore:

- cub-scout must never reuse the production `argobot` worker and subscription
  name.
- If cub-scout reads the event stream directly, it must use its own dedicated
  read-only worker/subscription name.
- Prefer a non-consuming REST/history surface for observation if one exists.
- Any event-log observation must state its coverage: first connect starts at the
  log tail unless an explicit earlier cursor is requested.

This is a correctness guard, not just an implementation detail. Sharing a cursor
could steal events from the delivery consumer.

## User Questions To Cover

| User question | Evidence path |
|---|---|
| Was a release published for this desired state? | Connected release/event evidence, with release number, digest, target, and `observedAt` where available. |
| Did the immediacy layer react? | Argobot structured evidence when available; otherwise only ordinary workload health/log-adjacent evidence, with omissions. |
| Did Argo actually sync the app? | Existing Argo CD Application status, operation state, source revision/digest, and Kubernetes Events. |
| Did the workload converge after sync? | Existing generation-aware rollout, kstatus, pod symptoms, and event evidence. |
| Can an agent answer this without hammering clusters? | Snapshot/watch/summary/receipt evidence today; future event-log reads only through observer-safe cursors or non-consuming history. |
| Is this delivery failure or runtime failure? | Keep release/event facts, Argo sync state, and Kubernetes runtime symptoms separate in output. |

These rows should inform the README user-question table once a user-visible
feature ships. Until then, the README must not imply that cub-scout already
reads the ConfigHub event log or Argobot feedback.

## Proposed Implementation Phases

1. Document the production shape and safety rule.
2. Add fixtures for OCI release, argobot Deployment, Argo Application sync, and
   workload rollout evidence.
3. Add a normalized evidence model for external event-consumer observations:
   source type, subject, target, release number, digest, app ref, `observedAt`,
   and omissions.
4. Wire read-only surfaces only where deterministic identifiers exist:
   `trace`, `explain`, `map activity`, `doctor`, and receipts.
5. Add feedback write-back parsing after ConfigHub exposes a stable REST shape
   for reported delivery/application observations.

## Open Questions

- Is there a non-consuming REST/history API for event-log observation, or must
  cub-scout use a dedicated observer cursor?
- What stable fields link `release.published` to an Argo CD Application in the
  general case: space slug, target, release digest, app annotation, or an
  explicit mapping?
- Will `argobot` emit structured Kubernetes Events or ConfigHub feedback records
  for attempted sync, sync failure, and sync accepted states?
- What labels or annotations identify an `argobot` Deployment without relying on
  image/name heuristics?
- Which `cub` version first supports `CUB_CONTEXT`, and should cub-scout docs
  mention a minimum version for multi-agent examples?

## Acceptance Criteria

- Missing event access, absent `argobot`, or absent feedback write-back produces
  structured omissions, never false healthy/synced claims.
- JSON output separates release/event, event-consumer, Argo delivery, and
  Kubernetes runtime evidence.
- ASCII and Markdown output labels Argobot evidence as external context, not as
  cub-scout-owned truth.
- Tests cover present, partial, malformed, stale, and absent event-consumer
  evidence.
- No implementation path mutates ConfigHub, Argo CD, `argobot`, or Kubernetes.
