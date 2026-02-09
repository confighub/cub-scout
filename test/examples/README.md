# Real Examples Catalog

This catalog enforces pre-1.0 coverage for real repository skeletons and demo scenarios.

## Files

- `real-examples-catalog.yaml` - source of truth for example coverage

## Validation Command

```bash
go test -tags=integration ./test/integration/... -run '^TestRealExamplesCatalog$' -count=1
```

## Purpose

The catalog links tests to:

- value propositions from rendered manifest planning
- high-value use cases for connected and SaaS workflows
- demo scenarios normalized by repo skeleton taxonomy

The test is local-first:

- examples inside this repo are required
- sibling repos are optional but validated when present
