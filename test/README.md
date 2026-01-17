# ConfigHub Agent Test Suite

All tests live in this directory, organized by type.

---

## Uber Test: Prove It Works

**Goal:** PROVE that confighub-agent (CLI and TUI) works in all main user scenarios.

The "Uber Test" is a comprehensive proof test that verifies everything works:

```bash
# UBER TEST - Run this to prove confighub-agent works
go build ./cmd/cub-agent                           # Build
go test ./...                                      # Unit + TUI tests
go test -tags=integration ./test/integration/...  # Integration tests
./test/run-all.sh                                 # E2E demos + examples
```

**What the Uber Test Proves:**

| Scenario | Verified By |
|----------|-------------|
| Disconnected mode (no ConfigHub) | Unit tests, TUI tests, Integration |
| Connected mode (ConfigHub auth) | Integration tests, demo connected |
| Fleet mode (multi-cluster) | Integration tests, map fleet |

| Feature | Verified By |
|---------|-------------|
| `map status/list/deployers/orphans/issues` | Integration + E2E |
| `map drift/crashes/workloads` | TUI tests |
| `scan` (CCVE detection) | Integration + E2E |
| `trace` (ownership chains) | Unit + Integration |
| `import` (ConfigHub onboarding) | TUI + E2E |
| Query language | Unit + Integration + E2E |
| All 6 ownership types | Unit tests |

**Expected Results:**

```
═══════════════════════════════════════════════════════════════
                    UBER TEST RESULTS
═══════════════════════════════════════════════════════════════

Go Unit + TUI Tests:     ~180 passed
Go Integration Tests:     12 passed
Bash Phase 1 (Standard):   8 passed
Bash Phase 2 (Demos):      4 passed
Bash Phase 3 (Examples):   5 passed

TOTAL: 200+ tests passed
═══════════════════════════════════════════════════════════════
```

---

## Test Tiers

### Tier 1: Go Tests (Fast, No Cluster)

```bash
go test ./...
```

| Category | Tests | What It Proves |
|----------|-------|----------------|
| Unit tests | ~68 | Ownership detection, query parsing, CCVE patterns |
| TUI tests (teatest) | ~77 | All keybindings work, views render, snapshots |
| CLI tests | ~34 | Logger, suggestions, import wizard |

**Run time:** ~10 seconds

### Tier 2: Integration Tests (Requires Cluster)

```bash
go test -tags=integration ./test/integration/...
```

| Test | What It Proves |
|------|----------------|
| TestMapStatus | `cub-agent map status` output format |
| TestMapList | `cub-agent map list` produces output |
| TestMapListJSON | JSON output is valid with required fields |
| TestMapDeployers | GitOps deployer listing works |
| TestMapOrphans | Orphan detection works |
| TestMapIssues | Issue listing works |
| TestScan | CCVE scanning produces output |
| TestScanJSON | Scan JSON output is valid |
| TestTrace | Ownership tracing works on real deployment |
| TestQuery | Query language filters work |
| TestFleetView | Fleet view works |
| TestConnectedModePrerequisites | Worker/target slugs not null |

**Run time:** ~6 seconds (requires cluster)

### Tier 3: E2E Demos & Examples (Full System)

```bash
./test/run-all.sh
```

| Phase | Tests | What It Proves |
|-------|-------|----------------|
| Phase 1: Standard | 8 | Preflight, build, ATK verify/map/scan |
| Phase 2: Demos | 4 | quick, ccve, healthy, unhealthy work |
| Phase 3: Examples | 5 | ArgoCD extension, Flux operator, impressive-demo |

**Run time:** ~4 minutes (requires cluster)

---

## Quick Reference

```bash
# Fast check (no cluster)
go test ./...

# Full check (with cluster)
go test ./... && \
go test -tags=integration ./test/integration/... && \
./test/run-all.sh

# Individual phases
./test/run-all.sh --phase=1    # Standard tests
./test/run-all.sh --phase=2    # Demos only
./test/run-all.sh --phase=3    # Examples only
```

---

## Directory Structure

```
test/
├── README.md                      # This file
├── TEST-INVENTORY.md              # Complete test inventory
├── run-all.sh                     # Run all E2E phases
├── preflight/                     # Pre-flight validation
│   └── mini-tck                   # Technology Compatibility Kit
├── unit/                          # Go unit tests (no cluster)
│   ├── ownership_test.go          # Ownership detection logic
│   └── cub_cli_test.go            # cub CLI output parsing
├── integration/                   # Go integration tests (cluster)
│   └── connected_test.go          # 12 integration tests
├── atk/                           # Agent Test Kit (E2E)
│   ├── demo                       # Interactive demos
│   ├── verify                     # Ownership detection E2E
│   ├── verify-connected           # Connected mode verification
│   └── fixtures/                  # K8s manifests
└── fixtures/                      # Shared test data

examples/                          # Phase 3: Integration examples
├── argocd-extension/              # Argo CD UI extension
├── flux-operator/                 # Flux Operator integration
└── impressive-demo/               # Full demo with script
```

---

## Detailed Test Coverage

### Go Unit Tests (`go test ./...`)

| File | Tests | What It Proves |
|------|-------|----------------|
| `pkg/agent/ownership_test.go` | 14 | All 6 ownership types detected correctly |
| `pkg/agent/flux_trace_test.go` | 3 | Flux ownership chain tracing |
| `pkg/agent/argo_trace_test.go` | 3 | ArgoCD ownership chain tracing |
| `pkg/agent/trace_test.go` | 5 | General trace functionality |
| `pkg/agent/relationships_test.go` | 4 | Resource relationship detection |
| `pkg/agent/state_scanner_test.go` | 8 | CCVE state scanning |
| `pkg/query/query_test.go` | 12 | Query language parsing |
| `pkg/remedy/executor_test.go` | 6 | Remedy execution |
| `cmd/cub-agent/localcluster_test.go` | 36 | Local cluster TUI keybindings |
| `cmd/cub-agent/hierarchy_test.go` | 27 | Hub TUI navigation |
| `cmd/cub-agent/import_wizard_test.go` | 8 | Import wizard flow |

