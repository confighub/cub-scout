# Configuration investigation: release continuity and responsibility

Reviewed 2026-10-05 against published release metadata, the `v2.12.4` source
tree, current candidate source, contracts, tests, examples and execution plans.
This is a reuse review, not a new implementation specification or release gate.
At review the published baseline was **v2.12.4**. **v2.13.1 is now published**;
v2.14 remains unreleased. The [delivery contracts](configuration-investigation-delivery.md)
and #778/#779/#780/#781 carry the new ideas into tracked P4/P5 work.

## What the releases already established

| Release foundation | What to reuse | Evidence and limits |
|---|---|---|
| [v0.14](../releases/v0.14.0.md), [v1.0](../releases/v1.0.0.md) | Portable output and stable observation contracts | [Semantic contract](../semantic-contract.md), [CLI contract](cli-contract.md), [JSON contracts](json-contracts.md). Extend the existing factual model; keep CLI/TUI renderings consistent. |
| [v1.7](../releases/v1.7.0.md) | Composition observation for Crossplane and kro | [Composition guide](../howto/tree-hierarchies.md), `pkg/agent/attribution_crossplane.go` and its tests. Existing composition support does not establish Crossplane v2 acceptance or arbitrary generator field provenance. |
| [v1.9](../releases/v1.9.0.md) | Import conformance and secret dependency evidence | `pkg/agent/secret_evidence.go`, `secret_evidence_test.go`. Reuse missing/unreadable dependency handling without exporting secret values. |
| [v1.10–1.13](../releases/v1.13.0.md) | Troubleshooting events, clearer routing, connected proof and revision-aware guidance | [v1.10](../releases/v1.10.0.md), [v1.11](../releases/v1.11.0.md), [v1.12](../releases/v1.12.0.md). Continue the investigation loop instead of introducing another command catalog. |
| [v2.0](../releases/v2.0.0.md) | Standalone/plugin packaging of the same observer | Plugin invocation adds inherited context; it does not make ConfigHub a standalone dependency. |
| [v2.1](../releases/v2.1.0.md) | ConfigHub Views, strategy-relative source truth, kstatus readiness | `pkg/agent/source_truth.go`, `source_truth_logic.go`, `source_truth_fixtures_test.go`; [source-truth fixtures](../../test/fixtures/source-truth/). Source-truth's CLI remains connected; standalone supplied-manifest comparison is a different path. |
| [v2.2](../releases/v2.2.0.md), [v2.3](../releases/v2.3.0.md) | Read-only remediation suggestions, field mutation attribution, source anchors, incoming Links and per-field bindings | `cmd/cub-scout/compare_bindings_c2_test.go`, `compare_back_resolution_test.go`; [mutation attribution example](../../examples/drift/mutation-cause-attribution/). A managedFields writer, a source anchor and a binding are distinct evidence. |
| [v2.3.1–2.5](../releases/v2.5.0.md) | Rendered object sets, readiness/prerequisites, normalization, canonical digests and receipt chains | [Receipts examples](../../examples/receipts/), `pkg/agent/receipt_object_set_test.go`, `receipt_fingerprint_test.go`; [v2.4](../releases/v2.4.0.md). Fingerprints establish integrity, not producer authentication or current truth. |
| [v2.6](../releases/v2.6.0.md), [v2.7](../releases/v2.7.0.md) | Sveltos/Modelplane evidence and shared rollout semantics | [Controller examples](../../examples/controller-ownership/), generation-aware readiness contracts. Owner support does not establish exact-release health for every controller. |
| [v2.8–2.10.1](../releases/v2.10.1.md) | Bounded connected evidence, scoped intended-resource queries, exact object reads and freshness-preserving reuse | [Bounded resource example](../../examples/bounded-resource-read/), `pkg/agent/bounded_read_test.go`. Stored intended configuration is not live health; a repeated read does not renew a report timestamp. |
| [v2.11–2.12.4](../releases/v2.12.4.md) | Exact OCI/configuration/image checks, cheaper watching, explicit space, truthful event outcomes, working MCP and agent correctness repairs | [Observation budgets](../../examples/observation-budget/), [image verification](../howto/is-this-image-deployed.md), [space example](../../examples/confighub-space-scope/). Deployment completeness is bounded; broader workload and index/platform resolution remain follow-ups. |

