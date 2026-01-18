# cub-scout Test Suite

All tests live in this directory, organized by type.

---

## Our goal when doing AI-Assisted Development is 100% Test Coverage

**CRITICAL:** When using AI to write code, 100% test coverage is non-negotiable.

> "If you can't prove it works, it doesn't work."

AI can generate code that looks correct but doesn't function. Tests are the only proof. We attempt to verify every feature.  

### The Four Test Groups (25% each)

| Test Group | Weight | Verification | What It Proves |
|------------|--------|--------------|----------------|
| **Unit Tests** | 25% | `go test ./...` | Ownership detection, query parsing, CCVE patterns |
| **Integration** | 25% | `./test/prove-it-works.sh --level=integration` | CLI commands work, JSON output valid |
| **GitOps E2E** | 25% | `./test/prove-it-works.sh --level=gitops` | Flux + ArgoCD ownership, trace, deep-dive |
| **Connected** | 25% | `./test/prove-it-works.sh --level=connected` | ConfigHub worker, import, app-space list |

**Total: 500+ tests for 100% PROOF**

**IMPORTANT:**
- Always use `./cub-scout`, not `cub-scout` (binary is local, not in PATH)
- Always use `prove-it-works.sh`, not `run-all.sh` (legacy)
- See [CLI-GUIDE.md](../CLI-GUIDE.md) for the complete CLI reference (14 commands, 17 map subcommands, 17 TUI views)

---

## Full Test Suite: "Prove It Works"

**Goal:** PROVE that cub-scout works in main user scenarios.

The "Full Test" is a comprehensive proof test that verifies everything works:

