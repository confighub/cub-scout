# cub-scout Unified Roadmap

> **Positioning:** The read-only agentic observation layer that makes Kubernetes and GitOps understandable.

> **Status:** Authoritative
>
> This document **replaces and subsumes**:
>
> * `/ROADMAP.md`
> * `/docs/archive/NEXT-PLAN.md` (historical planning draft)
> * `/SESSION.md` roadmap/status snapshots
> * scattered milestone notes and implicit issue groupings
>
> It reconciles *historical releases*, *current shipped reality*, and the *forward execution plan*.

---

## Untracked Backlog Checklist

This checklist tracks ideas from planning docs. Items marked "scoped" had issues filed and
closed as scope definitions — the contract/design was documented but runtime implementation
remains future work. Items marked "resolved" had issues filed, implemented, and closed.

Tracking: issue **#154** is closed. This checklist is now the live tracker.

### v2.9.0 - Scoped Resource Reads and Release Readiness

**v2.9.0 is published** (2026-09-11), following v2.8.0. This is an
additive release with existing CLI, JSON, MCP, and receipt contracts preserved.
Tracking: [#520](https://github.com/confighub/cub-scout/issues/520). The release
proof and remaining checks live in [v2.9.0 notes](releases/v2.9.0.md).

- [x] ConfigHub Resource-backed Kubernetes MCP reads (`#516`) and generic
  Resource-index queries (`#517`) are merged; they read intended configuration,
  not live Kubernetes health.
- [x] Finish `#518` activity joins in `#521`. Prove Application-to-space identity from
  observed metadata/source, reject ambiguous names or conflicting statuses,
  and preserve controller-owned results.
- [x] Prove API-load behavior: count scoped commands and repeated queries, verify
  freshness handling, and distinguish output limits from network request or
  response-size limits. Observation timestamps alone are not a load reduction.
- [x] Validate connected read contracts against supported `cub` capabilities, context
  isolation, missing APIs/auth, and stale/malformed evidence. Use deterministic
  fixtures. Live authenticated smoke is unavailable because the token expired;
  this is documented, not counted as a passing live test.
- [x] Update release notes, handover, compatibility guidance, and the README
  user-question table. Every new capability must have a question, an exact
  command/tool, evidence semantics, and a test/example proving the answer.
- [x] Verify the final release commit: build, full tests, read-only contracts,
  standalone/plugin parity, MCP, watch, bot, release archives, and locally built container.
- [x] Reconcile `#502` and `#505` with explicit completed and deferred scope.
- [x] Publish the tag after release checks; verify all six archive checksums,
  standalone/plugin versions, release workflow, and Homebrew publication.
- [ ] Complete the registry-pull check in `#520`: anonymous pulls return 403;
  package visibility/read access needs an authorized owner to inspect it.
  Live authenticated ConfigHub smoke also remains unavailable (expired token).

### v2.10.0 - Bounded Explorer Integration

Published 2026-09-11, followed by companion v0.3.0. Additive explorer integration and real observation reuse,
not a replacement for the standalone TUI or a 3.0 contract break. Tracking stays
in this repository: `#519`, `#502`, and `#505`.
Release publication, final checks and explicit auth/registry limitations are
tracked in [#525](https://github.com/confighub/cub-scout/issues/525) and
[v2.10.0 notes](releases/v2.10.0.md). The release is the bounded first integration;
deeper joins and broader explorer parity are follow-up scope, not silent claims
of completion.

- [x] First provider slice merged in `#522` (`#519`): opt-in `explain --bounded` with explicit
  kube context/API identity, object-local facts, and structured omissions.
  CLI/plugin, MCP, and the existing TUI share the read model. Use
  `--kube-context` to avoid the host's ConfigHub context flag. See the
  [bounded read example](../examples/bounded-resource-read/).
- [x] Prove first-slice session reuse: finite TTL/entry count, visible refresh/freshness,
  exact request counts, cancellation, context isolation, and no stale success
  after failed refresh. Do not describe this as a global cache for all commands.
- [x] Add observed origin metadata to bounded explain (`#523` merged; `#505`):
  space/unit IDs and slugs plus optional integer revision, using the existing
  object read. Reject malformed/duplicate/conflicting metadata; do not infer
  controller ownership, target/context, release, component, or variant joins.
  CLI/plugin, MCP, and the current TUI share the evidence and omissions.
- [x] Wire the first companion evidence panel through the read-only CLI JSON
  boundary, with explicit target-to-kube-context binding and late-result rejection.
  Merged in [companion PR #1](https://github.com/confighub/cub-commander/pull/1),
  for separate v0.3.0 publication after Scout v2.10.0. Resource-only, explicit
  Target-ID bindings, one
  captured snapshot, zero-read tab revisits, stale labels and explicit refresh;
  no implicit source/release joins. Read-only live proof uses a mocked intended-state
  row plus an existing non-production object. Issue tracking stays in `#519`;
  no replacement of the existing standalone TUI is authorized by this item.
- [ ] Continue stable release/controller/workload joins and receipt evidence
  (`#502`) and component/variant/target provenance (`#505`) after this first slice.
- [x] First-slice capabilities have README user questions, exact invocations,
  deterministic tests, and a worked example, with unsupported parity explicit.
  Repeat this requirement for each subsequent explorer capability.

### Post-v2.10 Priorities

Source-pinned assessment: [explorer comparison](reference/explorer-comparison.md).
These are tracked follow-ups, not claims of competitive leadership:

- [x] First-run documentation and current-version install guidance (#527).
  Preserve the README User questions and all five run modes; distinguish live
  inspection from offline artifacts and observation from delivery automation.
- [x] Timestamp-gated feedback verdicts (#502), merged in #529 for v2.10.1
  (publication tracked in #530): invalid/missing/zero/future times are inconclusive; stale
  success/failure reports ask for a recheck. Original reports, timestamps and
  omissions survive MCP, activity, correlation and receipts. Recorded proof:
  [feedback freshness](../examples/live-delivery-observability/#trusting-feedback-freshness).
- [ ] Remaining feedback/identity correctness (#502/#505): deleted Applications,
  out-of-order observations, missing history, and exact expected-release to
  controller/workload joins. Timestamp validation alone cannot prove delivery.
- [ ] Measured explorer parity and API-load-aware continuous observation (#519).
  Compare identical task/permission/scale fixtures; specify native watch/store
  lifecycle, reconnection, freshness and cache bounds before implementation.
- [ ] Distribution completion (#520): anonymous registry pull, container
  architecture coverage, and a deliberate Go module major-version strategy.
  `go install ...@latest` currently resolves the older v1 line; archives,
  Homebrew, plugins and tagged checkout builds are the v2.10 paths.

### v2.11 - Delivery Proof And Measured Observation

Agreed scope: [#532](https://github.com/confighub/cub-scout/issues/532).
Ship the v2.10.1 correction first; keep the next minor release additive.

v2.10.1 is published (#530). User priority: scanner #534 and connected
authentication/authority verification are deferred, not release blockers.
Focus on standalone/plugin delivery evidence, efficient ongoing observation,
and dependable bot installation; do not change registry permissions implicitly.

- [x] First scoped intended-release check (#502/#505): #535 and #536 are merged,
  unreleased. Explicit OCI configuration digest, supported controller/source/
  target/inventory binding, live authored-field comparison and workload-controller
  convergence share bounded reads. CLI/plugin, MCP and refreshable TUI have
  tests, examples and README User questions. Both controller adapters have
  deterministic HTTP integration proof; live workload-only smoke remains
  explicitly inconclusive for controller identity.
  [Completed implementation plan](proposals/oci-release-check.md).
- [x] Running-image identity tier (#502/#505), opt-in, unreleased. `release check
  --check-running-image` (MCP `check_running_image`) compares the digest running
  in live pods with the intended workload image via one bounded, selector-scoped
  pod read per workload. Mutable tags stay UNKNOWN, a differing digest is MISMATCH
  with a multi-architecture caveat, and the headline never claims running when it
  is unconfirmed. Off by default; CLI/plugin, MCP and TUI have deterministic
  tests. [Plan](proposals/running-image-identity.md).
- [ ] Extend live OCI controller-join proof and adapter coverage (#502/#505).
  Broader target/source shapes, registry-side index/digest resolution, initContainer
  and matchExpressions selector coverage, and application-success policy are not
  implied by the first checks. Watch/bot do not schedule these checks yet.
- [x] First polling baseline (#533): actual HTTP requests and response-body
  bytes for 100/1,000-object cold, idle and ownership-change cycles; latency and
  allocations measured separately. [Reproducible example](../examples/observation-budget/).
  This does not complete mixed-controller, fleet, interactive or competitive
  benchmarks, nor implement native watches.
- [x] Observation efficiency Slice 1 (#539): within-cycle read coalescing. A
  per-cycle memoizing dynamic client shares one read of each type between the
  inventory sweep and the state scanner, cutting each cycle from 48 to 43
  requests and removing duplicated list bytes, with no change to observed data.
  Idle cycles still re-read full inventory. [Plan](proposals/observation-efficiency.md).
- [x] Observation efficiency Slice 2 (#539): opt-in `--watch-backed` for
  `watch`/`bot`. Inventory is served from Kubernetes watch informers (client-go
  handles relist/410/resync), so idle cycles read from cache instead of
  re-listing; only served, synced types are cached and any setup failure falls
  back to per-cycle polling. Adds a `resource.deleted` event (deletions were
  previously silent, in both modes) and a `watch-informer` observation mode.
  Long-running only; the state scan's own reads and full watch-backing of the
  scanner remain follow-up. Note: #519 is a different topic (Commander/TUI
  convergence) and was previously mis-cited for this work.
- [ ] Observation efficiency follow-ups (#539): move the state scan onto the
  watch cache (near-zero idle cost), a distinct freshness/staleness signal when
  a stream is degraded, and evaluate default-on after field validation.
- [x] Each implemented capability has a README User question, exact invocation,
  interactive equivalent or explicit limitation, tests and a worked example.
- [ ] Continue distribution checks (#520); do not silently equate recorded
  connected proof with authenticated live verification.
  The unreleased [bot image helper](../examples/bot/#build-a-local-image-from-a-release)
  builds a local amd64/arm64 runtime image from a checksum-verified release
  archive. It does not fix public registry pulls, push images or deploy a bot.

### After v2.9.0

Tracked follow-ups, **not v2.9.0 release blockers**:

- Commander integration: [#519](https://github.com/confighub/cub-scout/issues/519).
  Start with Scout JSON evidence panels and exact command/scope visibility;
  consider a structured provider later. Decide on a preferred connected TUI
  only after parity and usability proof; preserve standalone exploration.
  Reassess overlap with `#422` before expanding the existing connected TUI.
- Deeper release/controller/workload joins and aggregate/object-set/workload
  receipt enrichment (`#502`), requiring stable identities.
- Aggregate controller failures and generated-resource lineage in diagnostics;
  deeper Modelplane/Crossplane source and generation evidence.
- Component/variant/target provenance and broader Resource-backed fleet reads
  beyond the new MCP entry points (`#505`).
- Publication series `#475` stays delayed pending the evidence-boundary work.

### 3.0 Candidate Criteria

This is a direction and decision bar, not a committed release date or a reason
to introduce unnecessary breaking changes. Graduate new work into issues
before implementation.

- [ ] A coherent explorer: navigate intended configuration, controller source,
  workload, diagnostics, and comparisons; retain an effective standalone path.
  Commander integration is tracked in `#519`.
- [ ] Consistent evidence semantics across CLI, plugin, MCP, watch, bot, and TUI
  for deployment, revision, progress, drift, freshness, and missing evidence.
- [ ] Measured efficiency: reusable observations, explicit refresh, tested
  request budgets, cancellation, and visible freshness/coverage at fleet scale.
- [ ] A published fixture-backed controller capability matrix covering Argo,
  Flux, Sveltos, Modelplane, Crossplane, kro, Helm, and native workloads, with
  explicit limitations when equivalent status/source/generation evidence is absent.
- [ ] A necessary, deliberate public-contract transition, with deprecations,
  migration guidance, compatibility tests, and coordinated standalone/plugin
  delivery. Examples include retiring legacy commands or replacing existing
  JSON/MCP contracts. Under [Semantic Versioning](https://semver.org/spec/v2.0.0.html),
  additive features and an improved TUI can continue in 2.x; they do not by
  themselves require 3.0.0.

### Post-v2.8 ConfigHub-Native Boundary ([#505](https://github.com/confighub/cub-scout/issues/505))

- [x] Initial MCP Resource / `cub k8s get` / `cub k8s types` read paths for
  intended Kubernetes configuration and scoped fleet-resource surveys
  (`#516`, `#517`). Broader migration beyond MCP remains future work.
- [ ] Preserve cub-scout's live-cluster role for drift, runtime/workload
  evidence, non-Argo runtimes, receipts, MCP, and joined explanations that need
  Kubernetes/controller evidence
- [ ] Add component / variant / target provenance to cub-scout evidence where
  ConfigHub exposes stable identifiers
- [x] Treat `cub k8s source` as operator browser UX; bounded explain now parses
  `confighub.com/origin` directly as observed metadata without browser/network
  enrichment. No reverse lookup or connected join is claimed. Broader origin
  propagation and stable Resource joins remain in `#505`.
- [ ] Keep mutating `cub k8s refresh` and non-dry-run `cub k8s collect` outside
  cub-scout read-only surfaces

### Live Delivery Observability (`proposals/next-release-live-delivery-observability.md`)

- [x] First-class aggregate delivery resources (`FluxInstance`, `FluxReport`, `ResourceSet`, `ResourceSetInputProvider`, `ExternalArtifact`, `ArtifactGenerator`) in controller-resource discovery, ownership, deployer/status, trace, and activity surfaces
- [x] Audited user-action event ingestion for activity, explain, and trace event summaries
- [x] Generation-aware rollout progress/verdict UX promoted from receipts into doctor, explain, and compare surfaces
- [x] README user-question table for live delivery decisions
- [x] Initial API-load-aware ConfigHub delivery-evidence path: `gitops status --with-confighub` uses current-space/time-window reads for release history, unit events, live-status writeback, and event-consumer health
- [x] Initial ConfigHub history-backed event / Argobot evidence path with no production cursor sharing — tracked in [#502](https://github.com/confighub/cub-scout/issues/502)
- [x] MCP parity for release/event/live-status reads and `compare_source_truth` strategy enum
- [ ] Aggregate delivery failures as doctor top-level findings with deeper source/generated-artifact lineage where status refs expose it
- [ ] Audited user-action event ingestion for history and receipt supporting evidence
- [x] Broader API-load-aware inventory and search paths: map JSON entries, snapshot JSON, summary list JSON, and watch/bot events carry point-in-time observation metadata; `receipt list` summarizes saved receipt TTL stamps as fresh/stale/not-declared/invalid
- [ ] Initial live-status joins in `map activity --with-confighub` (`#518` / `#520`): require matching observed Application Space ID and unique exact name in a non-wildcard space; awaiting v2.9 release validation
- [ ] Deeper OCI release-to-controller-to-workload correlation, including aggregate/object-set/workload receipts, with direct observer cursors only as fallback — tracked in [#502](https://github.com/confighub/cub-scout/issues/502)
- [ ] Controller-family parity rules and fallback omissions for controllers without status, source, event, or generation evidence
- [ ] Modelplane-on-Crossplane hardening: trace, map JSON, watch/bot events, and receipts now surface substrate evidence; remaining work is deeper source/generation joins for Modelplane resources backed by Crossplane composition managers, with structured omissions where parity is not possible

### Rendered Manifest + Argo (`roadmap-rendered-manifest-and-argo.md`)

- [x] Workstream A: Distinguish "ConfigHub via OCI" ownership in trace outputs (relates to [ConfigHub rendered manifests](https://docs.confighub.com/guide/rendered-manifests/)) — validated by `TestArgoTracerConfigHubOCIDetection` + trace ASCII/JSON proofs
- [x] Workstream A: Show explicit `source → render → OCI → deployer → workload` chain — scoped in #149
- [x] Workstream A: Source staleness/sync signals where evidence is available — validated by `TestArgoTrace` + trace ASCII/JSON proofs
- [x] Workstream B: Source inventory view — resolved #148
- [x] Workstream C: Schema extension for `rendered_from` / `original_source` — scoped in #150
- [x] Workstream D: Bridge patterns (Git→Flux, Git→Argo, Git→ConfigHub→OCI, Live→ConfigHub→OCI) — scoped in #151
- [x] Workstream E: Orphan detection for broken ApplicationSet generator links — graduated to #232
- [x] Workstream F: Fleet query ergonomics and provenance readability — graduated to #242, #251
- [x] Workstream F: Impact analysis ergonomics and multi-cluster context clarity — graduated to #243, #254
- [ ] Workstream G: Platform-only surfaces (Functions, Actions, ChangeSets, saved queries, alert triggers, dependency graphing, time-travel UX, three-state drift resolution, bulk operations)

### Publication / GTM

- [ ] Blog/documentation publication series ([#475](https://github.com/confighub/cub-scout/issues/475)) — delayed until [#505](https://github.com/confighub/cub-scout/issues/505) and [#502](https://github.com/confighub/cub-scout/issues/502) settle, so external docs describe the current OCI/argobot/Resource boundary

### Connected Views + Launch (`roadmap-connected-views-and-launch.md`)

- [x] Workstream A: Navigation-first messaging and "aha in seconds" walkthrough — graduated to #224
- [x] Workstream B: Scale-real demo assets — resolved #158
- [x] Workstream B: Expected-output snapshots for core navigation flows — validated by `test/ascii` map status/list + tree golden proofs
- [ ] Workstream C: WET-LIVE panel clarity and causality messaging
- [ ] Workstream C: Break-glass decision ergonomics and audit visibility
- [ ] Workstream C: Hierarchy consistency across map/tree/unit surfaces
- [ ] Workstream D: Fleet view delivery sequence (by-cluster → compare → matrix → revisions → rollout/deps/incident/security)
- [ ] Workstream E: Claim→demo→status proof matrix
- [x] Workstream F: Testing gate contract (coverage matrix, CI-enforced gate, per-run proof artifact) — graduated to #244, #245, #256, #258, #259, #260

### Pattern Contract (`reference/patterns-contract.md`)

- [x] `gitops.argocd.applicationset_generators` pattern (implemented; doc ID drift fixed in #153)
- [x] `gitops.flux_kustomization_paths` pattern (implemented)

### Connected Mode Ideas

- [x] ConfigHub summary storage and Slack integration for drift/sync notifications — graduated to #209, #210
- [ ] Connected import flows (`cub-scout connected import bundle/git/cluster`)
- [x] Intent vs render vs observed three-way comparison — graduated to #213
- [x] Hook compatibility verifier — resolved #315
- [ ] Import wizard with auto-detection (`--wizard` flag, from `gitops-repo-structures.md`)
- [ ] Snapshot enrichment with ConfigHub intent/history metadata (from `state-and-snapshots.md`)

### Extension & Integration Ideas (`howto/extending.md`)

- [x] Webhook event streaming (entry/drift/finding events) — graduated to #234
- [x] Output plugin architecture (file sink foundation; Kafka/custom destinations follow-up) — graduated to #308
- [x] Config-based custom ownership detectors (YAML, no Go required) — graduated to #233
- [x] Config-based CRD watching (YAML resource registration for map/watch; status extraction fields reserved) — graduated to #311

### Verify / Receipt Capability (`cub-scout receipt`, 8th verb group)

cub-scout's evidence is point-in-time and ephemeral today. A **receipt** is the missing **historical, immutable record** of "what cub-scout observed at time T" — a typed, fingerprinted artifact wrapping the existing evidence (`compareResourceResult`, `compareSourceTruthResult`, attribution, `gitSource`, `bindingSource`) into a verifiable claim that consumers (CI/CD, audit, postmortem, acceptance-judge tooling) can attach to a decision and later prove the inputs were what they claim to be. Receipts are an envelope, not new evidence.

cub-scout produces the **Runtime-layer (layer 3)** receipt in a four-layer proof model (governance-of-mutation / controller delivery / runtime fact / GUI proof). Layer 3 is cub-scout's territory; the other three layers belong to other tools that compose with cub-scout's output via `inputAttestations[]` digest references. The governance-of-mutation layer (layer 1) currently has no dedicated tool in the ConfigHub stack — concept is in design, out of scope for this issue.

Full design: [`docs/proposals/receipts-way-forward.md`](proposals/receipts-way-forward.md).

**v1 scope (post external review): single-resource verify + show + validate only.** Batch / aggregate / chained / watch-driven receipts are explicit v2 work tracked separately; the chained half and the CI-gate / real-time emission UX shipped in #463.

- [x] v1 foundation — `pkg/agent/receipt.go` types, **RFC 8785 canonical-JSON** (via `gowebpki/jcs`, post Codex round-4 review), SHA-256 fingerprint over the **full Statement minus `predicate.fingerprint`** (delete-key, not zero-value), in-toto Statement v1 envelope with **dual subjects** (`k8s-live://` + `confighub-unit://`), auto-detection priority order, `cub-scout receipt verify` command + ASCII renderer + tests + JSON contract docs — shipped in #446 batch 1 (PR #454)
- [x] v1 predicates — `applied-matches-spec` (batch 1, #454), `source-truth-pass` (explicit `--strategy` required, batch 2 #455), `no-manual-edits-since` (batch 2 #455)
- [x] v1 management UX — `receipt show` / `validate` / `list` subcommands; local-directory storage (`$XDG_DATA_HOME/cub-scout/receipts/`) with immutable canonical filenames; `--save` flag on `receipt verify` — shipped in #446 batch 3 (PR #456)
- [x] `scout-verify` skill added as 8th verb-group skill in #442 batch 2
- [x] **v2 CI-gate exit semantics** — `receipt verify --fail-on <verdict>` accepting `WATCH` / `BLOCK` / `INCONCLUSIVE` (comma-separated) or sugar `any-non-pass`. PASS rejected upfront. Exit 0 / 2 / 1 semantics; `--fail-on` parses upfront so a bad value can never leak stdout / `--out` / `--save` before erroring — shipped in #463 (#451 closed)
- [x] **v2 chained receipts (explicit half)** — `--input-attestation <path>` (repeatable) on `receipt verify`. `cub-scout-receipt://<short-fingerprint>` URI scheme; each referenced receipt's fingerprint is verified at chain-construction time; tampered receipts refused. API-boundary enforcement via `VerifiedAttestationRef` typed wrapper so programmatic callers can't bypass the verify step — shipped in #463 (#448 chained half)
- [x] **v2 real-time emission** — `watch --emit-receipt-on <event-types>` with the event-type set `drift.detected` / `ownership.changed` building receipts in v1; **v2 expansion in #470 adds `resource.discovered` / `scan.finding` support** gated by the new per-poll `--emit-receipt-batch-cap` (default 10) backpressure. Receipt-build failures non-fatal; rate-limited stderr warnings (first 10 + summary every 100); `omitempty` wire shape preserved. **`#449` closed via #470** (v1 was #463; full event-type set + backpressure shipped in #470).
- [x] **v2 aggregate-with-discovery half** — `cub-scout receipt verify --scope namespace/<ns>` auto-discovery (walks Deployment / StatefulSet / DaemonSet / CronJob / Job) + comma-list batch shape + `synthetic-aggregate://sha256/<id>` subject (order-independent) + max-severity verdict synthesis (`BLOCK > INCONCLUSIVE > WATCH > PASS`) + `--aggregate-policy` knob for future policies + `aggregate-partial-coverage` omission entry on per-resource failures. `BuildAggregateReceipt` enforces the same `VerifiedAttestationRef` API-boundary check as `BuildReceipt`. **`#448` closed via #469** (chained half was #463; aggregate half in #469).
- [x] **#444 Pilot–cub-scout integration skills** (9 consumer-side scenarios; 5 batch A in #466/#467 + 4 batch B in #468). Same cub-scout verbs framed around Pilot rendering verdicts; `pilot-cd-gate`, `pilot-fleet-conformance`, `pilot-patch-and-drift`, `pilot-watch-alert-response`, `pilot-incident-evidence`, `pilot-rollback-decision`, `pilot-promotion-gate`, `pilot-compliance-audit`, `pilot-release-verification`. Read-only-triad invariant preserved; 0 mutating verbs in any `allowed-tools` line. **`#444` closed via #468.**
- [ ] Future hardening direction — cryptographic signing on top of fingerprint-only (e.g., DSSE wrapped in a Sigstore Bundle, or a comparable scheme) — purely additive to the envelope; not committed to a specific scheme or timeline

**`#446` v1 + v2 status: shipped end-to-end on `main`** (#454 + #455 + #456 + #461 + #463 + #469 + #470 + Pilot consumer-side skill catalog in #466 + #467 + #468; supporting docs in #462 + #464 + #471). The capability is documented in `docs/reference/json-contracts.md` § Receipt Contract, `docs/reference/commands.md` § receipt, `docs/reference/cli-contract.md` § `cub-scout receipt`, and the new `docs/howto/receipts-end-to-end.md` task-shaped tutorial. Four worked examples under `examples/receipts/`: `ci-gate/`, `chained/`, `aggregate/`, `watch-emit/`. All four spin-off issues (`#448`, `#449`, `#451`, `#444`) closed; only the future-hardening cryptographic-signing direction remains open (uncommitted).

Read-only-triad invariant: receipts emit artifacts, never mutate. `nextSteps[]` rejects `actionType=mutating` entries at receipt-emit time. The `receipt` package exposes only `Get` / `List` / `Watch` cluster operations — enforced by `TestReceiptPackageReadOnlyClient`. PolicyReport CRD emission is explicitly out of scope (writes cluster state); spin off as a separate external adapter.

Standalone-mode contract: cub-scout receipts work without ConfigHub auth. Missing connected-mode evidence (no `confighub-unit://` subject, no `bindingSource`, no `confighubUrl`) is recorded as structured `omissions[]` entries, never as silent failure.

Locked design decisions (post external review): predicate URI = `https://cub-scout.dev/receipt/v1`; subject digest = dual subjects with documented field pruning; VSA interop = keep PASS/WATCH/BLOCK/INCONCLUSIVE pure (no verdict distortion).

R&D companion: research pass against existing receipt / attestation patterns synthesized in `docs/proposals/receipts-way-forward.md` — in-toto Statement v1, SLSA VSA, Sigstore Bundle v0.3, RFC 8785 JCS, OCI 1.1 referrers, GUAC, CycloneDX Attestations, Kyverno PolicyReport, Tekton Chains, GitHub artifact attestations, Falco / Tracee (for `no-manual-edits-since` field-manager-evidence caveat). The synthesis lists DSSE-in-Sigstore-Bundle as one candidate signing scheme on top of fingerprint-only integrity; this remains a future hardening direction, not a committed roadmap line.

Spin-off issues status: **all closed.** `#451` (`--fail-on` exit semantics) closed via `#463`. `#448` (aggregate / chained receipts) chained half via `#463`, aggregate half via `#469` — closed. `#449` (`watch --emit-receipt-on`) v1 via `#463`, full event-type set + backpressure via `#470` — closed. `#450` (source-truth help-text drift) closed earlier. `#444` (Pilot consumer skills) closed via `#468`. PolicyReport / Kyverno integration remains an external adapter project, not cub-scout's job.

### AI Agent Skills (`skills/`, modeled on [`confighub/confighub-skills`](https://github.com/confighub/confighub-skills))

Coverage gap: cub-scout ships one umbrella `SKILL.md` while `cub` has 23+ scenario-grouped skills in the reference repo. AI agents picking the right cub-scout verb (`doctor` / `map` / `trace` / `compare three-way` / `compare source-truth` / `explain` / `import …` / `views …` / `mcp serve` / …) have to navigate every verb through one router, diluting triggering accuracy. The attribution layer (#435) added a whole evidence surface with no dedicated skill rules.

- [x] Scaffolding — `SKILL_TEMPLATE.md`, top-level `skills/README.md` router, read-only-triad allowed-tools convention — shipped in #442 batch 1 (PR #452)
- [x] Verb-grouped scenario skills (8) — `scout-observe` / `scout-diagnose` / `scout-compare` / `scout-attribute` (batch 1) + `scout-ingest` / `scout-govern` / `scout-mcp` / `scout-verify` (batch 2 PR #457). All eight verb groups now covered with their own scenario skill.
- [x] Controller observer skills (7) — `observe-argocd` / `observe-flux` / `observe-helm` / `observe-crossplane` / `observe-kro` / `observe-confighub-managed` / `observe-native` — shipped in #442 batch 3. Each verified against the Go enumeration in `pkg/agent/ownership.go` + `pkg/agent/manager_strings.go`.
- [x] Workflow / scenario skills (8) — `triage-unhealthy-workload` / `investigate-drift` / `audit-fleet-conformance` / `prepare-for-confighub` / `migrate-from-kubectl` / `ai-agent-readonly-context` / `operator-incident-evidence` / `confighub-source-truth` — shipped in #442 batch 4. Each composes verb-group + controller-observer skills into a situation-shaped loop with worked examples.
- [x] Shared references (9) — `kubernetes-managedfields` / `verified-manager-strings` (shipped in batch 1) + `source-truth-strategies` / `standalone-vs-connected` / `read-only-triad` / `plugin-vs-standalone` / `argocd-applicationset` / `flux-source-types` / `mcp-tool-catalog` (shipped in batch 5). All 9 in place.

**`#442` status: complete and merged on `main` (`#452` + `#457` + `#458` + `#459` + batch-5 PR).** All ~33 skills + 9 references shipped. The verb-grouped scenario skills, controller observers, workflow scenarios, and shared references are now first-class citizens of the AI-agent surface alongside the umbrella router.

Read-only-triad invariant: every cub-scout skill's `allowed-tools` line stays inside #410/#428. No `apply`/`edit`/`patch`/`delete`/`mutate` patterns anywhere. Skills for `cub` (mutation-capable) belong in [`confighub/confighub-skills`](https://github.com/confighub/confighub-skills), not here.

### Pilot–cub-scout Integration Skills (`skills/pilot-*`)

Consumer-side complement to the AI Agent Skills above: these are scenario skills covering how **Pilot** (the architectural-triad acceptance judge) reads cub-scout's read-only evidence and produces verdicts that gate CD, fleet conformance, drift response, promotion, rollback, compliance, release verification, and event-driven flows. The 9th skill (`pilot-watch-alert-response`) makes `cub-scout watch` the real-time channel into Pilot; implementing it may surface follow-on feature requests against `cub-scout watch` itself (attribution-cause-flip events, source-truth verdict change events, Pilot-shaped event format).

- [x] `pilot-cd-gate` — pre-deploy gate consuming `compare source-truth` per-strategy verdict — shipped in #444 batch A
- [x] `pilot-fleet-conformance` — multi-cluster audit via `compare three-way --scope cluster` + `fleet outliers` — shipped in #444 batch A
- [x] `pilot-patch-and-drift` — manual edit + drift attribution → revert / quarantine / accept-as-canonical — shipped in #444 batch A
- [x] `pilot-watch-alert-response` — real-time event-driven response over `cub-scout watch` + on-demand call-back into `explain` / `compare three-way` for context — shipped in #444 batch A
- [x] `pilot-incident-evidence` — postmortem evidence package from `trace` + `explain` + attribution + events + `bundle` — shipped in #444 batch A
- [x] `pilot-rollback-decision` — when and which revision to roll back to — shipped in #444 batch B
- [x] `pilot-promotion-gate` — variant A → variant B promotion safety via per-variant three-way + `bindingSource` — shipped in #444 batch B
- [x] `pilot-compliance-audit` — periodic policy conformance via scope-wide source-truth + `scan` findings — shipped in #444 batch B
- [x] `pilot-release-verification` — post-deploy validation via three-way + `bindingSource` + `history` since deploy — shipped in #444 batch B

Same read-only-triad invariant applies: every `pilot-*` skill's `allowed-tools` stays in the read set. Pilot itself may mutate (via `cub` / Argo / Flux / whatever), but the cub-scout skill surface stays witness-shaped.

### Scale and Testing

- [x] Production-scale E2E testing (500+ resources, mixed ownership, deep hierarchies) — resolved by #155, #156
- [x] TUI performance profiling at scale (500+ resources) — resolved by #157
- [x] Testing best practices cookbook (`docs/testing/BEST-PRACTICES.md`) — build tag standards, fixture conventions, golden patterns, cluster lifecycle, example-driven testing (#176)
- [x] CI-enforced coverage metrics — graduated to #245, #256, #260

### 1.x Connected Upsell (`roadmap-1x-connected-upsell.md`) — Complete

- [x] Connected hierarchy navigation defaults (cluster-aware filtering, auto-nav to App Hierarchy when connected, mode-aware query presets)
- [x] Mode state visibility in TUI (QuickMode for instant display, Offline/Standalone/Connected distinction)
- [x] Canonical migration guide from Argo/Helm to ConfigHub (`docs/howto/migration-playbook.md`)
- [x] Trace context diagnostics and documented reset path for connected workflows
- [x] "Break-glass to managed" flow documentation and guided testing
- [x] User-facing explainer for ASCII vs JSON model (canonical data vs presentation)

### Documentation Gaps (`archive/NEXT-PLAN.md`) — Complete

- [x] Unified examples index (consolidate EXAMPLES-OVERVIEW.md, examples/README.md, demos/README.md)
- [x] Integrate concepts/why-connected-mode.md content into roadmap v1.x section with concrete issues

### Patterns (`reference/patterns-contract.md`)

- [ ] Connected mode `--git-url` / `--git-ref` / `--git-subpath` flags for remote Git pattern enrichment

---

## How to Read This Roadmap

* **Released** sections are sealed unless explicitly reopened.
* **Planned** sections describe *intent*, not promises.
* Each version has a **theme** explaining why it exists.
* Issues are grouped where they belong *now*, not where they were first imagined.

Semantic contracts, determinism guarantees, and the ASCII = f(JSON) + g model are assumed and not re-litigated here.

---

## Current Planning and Backlog Docs

`docs/roadmap.md` is the canonical roadmap. The files below are active planning/backlog inputs and must not override this document.

| File | Status | Role |
|------|--------|------|
| `docs/roadmap-rendered-manifest-and-argo.md` | Planning only (non-authoritative) | Workstream backlog for rendered-manifest + Argo hierarchy |
| `docs/roadmap-connected-views-and-launch.md` | Planning only (non-authoritative) | Workstream backlog for connected views and launch narrative |
| `docs/roadmap-1x-connected-upsell.md` | Draft for review (non-authoritative) | Candidate sequencing for 1.x connected packaging/upsell |
| `docs/releases/v2.0.0-plugin-plan.md` | Active milestone plan (non-authoritative) | Plugin switchover plan for `cub scout`, MCP positioning, and milestones to 2.0 |
| `docs/releases/v0.20.0-slice-plan.md` | Delivered historical plan (non-authoritative) | Scoped implementation plan used for shipped v0.20.0 slice |
| `docs/reference/ai-ops-gateway-prd.md` | Tracked design reference | AI gateway, presentation modes, plugin-first packaging, and explorer-vs-provider boundaries |

Graduation rule: move an item from these files into this roadmap (or a release plan) before treating it as committed execution scope.

---

## Released History (Locked)

### v0.5 — Contract-Locked CLI

**Status:** Released

Theme: *Trustworthy, script-safe CLI*

* Canonical CLI surfaces
* Deterministic output
* Stable exit codes and schemas

Contracts are sealed.

---

### v0.6 — Graph Foundation

**Status:** Released

Theme: *First-class resource graph*

* Unified graph model
* Ownership chains
* Graph export and explain

---

### v0.7–v0.10 — Explainable Inference

**Status:** Released

Theme: *Correctness before cleverness*

* Pattern detection and explanation
* Deterministic inference
* Git-aware evidence (optional, additive)

---

## v0.14 Line — Explainable Debugging

**Status:** Released

Theme: *Share explanations, not just output*

Delivered across v0.14.0–v0.14.6:

* Guided GitOps debug UX
* Drift detection + expansion
* Drift ↔ debug correlation
* Debug Bundle v1 (capture, replay, docs)

This arc is complete and sealed.

---

## v0.15 — Replay & Time-Series Reasoning

**Status:** Released

Theme: *What changed over time?*

* Catalog v1
* Bundle diff v1
* Bundle timeline v1
* Offline, read-only, deterministic

All semantics complete.

---

## v0.16 — Platform Composition & Attribution

**Status:** Released

Theme: *Why this change exists*

* Attribution Graph v1
* Attribution Report v1
* Crossplane lineage (XR/MR/Composition)
* Kustomize overlay attribution
* Deterministic scoring and replay

### Open (Future)

* Platform composition beyond Crossplane (kro) — #3, #21

---

## v0.17 — Stabilization

**Status:** Released (Non-semantic)

Theme: *Harden before workflows*

Delivered:

* CI workflows
* Benchmark harness
* Contract audit (schema immutability + golden determinism)
* Epic cleanup

---

## v0.18 — Connected Workflows

**Status:** Released

Theme: *Bring cub-scout to where decisions happen*

This release fulfills the **original v0.2–v0.3 "why connected" intent** *without* introducing controllers or mutation.

### Delivered

* Artifact-first workflows (CI → bundle → replay)
* Git-aware, read-only workflows (context captured, not authoritative)
* Explicit fleet & environment views (catalog + diff)
* CI-backed demos and examples
* Zero new schemas or inference

cub-scout is now **operationally connected** while remaining offline-capable.

---

## v0.19 — TUI Polish & Evidence Workflows

**Status:** Released (v0.19.0–v0.19.6)

Theme: *Delight on top of substance*

### v0.19.0–v0.19.3: TUI Polish

* Canonical visual vocabulary
* TUI snapshot golden tests
* CLI ↔ TUI symmetry flags
* Context-aware suggestions
* Shell completion
* Resolver pattern documentation (#24) ✅
* Crossplane walkthrough (#23) ✅
* Performance & scale guardrails (#22) ✅

### v0.19.4: Evidence Adjacency

* `bundle summarize` command — export for tickets, PRs, Slack
* Docs audit — archived stale docs, fixed DEPRECATED references

### v0.19.5: GitOps Lifecycle Hazards

* `scan --lifecycle-hazards` — detect Helm hook risks under ArgoCD
* `map hooks` — list resources with lifecycle hook annotations
* Helm-to-ArgoCD phase mapping (pre-install→PreSync, post-install→PostSync)
* `examples/lifecycle-hazards/` — documentation and example manifests

### v0.19.6: 1.0 Readiness

* Real connected mode auth (LoadAuth/SaveAuth with JSON persistence)
* Real connectivity check (HTTP HEAD to ConfigHub with timeout)
* CLI contract v1.0 frozen (`docs/reference/cli-contract.md`)
* Hub package tests added
* Removed "experimental" messaging from README
* GOTOOLCHAIN=local for air-gapped build compatibility
* Release-readiness Argo regression audit command + fixtures added (`test/regression/argo-version-audit.sh`, issue #125)

The UX surface is now **stable and locked**.

---

## v0.20.0 — Flux Operator Interop Slice

**Status:** Released (2026-02-12)
**Theme:** *Read-only operator workflows with stronger runtime evidence*

Delivered:

* `map cronjobs` and `map jobs` for schedule/run visibility
* `map actions` for read-only runbook/action preview
* `trace --artifacts` for source artifact provenance
* `map activity` for normalized GitOps action timeline
* `map previews` for ephemeral PR environment detection

See details:

* `docs/releases/v0.20.0-release-notes.md`
* `docs/releases/v0.20.0-slice-plan.md` (historical planning record)

---

## v1.0.0 — Stable Contract Baseline

**Status:** Released (2026-02-20)
**Theme:** *General availability for the standalone contract surface*

Delivered:

* First stable major tag: `v1.0.0`
* CLI contract v1.0 is now GA (`docs/reference/cli-contract.md`)
* Standalone map/tree/trace/scan and JSON contract surfaces remain read-only and deterministic
* Standard GitHub Release + Homebrew + container artifacts via existing release automation

See details:

* `docs/releases/v1.0.0.md`

---

# v1.x Roadmap Track

Sections below include both shipped and future v1.x work.
Nothing here represents unfinished 0.x promises.

---

## v1.1+ — Connected Mode (ConfigHub)

**Status:** Planned
**Theme:** *Shared intent, history, and fleets*

Connected Mode integrates cub-scout with **ConfigHub**, the system of record for configuration intent, history, and fleets.

### Why Connected Mode Exists

Standalone cub-scout answers: *what exists now, who owns it, and what looks risky.*

Connected mode answers: *what should exist, what changed over time, and what this affects across environments.*

A cluster API can only show current observed state. It cannot reliably answer what changed last week, whether one cluster is an outlier, what should happen before a rollout, or how imported state maps to org structure. Those require durable history, indexing, and cross-environment context outside a single cluster.

Connected mode adds context, not control. cub-scout remains read-only.

### What Connected Mode Unlocks

* Intent context (DRY/WET/LIVE)
* Change history and timeline correlations
* Fleet comparison and outlier detection
* Import/adoption workflows (break-glass to managed)
* Dependency-aware impact analysis
* Governance context and approvals metadata

### Interface Boundaries

* `cub` CLI is the external interface contract for connected workflows
* `confighub-agent` depends on `cub` command behavior (arguments, exit codes, JSON shape)
* `cub-scout` connected mode depends on `cub auth login` for credential/session management
* Standalone `cub-scout` must continue to function without `cub`

Ownership split:

* **cub-scout:** deterministic discovery, explanation, evidence export
* **ConfigHub:** system of record, lifecycle state, migration semantics

### Paid Value Boundary

Connected value is paid because it requires hosted platform capabilities: durable multi-tenant storage, cross-cluster indexing, fleet/governance APIs, retention and auditability at scale. The CLI remains free and safe to run offline.

### Definition

Connected Mode means:

* cub-scout participates in **importing state into ConfigHub**
* ConfigHub owns **storage, indexing, and lifecycle**
* cub-scout remains:

  * read-only
  * bundle-first
  * deterministic
  * non-controller

---

### Archive MAP Doc Reconciliation

The archived MAP series under `docs/archive/from-confighub-agent/2026-02-09/map/` is historical planning input.
Current canonical behavior and scope are defined by:

* `docs/reference/connected-tiers-and-views-product-guide.md`
* `docs/reference/gitops-repo-structures.md`
* `docs/reference/app-model-examples.md`
* `docs/reference/import-docs-crosswalk.md`

Coverage currently represented in this roadmap:

* Standalone -> discovery -> connected adoption path (read-only exploration + import context)
* Git/source visibility and comparison direction
* Fleet topology/history surfaces as connected/fleet features

Not in scope for `cub-scout` implementation (by design):

* Worker/controller lifecycle commands in `cub-scout`
* Continuous reconciliation or policy enforcement in `cub-scout`
* Mutating platform workflows (`drift accept`, `mutate`, `promote`) implemented directly in `cub-scout`

Legacy `cub-agent` command examples in archive docs are non-canonical; current command contract is `docs/reference/commands.md`.

---

### Remaining Capabilities (Authoritative)

#### 1. Connected Import Evolution (1.1+)

Basic `cub-scout import` shipped in v1.0:
* Namespace-scoped workload discovery and import (`cub-scout import -n <ns>`)
* Interactive wizard (`cub-scout import --wizard`)
* Dry-run and JSON proposal modes
* See `docs/reference/commands.md` and `docs/howto/import-to-confighub.md`

Remaining 1.1+ scope — evolve import, not introduce it:
* App-centric transition mapping (App/Deployment/Target to Space/Unit) — see #186
* OCI-first source language alignment
* Evidence contract integration with bundle/scan surfaces
* Import from bundle artifacts (not just live cluster)

#### 2. Git as a First-Class Source

* Import Git intent (not just metadata)
* Compare:

  * Git ↔ cluster
  * Git ↔ Git
* No mutation, no continuous sync

#### 3. Shared Fleet Definitions

* Fleets stored durably in ConfigHub
* Fleets spanning clusters, git sources, history
* cub-scout as the exploration and explanation surface

#### 4. Surfacing ConfigHub Engines

* History (ChangeSets)
* Views / projections
* Impact & blast radius
* Policy context (explanatory only)

cub-scout surfaces results; it does not reimplement engines.

---

### Explicit Non-Goals

Connected Mode does **not** introduce:

* Controllers or agents
* Continuous reconciliation
* Policy enforcement
* Silent semantic expansion

---

### Linked v1.1 Issues (Docs + Contract Alignment) — All Closed

* ~~#183~~ — align connected-import roadmap scope with shipped `cub-scout import`
* ~~#184~~ — reconcile `commands.md` completeness claim with actual coverage
* ~~#185~~ — fix `bundle summarize` default-format help mismatch
* ~~#186~~ — add app-centric transition mapping (App/Deployment/Target ↔ Space/Unit)

### v1.2 — cub-scan Integration + Argo Hierarchy

**Status:** Released (2026-02-26)
**Theme:** *External scan engine + Argo lineage depth*

Delivered:

* #190 — wire `ConfighubScanProvider.ScanFile()` to `cub-scan` binary
* #191 — wire `ListPolicies()` to `risk-catalog-v1.json`
* #192 — schema parity tests between `cub-scan Finding` and `scan.normalized.v1`
* #193 — provider selection logic (auto-detect cub-scan)
* #194 — Argo App-of-Apps parent/child lineage
* #195 — ApplicationSet → generated Application lineage

Quality/hardening (non-issue):

* Fix TUI test isolation (`noInit` to prevent Init() clobbering test fixtures)
* Debug summary clipboard (`c`) and file export (`e`) implemented
* Import wizard test step re-enabled after unit apply
* Scan auth surfaced under `--verbose` instead of silently swallowed

Follow-up delivered:

* #200 — cluster scan path now exports live resources and invokes `cub-scan`
  for static findings, while preserving legacy runtime signals.
* Fallback remains safe and deterministic: if export or `cub-scan` fails,
  scan output falls back to legacy provider results.

---

### v1.3.1 — Housekeeping

**Status:** Released (2026-03-06)
**Theme:** *Release hygiene*

Delivered:

* #204 — argo-import-confighub-demo determinism (pin versions + stronger readiness)
* #206 — roadmap summary cleanup (remove closed #149-#151 from open backlog tracking language)

---

## v1.4 — Discover & Connect

**Status:** Released (delivered in v1.x line)
**Theme:** *New user onboarding, immediate cluster value, and AI observation gateway*

This milestone focuses on making cub-scout immediately useful to new users and positioning it as the read-only agentic observation layer for AI tools.

### New User Experience

* #218 — `cub-scout doctor` — single-command cluster health summary
* #219 — `cub-scout explain <resource>` — plain-English ownership and lineage
* #223 — interactive quickstart wizard for new users
* #224 — navigation-first messaging and "aha in seconds" walkthrough

### Distribution & Accessibility

* #221 — kubectl plugin (`kubectl cub-scout`)
* #222 — Homebrew formula + goreleaser pre-built binaries

### Visual & Shareable Output

* #220 — visual graph export (HTML/SVG) — shareable ownership topology

### AI Observation Gateway

* #214 — MCP gateway — read-only observation with ConfigHub routing

### Connected Import

* #208 — created units linked back to cluster resources

---

## v1.5 — AI-Native Ops

**Status:** Released (delivered in v1.x line)
**Theme:** *AI tooling platform for Kubernetes and GitOps observation*

### MCP Architecture

ConfigHub is the MCP server (owns the full read-write loop):

```
AI chat (any interface) → MCP → ConfigHub API endpoints → trigger changes → GUI updates display
```

cub-scout provides a read-only MCP gateway:
- **Standalone:** serves basic cluster observation tools (map, trace, scan, explain)
- **Connected:** routes to ConfigHub for richer responses (history, fleet, impact)

This positions cub-scout as the local observation interface for AI agents,
while ConfigHub provides the durable backend and full MCP server capabilities.

### Issues

* #212 — context-pack v2: model-agnostic Kubernetes evidence for any LLM
* #215 — safe ask-mode contract: verify, dry-run, explicit confirm for AI operations
* #217 — run-to-issue evidence capture for failed AI sessions
* #225 — AI tool integration examples (Claude Code, Copilot, Cursor)

---

## v1.6 — Connected Value

**Status:** Released (delivered in v1.x line)
**Theme:** *ConfigHub-powered insights that clusters cannot provide alone*

This milestone delivers the "aha moment" for connected mode: what ConfigHub knows that your cluster API cannot tell you.

### DRY/WET/LIVE Comparison

* #226 — WET vs LIVE vs DRY comparison view (TUI panel + CLI report)
* #213 — intent vs render vs observed three-way compare command

### Change History & Audit

* #227 — change history/timeline from ConfigHub ChangeSets
* #231 — audit trail for break-glass accept/reject decisions

### Fleet Intelligence

* #229 — fleet outlier detection — identify clusters that diverge
* #228 — impact preview — blast radius analysis before config changes

### Demo & Onboarding

* #230 — "connect and compare" 60-second demo flow

---

## v1.7 — Platform Scale

**Status:** Released (2026-03-07)
**Theme:** *Composition tools, fleet infrastructure, and extensibility*

### Platform Composition

* #3 — Support platform composition tools (Crossplane, kro)
* #21 — Platform composition support beyond Crossplane (kro)

### Fleet Infrastructure

* #209 — persist drift/sync/risk summary storage for query and trend
* #210 — Slack: publish connected drift/sync/risk digests with deep links

### Extensibility

* #233 — config-based custom ownership detectors (YAML, no Go required)
* #234 — webhook event streaming for entry/drift/finding events

### Experiments

* #163 — Comparative Labels for Grouped Map Output
* #164 — Meaning-First Browse Mode (Read-only)
* #165 — Hybrid Semantic + Structural Grouping

---

## v1.8 — AI Gateway Foundations

**Status:** Released (2026-04-05)
**Theme:** *Explicit AI/human presentation, shared read-only seams, and plugin-ready gateway evolution*

This milestone turns the AI ops gateway direction into tracked execution scope without collapsing too much into one `cub-scout` add-on.

The rule for this milestone is:

* `cub-scout` remains an explorer and evidence surface
* presentation changes are narrative-only and must preserve the existing semantic contract
* connected/governed execution stays outside `cub-scout` ownership even when the long-term gateway model includes delegated execution flows
* the implementation should keep future `cub` plugin hosting straightforward

The tracked design reference for this milestone is `docs/reference/ai-ops-gateway-prd.md`.

### Phase A — Presentation MVP (Delivered)

* #352 — explicit `human|ai|paired` presentation modes for read-only `doctor` and `explain`
* #351 — CLI color output with `NO_COLOR` support

### Phase B — Stable Internal Seams (Delivered)

* #354 — invocation-context model: `requested_mode`, `detected_context`, `effective_mode`, `transport`
* #353 — align `doctor`/`explain` with shared-flow seams without changing current contracts

### Phase C — Deterministic Follow-Ons (Delivered)

* #349 — improve deterministic next-step hints while keeping them testable
* #350 — suggest connected ConfigHub URLs as standard handoff behavior

### Longer-Range Gateway Evolution

* #214 — MCP gateway direction remains valid, but future work should keep MCP as a transport and avoid assuming all gateway logic permanently lives inside `cub-scout`

---

## v1.9 — Conformance & Secrets Evidence

**Status:** Released (2026-04-05)
**Theme:** *Verifiable import workflows and secret dependency visibility*
**Release notes:** [docs/releases/v1.9.0.md](releases/v1.9.0.md)

### Conformance Workflows (#342) — Delivered

Bidirectional snapshot and conformance workflow enables verifiable import proposals:

* `compare three-way --fail-on` — conformance reporting with exit codes for CI gates
* `import --resource` — curated import selection with include/exclude filtering
* Enables operators to verify import proposals match expectations before execution

### Secret Evidence (#328) — Complete

Secret evidence provides visibility into secret dependencies without exposing secret data:

**Slice 1 — CLI Trace:**
* Secret evidence in `trace` for workloads (Deployment, StatefulSet, DaemonSet, Pod)
* Secret evidence in `trace` for Flux sources (GitRepository, HelmRepository, Bucket)
* Secret evidence in `trace` for Flux deployers (Kustomization, HelmRelease)
* Status classification: `present`, `missing`, `unreadable`, `unresolved`
* RBAC-aware error detection (Forbidden → unreadable, NotFound → missing)
* Safe metadata only — `.data` and `.stringData` are never read or exposed

**Slice 2 — Crossplane:**
* Crossplane ProviderConfig secret evidence with cross-namespace resolution
* Dynamic CRD discovery for any `*.crossplane.io` / `*.upbound.io` provider
* Degraded trace output for Crossplane resources (single-node observed chain)

**Slice 3 — Map Issues:**
* Secret issues in `map issues` output — missing/unreadable secrets across scope
* Covers workloads, Flux deployers, and Flux sources
* Deduplication for repeated references to the same secret
* Actionable output format with resource, namespace, secret name, and ref type

**Slice 4 — TUI Integration:**
* Secret panel in TUI trace view
* Styled summary with status breakdown (present/missing/unreadable)
* Individual secret display with status indicators and reference types

**Post-v1.9 follow-on queue:**
* #357 — initial local Git import preview slice shipped and closed
* #363 — ApplicationSet git-generator parsing shipped and closed
* #364 — render-integration investigation complete and closed; keep `cub-scout` preview separate from `cub gitops import`
* #369 — first-class MCP `doctor` tool shipped and closed
* #370 — structured action-typed next-step hints shipped and closed (nextSteps in doctor/explain JSON)
* #360 — v0.14 trace secret evidence shipped and closed (full safe metadata: createdAt, owner)
* #359 — extend `--presentation` to trace command shipped and closed (ai/human/paired modes)
* #362 — test runner timeout fix shipped and closed (30s→90s)
* #368 — recent K8s events in explain/trace shipped and closed (v1.10 wedge)
* #372 — README trimmed and restructured shipped and closed
* #373 — canonical import subcommand migration shipped and closed
* #374 — CLI reference doc consolidation shipped and closed
* #375 — top-level command sprawl reduction shipped and closed
* #376 — duplicate skill file reconciliation shipped and closed
* #377 — MCP cold-test/tool-description hardening + `compare_three_way` tool shipped and closed

---

## v1.10 — Troubleshooting Flow Tightening

**Status:** Released (2026-04-09)
**Theme:** *Stay in cub-scout longer during incident response and verification*
**Release notes:** [docs/releases/v1.10.0.md](releases/v1.10.0.md)

Delivered:

* #368 — recent K8s events in `explain` and `trace`
* #359 — `--presentation` extended to `trace`
* #362 — `TestContextPack_FormatJSON` stability fix

---

## v1.11 — AI Routing and Surface Cleanup

**Status:** Released (2026-04-11)
**Theme:** *Make cub-scout easier to choose, easier to route, and easier for AI to use correctly*
**Release notes:** [docs/releases/v1.11.0.md](releases/v1.11.0.md)

Delivered:

* #375 — reduce top-level command sprawl
* #377 — improve MCP tool descriptions and add connected `compare_three_way`
* #376 — reconcile duplicate cub-scout skill files
* #374 — consolidate overlapping CLI reference docs
* #373 — canonicalize import subcommand paths
* #372 — trim and restructure `README.md`

---

## v1.12 — Trustworthy Proof Surfaces

**Status:** Released (2026-04-11)
**Theme:** *Tighten governed proof where connected and Argo-managed workflows were still under-expressive*
**Release notes:** [docs/releases/v1.12.0.md](releases/v1.12.0.md)

Delivered:

* reconnect governed resources in `compare three-way`
* make `explain` materially more authoritative for Argo-managed workloads
* reduce silent under-connection in the governed proof path

---

## v1.13 — Connected Trust Surfaces and Release Hygiene

**Status:** Released (2026-04-14)
**Theme:** *Make connected proof paths easier to verify, easier for AI to route, and boringly green in CI again*
**Release notes:** [docs/releases/v1.13.0.md](releases/v1.13.0.md)

Delivered:

* canonical ConfigHub unit and revision-history trust URLs across `compare`, `trace`, `explain`, MCP, and `history`
* revision-aware read-only hints that say when to review revision history before treating a unit as converged
* connected `history` JSON trust guidance (`confighubUrl`, `confighubRevisionsUrl`, `nextSteps`)
* release-gate cleanup: README/install parity, fixture/script drift fixes, and `go test ./...` back to green

---

## Guiding Principles (Still True)

* **Explainability first**
* **No silent contract expansion**
* **Determinism over convenience**
* **Artifacts over live dependencies**
* **CLI/TUI parity** — CLI and TUI are two renderings of one model. Every feature must have both a CLI command (with `--format ascii|json|md`) and a TUI equivalent. CLI is not a second-class citizen.

---

## Summary

* v0.x is **complete**
* v1.0.0 is the stable contract baseline (2026-02-20)
* v1.1.0 released (2026-02-25) — connected foundation, scan provider boundary, docs alignment
* v1.2.0 released (2026-02-26) — cub-scan file integration, Argo hierarchy lineage, quality fixes
* v1.3.0 released (2026-03-04) — determinism hardening + release hygiene
* v1.7.0 released (2026-03-07) — platform composition (Crossplane + kro), meaning-first grouping experiments, extensibility/fleet slices
* v1.8.0 released (2026-04-05) — AI gateway foundations, presentation modes, deterministic hints
* v1.9.0 released (2026-04-05) — conformance workflows (#342), secret evidence (#328 complete); see [release notes](releases/v1.9.0.md)
* v1.10.0 released (2026-04-09) — troubleshooting flow tightening via recent events, trace presentation, and test stabilization; see [release notes](releases/v1.10.0.md)
* v1.11.0 released (2026-04-11) — AI routing and surface cleanup via command consolidation, MCP gateway hardening, and docs cleanup; see [release notes](releases/v1.11.0.md)
* v1.12.0 released (2026-04-11) — governed proof tightening for `compare three-way` and Argo-aware `explain`; see [release notes](releases/v1.12.0.md)
* v1.13.0 released (2026-04-14) — connected trust URLs, revision-aware hints, history trust guidance, and release hygiene; see [release notes](releases/v1.13.0.md)

### Recent Milestones

| Milestone | Theme | Key Deliverables |
|-----------|-------|-----------------|
| **v1.3.1** | Housekeeping | Demo determinism, roadmap cleanup |
| **v1.4** | Discover & Connect | `doctor`, `explain`, kubectl plugin, Homebrew, MCP gateway, quickstart wizard |
| **v1.5** | AI-Native Ops | Context-pack v2, safe ask-mode, evidence capture, AI tool integration examples |
| **v1.6** | Connected Value | WET/LIVE/DRY comparison, change history, impact preview, fleet outliers, audit trail |
| **v1.7** | Platform Scale | kro, summary storage, Slack digests, custom ownership detectors, webhook streaming, meaning-first grouping experiments |
| **v1.8** | AI Gateway Foundations | Presentation modes (`--presentation`), invocation-context model, shared-flow seams, deterministic hints |
| **v1.9** | Conformance & Secrets | Conformance reporting (#342), curated import (#342), secret evidence (#328 complete: CLI + Crossplane + map issues + TUI) |
| **v1.10** | Troubleshooting Flow | Recent K8s events in `explain`/`trace`, `trace --presentation`, test stability hardening |
| **v1.11** | AI Routing & Surface Cleanup | Command-tree cleanup, MCP `compare_three_way`, skill consolidation, canonical CLI/docs structure |
| **v1.12** | Trustworthy Proof | Governed reconnect in `compare three-way`, stronger Argo-aware `explain` proof surfaces |
| **v1.13** | Connected Trust Surfaces | Canonical ConfigHub URLs, revision-aware hints, `history` trust guidance, release gate cleanup |

### Strategic Positioning

**"The read-only agentic observation layer that makes Kubernetes and GitOps understandable."**

* For **new users:** instant cluster understanding in one command (v1.4)
* For **ConfigHub:** the bridge from "I can see what's happening" to "I can see what SHOULD be happening" (v1.6)
* For **AI + K8s users:** the safe, deterministic, read-only interface that every AI tool uses to understand clusters (v1.5)

### MCP Architecture

ConfigHub is the MCP server (full read-write loop):
`AI chat → MCP → ConfigHub API endpoints → trigger changes → GUI updates display`

cub-scout provides the read-only MCP gateway — standalone observation + ConfigHub routing when connected.

There is no other hidden work.
