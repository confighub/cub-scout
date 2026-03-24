# cub-scout Testing

> **The Golden Rule:** If a behavior matters, it has a golden file, snapshot, or hash.

This is the authoritative testing reference for cub-scout. All other testing docs defer to this one.

> **Writing new tests?** See [BEST-PRACTICES.md](BEST-PRACTICES.md) for the cookbook — decision tables, copy-paste patterns, and fixture conventions.

---

## Quick Start

```bash
# Fast check (no cluster, ~30 sec)
go build ./cmd/cub-scout && go test ./...

# Connected import delegation check (no cluster, ~10 sec)
make test-import-delegation

# Full proof (with cluster, ~5 min)
./scripts/full-test.sh

# Extended proof (all levels, ~30 min)
./test/prove-it-works.sh --level=full
```

**IMPORTANT:** Always use `./cub-scout` (local binary), not `cub-scout`.

---

## CI Coverage Gate and Proof Artifact Contract

CI unit tests enforce a coverage floor with `scripts/ci/check-coverage.sh`.

- `COVERAGE_MIN_PERCENT` is currently `25.0` in `.github/workflows/ci.yaml`.
- The unit tier must publish `coverage_total` and `coverage_min`.
- Proof publishing writes both:
  - `proof-matrix.json`
  - `proof-summary.md`

When `PROOF_UNIT=success`, `scripts/ci/emit-proof-artifact.sh` requires:

- `coverage_total` and `coverage_min` to be present (not `unknown`)
- numeric percentage values in `[0, 100]`
- `coverage_total >= coverage_min`

Local reproducibility:

```bash
go test ./scripts/ci -count=1
go test ./scripts/ci -run 'TestCoveragePolicy_|TestEmitProofArtifact_' -count=1
```

---

## The Five Test Groups

When using AI to write code, 100% test coverage is non-negotiable.

> "If you can't prove it works, it doesn't work."

| Test Group | Weight | Verification | What It Proves |
|------------|--------|--------------|----------------|
| **Unit Tests** | 20% | `go test ./...` | Ownership detection, query parsing, risk pattern checks |
| **Integration** | 20% | `go test -tags=integration ./test/integration/...` | CLI commands work, JSON output valid |
| **GitOps E2E** | 20% | `./test/prove-it-works.sh --level=gitops` | Flux + ArgoCD ownership, trace, deep-dive |
| **Attribution Contract** | 20% | `go test ./pkg/agent/... -run Attribution` | Determinism, scoring, bundle replay (v0.16+) |
| **Connected** | 20% | `./test/prove-it-works.sh --level=connected` | ConfigHub worker, import, app list |

**Target: >90% score across all groups = FULLY PROVEN**

---

## Test Tiers

### Tier 1: Unit + TUI Tests (No Cluster)

```bash
go test ./...
```

| Category | Tests | What It Proves |
|----------|-------|----------------|
| Unit tests | ~68 | Ownership detection (all 6 types), query parsing, risk pattern checks |
| TUI tests (teatest) | ~77 | All keybindings work, views render, snapshots match |
| CLI tests | ~34 | Logger, suggestions, import wizard |

**Run time:** ~10 seconds

### Tier 1.5: Import Delegation Checks (No Cluster)

```bash
make test-import-delegation
# or:
./scripts/test-import-delegation.sh
```

What this proves:
- Target selection for Kubernetes + Argo/Flux renderer targets is deterministic.
- Workload filtering after delegation keeps Helm/Native leftovers only.
- Namespace extraction for delegated GitOps import scopes is stable.
- CLI help exposes delegation behavior and `--connect` / `--no-connect` flags.

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
| TestScan | Risk scanning produces output |
| TestScanJSON | Scan JSON output is valid |
| TestTrace | Ownership tracing works |
| TestQuery | Query language filters work |
| TestFleetView | Fleet view works |
| TestConnectedModePrerequisites | Worker/target slugs not null |

**Run time:** ~6 seconds (requires cluster)

### Tier 3: E2E Demos & Examples (Full System)

```bash
./test/prove-it-works.sh --level=full
```

| Phase | Tests | What It Proves |
|-------|-------|----------------|
| Phase 1: Standard | 8 | Preflight, build, unit+integration checks |
| Phase 2: Demos | 5 | `cub-scout demo` quick/risk/query + scenarios |
| Phase 3: Examples | 8+ | Real examples catalog + cluster example validation |

**Run time:** ~4 minutes (requires cluster)

`--level=examples` and `--level=full` also run real examples catalog verification:
`go test -tags=integration ./test/integration/... -run '^TestRealExamplesCatalog$' -count=1`

---

## Test Levels

The `prove-it-works.sh` script supports 7 levels:

| Level | Time | Cluster | ConfigHub | What It Tests |
|-------|------|---------|-----------|---------------|
| **smoke** | 10s | No | No | Build + version |
| **unit** | 30s | No | No | All `go test ./...` |
| **integration** | 2m | Yes | No | CLI commands work |
| **gitops** | 5m | Yes | No | Flux + ArgoCD ownership, trace |
| **demos** | 10m | Yes | No | Demo scripts run |
| **examples** | 15m | Yes | No | Example apps deploy |
| **connected** | 20m | Yes | Yes | Worker, import, app |
| **full** | 30m | Yes | Yes | EVERYTHING |

```bash
./test/prove-it-works.sh --level=<level>
```

---

## Pre-1.0 ATK Cleanup Plan

Goal: keep behavior stable while moving daily testing and demos to `cub-scout` commands and Go integration tests.

### Phase 1: Default path (done)

- `./test/prove-it-works.sh` demo coverage runs through `./cub-scout demo ...`
- CI demos run through `./cub-scout demo ...`
- Real examples are gated by `TestRealExamplesCatalog`

