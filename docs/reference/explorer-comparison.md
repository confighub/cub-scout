# Where Scout Fits

Reviewed 2026-09-11 against Scout **v2.10.0** and companion **v0.3.0**.
This is a capability assessment, not a performance benchmark or a claim that
every competing feature was exercised live. Tracking: [#527](https://github.com/confighub/cub-scout/issues/527).

## Assessment

Scout has a differentiated combination: cross-controller ownership and delivery
evidence, CLI/plugin/JSON/MCP access, explicit missing-evidence semantics,
desired/live comparisons, and portable fingerprinted receipts. Its bounded
resource path adds a tested request budget and observation reuse.

**Overall competitive leadership is not yet proved.** Read-only modes,
deterministic diagnosis, GitOps graphs, and MCP are not unique to Scout. Dedicated
explorers offer native watch-based interfaces and substantial drill-down. Scout's
watch/bot modes still poll; its 15-second bounded cache is not a shared inventory
service. No same-workload latency, request-load, or operator-task benchmark was
run for this review.

The product aim should be **the most trustworthy reusable answer to a GitOps
question**, across people and agents. It should not become another sync engine
merely to match mutation buttons.

## Versions And Evidence

**Verified** means Scout source/tests and the scoped live checks in
[#525](https://github.com/confighub/cub-scout/issues/525). **Documented** means the
external project's pinned documentation or source, not an independently tested
runtime result. An unreviewed capability is unknown, not absent.

| Project | Reviewed version and source | Relevant capability |
|---|---|---|
| Scout | [v2.10.0, `f9f5512`](https://github.com/confighub/cub-scout/tree/f9f5512606ca4abd393625d33ff453114aaf68af) | Verified: bounded CLI/plugin/MCP/TUI evidence, origin parsing, exact identity, refresh and omissions; broader existing compare/receipt/controller contracts. |
| Companion explorer | [v0.3.0, `7ffc9c5`](https://github.com/confighub/cub-commander/tree/7ffc9c5fa04027e1df19132d09b14364bbba395e) | Verified: Resource-only evidence panel, explicit Target binding, retained snapshot and manual refresh. Authenticated intended-state mapping remains unverified. |
| argo9s | [v0.1.0, `f40e45a`](https://github.com/vvrnv/argo9s/blob/f40e45a346a3885fcce691122a65208a1d563653/README.md) | Documented: Kubernetes and Argo API backends, Application/ApplicationSet/AppProject views, live updates, resource graphs, logs and sync filters; read-only default with optional actions. |
| flux9s | [v1.0.4, `b743f1a`](https://github.com/dgunzy/flux9s/blob/b743f1a5d35e567c536b754f5e217ec1f705fcfa/README.md) | Documented: Kubernetes Watch API, source/workload graph drill-down, events/logs, history, favorites, explicit RBAC restrictions and opt-in Flux-labeled CRD discovery; read-only default. |
| sofka | [v0.25.5, `b02512f`](https://github.com/nklmilojevic/sofka/blob/b02512f889e03648902197455b146afb17c785ff/README.md) | Documented: generic CRD exploration, deterministic incident explanations, read-only mode, native Argo/Flux operations, Helm inspection, headless checks and snapshots. |
| Flux Operator UI/MCP | [v0.60.0, `483d170`](https://github.com/controlplaneio-fluxcd/flux-operator/tree/483d170eabe66118fa9959730396b634a9268717/docs) | Documented: controller-specific UI and MCP tools. The [current Web UI overview](https://fluxoperator.dev/web-ui/) also shows workload graphs, logs, history, favorites and SSO; that page is rolling documentation, not a frozen version claim. |
| argobot | [v0.1.7, `448f2e0`](https://github.com/confighub/argobot/blob/448f2e0c50f45afb38b6c36e1da62203784a7e60/README.md) | Documented/source-reviewed: event-driven Argo refresh/sync and informer-based status writeback. This is delivery automation and feedback, not an interactive cross-controller explorer. |

## Questions That Decide The Comparison

| User question | What Scout offers now | Competitive conclusion |
|---|---|---|
| Is this deployed? | Separate controller revision/sync, object-set agreement, workload convergence and application-health evidence; explicit inconclusive/partial results. | Other GitOps tools also report revision, readiness and sync. Scout must prove the complete identity-linked answer, not treat one green status as unique capability. |
| Why did delivery or the workload fail? | `doctor`, `explain`, `trace`, activity and optional connected evidence; reusable JSON and receipts. | Competing explain/graph/log views are substantial. Measure diagnosis tasks and gaps instead of comparing command counts. |
| Can I inspect one object cheaply from an agent or explorer? | Bounded reads: one discovery document plus one object GET; MCP/TUI hits make zero such requests, with unchanged timestamps. | A verified narrow-path strength. It is not evidence that all Scout queries use less API traffic than native watchers. [Proof](../../examples/bounded-resource-read/). |
| Can I browse continuously without repeated full polling? | Bounded MCP/TUI reuse and companion captured snapshots; broad watch/bot still poll. | Native watch/store approaches in other explorers and argobot identify a real architecture gap. |
| Can I use the same evidence outside a TUI? | CLI/plugin JSON, MCP, saved bundles, comparison and fingerprinted receipt contracts. | A strong combined workflow, but neither headless output nor MCP is exclusive to Scout. |
| Does any CRD get equally deep explanations? | Configurable inventory plus supported ownership/lineage resolvers; missing status/source/generation evidence is explicit. | Generic browsing is not semantic parity. Compare specific controller fixtures, including unknown and partially readable resources. |
| Will it deploy or force a sync for me? | No. Observation and suggested next checks remain separate from delivery authority. | This is an intentional boundary, not a feature gap to close in Scout. |

Sofka's [pinned GitOps view](https://github.com/nklmilojevic/sofka/blob/b02512f889e03648902197455b146afb17c785ff/docs/features.md#gitops-and-helm)
already follows Flux ownership, source revisions and dependency edges, with
fresh reads and late-result cancellation. Its generic browser uses native
watch updates. These are concrete explorer capabilities to evaluate, not just
terminal styling.

The Flux MCP [tool contract](https://github.com/controlplaneio-fluxcd/flux-operator/blob/483d170eabe66118fa9959730396b634a9268717/docs/mcp/tools.md)
includes ownership tracing, workload logs/events and server-side manifest diff.
Its [read-only configuration](https://github.com/controlplaneio-fluxcd/flux-operator/blob/483d170eabe66118fa9959730396b634a9268717/docs/mcp/mcp-config.md#read-only-mode)
disables mutating tools; dry-run diff still requires Kubernetes patch permission.
Scout's two-GET object read and supplied-object-set comparisons are different
contracts, not proof of superiority to controller-aware dry-run.

## Scout Bot And Delivery Bot

Use them together when both jobs are needed:

1. A release event reaches the delivery bot. In its default Kubernetes mode it
   refreshes Argo; auto-sync still gates deployment. Its Argo API mode requests
   sync directly. Argo remains the reconciler.
2. The delivery bot projects Application status into ConfigHub. Its
   [reporter](https://github.com/confighub/argobot/blob/448f2e0c50f45afb38b6c36e1da62203784a7e60/reporter.go)
   uses an informer, a queue, coalescing and deduplication.
3. Scout reads that external feedback, checks freshness and identity, and adds
   live object, source, drift or receipt evidence where supported. Scout does
   not share the production event-consumer cursor or take over status writes.

A freshness caveat matters: the reporter deduplicates unchanged status without
rewriting `observedAt`, and deletion does not clear the last annotation in this
reviewed version. Old feedback is therefore not proof of a failed or absent
application, nor proof it is still healthy. Keep that distinction in Scout's
connected answers. Integration continues in [#502](https://github.com/confighub/cub-scout/issues/502).

### Current Feedback Verdict Limit

A deterministic review probe against the unchanged v2.10.0
[consumer](https://github.com/confighub/cub-scout/blob/f9f5512606ca4abd393625d33ff453114aaf68af/cmd/cub-scout/gitops_delivery.go#L400)
also found a Scout-side gap. With a 15-minute freshness threshold:

| Report input | v2.10.0 result |
|---|---|
| Synced/Healthy/Succeeded, missing or invalid `observedAt` | Freshness `unknown`, but both verdicts remain `PASS`. |
| Same positive report, timestamp one day in the future | Age clamped to zero, freshness `fresh`, both verdicts `PASS`. |
| Failed/Degraded report, timestamp one day old | Freshness `stale`, both verdicts remain `BLOCK`. |

Do not use a connected live-status verdict alone as proof of current delivery
or health. Read its timestamp and freshness, and obtain current scoped evidence
where needed. Tightening this contract is a correctness priority under #502,
not a fix included in this documentation update. Bounded object-read timestamps
and cache behavior are a separate contract.

**v2.10.1 correction:** timestamp gating now makes
missing/invalid/zero/future reports inconclusive and stale reports `WATCH`,
including old failures. It retains reported fields and emits an explanatory
omission. [Recorded proof](../../examples/live-delivery-observability/#trusting-feedback-freshness)
covers the shared reader, MCP, doctor, activity, correlation and fingerprinted
receipts. The historical v2.10.0 behavior above is unchanged; broader producer
deletion/history and exact-release checks remain open.

## What Would Prove Leadership

These are follow-up acceptance criteria, not shipped features:

- **Explorer continuity and load (#519):** compare cold/warm task latency,
  API requests/bytes, memory and idle/churn traffic on the same 100/1,000-object
  fixtures and permissions. Exercise context switching, RBAC denial, deleted
  objects and interrupted watches. A watch-backed proposal must define relist,
  recovery, bounded storage, staleness and cancellation before implementation.
- **Complete delivery answers (#502/#505):** test revision/digest, controller,
  live object set and current workload generation together across supported
  controller families. Include same-name collisions and missing metadata;
  never turn incomplete coverage into a successful delivery verdict.
- **Status-feedback honesty (#502), correctness first:** prevent current-success
  claims from unknown/invalid/future timestamps; distinguish an old failure
  report from current failure. Test unchanged-but-old reports, deleted
  Applications, out-of-order updates and unavailable history. Prefer consuming
  producer-owned evidence to cloning its delivery or writeback duties.
- **Usable distribution (#520/#527):** correct current-version install paths,
  readable five-mode onboarding, verified public container access and an
  explicit architecture matrix. v2.10 archives/plugins work; registry access
  and Go module major-version distribution remain gaps.
- **Operator proof (#519):** run the same tasks in each relevant interface:
  find an app, trace its source, explain a stuck rollout, assess drift, revisit
  evidence and export it. Publish outcomes, missing capabilities and measured
  costs. Do not infer usability or speed from screenshots or implementation language.

Roadmap: [post-v2.10 priorities](../roadmap.md#post-v210-priorities).
