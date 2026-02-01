# Reviewer Checklist

Use this checklist when reviewing PRs to cub-scout.

---

## Identity Check

> **cub-scout is a read-only GitOps explorer and debugger.**

| Question | Pass | Fail |
|----------|------|------|
| Does this help users **explore** what exists? | Belongs in cub-scout | — |
| Does this help users **debug** why something failed? | Belongs in cub-scout | — |
| Does this require knowing what **should** exist? | — | Belongs in Connected mode or ConfigHub |
| Does this **apply**, **fix**, or **reconcile**? | — | Out of scope |

---

## Principles (Hard Requirements)

### 1. Read-Only

- [ ] No cluster mutations in default paths
- [ ] Any write operations require explicit flags (`--apply`, `--force`)
- [ ] Write exceptions are documented and rare

**Red flags:**
- `Create()`, `Update()`, `Patch()`, `Delete()` without explicit flag
- Missing confirmation prompts for destructive actions

### 2. Deterministic

- [ ] Same input produces same output
- [ ] No AI/ML in core logic
- [ ] No randomness affecting user-visible behavior

**Red flags:**
- `rand.` without seed
- LLM/AI calls in detection or display logic
- Time-dependent output (except timestamps)

### 3. Parse, Don't Guess

- [ ] Ownership from actual labels, annotations, owner references
- [ ] No heuristic-based ownership claims
- [ ] Unknown = "Native" or "Unknown", not guessed

**Red flags:**
- Ownership inferred from naming conventions alone
- "Looks like Helm" without `app.kubernetes.io/managed-by: Helm`

### 4. Graceful Degradation

- [ ] Works without live cluster (`--file` mode)
- [ ] Works without ConfigHub connection
- [ ] Works without internet
- [ ] Partial data shows partial results, not errors

**Red flags:**
- Hard failures when optional services unavailable
- Empty output instead of "no data" message

---

## Scope Check

### Standalone Mode

- [ ] Feature works with only kubectl context
- [ ] No ConfigHub API calls required
- [ ] Tests run without external dependencies

### Connected Mode (if applicable)

- [ ] Clearly documented as Connected-only
- [ ] Graceful fallback when disconnected
- [ ] Does not break Standalone functionality

---

## TUI/CLI Preservation

- [ ] `:` shell-out still works
- [ ] New keybindings documented in `?` help
- [ ] CLI commands remain accessible (TUI doesn't hide functionality)
- [ ] Context (cluster, namespace, resource) inherited correctly

**Red flags:**
- Keybinding conflicts with existing shortcuts
- Modal states that trap the user
- CLI-only features with no TUI equivalent (acceptable but note it)

---

## Testing

- [ ] `go test ./...` passes
- [ ] New code has test coverage
- [ ] Fixture-based tests (no live cluster required)
- [ ] Snapshot tests for CLI output (if user-visible)

**Red flags:**
- Tests that require live cluster without skip logic
- Tests that depend on external services
- Flaky tests (time-dependent, order-dependent)

---

## Code Quality

- [ ] `go fmt` clean
- [ ] `go vet` clean
- [ ] No new linter warnings
- [ ] Functions < 50 lines preferred
- [ ] Clear error messages (actionable, not cryptic)

---

## Documentation

- [ ] User-visible changes documented (CLI-GUIDE.md, README.md)
- [ ] Breaking changes called out
- [ ] Examples updated if behavior changed

---

## Quick Reject Criteria

Reject immediately if:

1. **Mutates cluster state** without explicit flag and documentation
2. **Requires ConfigHub** for Standalone-advertised feature
3. **Breaks `:` shell-out** or hides CLI functionality
4. **Guesses ownership** without controller metadata
5. **Fails without internet** for offline-capable feature

---

## Approval Criteria

Approve when:

- [ ] All hard requirements pass
- [ ] Tests pass and cover new code
- [ ] PR template checklist complete
- [ ] No quick-reject criteria triggered

---

## Review Comment Templates

### Principle Violation

```
This appears to violate the **[principle]** principle:
> [quote from CONTRIBUTING.md]

Specifically: [explanation]

Suggested fix: [alternative approach]
```

### Scope Question

```
Is this Standalone or Connected mode?

If Standalone: [what needs to change]
If Connected: Please document this clearly in the code/docs.
```

### Missing Test

```
This user-visible behavior needs a test.

Suggested approach:
- Fixture: [describe fixture]
- Expected: [describe expected output]
```
