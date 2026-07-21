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

Preferred architecture: ConfigHub owns durable release/event history, and
cub-scout reads that history as evidence. `argobot` owns the operational
event-consumer cursor and Argo force-sync reaction. A direct cub-scout
event-consumer subscription is a fallback only, used when no non-consuming
history/read API is available.

For the current production shape, cub-scout can use these evidence layers:

| Layer | Owner | What cub-scout may report |
|---|---|---|
| Release/event history | ConfigHub | Release-published facts, event ids/cursors, release numbers, digests, targets, timestamps, and actor/audit context when exposed through a non-consuming read API. |
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
- Prefer ConfigHub-owned history or another non-consuming read API for
  observation.
- If cub-scout reads the event stream directly as a fallback, it must use its
  own dedicated read-only worker/subscription name.
- Any event-log observation must state its coverage: first connect starts at the
  log tail unless an explicit earlier cursor is requested.

This is a correctness guard, not just an implementation detail. Sharing a cursor
could steal events from the delivery consumer.

## ConfigHub History Requirements

A ConfigHub-owned history surface is the best fit for cub-scout because it is
durable, retrospective, queryable, and non-consuming. It should allow cub-scout
to ask bounded questions without joining the operational delivery stream.

Minimum useful fields:

- Event id or cursor.
- Event type, especially `release.published`.
- Space id and slug.
- Target id and slug.
- Subject entity type and id.
- Release number.
- OCI digest and bundle base name.
- Created timestamp.
- Actor or audit context when available.
- Payload as raw JSON or a typed payload reference.

Useful query dimensions:

- By space, target, release number, digest, event type, time window, and cursor
  range.
- Bounded result limits with stable sort order.
- Explicit freshness/coverage metadata so cub-scout can explain whether it read
  history, a snapshot, live Argo/Kubernetes state, or a receipt.

## User Questions To Cover

| User question | Evidence path |
|---|---|
| Was a release published for this desired state? | ConfigHub history evidence, with release number, digest, target, event id/cursor, and `observedAt` where available. |
| Did the immediacy layer react? | Argobot structured evidence when available; otherwise only ordinary workload health/log-adjacent evidence, with omissions. |
| Did Argo actually sync the app? | Existing Argo CD Application status, operation state, source revision/digest, and Kubernetes Events. |
| Did the workload converge after sync? | Existing generation-aware rollout, kstatus, pod symptoms, and event evidence. |
| Can an agent answer this without hammering clusters? | Snapshot/watch/summary/receipt evidence today; ConfigHub history first for release/event evidence, dedicated observer cursor only as fallback. |
| Is this delivery failure or runtime failure? | Keep release/event facts, Argo sync state, and Kubernetes runtime symptoms separate in output. |

These rows should inform the README user-question table once a user-visible
feature ships. Until then, the README must not imply that cub-scout already
reads the ConfigHub event log or Argobot feedback.

## Proposed Implementation Phases

1. Document the production shape and safety rule.
2. Pin the ConfigHub history/read API shape that cub-scout should consume.
3. Add fixtures for OCI release, argobot Deployment, Argo Application sync, and
   workload rollout evidence.
4. Add a normalized evidence model for external history/consumer observations:
   source type, subject, target, release number, digest, app ref, `observedAt`,
   and omissions.
5. Wire read-only surfaces only where deterministic identifiers exist:
   `trace`, `explain`, `map activity`, `doctor`, and receipts.
6. Add feedback write-back parsing after ConfigHub exposes a stable REST shape
   for reported delivery/application observations.

## Open Questions

- What ConfigHub history endpoint should cub-scout read for release/event
  evidence, and what auth/context does it require?
- What stable fields link `release.published` to an Argo CD Application in the
  general case: space slug, target, release digest, app annotation, or an
  explicit mapping?
- Does the history API expose actor/audit context and immutable event ids, or
  only release payload data?
- Will `argobot` emit structured Kubernetes Events or ConfigHub feedback records
  for attempted sync, sync failure, and sync accepted states?
- What labels or annotations identify an `argobot` Deployment without relying on
  image/name heuristics?
- Which `cub` version first supports `CUB_CONTEXT`, and should cub-scout docs
  mention a minimum version for multi-agent examples?

## Acceptance Criteria

- Missing event access, absent `argobot`, or absent feedback write-back produces
  structured omissions, never false healthy/synced claims.
- JSON output separates ConfigHub history, event-consumer, Argo delivery, and
  Kubernetes runtime evidence.
- ASCII and Markdown output labels Argobot evidence as external context, not as
  cub-scout-owned truth.
- Tests cover present, partial, malformed, stale, and absent event-consumer
  evidence.
- No implementation path mutates ConfigHub, Argo CD, `argobot`, or Kubernetes.
