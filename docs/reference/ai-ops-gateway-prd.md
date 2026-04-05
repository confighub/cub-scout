# AI Ops Gateway PRD (Proposal)

**Status:** Tracked design reference  
**Date:** 2026-04-05  
**Audience:** Product, platform, and engineering teams shaping AI, MCP, and plugin architecture across `cub`, `cub-scout`, ConfigHub, and GitOps extensions
**Tracking:** `docs/roadmap.md`, issues `#352`, `#353`, `#354`, `#349`, `#350`, `#214`

## 1. Problem

`cub-scout` already exposes strong deterministic evidence for cluster exploration:

1. LIVE resource inventory
2. ownership and lineage
3. health and risk signals
4. trace and explain flows
5. JSON/MCP-friendly outputs

ConfigHub already adds connected context that a single cluster cannot answer alone:

1. configuration intent and lifecycle context
2. history over time
3. fleet and outlier context
4. impact and dependency context
5. governance and approval metadata
6. durable org structure and cross-environment understanding

AI assistants can sit on top of both, but today the experience is still too adapter-shaped:

1. local cluster evidence and connected evidence are surfaced as different worlds
2. MCP is tied closely to the current `cub-scout` command surface
3. extension-specific capabilities risk becoming separate AI sub-worlds
4. future pluginization into `cub` would be harder if the AI layer is bundled too tightly inside one `cub-scout` add-on

We need a product-level AI gateway that can unify read paths and governed action paths across:

1. Kubernetes LIVE state
2. ConfigHub connected context
3. supported GitOps and platform extensions such as Flux, Argo CD, Helm, Crossplane, kro, Kubara, and future integrations

## 2. Product Thesis

We should build a **separable AI ops gateway core, with MCP as one transport**.

That core should expose **shared operational flows**, not product silos.

So the AI interface should not ask:

1. "Am I talking to cub-scout?"
2. "Am I talking to ConfigHub?"
3. "Am I talking to the Flux plugin?"

Instead it should ask for flows such as:

1. resource context
2. scope summary
3. timeline
4. impact
5. action preview
6. dry-run
7. governed execution
8. handoff/context pack

Those flows are then enriched by plugins and providers behind the scenes.

## 3. Product Principles

### Core Principle: `cub-scout` is an explorer

`cub-scout` remains:

1. a read-first explorer
2. an evidence normalizer
3. a deterministic observation surface
4. a safe local entry point for operators and AI assistants

`cub-scout` is **not**:

1. the authoritative store of record
2. the approval authority
3. the execution control plane
4. the durable lifecycle owner for connected workflows

### System Principle: authority stays with the right layer

1. LIVE cluster state remains authoritative for observed runtime facts
2. Git remains authoritative for desired-state source artifacts where Git is the chosen source surface
3. ConfigHub remains authoritative for configuration intent, lifecycle state, and connected org/fleet/governance context
4. the AI gateway is an orchestrator and explainer, not a new source of truth

### Product Principle: many front doors, one authority layer

Teams will continue to use many front doors:

1. Git and GitHub
2. Argo CD and Flux
3. framework-native and platform-native generators
4. AI assistants and CLI workflows
5. GUI review and collaboration surfaces

The design should assume front doors stay plural while authority stays explicit.

Implications:

1. the AI gateway is a front door and routing layer, not the authority layer
2. in connected mode, ConfigHub is where governed configuration becomes authoritative, queryable, and inspectable
3. pluginization into `cub` should make front doors easier to compose, not blur the authority boundary
4. extension plugins should enrich shared flows without creating competing sources of truth

### Trust Principle: no success claim without verification evidence

The gateway should prefer explicit evidence and provenance over optimistic summaries.

This means:

1. observed runtime claims should be backed by cluster or controller evidence
2. connected claims should be backed by ConfigHub records or other explicit provider evidence
3. AI-facing summaries should distinguish observed fact, connected state, and inference
4. the product should avoid implying trustworthy apply or status semantics where verification evidence is weak

### Extensibility Principle: extension plugins enrich shared flows

Extension plugins must not create isolated AI experiences.

They should contribute to shared flows by registering:

1. evidence enrichers
2. trace and lineage resolvers
3. action preview providers
4. compare and impact adapters
5. capability metadata

This keeps the user experience coherent across Flux, Argo CD, Helm, Crossplane, kro, Kubara, and future integrations.

### Architecture Principle: design for `cub` pluginization

We should assume the likely trajectory is that more and more user-facing functionality becomes `cub` CLI plugins.

Therefore:

1. the gateway core must be separable from `cub-scout`
2. the MCP transport must be separable from `cub-scout`
3. `cub-scout` should be able to act as one provider/plugin in a broader host model
4. connected features should not require bundling everything into a single `cub-scout` add-on

## 4. Goals

### Primary Goals

1. present one coherent AI gateway across LIVE, connected, and extension-specific evidence
2. keep `cub-scout` aligned with its explorer role
3. make future migration to `cub` plugin hosting straightforward
4. allow MCP, CLI, and TUI to share one gateway core
5. ensure extension plugins enrich shared flows instead of fragmenting them

### Secondary Goals