```bash
# Full TEST - Run this to prove cub-scout works
go build ./cmd/cub-scout                           # Build
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
                    FULL TEST RESULTS
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
| TestMapStatus | `cub-scout map status` output format |
| TestMapList | `cub-scout map list` produces output |
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
| Phase 3: Examples | 8 | All example folders work |

**Examples covered in Phase 3:**

| Example | Type | What It Proves |
|---------|------|----------------|
| `apptique-examples/` | Working | Flux monorepo, Argo ApplicationSet, App of Apps |
| `flux-boutique/` | Working | 5-service Flux demo, TUI views, trace |
| `impressive-demo/` | Test Fixtures | CCVE scenarios, conference demo |
| `integrations/argocd-extension/` | Working | ArgoCD UI extension |
| `integrations/flux-operator/` | Working | Flux metrics exporter |
| `rm-demos-argocd/` | Concept | Rendered Manifest simulations |
| `app-config-rtmsg/` | Concept | Non-K8s config TUI mockup |
| `demos/` | Test Fixtures | Ownership label fixtures |

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

examples/                          # Phase 3: All examples
├── apptique-examples/             # Real GitOps patterns (Flux, Argo)
├── flux-boutique/                 # 5-service Flux demo
├── demos/                         # Test fixtures with ownership labels
├── impressive-demo/               # Conference demo with CCVE scenarios
├── integrations/                  # ArgoCD extension, Flux operator
├── rm-demos-argocd/               # Rendered Manifest simulations
├── app-config-rtmsg/              # Non-K8s config TUI mockup
└── scripts/                       # k9s, Slack, CI/CD scripts
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
| `cmd/cub-scout/localcluster_test.go` | 36 | Local cluster TUI keybindings |
| `cmd/cub-scout/hierarchy_test.go` | 27 | Hub TUI navigation |
| `cmd/cub-scout/import_wizard_test.go` | 8 | Import wizard flow |

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

### Top-Level Commands (14)

| Command | Go Unit | Go TUI | Integration | E2E Demo |
|---------|---------|--------|-------------|----------|
| `map` | - | ✓ | ✓ | ✓ |
| `trace` | ✓ | - | ✓ | - |
| `scan` | ✓ | ✓ | ✓ | ✓ |
| `snapshot` | - | - | - | - |
| `import` | - | ✓ | - | ✓ |
| `import-argocd` | - | - | - | - |
| `app-space` | - | - | ✓ | - |
| `remedy` | ✓ | - | - | - |
| `combined` | - | - | - | - |
| `parse-repo` | - | - | - | - |
| `demo` | - | - | - | ✓ |
| `version` | - | - | ✓ | - |
| `completion` | - | - | - | - |
| `setup` | - | - | - | - |

### Map Subcommands (17)

| Subcommand | Go Unit | Go TUI | Integration | E2E Demo |
|------------|---------|--------|-------------|----------|
| `map` (TUI) | - | ✓ | - | - |
| `map --hub` | - | ✓ | - | - |
| `map list` | - | ✓ | ✓ | ✓ |
| `map status` | - | ✓ | ✓ | ✓ |
| `map workloads` | - | ✓ | - | - |
| `map deployers` | - | ✓ | ✓ | - |
| `map orphans` | - | ✓ | ✓ | ✓ |
| `map crashes` | - | ✓ | - | - |
| `map issues` | - | ✓ | ✓ | ✓ |
| `map drift` | - | ✓ | - | - |
| `map bypass` | - | ✓ | - | - |
| `map sprawl` | - | ✓ | - | - |
| `map deep-dive` | - | ✓ | - | - |
| `map app-hierarchy` | - | ✓ | - | - |
| `map dashboard` | - | ✓ | - | - |
| `map queries` | - | ✓ | - | - |
| `map fleet` | - | ✓ | ✓ | - |
| `map hub` | - | ✓ | - | - |

### Other Features

| Feature | Go Unit | Go TUI | Integration | E2E Demo |
|---------|---------|--------|-------------|----------|
| Query language | ✓ | ✓ | ✓ | ✓ |
| Ownership (6 types) | ✓ | - | - | ✓ |
| TUI views (17) | - | ✓ | - | - |
| Command palette (`:`) | - | ✓ | - | - |
| Help overlay (`?`) | - | ✓ | - | - |

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
      - run: go build ./cmd/cub-scout
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
go build ./cmd/cub-scout
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

## Seven Test Levels

The test suite is organized into 7 levels, from quick smoke tests to full connected mode verification:

| Level | Time | Cluster | ConfigHub | What It Tests |
|-------|------|---------|-----------|---------------|
| **smoke** | 10s | No | No | Build + version |
| **unit** | 30s | No | No | All `go test ./...` (500+ tests) |
| **integration** | 2m | Yes | No | CLI commands work |
| **gitops** | 5m | Yes | No | Flux + ArgoCD ownership, trace |
| **demos** | 10m | Yes | No | Demo scripts run |
| **examples** | 15m | Yes | No | Example apps deploy |
| **connected** | 20m | Yes | Yes | Worker, import, app-space |
| **full** | 30m | Yes | Yes | EVERYTHING |

**Run with:** `./test/prove-it-works.sh --level=<level>`

---

## GitOps E2E Requirements

GitOps E2E tests verify BOTH Flux and ArgoCD work correctly:

### Flux Tests

| Test | What It Verifies |
|------|------------------|
| GitRepository created | Source controller fetches from Git |
| Kustomization created | Kustomize controller renders manifests |
| Flux ownership detection | Labels correctly identify Flux-managed resources |
| Flux trace command | `trace deploy/x -n y` shows ownership chain |

### ArgoCD Tests

| Test | What It Verifies |
|------|------------------|
| Application created | ArgoCD syncs application |
| ArgoCD ownership detection | Labels correctly identify ArgoCD-managed resources |
| ArgoCD trace --app | `trace --app appname` shows ArgoCD chain |

### Trace All Owner Types

```bash
# Forward trace (Flux)
./cub-scout trace deploy/cart -n boutique

# Forward trace (ArgoCD)
./cub-scout trace --app guestbook

# Reverse trace (ConfigHub)
./cub-scout trace deploy/feature-flags -n platform-core

# Reverse trace (Helm)
./cub-scout trace deploy/inventory-service -n team-inventory

# Reverse trace (Native - should warn)
./cub-scout trace deploy/legacy-auth -n legacy-apps
```

---

## Deep-Dive and App-Hierarchy Verification

### Deep-Dive

`map deep-dive` must show ALL cluster data sources:

```bash
./cub-scout map deep-dive | wc -l  # Should be 500+ lines

