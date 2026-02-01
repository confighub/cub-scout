## Summary

<!-- Brief description of what this PR does -->

## Problem

<!-- What user problem does this solve? -->

## Type

<!-- Check one -->

- [ ] Exploration improvement (helps users understand what exists)
- [ ] Debugging improvement (helps users understand why something failed)
- [ ] Both

## Scope

<!-- Check one -->

- [ ] Standalone mode (works without ConfigHub)
- [ ] Connected mode (requires ConfigHub)
- [ ] Both

---

## Checklist

### Core Principles

- [ ] **Read-only** — No cluster mutations (or explicitly flagged exception)
- [ ] **Deterministic** — Same input produces same output
- [ ] **Parse, don't guess** — Uses actual labels/annotations, not heuristics

### Testing

- [ ] `go test ./...` passes
- [ ] Added/updated tests for new functionality
- [ ] Fixture-based or snapshot-based tests where applicable

### TUI/CLI (if applicable)

- [ ] `:` shell-out behavior preserved
- [ ] CLI commands remain accessible
- [ ] Context (cluster, namespace, resource) inherited correctly

### Code Quality

- [ ] `go fmt` and `go vet` pass
- [ ] No new linter warnings
- [ ] Functions focused and testable

---

## Related Issues

<!-- Link related issues: Fixes #XX, Part of #XX -->

