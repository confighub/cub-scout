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