1. reduce AI guesswork by exposing normalized, typed evidence
2. support safer ops flows with propose -> preview -> confirm -> execute -> receipt stages
3. make connected-mode AI use feel like a continuation of local exploration, not a context switch

## 5. Non-Goals

1. replacing Flux, Argo CD, Helm, Crossplane, kro, or other controllers
2. turning `cub-scout` into the control plane or durable system of record
3. making MCP the core architecture instead of a transport adapter
4. requiring every feature to ship first as a `cub-scout` command
5. creating plugin-specific AI UX silos

## 6. Users and Jobs To Be Done

### Platform Engineer

Needs to:

1. understand what is running and why
2. correlate LIVE state with connected/fleet context
3. ask AI for safe next steps without losing determinism

### SRE / Incident Responder

Needs to:

1. move from failing resource to evidence quickly
2. combine local symptoms with history and impact context
3. preview safe actions before executing anything

### Product / Platform Architect

Needs to:

1. define a coherent AI architecture across tools
2. keep product boundaries clean
3. enable a future plugin-first `cub` model without a rewrite

## 7. Proposed Product Model

### 7.1 Shared Flows

The AI gateway should expose product-level flows such as:

Explorer-aligned shared flows:

1. `observe.resource_context`
2. `observe.scope_summary`
3. `timeline.resource`
4. `timeline.scope`
5. `compare.desired_vs_live`
6. `impact.resource_or_unit`
7. `action.preview`
8. `action.dry_run`
9. `handoff.context_pack`
10. `handoff.investigation_card`
11. `capabilities.list`

Optional governed or delegated flows:

1. `action.execute_governed`

These are example names, not a final API contract, but the principle is fixed:

1. flows are user-meaningful
2. flows can be enriched by multiple providers
3. flows stay stable even if the underlying plugin mix changes

Important boundary:

1. inclusion in the gateway flow model does not imply `cub-scout` ownership
2. `cub-scout` should focus on observe, explain, compare, preview, and handoff flows
3. governed execution, if present, should normally be hosted by connected/provider-owned plugins under `cub` or equivalent hosts
4. `#352` should not depend on governed execution work

### 7.2 Entitlement Tiers

The architecture should distinguish commercial or plan status from runtime capability state.

Preferred architecture term:

1. `entitlement_tier`

Avoid using product-language such as "fully paid user" as a core model term.

Suggested tiers:

1. `oss`
2. `connected`
3. `paid`

Entitlement tier answers:

1. which commercial or hosted features may be unlocked for this user or installation
2. which connected provider packs may be available to resolve
3. which governed or premium flows may be eligible in principle

Entitlement tier does not by itself determine:

1. the currently active runtime mode
2. the rendering style of the response
3. whether the current invocation should mutate anything

### 7.3 Capability Modes

The gateway should support explicit capability modes:

1. `standalone-observe`
2. `connected-read`
3. `connected-governed`

Mode changes what the gateway can return, but not the top-level mental model.

Example:

1. `observe.resource_context` in standalone mode returns LIVE evidence only
2. the same flow in connected-read mode enriches with ConfigHub history, impact, and URLs
3. the same flow in connected-governed mode may also include execution eligibility, approvals, and governed action paths

Capability mode answers:

1. what the system can actually do in this invocation
2. which providers are active and reachable
3. whether the current flow is observe-only, connected-read, or connected-governed

Capability mode should be derived from:

1. entitlement tier
2. runtime environment and connectivity
3. auth/session state
4. local policy and configured provider availability

### 7.4 Presentation Modes

The gateway and its host surfaces should also support explicit presentation modes:

1. `human`
2. `ai`
3. `paired`

Presentation mode is about framing and rendering, not evidence semantics.

Rules:

1. mode selection is explicit first
2. any environment or tool detection is advisory only
3. structured outputs such as JSON and MCP responses remain stable across presentation modes
4. text and markdown outputs may adapt more strongly for readability and handoff quality

The system should distinguish between:

1. `requested_mode` - the mode the caller asked for
2. `detected_context` - hints about whether the caller appears to be a human terminal session, AI host, IDE assistant, or paired workflow
3. `effective_mode` - the mode actually used after applying explicit input and safe defaults

Examples:

1. a human operator may explicitly request `--presentation ai` to produce handoff-ready output for an assistant
2. an AI host may be detected, but unless explicitly requested the gateway may still preserve neutral defaults
3. a paired terminal workflow may prefer `paired` output that is concise for the operator but structured for copy/paste into an assistant

### 7.5 Resolution Model: Three Independent Axes

The system should model three separate axes:

1. `entitlement_tier`
2. `capability_mode`
3. `presentation_mode`

These must remain independent.

Rules:

1. entitlement changes what may be unlocked
2. capability changes what is active and permitted now
3. presentation changes how the results are framed
4. none of these axes should silently redefine the others

Examples:

1. `oss` entitlement + `standalone-observe` capability + `ai` presentation
2. `paid` entitlement + `connected-read` capability + `paired` presentation
3. `paid` entitlement + `connected-governed` capability + `human` presentation

The gateway should expose an `effective_capabilities` view derived from entitlement and runtime state, while keeping presentation separate.

## 8. Architecture Requirements