The attribution, binding, raw-YAML back-resolution, templated-source honesty,
source-truth, secret-evidence and bounded-read test files above are present in
the published `v2.12.4` tree. The current `compare_three_way_session.go`
selection/session work is candidate work, not a capability inferred from that
published tag. Release notes are historical scope descriptions; some old notes
still carry prepublication wording. Actual publication is established by the
[release records](https://github.com/confighub/cub-scout/releases), not that wording.

## Planning and doctrine reconciliation

The archived 1.x upsell, connected-view/launch and rendered-manifest/Argo
plans are historical, non-authoritative inputs. Closed scope-definition issues
are not runtime completion. The April/May import spec is an unscoped proposal,
not authority to restore removed cub GitOps commands or move rendering into Scout.
The receipts R&D proposal now points to its shipped v2.3–2.5 successors; the
OCI release-check proposal now points to published v2.11/v2.12.1 scope.

Current doctrine previously retained LIVE-only TUI, universal-standalone,
cluster-mutating import, statelessness and automatic snapshot-redaction claims.
Those are corrected against executable contracts. Ordinary snapshot exports,
local UI caches, replayable bundles and immutable receipts now have distinct
semantics. Two README links to bot/feedback doctrine had lost their destination
sections during an earlier assessment rewrite; those sections are restored
with the shipped freshness correction. Historical schemas and speculative
performance figures remain accessible in source history, not current promises.

## Responsibility split

| Participant | Responsibility | Scout's interpretation boundary |
|---|---|---|
| Generator or installer | Resolve its inputs and render resources: chart values, overlays, generator parameters, compositions or other transformations | Observe available definitions, parentage and supplied output. Do not rerun a generator or invent its source map. |
| ConfigHub, through supported `cub` interfaces | Retain intended configuration, revisions, Links, targets and workflow/governance records that actually exist | Query explicitly scoped evidence. A prerequisite declaration is not an evaluated approval; a stored output is not proof of its complete generation history. |
| Delivery controller | Reconcile its selected source and report operations, revisions and conditions | Preserve controller-reported facts separately from independent workload observations. A successful operation does not automatically establish current application health. |
| Scout | Collect, join and explain deterministic evidence; compare supplied or connected intent with live state; export checks and omissions | Read Kubernetes without mutation. Explicit inventory import or fact publication to ConfigHub is a separate write boundary. CLI, TUI and MCP use the same supported facts. |
| User or governing consumer | Decide acceptance and authorize changes using evidence and policy | Scout predicates inform decisions; they do not grant release authority or perform repairs. |

The SDK renderer/bridge remains an implementation detail of the supported
intended-state toolchain. `import --git-path` parses local structure and produces
an import preview. `cub variant upload` ingests **already rendered** resources.
Neither implies that Scout renders, nor reinstates the removed `cub gitops`
command group. SDK work remains deferred under #758.

## Three generator questions, three evidence levels

1. **Who generated or manages this object?** Reuse controller trace, explicit
   owner references, composition evidence and ApplicationSet parsing. The local
   parser and its supported generators are in `pkg/gitops/parser.go` and
   `parser_test.go`; `cmd/cub-scout/import_git_test.go` covers ApplicationSet
   previews and duplicate basenames. Parsing structure does not execute a live
   generator or certify a complete generated estate.
2. **Which inputs and output belong to this generation?** Reuse source anchors,
   source-truth strategies, rendered-set digests, exact release identity and
   external-evidence receipt chains. A complete generation record is available
   only when a producer records its inputs, tool/version, output identity and
   binding. Do not infer that record from a parent name or source path.
3. **Which input set this field?** Reuse mutation attribution, ConfigHub
   `bindingSource` and opt-in raw-YAML `--source-path` back-resolution. These
   answer different parts of the question. Helm/Kustomize source maps remain
   unresolved: `pkg/agent/git_source_anchor_templated_test.go` locks the
   `templated-source-not-resolved` marker. Chart source identity is not a
   field-level value origin, and last-writer evidence is not generation lineage.

For Helm input investigations, the baseline already reports reference counts,
optional missing inputs, inline/reference overlap and secret dependencies:
`cmd/cub-scout/map.go`, `pkg/agent/state_scanner.go`, and
`pkg/agent/secret_evidence.go`. `gitops_extract.go` deliberately preserves
reference notes without resolving sensitive values. The remaining design is a
bounded, safe effective-input explanation with tested precedence, not a new
Helm detector or silent secret-reading renderer.

## Standalone remains useful

Without ConfigHub, preserve ownership/trace, events and health diagnostics,
graphs, risk/dependency observations, scoped resource reads, supported release
checks and supplied-rendered-manifest comparisons/receipts. Local raw-YAML
source enrichment and recorded evidence do not require a hosted service.
Cluster access is required for live observations; recorded inputs can be
inspected offline. Durable intended-state history and connected source-truth
queries require their respective evidence sources and credentials.

Connected mode enriches this investigation with recorded intent, bindings,
history and governance. Absence, unreadability, ambiguity, stale evidence and
unsupported semantics remain visible omissions. None establishes an orphan,
an approval, or a healthy current release by default.

## Extend the existing delivery packets

| Packet | Build on | Remaining proof or design |
|---|---|---|
| P3 / v2.13 | Existing trust/identity/freshness, context collection and connected adapters | Finish #591/#597/#641/#588/#599, exact-source CI, genuine acceptance and the admitted paired benchmark. Use the [ordered release sequence](../releases/v2.13-to-v2.14-sequence.md); no live test is excluded. |
| P4 / v2.14 | Existing comparison, bounded resource explorer, controller and attribution models | #596 six-surface conformance, #599 scope/identity, #594 controller matrix, #519 supported operator loop and the named adapter issues. Candidate session helpers are extensions of the collector, not a second truth model. |
| P5 / v2.15 | Bounded read reuse, informer-backed watch/bot and receipts | #539/#604/#600/#605 measured reuse, output and retained evidence. Existing caches are bounded foundations, not a finished fleet store. |
| P6 / 3.0 | Shipped contracts plus P3–P5 proof | Publish only demonstrated workflow, quality and cost results, checked distribution and migration. Version numbers do not establish leadership. |

Before opening new implementation scope, identify its existing issue, released
component, test/example and specific missing answer. Permission profiles (#780), local investigation history (#781), effective Helm
inputs (#779) and generation provenance (#778) are tracked design/implementation
follow-ups. Generic discovery/metrics remains in the
[roadmap checklist](../roadmap.md#untracked-backlog-checklist).
Do not duplicate the existing attribution, generator parser, receipt, explorer
or watch work. Preserve the frozen 24 agent cases and their per-case budgets;
those experimental controls are separate from the removed agent credit cap.

Current execution evidence belongs in [HANDOVER](../../HANDOVER.md),
[v2.13 readiness](../releases/v2.13-readiness.md) and
[#645](https://github.com/confighub/cub-scout/issues/645). Historical waivers
and closed scope-definition issues do not waive today's acceptance requirements.

## Maintainer-confirmed post-2.13 allocation

On 2026-10-05 the maintainer explicitly asked that all review findings and new
ideas be factored into post-2.13 delivery. The
[roadmap carry-forward table](../roadmap.md#post-213-commitments-from-the-continuity-review)
now assigns every finding to P4/P5/P6 with its reused foundation and acceptance
or design outcome. Tasks 23/31/32/33 in the release sequence apply these
checkpoints. Producer-supplied generation records/source maps have an explicit
untracked design entry; accepted runtime work graduates into scoped issues.
No new v2.13 scope, benchmark case or leadership claim is introduced.
