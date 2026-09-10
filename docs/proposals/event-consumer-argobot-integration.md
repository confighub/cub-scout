# Event-Consumer And Argobot Integration

Status: initial integration shipped for the next release. Remaining correlation
work is tracked in [#502](https://github.com/confighub/cub-scout/issues/502).

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
- `argobot` remains an immediacy layer for release events, and also reports a
  concise Argo-owned live-status summary back to ConfigHub on the Space
  annotation `confighub.com/live-status`.
- The live-status payload includes source, app, sync status, health status,
  operation phase, revision, message, and `observedAt`. It is best-effort
  reported evidence, not cub-scout-owned delivery truth.
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
| Feedback write-back | ConfigHub Space annotation | External observed status with `observedAt`, freshness, sync status, health status, operation phase, revision, and omissions when absent or malformed. |

cub-scout should not claim that `argobot` proves delivery success. A successful
event reaction or fresh writeback means the immediacy/status layer reported
what it saw; Argo and Kubernetes still own sync and runtime truth.

The first cub-scout integration is:

```bash
./cub-scout gitops status --with-confighub --confighub-space <space> --format json
```

This command adds bounded ConfigHub release history, unit-event history,
live-status writeback, and conservative event-consumer Deployment health under
`deliveryEvidence`. The event-consumer probe uses label-selected Deployment
reads, searches all namespaces when allowed, and reports a scope omission if
RBAC forces a namespace fallback. It never subscribes to or advances an
event-consumer cursor.

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

Minimum useful fields already consumed or planned:

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

## User Questions Covered

| User question | Evidence path |
|---|---|
| Was a release published for this desired state? | `gitops status --with-confighub` release evidence and MCP `confighub_releases`, bounded by space/time. |
| Did the immediacy layer react? | ConfigHub live-status writeback when present; conservative event-consumer Deployment readiness; omissions otherwise. |
| Did Argo actually sync the app? | Existing Argo CD Application status, operation state, source revision/digest, and Kubernetes Events. |
| Did the workload converge after sync? | Existing generation-aware rollout, kstatus, pod symptoms, and event evidence. |
| Can an agent answer this without hammering clusters? | Snapshot/watch/summary/receipt evidence plus bounded current-space/time-window ConfigHub reads; dedicated observer cursor only as fallback. |
| Is this delivery failure or runtime failure? | Keep release/event facts, Argo sync state, and Kubernetes runtime symptoms separate in output. |

These rows are mirrored in the README user-question table.

## Proposed Implementation Phases

1. [x] Document the production shape and safety rule.
2. [x] Pin the initial ConfigHub history/read surfaces available through `cub`.
3. [x] Add fixtures/tests for release evidence, unit events, event-consumer
   Deployment evidence, live-status freshness, and malformed/absent writeback.
4. [x] Add a normalized evidence model for bounded external
   history/consumer/writeback observations.
5. [x] Wire an initial user-visible read-only surface:
   `gitops status --with-confighub`.
6. [ ] Add deeper object-level correlation to `trace`, `explain`,
   `map activity`, `doctor`, and receipts once stable identifiers link release
   events, Space writeback, controller sources, and workload objects without
   guessing.

## Open Questions

- What stable fields link `release.published` to an Argo CD Application in the
  general case: space slug, target, release digest, app annotation, or an
  explicit mapping?
- Does the history API expose actor/audit context and immutable event ids, or
  only release payload data?
- Should live-status graduate from a Space annotation to a typed history/status
  API with server-side freshness/coverage metadata?
- Will the event consumer emit structured Kubernetes Events or ConfigHub records
  for attempted sync, sync failure, and sync accepted states beyond the current
  Space-level status summary?
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