### 8.1 Layered Design

The system should be structured as:

1. **AI gateway core**
2. **transport adapters**
3. **provider/plugin registry**

### 8.2 AI Gateway Core

The core is transport-agnostic and owns:

1. request routing by shared flow
2. provider composition
3. capability discovery
4. provenance and confidence labeling
5. safety stage enforcement
6. normalized result assembly

The core should be reusable from:

1. `cub-scout`
2. `cub`
3. future plugins
4. test harnesses and fixture sessions

### 8.3 Transport Adapters

MCP is the first-class transport, but only one of several adapters.

Adapters may include:

1. MCP
2. CLI commands
3. TUI actions
4. optional future HTTP transport if ever needed

The adapter layer should stay thin.

### 8.4 Provider / Plugin Model

Providers contribute to shared flows.

Provider classes include:

1. LIVE cluster providers
2. connected ConfigHub providers
3. extension-specific enrichers
4. action providers

Likely examples:

1. `cub-scout` provider pack for LIVE cluster evidence and exploration
2. ConfigHub provider pack for connected history, units, impact, summaries, approvals, and governance metadata
3. Flux provider pack for Kustomization, HelmRelease, and source enrichment
4. Argo CD provider pack for Application, ApplicationSet, sync status, and history enrichment
5. Helm provider pack for release-specific lineage and action previews
6. Crossplane provider pack for XR/composed-resource lineage
7. kro/Kubara provider packs for custom lineage and platform semantics

## 9. Normalized Domain Model Requirements

The gateway core should operate on normalized objects such as:

1. resource references
2. scope references
3. ownership chains
4. deployer/controller references
5. evidence items
6. findings and risk signals
7. activity and change timeline events
8. action proposals
9. dry-run results
10. execution receipts
11. provenance descriptors
12. confidence descriptors

Every shared flow response should be explainable in terms of these normalized objects.

## 10. UX Requirements

### 10.1 One Product, Multiple Surfaces

TUI, CLI, JSON, and MCP should feel like different windows into the same system.

For near-term product fit:

1. AI + CLI should remain the primary operating path for exploration and handoff workflows
2. GUI should be used where it is strongest: tables, trees, diffs, evidence review, approvals, and collaboration

This means:

1. the same selected resource should resolve to the same investigation context
2. the same recommended next steps should be available across surfaces
3. connected-mode enrichment should feel additive, not like switching products
4. presentation mode should change framing, not the underlying facts

### 10.2 Investigation Card

The gateway should support a unified investigation object for a resource or scope.

This card should be able to combine:

1. ownership and explain data
2. trace and lineage data
3. graph context
4. health and risk summary
5. recent activity
6. connected history
7. impact context
8. action previews
9. handoff links or URLs

This is a key joining surface across AI, CLI, and TUI.

### 10.3 Native Safety Stages

The gateway should support a consistent staged model:

1. propose
2. preview
3. confirm
4. execute
5. receipt

This should apply even when action backends differ.

### 10.4 Presentation-Mode UX Rules

Presentation modes should be designed to be testable and low-risk.

Rules:

1. `human` mode should optimize for direct operator readability
2. `ai` mode should optimize for concise machine-assistable summaries, stable section ordering, and explicit next steps
3. `paired` mode should optimize for human-plus-assistant workflows where the operator may read the output and then hand it to an AI tool
4. JSON and MCP outputs should preserve the same schema and evidence model regardless of presentation mode
5. text and markdown outputs may vary in headings, narrative framing, and follow-up hints
6. invocation context and presentation style should be recorded separately in logs and audit trails

## 11. Packaging and Ownership Model

### 11.1 Near-Term Packaging

Near term, `cub-scout` may continue to host:

1. local explorer commands
2. an MCP server adapter
3. some connected bridging

But these should be implemented on top of a reusable gateway core, not as the final architecture.

### 11.2 Long-Term Packaging Direction

The design should make it easy for all major functions to become `cub` CLI plugins.

Target direction:

1. `cub` becomes the host shell for plugin-discovered capabilities
2. `cub-scout` becomes one explorer/evidence plugin pack
3. ConfigHub connected capabilities become separate provider/plugin packs
4. extension-specific providers become their own plugins where appropriate
5. MCP serving can be hosted by `cub`, `cub-scout`, or a dedicated gateway plugin using the same core

### 11.3 Ownership Split

1. `cub-scout` owns exploration and evidence normalization in its scope
2. ConfigHub owns configuration authority, lifecycle state, and governance in its scope
3. extension plugins own enrichment for their domains
4. the gateway core owns shared flow composition and transport-independent orchestration
5. governed execution flows, if exposed, should be provider-owned or host-owned rather than assumed to belong to `cub-scout`

## 12. Functional Requirements

### 12.1 Capability Discovery

The gateway must report:

1. available flows
2. required mode
3. contributing providers
4. whether execution is possible
5. which fields are unavailable and why
6. entitlement tier and any gating relevant to the current host

### 12.2 Provider Composition

For a given flow, the gateway must be able to:

1. query one or more providers
2. merge their evidence deterministically
3. preserve provenance for each major field
4. degrade gracefully when some providers are unavailable

