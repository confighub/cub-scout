# cub-scout Architecture

> Status: Current (Deep Dive)
> Last reviewed: 2026-10-05
> Concepts index: [README.md](README.md)

Scout is a deterministic Kubernetes observer and configuration evidence
provider. Its existing collectors, typed models and output contracts support
standalone CLI/TUI, the `cub scout` plugin and MCP. Connected evidence enriches
that observer through supported `cub` interfaces.

The [release continuity review](../reference/configuration-investigation-continuity.md)
maps these foundations to released code, tests, examples and remaining roadmap
work. The earlier protocol sketch is retained in [the v2.12.4 source history](https://github.com/confighub/cub-scout/blob/v2.12.4/docs/concepts/architecture.md);
its illustrative schemas, command counts and performance estimates are not
current executable contracts.

## Evidence flow

```text
Kubernetes objects and controller reports ──┐
Supplied rendered manifests / local YAML ───┼─> collectors and bounded joins
ConfigHub intent/history/Links (optional) ──┘             │
                                                         v
                                            typed facts and omissions
                                                         │
                                        ┌────────────────┼──────────────┐
                                        v                v              v
                                    CLI / JSON          TUI             MCP
                                                         │
                                                         v
                                             exported checks / receipts
```

Not every command reads every source. A bounded exact-object observation has a
different request budget and claim from a full diagnostic or connected compare.
The [CLI contract](../reference/cli-contract.md), [JSON contracts](../reference/json-contracts.md)
and [semantic contract](../semantic-contract.md) define those obligations.
The [GSF reference](../reference/gsf-schema.md) documents snapshot state; GSF is
one existing output model, not a replacement for all command-specific envelopes.

## Existing implementation seams

| Concern | Foundation | Extension rule |
|---------|------------|----------------|
| Ownership and lineage | `pkg/agent` detection/trace; `pkg/gitops/parser.go` for local repository structure | Parse explicit metadata; separate detected owner, deliverer and generator parentage. Missing evidence stays unknown. |
| Configuration comparison | `cmd/cub-scout/compare*.go`; `pkg/agent/source_truth*.go` | Reuse the collector and strategy-relative evidence. Standalone rendered inputs and connected source truth have different prerequisites. |
| Field attribution | `pkg/agent/git_source_anchor.go`, attribution models; `cmd/cub-scout/compare_bindings.go` | Keep writer, source anchor, binding and source-file evidence distinct. Do not infer templated source maps. |
| Bounded reads and reuse | `pkg/agent/bounded_read.go`; bounded explain/TUI; informer-backed watch/bot | Bind scope and freshness. Cache hits preserve original observation time; denied refresh must not return old success. |
| Portable verification | `pkg/agent/receipt*.go`; receipt commands | Predicates have explicit subjects, bounds and omissions. Fingerprint integrity is not authentication, approval or current health. |
| Connected evidence | Supported `cub` queries and shared delivery envelopes | Name the space and correlate exact identity. Stored intent and reported operations remain separate from live observation. |

## Responsibilities and invariants

- Generators and installers render; controllers reconcile. Scout observes and
  compares their available evidence without applying, syncing or deleting.
- ConfigHub retains intended state and workflow records; the user or governing
  consumer decides acceptance. Scout does not turn prerequisite declarations
  into evaluated approvals.
- Standalone observes one selected kubectl context. Connected mode supplies
  explicit fleet context; it does not silently turn local observation into
  multi-cluster traversal.
- Kubernetes observation is read-only. Explicit ConfigHub inventory import or
  fact publication has a separate write boundary. Local receipts and captures
  are retained artifacts; Scout is not universally stateless.
- JSON carries structural facts; CLI/TUI render those facts. MCP must preserve
  the same supported semantics and omissions. New user-visible behavior needs
  parity, deterministic fixtures, an example and appropriate integration proof.
- Cluster API permissions must enforce the read boundary. Generic wildcard
  reads are not a least-privilege profile; secret values must not appear in
  ordinary investigation evidence.

## Generators and standalone comparison

`import --git-path` parses repository structure and previews import; it does not
render charts, overlays or ApplicationSet outputs. `cub variant upload` consumes
already rendered resources. The SDK renderer/bridge is not a Scout capability.

Supplied rendered YAML already enables standalone comparison and install
receipts. Source anchors, raw-YAML back-resolution, incoming Links and receipt
chains already provide several provenance layers. Complete generation records
and templated field maps require producer evidence that may be absent. Extend
these contracts rather than introducing a parallel provenance model.

## Execution status

Published capabilities and candidate work must remain distinct. Use
[HANDOVER](../../HANDOVER.md), [v2.13 readiness](../releases/v2.13-readiness.md)
and the [ordered v2.13 → v2.14 sequence](../releases/v2.13-to-v2.14-sequence.md)
for current proof and gates. The [roadmap](../roadmap.md) defines future outcomes;
a planned adapter, historical release receipt or closed design issue is not
current runtime acceptance.