### E2E Demos (`./test/atk/demo`)

| Demo | What It Proves |
|------|----------------|
| `quick` | Apply fixtures → map status/list/issues works |
| `ccve` | CCVE-2025-0027 detection scenario |
| `healthy` | Enterprise healthy cluster view |
| `unhealthy` | Enterprise problem detection |
| `connected` | ConfigHub connected mode (requires auth) |
| `scenario bigbank-incident` | Full BIGBANK narrative |
| `scenario orphan-hunt` | Orphan detection workflow |
| `scenario monday-morning` | Health check workflow |

---

## Scenario Coverage Matrix

| Scenario | Go Unit | Go TUI | Integration | E2E Demo |
|----------|---------|--------|-------------|----------|
| **Disconnected** | ✓ | ✓ | ✓ | ✓ |
| **Connected** | - | ✓ | ✓ | ✓ |
| **Fleet mode** | - | ✓ | ✓ | - |

## Feature Coverage Matrix

| Feature | Go Unit | Go TUI | Integration | E2E Demo |
|---------|---------|--------|-------------|----------|
| `map status` | - | ✓ | ✓ | ✓ |
| `map list` | - | ✓ | ✓ | ✓ |
| `map deployers` | - | ✓ | ✓ | - |
| `map orphans` | - | ✓ | ✓ | ✓ |
| `map issues` | - | ✓ | ✓ | ✓ |
| `map drift` | - | ✓ | - | - |
| `map crashes` | - | ✓ | - | - |
| `map fleet` | - | ✓ | ✓ | - |
| `scan` | ✓ | ✓ | ✓ | ✓ |
| `trace` | ✓ | - | ✓ | - |
| `import` | - | ✓ | - | ✓ |
| Query language | ✓ | ✓ | ✓ | ✓ |
| Ownership (6 types) | ✓ | - | - | ✓ |

## GitOps Tool Coverage

| Tool | Go Unit | Integration | E2E Demo |
|------|---------|-------------|----------|
| Flux Kustomization | ✓ | ✓ | ✓ |
| Flux HelmRelease | ✓ | ✓ | ✓ |
| ArgoCD Application | ✓ | ✓ | ✓ |
| Helm Release | ✓ | - | - |
| Native/kubectl | ✓ | ✓ | ✓ |
| ConfigHub Unit | ✓ | - | - |

---

## Common Errors (Tests Prevent These)

| Error | Root Cause | Test That Catches It |
|-------|------------|---------------------|
| `.Unit.Slug` returns null | cub CLI returns flat objects | `cub_cli_test.go` |
| "unit has no worker" | Precondition not checked | `requireWorker()` |
| "→ no target" | Precondition not checked | `requireTarget()` |
| "null - unknown" worker | Worker not connected | Integration tests |
| Wrong owner detected | Label detection bug | `ownership_test.go` |

---

## CI Integration

```yaml
# .github/workflows/test.yml
jobs:
  tier1-unit:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - run: go test ./...

  tier2-integration:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - uses: helm/kind-action@v1
      - run: go build ./cmd/cub-agent
      - run: go test -tags=integration ./test/integration/...

  tier3-e2e:
    runs-on: ubuntu-latest
    needs: tier2-integration
    steps:
      - uses: actions/checkout@v4
      - uses: helm/kind-action@v1
      - run: ./test/run-all.sh
```

---

## Pre-flight Validation

```bash
./test/preflight/mini-tck              # Basic check
./test/preflight/mini-tck --connected  # Full check with ConfigHub
```

Checks:
- Go installed and correct version
- kubectl access to cluster
- cub CLI installed and authenticated (for connected mode)
- Worker connected (for connected mode)
- Target exists (for connected mode)

**If pre-flight fails, fix your environment before running tests.**

---

## Coverage Goals

| Category | Target | Status |
|----------|--------|--------|
| Ownership detection (6 types) | 100% | ✓ |
| CCVE patterns (46) | 100% | ✓ |
| cub CLI parsing | 100% | ✓ |
| TUI keybindings | 100% | ✓ |
| All demos run without error | 100% | ✓ |
| Demo cleanup works | 100% | ✓ |
| Example YAML valid | 100% | ✓ |

---

## Periodic Code Cleanup

Run dead code analysis periodically to find and remove unused code:

```bash
# Install deadcode tool
go install golang.org/x/tools/cmd/deadcode@latest

# Find dead code (production paths only)
~/go/bin/deadcode ./...

# Run go vet for static analysis
go vet ./...

# Build to verify
go build ./cmd/cub-agent
```

**What to look for:**
- Unreachable functions (never called from main paths)
- Unused helper methods
- Unused type definitions
- Entire packages never imported

**Cleanup checklist:**
1. Create a cleanup branch: `git checkout -b cleanup/dead-code-removal`
2. Delete dead functions/files
3. Fix any broken imports
4. Run `go build ./...` to verify
5. Run `go test ./...` to verify tests pass
6. Commit with summary of what was removed

**Last cleanup:** 2026-01-14 (~2,800 lines removed)

---

## See Also

- [TEST-INVENTORY.md](TEST-INVENTORY.md) - Complete test inventory with all test files
- [atk/README.md](atk/README.md) - Agent Test Kit documentation
- [CLAUDE.md](../CLAUDE.md) - Testing strategy in project instructions