### 12.3 Deterministic Read Paths

Read flows should preserve `cub-scout` principles:

1. deterministic ordering where feasible
2. explicit provenance
3. explicit confidence
4. safe degradation when connected context is absent

### 12.4 Safe Action Paths

Action-oriented flows must:

1. expose action previews
2. prefer dry-run first
3. require explicit confirmation where appropriate
4. return receipts and outcome metadata
5. avoid blurring explorer and control-plane responsibilities

### 12.5 Invocation Context and Audit Metadata

The gateway should preserve invocation metadata separately from rendered output style.

At minimum, the system should be able to record:

1. `requested_mode`
2. `detected_context`
3. `effective_mode`
4. transport used (`cli`, `tui`, `mcp`, other)
5. whether the output was rendered as text, markdown, JSON, or MCP content
6. `entitlement_tier`
7. `capability_mode`

This metadata should not alter the underlying evidence model, but it is useful for:

1. auditability
2. UX evaluation
3. later improvements to presentation defaults

### 12.6 Current Surface Mapping

To keep this architecture grounded in the current product, the first tracked issues should map today's commands to the proposed shared flows.

This table is a migration anchor, not a claim that:

1. every current command must survive unchanged forever
2. every shared flow is permanently locked to one CLI surface
3. the long-term plugin architecture cannot introduce better host surfaces later

| Current surface | Proposed shared flow(s) | Notes |
|-----------------|-------------------------|-------|
| `doctor` | `observe.scope_summary` | Best current anchor for cluster or namespace summary; primary `#352` surface |
| `explain` | `observe.resource_context` | Best current anchor for resource context; primary `#352` surface |
| `trace` | `observe.resource_context` | Supplies ownership and lineage evidence into resource context |
| `graph explain` | `observe.resource_context`, `handoff.investigation_card` | Supplies relationship and graph evidence |
| `map activity` | `timeline.scope`, `timeline.resource` | Timeline-oriented read flow over Flux, Argo, Helm, and Events |
| `history` | `timeline.resource` | Connected ChangeSet history for one resource |
| `summary list` | `observe.scope_summary`, `timeline.scope` | Connected persisted summary snapshots and recent scope state |
| `map actions` | `action.preview` | Read-only action/runbook preview, not execution |
| `context-pack` | `handoff.context_pack` | Deterministic AI handoff/export surface |
| `quickstart` | composed workflow over `observe.scope_summary` and `observe.resource_context` | Useful workflow shell, but not necessarily a first-class shared flow itself |

## 13. Example Shared Flows

### 13.1 `observe.resource_context`

Returns a normalized view of one resource, potentially including:

1. LIVE ownership and trace evidence
2. current health and findings
3. related graph relationships
4. connected unit mapping when available
5. recent history when available
6. recommended next steps

### 13.2 `observe.scope_summary`

Returns a normalized summary for a namespace, app, or cluster:

1. inventory totals
2. ownership summary
3. top risks
4. drift summary
5. fleet outlier context when connected
6. handoff-ready compact context

### 13.3 `timeline.resource`

Returns a normalized timeline:

1. Kubernetes and GitOps activity
2. connected ChangeSet or history events
3. provider-specific enrichments
4. explicit source attribution for each event

### 13.4 `action.preview`

Returns safe next actions:

1. recommended command or action path
2. risk level
3. expected impact
4. prerequisites
5. whether dry-run is available

## 14. Technical Design Constraints

### 14.1 Do Not Make MCP the Core

The gateway core must not depend on MCP semantics for its internal model.

Instead:

1. core package first
2. MCP adapter second

### 14.2 Do Not Hard-Bundle Connected Logic Into `cub-scout`

Connected-mode enrichments should be separable so they can move into `cub` plugins cleanly.

### 14.3 Do Not Let Plugin-Specific Semantics Leak Into Top-Level UX

Plugin details may appear as evidence or provider labels, but the user-facing flow model should remain stable.

## 15. Rollout Plan

### Tracked MVP / `#352`

The first tracked slice should be intentionally narrow.

It should establish the presentation-mode model without forcing the full gateway program to land first.

Initial scope:

1. add explicit presentation modes for read-only `doctor` and `explain`
2. keep JSON and MCP schemas unchanged
3. limit initial adaptation to text and markdown framing
4. make presentation opt-in rather than detection-driven
5. preserve current evidence collection and command semantics
6. preserve the explorer boundary: no governed execution work is required for `#352`
7. preserve the semantic contract: presentation changes must remain narrative-only and pass the existing leak-test standard

What `#352` is not:

1. not a commitment to finish gateway extraction first
2. not a commitment to finalize every shared flow name up front
3. not a commitment to solve auto-detection, routing, or plugin-host propagation in the first slice
4. not a reason to expand `cub-scout` into a control plane

Why this is the right first slice:

1. it improves AI/human-assisted operation immediately
2. it exercises the presentation model in a low-risk way
3. it gives the team a concrete, testable step before broader gateway extraction and pluginization work
4. it keeps future `cub` plugin hosting open rather than prematurely hard-coding packaging decisions

Initial tracked sequence:

