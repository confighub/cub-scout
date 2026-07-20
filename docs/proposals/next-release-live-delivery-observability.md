# Next-Release Mini PRD: Live Delivery Observability

Status: partially shipped in v2.7.0; remaining follow-ups are tracked in
`docs/roadmap.md`

Audience: maintainers and reviewers

Anonymization note: this document intentionally uses controller families,
evidence types, and user questions instead of personal names, company names, or
external product names.

## Problem

Users need one read-only place to answer:

- What config should be where?
- Has delegated delivery accepted and applied that config?
- Is the current rollout still progressing, done, or blocked?
- If blocked, is the problem delivery, live drift, or application/runtime health?
- Can I move to the next task, wait, or retry?

The observer should not own deployment, rollout strategy, application success
definitions, or workflow orchestration. It should collect and explain the best
available evidence from the cluster and connected intent sources, then report
clear proof gaps when evidence is missing.

## Non-Goals

- No last-mile deploy, apply, sync, reconcile, restart, suspend, resume, or
  delete actions.
- No ownership of application-success policy. The observer may surface health
  and verdict evidence, but it must not become the authority for custom success
  definitions.
- No complex workflow orchestration.
- No hidden inference. If a controller does not expose status, lineage, source,
  artifact, event, or generation evidence, the result must say so.

## A. v2.7.0 Slice And Remaining Follow-Ups

### A1. First-Class Aggregate Delivery Resources

v2.7.0 status: partial. First-class resource discovery, ownership,
deployer/status, direct trace lookup, `gitops status`, and activity support
shipped. Deeper source/generated-artifact lineage and aggregate delivery
failures as top-level `doctor` findings remain follow-ups.

Add first-class observation for aggregate-report, app-bundle, input-provider,
external-artifact, and generated-artifact resource families.

User value:

- Operators can see delivery readiness, inventory, source freshness, rendered
  artifact freshness, and aggregate failure reasons from the same read-only
  surface they use for workloads.
- Reviewers can check whether the observer is reading controller status instead
  of guessing.

Surfaces:

- `gitops status`: include aggregate report and app-bundle health alongside
  existing delivery health.
- `map activity`: show recently changed aggregate delivery resources and their
  associated events.
- `trace`: include source, input-provider, generated-artifact, delivery
  resource, and workload edges when labels, owner references, status refs, or
  annotations provide deterministic links.
- `doctor`: include aggregate delivery failures as top-level findings when they
  block source, render, apply, or reconciliation progress.

Acceptance criteria:

- Deterministic fixtures cover ready, progressing, failed, missing-status, and
  stale-generation cases.
- JSON output contains resource kind, namespace, name, observed generation,
  ready condition, reason, message, last transition time where present, and
  evidence omissions where absent.
- ASCII/Markdown output avoids false "unmanaged" or false "healthy" claims
  when aggregate resources are present but incomplete.
- The feature remains read-only and works when only a subset of the CRDs are
  installed.

### A2. Audited User-Action Event Ingestion

v2.7.0 status: partial. `map activity`, `trace`, and `explain` event summaries
ship action metadata when Kubernetes Events expose it. History and receipt
supporting evidence remain follow-ups.

Parse audited action events emitted by controller web consoles or automation
frontends when they are available as standard cluster events.

User value:

- Operators can answer "what triggered this reconcile/check/action?" without
  opening a separate UI.
- Incident and audit receipts can include human or automation action context
  when the cluster already exposes it.

Surfaces:

- `map activity`: list audited action events near the managed resource they
  affected.
- `trace`: include recent action events in the resource history section.
- `explain`: summarize action, actor, subject, and timestamp when those fields
  are present.
- `receipt verify`: optionally include audited action events as supporting
  evidence, without changing the verdict.

Acceptance criteria:

- Event parsing is schema-tolerant: unknown action annotations are preserved as
  raw evidence instead of dropped.
- Missing actor, group, or subject fields produce omissions, not guesses.
- Action events never imply a write by the observer.
- Tests cover present, partial, malformed, and unrelated event shapes.

### A3. Promote Rollout Progress and Verdicts

v2.7.0 status: shipped for `doctor`, `explain`, `compare three-way`, and
`receipt verify --predicate workloads-converged`.

Move generation-aware rollout evidence from receipt-only workflows into primary
diagnostic UX.

