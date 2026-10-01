# Trace Markdown: unit events

This is a small rendered-output example for the ConfigHub delivery section of
`trace --format md`. It uses typed fixture evidence and is not a captured
server response. Event fields are reported as given: `Result=None` is not
interpreted as success, and missing values remain `-`.

```text
ConfigHub delivery evidence:
  ConfigHub unitEvents=2
  Recent unit events:
    - Apply unit=payments-api result=None status=Applied at=2026-09-30T12:00:00Z
    - - unit=unit-456 result=- status=- at=-
```

The renderer test `TestTraceDeliveryMarkdownRendersUnitEvents` constructs these
two rows directly and also verifies that an empty event list emits no section.

## Exact ConfigHub slug joins

Trace joins ConfigHub unit and target **slugs** case-sensitively. If the traced
resource identifies unit `PaymentsAPI` in space `prod`, a bounded unit-event
row for `paymentsapi` is not attached; the Trace evidence keeps its
`confighub.unitEvents` no-match omission instead. Likewise a target slug `west`
does not join to `West`. This prevents case-distinct identifiers from becoming
false evidence. When both sides have IDs, the ID remains the stronger key:
conflicting IDs reject the row, and UUID casing remains normalized.

The synthetic agent case
[`trace-case-sensitive-slugs`](../../evals/trace-case-sensitive-slugs/README.md)
shows the mismatched candidate rows beside the shared Trace output. The rows
are test fixtures, not a ConfigHub server capture.