1. `#352` - explicit `human|ai|paired` presentation modes for read-only `doctor` and `explain`
2. `#354` - invocation-context model (`requested_mode`, `detected_context`, `effective_mode`) kept separate from output style
3. `#353` - shared-flow seam for `doctor` and `explain` aligned to `observe.scope_summary` and `observe.resource_context`
4. `#349` - richer deterministic next-step hints built on the same read-only, testable UX principles
5. `#350` - connected ConfigHub URL suggestions as standard handoff behavior
6. `#214` - broader MCP/gateway evolution, still constrained by the explorer/read-only boundary on the `cub-scout` side

### Phase 0: Model and Extraction

1. define normalized gateway domain objects
2. extract a reusable gateway core package
3. define provider interfaces and shared flow contracts

### Phase 1: Read-Only Gateway

1. implement `observe.resource_context`
2. implement `observe.scope_summary`
3. implement `timeline.resource`
4. add capability discovery
5. ship MCP as adapter over the core

### Phase 1A: Explicit Presentation Modes

1. define `human`, `ai`, and `paired` presentation modes
2. add `requested_mode`, `detected_context`, and `effective_mode` concepts to the gateway model
3. keep JSON and MCP schemas unchanged
4. add a minimal CLI flag such as `--presentation` on a small number of read-only commands first
5. defer auto-detection, prompting, and richer propagation to later phases

### Phase 2: Connected Enrichment

1. add ConfigHub provider pack for connected read flows
2. merge LIVE and connected context in shared flow responses
3. ensure graceful standalone degradation

### Phase 3: Extension Enrichment

1. register Flux enrichers
2. register Argo CD enrichers
3. register Helm enrichers
4. register Crossplane, kro, Kubara, and other enrichers

### Phase 4: Safe Action Flows

1. add `action.preview`
2. add `action.dry_run`
3. add governed execution handoff and receipts where ownership belongs outside `cub-scout`
4. prefer connected/provider-hosted implementations, for example `cub` plugins or ConfigHub-backed hosts, over permanent `cub-scout` ownership

### Phase 5: Host Flexibility

1. enable the same gateway core to be hosted from `cub`
2. reduce assumptions that MCP must be served from `cub-scout`
3. validate plugin-discovered capability composition

## 16. Success Criteria

### Product Success

1. users experience one coherent AI surface across standalone and connected modes
2. extension support increases without creating fragmented AI UX
3. `cub-scout` keeps a clear explorer identity
4. the architecture supports migration toward `cub` plugin hosting with minimal rework

### Technical Success

1. core gateway package is transport-agnostic
2. MCP adapter is thin and swappable
3. providers can be composed deterministically
4. standalone mode still works cleanly with no connected dependency

## 17. External Signals From FluxCon 2026

The following themes from FluxCon 2026 transcripts are directionally aligned with this PRD and strengthen several of the design choices above.

### 17.1 Domain-Specific MCP Beats Generic MCP

The Flux MCP talk strongly supports using domain-aware MCP/provider packs instead of relying only on a generic Kubernetes surface.

Implication for this PRD:

1. domain-specific providers should know the resource model, documentation, and guardrails of their systems
2. generic Kubernetes access alone is not enough for high-quality AI guidance in GitOps workflows
3. extension plugins should enrich shared flows through domain knowledge, not just expose raw CRUD actions

### 17.2 Skills and Progressive Discovery Should Sit On Top of MCP

The Flux talks argue against a flat model where every MCP tool is loaded into context at once.

Implication for this PRD:

1. MCP remains useful and stable
2. skills, routing, and progressive capability discovery should sit above MCP
3. the gateway should avoid creating a giant undifferentiated tool buffet
4. stable shared flows are preferable to exposing every plugin capability as a first-class top-level concept

### 17.3 Read-Only by Default and Delegated Authority Matter

The agentic GitOps security framing from FluxCon aligns closely with the product boundary we want:

1. bounded access matters
2. human identity and delegated authority still matter
3. readonly-by-default remains a strong baseline
4. AI-facing interfaces should inherit the same trust model as the underlying delivery system

Implication for this PRD:

1. capability modes (`standalone-observe`, `connected-read`, `connected-governed`) are the right abstraction
2. propose -> preview -> confirm -> execute -> receipt remains important
3. `cub-scout` should stay on the explorer/evidence side rather than absorbing control-plane authority

### 17.4 Shared Context and Anchoring Improve Predictability

The Flux MCP talk emphasized context anchoring, strong system guidance, and shared workspace/context as ways to make probabilistic agents behave more predictably in platform engineering environments.

Implication for this PRD:

1. the gateway should expose shared investigation objects and normalized flow inputs
2. provider composition should prefer explicit provenance over implicit tool wandering
3. stable flow contracts are better than freeform prompt-only integration

### 17.5 AI Helps GitOps Adoption When It Reduces Friction

The FluxCon talks also reinforce that AI is useful when it lowers the barrier to GitOps understanding and operation, especially for newcomers and for teams navigating complex multi-cluster systems.

Implication for this PRD:

1. the gateway should accelerate onboarding and exploration
2. AI should help users navigate GitOps philosophy and workflow shifts
3. the product should treat adoption and operator fit as seriously as raw technical capability