User value:

- The operator can answer the operational question directly: proceed, wait,
  retry delivery, or inspect runtime failure evidence.
- The observer can distinguish "not applied", "applied but not observed",
  "observed and progressing", "stalled", "runtime failed", and "insufficient
  evidence".

Surfaces:

- `doctor`: show rollout progress and verdict for unhealthy or changing
  workloads.
- `explain`: add a "current change" section with generation, observed
  generation, progress clock, workload status, pod symptoms, and verdict.
- `compare three-way`: include rollout verdict when LIVE differs from intended
  or rendered state.
- `receipt verify --predicate workloads-converged`: remain the auditable source
  of the same underlying evidence.

Acceptance criteria:

- JSON includes separate `progress` and `verdict` fields.
- Verdicts keep the existing receipt vocabulary: `PASS`, `WATCH`, `BLOCK`,
  `INCONCLUSIVE`.
- Progress is generation-scoped; stale observed generations cannot be reported
  as successful current-change completion.
- Runtime evidence is attached as evidence, not reclassified as delivery
  authority.

### A4. API-Load-Aware Inventory And Search

v2.7.0 status: follow-up. The README now points users at existing snapshot,
watch, summary, and receipt surfaces for repeated review; broader source
freshness metadata and low-load search plumbing remain open.

Make repeated and fleet-shaped observation cheap by preferring captured,
summarized, or watched evidence when a fresh wide cluster crawl is not required.

User value:

- Operators can ask broad questions across many namespaces or clusters without
  every UI, CLI, or agent interaction hammering live APIs.
- Reviewers can see whether an answer came from a fresh read, a watch-fed
  observation, a snapshot, a connected summary, or a receipt.

Surfaces:

- `snapshot`: remain the portable captured-state format for offline review and
  repeat analysis.
- `watch`: support low-cardinality event streams that external stores can index
  without requiring the observer to own a database.
- `summary` and fleet-oriented connected surfaces: prefer stored summaries for
  repeated overview/search flows, with freshness metadata in the output.
- `doctor`, `map`, `trace`, and `explain`: expose evidence freshness and source
  type when reading from captured or summarized inputs.

Acceptance criteria:

- JSON includes evidence source type: `live`, `snapshot`, `watch`, `summary`, or
  `receipt` where that distinction is known.
- JSON includes `observedAt` and, when applicable, `staleness` or freshness
  omissions.
- Wide queries have a documented low-load path using snapshot, watch output, or
  connected summaries.
- The observer never hides stale evidence behind fresh-sounding language.
- No mandatory persistent database is added to standalone mode.

ConfigHub event-consumer / Argobot follow-up:

- Current production shape: ConfigHub emits release events; `argobot` consumes
  them in-cluster and force-syncs Argo CD Applications.
- Treat this as trigger/immediacy evidence, not as delivery or application
  success evidence.
- Prefer ConfigHub-owned history or another non-consuming read API for
  release/event evidence.
- Do not reuse the production `argobot` worker/subscription cursor. ConfigHub
  event-consumer cursors are server-held and at-most-once; a direct observer
  subscription is fallback-only and must use its own dedicated cursor.
- Correlate OCI release evidence to Argo sync and Kubernetes rollout evidence
  only where deterministic identifiers exist.
- Status feedback from event consumers is future work until ConfigHub exposes a
  stable REST shape for those observations.

Detailed plan: [`event-consumer-argobot-integration.md`](event-consumer-argobot-integration.md).

## B. Controller-Family Parity

v2.7.0 status: follow-up for parity audit and structured fallback omissions
beyond the controller/resource support already shipped.

Parity means the observer can answer the same user questions for a controller
family using deterministic evidence. It does not mean every controller exposes
the same data or that the observer can make up missing data.

Required parity dimensions:

- Ownership and lineage.
- Delivery or reconciliation status.
- Desired/rendered/live comparison where a desired or rendered source exists.
- Generation-aware rollout progress and verdict for workload objects.
- Recent event and action history.
- API-load-aware inventory and search paths for repeated or fleet-shaped reads.
- Receipt-ready evidence with omissions for gaps.