# Must include:
# - Flux GitRepositories (with status)
# - Flux HelmRepositories (with status)
# - Flux Kustomizations (with applied revision)
# - Flux HelmReleases (with chart version)
# - ArgoCD Applications (with sync status)
# - Workloads by owner (grouped by Flux/ArgoCD/Helm/ConfigHub/Native)
# - LiveTree (Deployment → ReplicaSet → Pod hierarchy)
```

### App-Hierarchy

`map app-hierarchy` must show inferred ConfigHub model:

```bash
./cub-scout map app-hierarchy | wc -l  # Should be 400+ lines

# Must include:
# - Units tree with workload expansion
# - Namespace → AppSpace inference
# - Ownership graph (which owner type manages each unit)
# - Label analysis (app.kubernetes.io/* labels)
# - ConfigHub mapping suggestions
```

---

## Connected Mode Testing

Connected mode requires a ConfigHub worker running:

### Prerequisites

```bash
# 1. Start worker
cub worker run dev --space tutorial

# 2. Verify worker is Ready
cub worker list
```

### Connected Tests

| Test | What It Verifies |
|------|------------------|
| `cub auth` | User is logged in |
| `cub worker run` | Worker starts and shows "Ready" |
| `app-space list` | Can list spaces (should show 150+) |
| `import --dry-run boutique` | Discovers workloads |
| `import --dry-run online-boutique` | Discovers 12 microservices |
| `import boutique` | Creates unit in ConfigHub |
| `import online-boutique` | Creates 12 units |
| `cub unit list` | Shows imported units |
| `map fleet` | Fleet view works |

---

## Test Scorecard

After comprehensive testing sessions, create a scorecard in `test/SCORECARD-YYYY-MM-DD.md`:

```markdown
## EXECUTIVE SUMMARY

### Primary Test Groups (25% each)

| Test Group | Weight | Score | Status |
|------------|--------|-------|--------|
| **Unit Tests** | 25% | 100% | PASS |
| **Integration** | 25% | 100% | PASS |
| **GitOps E2E** | 25% | 100% | PASS |
| **Connected** | 25% | 100% | PASS |
| **TOTAL** | 100% | **100%** | **FULLY PROVEN** |

### Additional Verification

| Category | Status | What to Check |
|----------|--------|---------------|
| Flux Tests | PASS/FAIL | GitRepository, Kustomization, trace |
| ArgoCD Tests | PASS/FAIL | Application, trace --app |
| Deep-Dive | PASS/FAIL | 500+ lines, all data sources |
| App-Hierarchy | PASS/FAIL | 400+ lines, units/namespaces |
| Trace (all owners) | PASS/FAIL | Flux, ArgoCD, ConfigHub, Helm, Native |
```

**Latest:** [SCORECARD-2026-01-17.md](SCORECARD-2026-01-17.md)

---

## MoSCoW Prioritization

| Priority | Requirement | Status |
|----------|-------------|--------|
| **MUST** | Unit tests pass | Required for merge |
| **MUST** | Integration tests pass | Required for merge |
| **MUST** | Flux ownership detection works | Required |
| **MUST** | ArgoCD ownership detection works | Required |
| **SHOULD** | deep-dive shows all data | Expected |
| **SHOULD** | app-hierarchy shows hierarchy | Expected |
| **SHOULD** | trace works for all owner types | Expected |
| **COULD** | Connected mode tests pass | Nice to have |
| **COULD** | Examples all deploy cleanly | Nice to have |
| **WON'T** | ConfigHub-to-cluster sync | Out of scope |

---

## See Also

- [SCORECARD-2026-01-17.md](SCORECARD-2026-01-17.md) - Latest test scorecard
- [TEST-INVENTORY.md](TEST-INVENTORY.md) - Complete test inventory with all test files
- [atk/README.md](atk/README.md) - Agent Test Kit documentation
- [CLAUDE.md](../CLAUDE.md) - Testing strategy in project instructions