## 18. Open Questions

1. which shared flow names should become the first stable contract surface?
2. which package should host the gateway core if the long-term owner is `cub` rather than `cub-scout`?
3. how much provider-specific metadata should be surfaced before it starts leaking implementation details into UX?
4. which governed action stages should remain outside `cub-scout` entirely even when preview flows are local?

## 19. External References

This PRD is based on internal product direction discussed across:

1. `docs/concepts/mental-model.md`
2. `docs/concepts/why-connected-mode.md`
3. `docs/concepts/tui-vs-gui.md`
4. `docs/reference/next-gen-gitops-ai-era.md`
5. `docs/archive/from-confighub-agent/2026-02-09/TUI-GUI-UNIFIED-PRODUCT.md`
6. current MCP, context-pack, explain, doctor, and map action surfaces in the codebase

External talks reviewed via available YouTube transcripts:

1. [FluxCon 2026 playlist](https://www.youtube.com/playlist?list=PLj6h78yzYM2MeuSNqpcDl-qdMeCe6vp6p)
2. [Vibe Coding Meets GitOps](https://www.youtube.com/watch?v=efpqMLQJaW4)
3. [FluxCon | Sponsored Keynote: Agentic GitOps: Evolving Enterprise Delivery](https://www.youtube.com/watch?v=2zudwGs3bMM)
4. [Talking to Your Cluster: Conversational GitOps with Flux MCP](https://www.youtube.com/watch?v=zJg1crt5hBo)
5. [Air France–KLM’s GitOps Takeoff](https://www.youtube.com/watch?v=yyjzEzWfGmo)

Official external materials that explain and reinforce the design ideas in this PRD:

6. [Model Context Protocol design principles](https://modelcontextprotocol.io/community/design-principles)
7. [Model Context Protocol specification](https://modelcontextprotocol.io/specification/)
8. [Connect to remote MCP servers](https://modelcontextprotocol.io/docs/develop/connect-remote-servers)
9. [Build with Agent Skills](https://modelcontextprotocol.io/docs/develop/build-with-agent-skills)
10. [Server Instructions: Giving LLMs a user manual for your server](https://blog.modelcontextprotocol.io/posts/2025-11-03-using-server-instructions/)
11. [AI-Assisted GitOps with Flux Operator MCP Server](https://fluxcd.io/blog/2025/05/ai-assisted-gitops/)
12. [Flux CLI Quick Reference](https://fluxcd.control-plane.io/guides/flux-cli-quick-reference/)

## 20. Appendix: Transcript-Derived Design Implications

This appendix maps the strongest ideas from the FluxCon talks and MCP documentation to concrete package and interface suggestions.

### 20.1 Signal: Domain-Specific MCP Beats Generic MCP

Observed in:

1. [Talking to Your Cluster: Conversational GitOps with Flux MCP](https://www.youtube.com/watch?v=zJg1crt5hBo)
2. [AI-Assisted GitOps with Flux Operator MCP Server](https://fluxcd.io/blog/2025/05/ai-assisted-gitops/)

Why it matters:

1. generic Kubernetes tools can mutate or inspect objects without understanding GitOps ownership, documentation, or controller guardrails
2. domain-aware tools can steer the AI toward the right path, for example Git-based changes for Flux-managed objects instead of ad hoc cluster mutation

Suggested package/interface shape:

1. `pkg/aigateway/providers/flux`
2. `pkg/aigateway/providers/argocd`
3. `pkg/aigateway/providers/helm`
4. `pkg/aigateway/providers/crossplane`
5. `pkg/aigateway/providers/kro`
6. `pkg/aigateway/providers/kubara`

Each provider should implement a shared interface such as:

```go
type FlowProvider interface {
    Name() string
    Capabilities(ctx context.Context) []Capability
    Contribute(ctx context.Context, req FlowRequest) ([]Contribution, error)
}
```

Design note:

1. providers contribute to shared flows
2. providers do not define the top-level user journey

Supporting external materials:

1. [Model Context Protocol design principles](https://modelcontextprotocol.io/community/design-principles)
2. [AI-Assisted GitOps with Flux Operator MCP Server](https://fluxcd.io/blog/2025/05/ai-assisted-gitops/)

### 20.2 Signal: Skills and Progressive Discovery Should Sit Above MCP

Observed in:

1. [Vibe Coding Meets GitOps](https://www.youtube.com/watch?v=efpqMLQJaW4)
2. [Talking to Your Cluster: Conversational GitOps with Flux MCP](https://www.youtube.com/watch?v=zJg1crt5hBo)

Why it matters:

1. too many MCP servers or tools can waste context and confuse tool selection
2. routing and workflow instructions should help the agent discover the right capabilities progressively
3. this supports the plugin-first future where many capabilities may exist, but not all should be surfaced equally at once

Suggested package/interface shape:

1. `pkg/aigateway/flows`
2. `pkg/aigateway/routing`
3. `pkg/aigateway/instructions`
4. `pkg/aigateway/transports/mcp`

Suggested responsibility split:

1. `flows` defines stable user-facing operations such as `observe.resource_context`
2. `routing` selects contributing providers based on mode, scope, and installed plugins
3. `instructions` emits concise server instructions and prompt fragments for workflow guidance
4. `transports/mcp` exposes the selected flows over MCP without leaking the full internal provider graph

Supporting external materials:

1. [Build with Agent Skills](https://modelcontextprotocol.io/docs/develop/build-with-agent-skills)
2. [Server Instructions: Giving LLMs a user manual for your server](https://blog.modelcontextprotocol.io/posts/2025-11-03-using-server-instructions/)
3. [Connect to remote MCP servers](https://modelcontextprotocol.io/docs/develop/connect-remote-servers)

### 20.3 Signal: Read-Only by Default and Delegated Authority Matter

Observed in:

1. [FluxCon | Sponsored Keynote: Agentic GitOps: Evolving Enterprise Delivery](https://www.youtube.com/watch?v=2zudwGs3bMM)
2. [Talking to Your Cluster: Conversational GitOps with Flux MCP](https://www.youtube.com/watch?v=zJg1crt5hBo)

Why it matters:

1. the AI layer must inherit trust boundaries from the underlying system
2. read-only and governed modes should be explicit
3. not every host should be allowed to execute mutating flows, even if it can observe everything

Suggested package/interface shape:

1. `pkg/aigateway/authz`
2. `pkg/aigateway/modes`
3. `pkg/aigateway/actions`

Suggested interfaces:

```go
type GatewayMode string

const (
    ModeStandaloneObserve GatewayMode = "standalone-observe"
    ModeConnectedRead     GatewayMode = "connected-read"
    ModeConnectedGoverned GatewayMode = "connected-governed"
)

type ActionPolicy interface {
    Evaluate(ctx context.Context, req ActionRequest) (ActionDecision, error)
}
```

Design note:

1. `cub-scout` should implement strong observe and preview paths
2. governed execution should remain separable, likely via connected providers and `cub` plugins rather than permanent `cub-scout` ownership

Supporting external materials:

1. [Flux CLI Quick Reference](https://fluxcd.control-plane.io/guides/flux-cli-quick-reference/)
2. [AI-Assisted GitOps with Flux Operator MCP Server](https://fluxcd.io/blog/2025/05/ai-assisted-gitops/)
3. [Model Context Protocol specification](https://modelcontextprotocol.io/specification/)

### 20.4 Signal: Shared Context and Stable Investigation Objects Improve Predictability

Observed in:

1. [Talking to Your Cluster: Conversational GitOps with Flux MCP](https://www.youtube.com/watch?v=zJg1crt5hBo)
2. [Server Instructions: Giving LLMs a user manual for your server](https://blog.modelcontextprotocol.io/posts/2025-11-03-using-server-instructions/)

Why it matters:

1. probabilistic agents benefit from anchored context and explicit workflow guidance
2. a stable investigation object gives TUI, CLI, MCP, and future `cub` plugins a common join point
3. this reduces tool wandering and makes provenance easier to preserve

Suggested package/interface shape:

1. `pkg/aigateway/model`
2. `pkg/aigateway/cards`
3. `pkg/aigateway/evidence`

Suggested core objects:

```go
type ResourceRef struct {
    Cluster   string
    Namespace string
    Kind      string
    Name      string
}

type EvidenceItem struct {
    Source     string
    Provider   string
    Kind       string
    Confidence string
    Summary    string
}

type InvestigationCard struct {
    Resource     ResourceRef
    Ownership    []EvidenceItem
    Timeline     []TimelineEvent
    Findings     []Finding
    NextSteps    []NextStep
    Links        []Link
    Provenance   []Provenance
}
```

Design note:

1. the investigation card is not an MCP-specific object
2. MCP, CLI, and TUI should all render or query the same card model

Supporting external materials:

1. [Model Context Protocol design principles](https://modelcontextprotocol.io/community/design-principles)
2. [Server Instructions: Giving LLMs a user manual for your server](https://blog.modelcontextprotocol.io/posts/2025-11-03-using-server-instructions/)

### 20.5 Signal: MCP Is a Transport, Not the Whole Architecture

Observed in:

1. current MCP ecosystem docs
2. the likely `cub` plugin trajectory discussed in this PRD

Why it matters:

1. the product should not be locked into one adapter
2. the same core should be hostable from `cub-scout`, `cub`, or a dedicated gateway plugin
3. this supports gradual migration rather than a rewrite

Suggested package/interface shape:

1. `pkg/aigateway`
2. `pkg/aigateway/transports/mcp`
3. `pkg/aigateway/transports/cli`
4. `pkg/aigateway/transports/tui`

Suggested top-level composition:

```go
type Gateway struct {
    Registry ProviderRegistry
    Router   FlowRouter
    Policy   ActionPolicy
}
```

Design note:

1. `cmd/cub-scout/mcp.go` should become a thin adapter
2. the future `cub` host should be able to reuse the same `Gateway`

Supporting external materials:

1. [Model Context Protocol specification](https://modelcontextprotocol.io/specification/)
2. [MCP TypeScript SDK overview](https://ts.sdk.modelcontextprotocol.io/)
3. [Connect to remote MCP servers](https://modelcontextprotocol.io/docs/develop/connect-remote-servers)

### 20.6 Signal: MCP Has More Than Tools

Observed in:

1. MCP documentation and SDK docs
2. the need for richer AI UX than simple tool calls

Why it matters:

1. not everything should be flattened into tools
2. prompts and resources can help preserve clarity and reduce context waste
3. the gateway can expose stable context objects as resources while keeping operational flows as tools

Suggested package/interface shape:

1. `pkg/aigateway/transports/mcp/tools`
2. `pkg/aigateway/transports/mcp/resources`
3. `pkg/aigateway/transports/mcp/prompts`

Suggested mapping:

1. tools: `observe.resource_context`, `timeline.resource`, `action.preview`
2. resources: serialized investigation cards, context packs, scope summaries, static reference bundles
3. prompts/instructions: workflow hints, capability descriptions, policy-aware guidance

Design note:

1. this supports richer hosts over time without changing the gateway core

Supporting external materials:

1. [Model Context Protocol specification](https://modelcontextprotocol.io/specification/)
2. [Connect to remote MCP servers](https://modelcontextprotocol.io/docs/develop/connect-remote-servers)
3. [MCP TypeScript SDK overview](https://ts.sdk.modelcontextprotocol.io/)

### 20.7 Signal: AI’s Best Product Value Is Reduced GitOps Friction

Observed in:

1. [Air France–KLM’s GitOps Takeoff](https://www.youtube.com/watch?v=yyjzEzWfGmo)
2. [AI-Assisted GitOps with Flux Operator MCP Server](https://fluxcd.io/blog/2025/05/ai-assisted-gitops/)
3. [Vibe Coding Meets GitOps](https://www.youtube.com/watch?v=efpqMLQJaW4)

Why it matters:

1. adoption is often blocked by cognitive load, not just missing features
2. AI should help users understand and navigate complex GitOps systems
3. this supports keeping `cub-scout` focused on exploration, explanation, and safe guidance

Suggested package/interface shape:

1. `pkg/aigateway/onboarding`
2. `pkg/aigateway/handoffs`
3. `pkg/aigateway/recommendations`

Suggested outputs:

1. investigation cards
2. compact context packs
3. structured next steps
4. capability explanations by mode

Supporting external materials:

1. [AI-Assisted GitOps with Flux Operator MCP Server](https://fluxcd.io/blog/2025/05/ai-assisted-gitops/)
2. [Build with Agent Skills](https://modelcontextprotocol.io/docs/develop/build-with-agent-skills)

### 20.8 Signal: Presentation Style Should Be Explicit and Separable From Evidence

Observed in:

1. the current split between human operator UX and AI-tool usage guidance
2. the need to avoid brittle host detection as the first implementation slice

Why it matters:

1. host detection is fragile and should not become the semantic source of truth
2. the product needs a testable way to adapt human-readable framing without destabilizing structured outputs
3. this fits the plugin-first architecture because rendering concerns can vary by host while the gateway core remains stable

Suggested package/interface shape:

1. `pkg/aigateway/presentation`
2. `pkg/aigateway/model`
3. `pkg/aigateway/transports/cli`

Suggested types:

```go
type PresentationMode string

const (
    PresentationHuman  PresentationMode = "human"
    PresentationAI     PresentationMode = "ai"
    PresentationPaired PresentationMode = "paired"
)

type InvocationContext struct {
    RequestedMode  PresentationMode
    DetectedContext string
    EffectiveMode  PresentationMode
    Transport      string
}
```

Design note:

1. presentation mode should affect framing, formatting, and trace metadata
2. presentation mode should not affect evidence collection or command semantics

Supporting external materials:

1. [Server Instructions: Giving LLMs a user manual for your server](https://blog.modelcontextprotocol.io/posts/2025-11-03-using-server-instructions/)
2. [Model Context Protocol specification](https://modelcontextprotocol.io/specification/)

### 20.9 Signal: Entitlement, Capability, and Presentation Must Stay Separate

Observed in:

1. the interaction between standalone vs connected vs paid product states
2. the need to support explicit human/ai/paired presentation without coupling it to commercial tier

Why it matters:

1. product packaging should not leak into rendering semantics
2. paid features may unlock richer providers without forcing a different presentation style
3. a user may want AI-oriented output even in OSS standalone mode
4. a paid connected user may still want operator-oriented human output

Suggested package/interface shape:

1. `pkg/aigateway/model`
2. `pkg/aigateway/capabilities`
3. `pkg/aigateway/presentation`

Suggested types:

```go
type EntitlementTier string

const (
    EntitlementOSS       EntitlementTier = "oss"
    EntitlementConnected EntitlementTier = "connected"
    EntitlementPaid      EntitlementTier = "paid"
)

type EffectiveCapabilities struct {
    EntitlementTier EntitlementTier
    CapabilityMode  GatewayMode
    AvailableFlows  []Capability
}
```

Design note:

1. entitlement determines what can be unlocked commercially
2. capability determines what is active in this invocation
3. presentation determines framing only

Supporting external materials:

1. [Model Context Protocol design principles](https://modelcontextprotocol.io/community/design-principles)
2. [Model Context Protocol specification](https://modelcontextprotocol.io/specification/)