| Controller family | Can reach parity? | What blocks exact parity |
|---|---:|---|
| Declarative application controllers | Yes, for status, lineage, events, drift, and rollout evidence when source refs and status conditions are present. | Template-only or opaque render paths may prevent exact desired/rendered/live proof. |
| Package release controllers | Mostly. Release status, chart/source refs, history, and workload ownership usually support the model. | Rendered values and hook-side effects may be unavailable unless the controller records them. |
| Fleet placement controllers | Partial to strong. Placement status and per-cluster rollout state can fit the same UX. | Source diff parity depends on whether the controller exposes a desired object set per cluster. |
| Composition controllers | Partial. Claim, composed resource, and managed resource lineage can be explained. | Full parity requires deterministic refs from claim to composed resources plus current-generation status on each layer. |
| Resource-graph orchestration controllers | Partial. Ownership and graph lineage can be shown where refs are explicit. | Controllers that hide intermediate resources or do not expose conditions cannot support complete verdicts. |
| Native workload controllers | Yes for rollout progress and runtime verdicts. | They do not answer delegated delivery status by themselves. |
| Custom controllers | Config-dependent. | Without registered resources, status paths, ownership labels, or event conventions, the observer can only report LIVE facts and omissions. |

Next-release parity rule:

- If a controller exposes status, observed generation, conditions, owner refs,
  source/artifact refs, or events, parse them.
- If it does not, return structured omissions.
- Do not downgrade missing evidence into "healthy", "synced", "orphaned", or
  "unmanaged".

## C. Delivery-Done Research Synthesis

### C1. Improve The Observer

Remaining work should preserve the release decision model that treats "done" as
two separate outputs:

- Progress: where the current change appears to be in the delivery/runtime
  path.
- Verdict: whether the current evidence supports proceeding.

Recommended model:

| Field | Meaning |
|---|---|
| `changeRef` | Controller-provided change id when available, otherwise a deterministic fingerprint of source, resource, generation, and timestamp evidence. |
| `progress.phase` | `pending`, `accepted`, `applied`, `observed`, `rolling_out`, `stalled`, `complete`, or `unknown`. |
| `progress.clock` | The timestamp used to decide whether progress is fresh, stale, or absent. |
| `verdict` | `PASS`, `WATCH`, `BLOCK`, or `INCONCLUSIVE`. |
| `reason` | Stable machine-readable reason such as `source_unavailable`, `apply_failed`, `stale_generation`, `runtime_failed`, or `evidence_missing`. |
| `evidence[]` | Conditions, events, generation values, managed fields, pod symptoms, source refs, rendered refs, and audit events. |
| `omissions[]` | Missing evidence required for stronger claims. |

Implementation recommendations:

- Add a shared rollout-decision builder used by `doctor`, `explain`,
  `compare three-way`, and receipts.
- Treat current-generation success as a point-in-time statement, not a permanent
  application-health guarantee.
- Surface runtime failures in user language, but keep raw cluster evidence
  beside the summary for reviewability.
- Support optional external evidence stores through existing watch and receipt
  outputs rather than adding a mandatory database to the observer.
- Prefer low-cardinality summaries in interactive views and keep high-cardinality
  evidence available in JSON/receipts.
- Add fixtures for conflict, retry-needed, runtime-startup-failure,
  stale-generation, and all-parts-succeeded cases.

### C2. Improve The User-Questions Table

The README table should stay near the top and remain framed around user
questions, not controller internals. The README table should cover:

- What is running, and who owns it?
- Is delegated delivery healthy?
- Has intended configuration reached the cluster?
- Is this rollout still progressing, complete, or stuck?
- Can I move to the next task, wait, or retry delivery?
- Is live state drifting from desired state?
- Is this a delivery problem or an application/runtime problem?
- Who changed this field, and where did the value come from?
- Can I keep auditable evidence of the check?

Acceptance criteria:

- The table avoids claims that the observer owns deployment or application
  success.
- Each row maps to an existing command, a shipped v2.7.0 enhancement, a
  follow-up enhancement, or an explicit evidence gap.
- The table uses "user question" language, not private stakeholder language.

## Definition Of Done

- Unit tests cover status extraction, event parsing, lineage construction,
  rollout-decision synthesis, and omission behavior.
- At least one example fixture demonstrates aggregate delivery status, audited
  action events, drift, and current-generation rollout verdicts together.
- `--format ascii|json|md` output exists for every changed user-visible surface.
- TUI and CLI stay semantically aligned.
- `go test ./...` passes.