### Phase 2: Active-doc cleanup (in progress)

- Replace active references to legacy wrapper scripts in docs and examples
- Keep `test/atk/` documented as legacy/manual only during migration
- Avoid introducing new active-doc dependencies on wrapper commands

### Phase 3: Removal readiness (before 1.0)

- Ensure required fixtures and demos are reachable without wrappers
- Ensure regression coverage lives in Go/integration tests
- Archive or delete wrapper scripts that no longer add unique value

---

## Writing Tests

### The Golden Rule

Every testable behavior needs proof:

| Type | Proof Mechanism | Example |
|------|-----------------|---------|
| Function output | Golden file | `testdata/expected.json` |
| TUI rendering | Snapshot (teatest) | `localcluster_test.go` |
| CLI output | JSON schema validation | `TestMapListJSON` |
| Determinism | Hash comparison | `scripts/full-test.sh` |

### What Must Be Tested

1. **Ownership detection** — Every owner type (Flux, ArgoCD, Helm, ConfigHub, Crossplane, Native)
2. **Risk patterns** — Every pattern in the catalog (46 patterns)
3. **TUI keybindings** — Every key press has expected behavior
4. **CLI commands** — Every command produces valid output
5. **Attribution** — Deterministic graph and report generation

### What NOT to Test

- Implementation details that may change
- Third-party library behavior
- ConfigHub API (mock only)

---

## Attribution Contract Tests (v0.16+)

Attribution tests verify determinism, scoring, and offline replay.

```bash
# All attribution tests
go test ./pkg/agent/... -run Attribution -v

# Specific subsystems
go test ./pkg/agent/... -run TestBuildAttributionGraph -v
go test ./pkg/agent/... -run TestAttributionReport -v
go test ./pkg/agent/... -run TestMergeAttributionGraphs -v
```

### Evidence Scoring Rules

| Evidence | Score | Reason Code |
|----------|-------|-------------|
| `owner_reference` | 100 | `owned_via_owner_ref` |
| `kustomize_origin` | 90 | *(reserved)* |
| `composite_label` | 80 | `owned_via_label` |
| `kustomize_overlay` | 75 | `owned_via_kustomize` |
| `claim_label` | 60 | `owned_via_label` |

### Contract Guarantees

All attribution tests verify:

1. **Deterministic output** — Same input always produces same output
2. **Bundle-first reasoning** — No computation at replay time
3. **No silent inference** — All ownership is explicit evidence
4. **ASCII = f(JSON) + g** — ASCII always derived from JSON facts
5. **Stable sorting** — Nodes, edges, and items sorted by ID/ref

---

## Directory Structure

```
test/
├── README.md                      # Quick reference (defers here)
├── TEST-INVENTORY.md              # Complete test inventory
├── prove-it-works.sh              # Run all E2E phases
├── preflight/                     # Pre-flight validation
│   └── mini-tck                   # Technology Compatibility Kit
├── unit/                          # Go unit tests (no cluster)
│   ├── ownership_test.go          # Ownership detection logic
│   └── cub_cli_test.go            # cub CLI output parsing
├── integration/                   # Go integration tests (cluster)
│   └── connected_test.go          # 12 integration tests
├── examples/                      # Real examples catalog + references
│   ├── README.md                  # Catalog usage
│   └── real-examples-catalog.yaml # Skeleton/value-prop/use-case matrix
├── atk/                           # Legacy Agent Test Kit (deprecated/manual)
│   └── ...                        # retained for migration period
├── fixtures/                      # Shared test data
└── golden/                        # TUI golden snapshots
```

---

## GitOps E2E Requirements

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
| ArgoCD ownership detection | Labels identify ArgoCD-managed resources |
| ArgoCD trace --app | `trace --app appname` shows ArgoCD chain |

### Trace All Owner Types

```bash
# Flux (forward trace)
./cub-scout trace deploy/cart -n boutique

# ArgoCD (forward trace)
./cub-scout trace --app guestbook

# ConfigHub (reverse trace)
./cub-scout trace deploy/feature-flags -n platform-core

# Helm (reverse trace)
./cub-scout trace deploy/inventory-service -n team-inventory

# Native (reverse trace - should warn about unmanaged)
./cub-scout trace deploy/legacy-auth -n legacy-apps
```

---

## Connected Mode Testing

### Prerequisites

```bash
# 1. Check authentication
cub auth status

# 2. Start a worker
cub worker run dev --space tutorial

# 3. Verify worker is Ready
cub worker list
```

### Connected Tests

| Test | What It Verifies |
|------|------------------|
| `cub auth` | User is logged in |
| `cub worker run` | Worker starts and shows "Ready" |
| `app list` | Can list spaces |
| `import --dry-run` | Discovers workloads |
| `import` | Creates unit in ConfigHub |
| `map fleet` | Fleet view works |

---

## Coverage Goals

| Category | Target | Status |
|----------|--------|--------|
| Ownership detection (6 types) | 100% | PASS |
| Risk patterns (46) | 100% | PASS |
| TUI keybindings | 100% | PASS |
| All demos run without error | 100% | PASS |
| Demo cleanup works | 100% | PASS |
| Attribution determinism | 100% | PASS |

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
      - run: ./test/prove-it-works.sh --level=full
```

---

## See Also

- [test/README.md](../../test/README.md) — Quick test reference
- [test/TEST-INVENTORY.md](../../test/TEST-INVENTORY.md) — Complete test inventory
- [test/atk/README.md](../../test/atk/README.md) — Agent Test Kit documentation
- [CLAUDE.md](../../CLAUDE.md) — Testing strategy in project instructions
