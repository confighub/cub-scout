# Testing Strategy Gap Analysis

**Date:** 2026-01-13
**Goal:** PROVE that confighub-agent (CLI and TUI) works in main user scenarios

## Current State

We have 127+ Go test functions and extensive bash scripts, but critical gaps exist.

## Gap Analysis

### GAP 1: No Output Comparison

**Problem:** Expected output fixtures exist but aren't compared against actual output.

**Evidence:**
```
test/fixtures/expected-output/
├── demo-healthy-connected.txt     # EXISTS but not compared
├── demo-unhealthy-connected.txt   # EXISTS but not compared
├── tutorial-admin-mode.txt        # EXISTS but not compared
├── tutorial-fleet-mode.txt        # EXISTS but not compared
└── examples/                      # EXISTS but not compared
```

**Impact:** Tests pass if code runs, even if output is wrong.

**Fix needed:** Add output comparison to run-all.sh or create dedicated validation script.

---

### GAP 2: Feature x Mode x Tool Matrix Not Verified

**Problem:** No systematic verification that each feature works in each mode with each tool.

**Features:**
- `map` - View cluster state
- `scan` - Find risks
- `trace` - Trace ownership
- `import` - Import to ConfigHub
- TUI - Interactive interface

**Modes:**
- Standalone (no ConfigHub)
- Connected (ConfigHub + workers)
- Fleet (multi-cluster)

**Tools:**
- Flux (Kustomization, HelmRelease)
- Argo CD (Application)
- Helm (standalone)
- Native K8s (kubectl)
- ConfigHub-labeled

**Current test counts (Go only):**
| Feature | Tests |
|---------|-------|
| map | 4 |
| scan | 6 |
| trace | 11 |
| import | 18 |
| query | 1 |
| hierarchy/TUI | 13 |

**Impact:** We don't know if `map` works with Argo in fleet mode, for example.

**Fix needed:** Create verification matrix and tests for each cell.

---

### GAP 3: Query Language Undertested

**Problem:** Only 1 query test function for a complex feature.

**Evidence:**
```bash
$ grep -r "func Test.*[Qq]uery" --include="*_test.go" | wc -l
1
```

**Impact:** Query language may have bugs we don't catch.

**Fix needed:** Add comprehensive query language tests.

---

### GAP 4: Connected Mode Tests Are Optional

**Problem:** `--connected` flag is opt-in, easy to forget.

**Evidence:** run-all.sh auto-detects but doesn't enforce.

**Impact:** Connected features may break without notice (8-day incident).

**Fix needed:** Clear enforcement when ConfigHub features are being developed.

---

### GAP 5: Demo Requirements Not Enforced in Tests

**Problem:** DEMO-REQUIREMENTS.yaml exists but run-all.sh doesn't validate demos against it.

**Impact:** Demos may fail in unexpected ways.

**Fix needed:** Validate demo requirements before running each demo.

---

### GAP 6: No "Proof Document" Generated

**Problem:** Tests run but no clear summary of what was proven.

**Impact:** Can't easily show "all scenarios work."

**Fix needed:** Generate proof document showing feature x mode x tool coverage.

---

## Required Test Matrix

### Features x Modes

| Feature | Standalone | Connected | Fleet |
|---------|:----------:|:---------:|:-----:|
| `map` CLI | MUST | MUST | MUST |
| `map` TUI | MUST | MUST | MUST |
| `scan` CLI | MUST | SHOULD | SHOULD |
| `trace` CLI | MUST | SHOULD | N/A |
| `import` | N/A | MUST | MUST |
| Query language | MUST | MUST | MUST |

### Features x Tools

| Feature | Flux | Argo | Helm | Native | ConfigHub |
|---------|:----:|:----:|:----:|:------:|:---------:|
| `map` | MUST | MUST | MUST | MUST | MUST |
| `scan` | MUST | MUST | SHOULD | SHOULD | N/A |
| `trace` | MUST | MUST | MUST | SHOULD | MUST |
| Ownership detection | MUST | MUST | MUST | MUST | MUST |

### Demos x Requirements

| Demo | Standalone | Workers | Space |
|------|:----------:|:-------:|:-----:|
| quick | Yes | No | - |
| ccve | Yes | No | - |
| query | Yes | No | - |
| healthy | Yes | No | - |
| unhealthy | Yes | No | - |
| connected | No | Yes | current |
| scenarios | Yes | No | - |

---

## Recommended Fixes

### Priority 1: Output Comparison

Create `test/validate-output.sh`:
```bash
#!/bin/bash
# Compare actual output against expected fixtures
# Exit 1 if any mismatch

for expected in test/fixtures/expected-output/*.txt; do
    scenario=$(basename "$expected" .txt)
    actual=$(run_scenario "$scenario")
    if ! diff -q <(echo "$actual") "$expected"; then
        echo "MISMATCH: $scenario"
        exit 1
    fi
done
```

### Priority 2: Feature Matrix Test

Create `test/verify-matrix.sh`:
```bash
#!/bin/bash
# Test each feature x mode x tool combination

FEATURES=(map scan trace)
MODES=(standalone connected)
TOOLS=(flux argo helm native)

for feature in "${FEATURES[@]}"; do
    for mode in "${MODES[@]}"; do
        for tool in "${TOOLS[@]}"; do
            test_combination "$feature" "$mode" "$tool"
        done
    done
done
```

### Priority 3: Proof Document

Add to run-all.sh:
```bash
# Generate proof document
cat > "test-proof-$(date +%Y%m%d).md" << EOF
# Test Proof: $(date)

## Coverage Matrix
| Feature | Standalone | Connected | Fleet |
|---------|------------|-----------|-------|
| map     | ✓          | ✓         | ✓     |
...

## All Tests Passed
- Unit: $UNIT_PASSED
- Integration: $INT_PASSED
- E2E: $E2E_PASSED
- Demos: $DEMO_PASSED

## Conclusion
PROVEN: confighub-agent works in all main scenarios.
EOF
```

---

## Action Items

- [ ] Create output comparison validation
- [ ] Create feature x mode x tool matrix test
- [ ] Add query language tests
- [ ] Generate proof document after test run
- [ ] Validate demo requirements before running
- [ ] Document what "passing tests" proves

---

## What "Passing Tests" Should Prove

When `./test/run-all.sh` passes, it should prove:

1. **Ownership detection works** for Flux, Argo, Helm, Native, ConfigHub
2. **map command works** in standalone, connected, and fleet modes
3. **scan command works** and detects risks correctly
4. **trace command works** for all ownership types
5. **import wizard works** with ConfigHub
6. **TUI renders correctly** (golden file comparison)
7. **Query language works** for all operators
8. **Demos run without error** and produce expected output
9. **Examples are valid** and work as documented

Currently, we can only prove #1, #5 (partially), #6 (partially), and #9.
