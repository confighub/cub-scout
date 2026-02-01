<!-- If this PR changes behavior or touches ownership/health/trace/CLI: Codex review is required before merge. See docs/codex-review-checklist.md -->

## Summary

<!-- 1-2 sentences: what this PR does -->

## Changes

<!-- Bullet list of changes -->

-

## Why

<!-- Why is this change needed? -->

## Review-by-Evidence

<!-- REQUIRED: All PRs must complete this section -->

### What changed (1-3 bullets)

-

### Why (1 bullet)

-

### Tests added/updated

<!-- List exact test files or "None" with justification -->

-

### Golden diffs?

<!-- yes/no - if yes, paste small excerpt and explain why the change is intentional -->

- [ ] No golden file changes
- [ ] Yes - golden files changed (see below)

<!-- If golden files changed:
```
[paste diff excerpt]
```
Explanation: [why this change is correct]
-->

### Cluster proof required?

<!-- Required for: ownership/provenance, failure/health modeling, TUI core flows -->

- [ ] Not required (change does not affect risky areas)
- [ ] Yes - cluster proof provided (see below)

<!-- If cluster proof required:
**Commands run:**
```bash
[commands]
```

**Output:**
```
[relevant output or screenshot description]
```
-->

### Risk assessment

**Could this create false certainty (false ownership claims, misleading health status)?**

- [ ] No - change does not affect ownership or health logic
- [ ] No - covered by existing tests
- [ ] Potential risk - mitigated by: [explain]

## Checklist

- [ ] CI passes
- [ ] No false ownership/health claims introduced
- [ ] Documentation updated if needed
- [ ] Review-by-evidence section complete
