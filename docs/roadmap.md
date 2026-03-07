# cub-scout Unified Roadmap

> **Positioning:** The read-only agentic observation layer that makes Kubernetes and GitOps understandable.

> **Status:** Authoritative
>
> This document **replaces and subsumes**:
>
> * `/ROADMAP.md`
> * `/docs/NEXT-PLAN.md` (historical planning draft)
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

### Documentation Gaps (`NEXT-PLAN.md`) — Complete

- [x] Unified examples index (consolidate EXAMPLES-OVERVIEW.md, examples/README.md, demos/README.md)
- [x] Integrate WHY_CONNECTED_MODE.md content into roadmap v1.x section with concrete issues

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
| `docs/releases/v0.20.0-slice-plan.md` | Delivered historical plan (non-authoritative) | Scoped implementation plan used for shipped v0.20.0 slice |

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

# What Is Left on the Roadmap

Everything below is **new work**.
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
* `docs/reference/hub-appspace-examples.md`
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

**Status:** Planned
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

**Status:** Planned
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

**Status:** Planned
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

### Recent Milestones

| Milestone | Theme | Key Deliverables |
|-----------|-------|-----------------|
| **v1.3.1** | Housekeeping | Demo determinism, roadmap cleanup |
| **v1.4** | Discover & Connect | `doctor`, `explain`, kubectl plugin, Homebrew, MCP gateway, quickstart wizard |
| **v1.5** | AI-Native Ops | Context-pack v2, safe ask-mode, evidence capture, AI tool integration examples |
| **v1.6** | Connected Value | WET/LIVE/DRY comparison, change history, impact preview, fleet outliers, audit trail |
| **v1.7** | Platform Scale | kro, summary storage, Slack digests, custom ownership detectors, webhook streaming, meaning-first grouping experiments |

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
