# Trace: exact ConfigHub slug joins

Trace joins ConfigHub unit and target **slugs** case-sensitively. Live-status
writebacks also require exact case for the Application name or unit slug, plus
the existing exact space identity. A writeback for `paymentsapi` cannot supply
delivery or health evidence for `PaymentsAPI`; a missing or rejected row keeps
the explicit `confighub.liveStatus` no-match omission. Deterministic tests cover
both row orders, missing metadata, conflicting space IDs and the shared CLI/TUI
projection. These mocked connected inputs require no live API read.

Likewise a release row for target slug `west` does not join to `West`; the
Trace evidence keeps its `confighub.releases` no-match omission instead. This
prevents case-distinct identifiers from becoming false evidence. When both
sides have IDs, the ID remains the stronger key: conflicting IDs reject the
row, and UUID casing remains normalized.

The synthetic agent case
[`trace-case-sensitive-slugs`](../../evals/trace-case-sensitive-slugs/README.md)
shows the mismatched candidate rows beside the shared Trace output. The rows
are test fixtures, not a ConfigHub server capture.
