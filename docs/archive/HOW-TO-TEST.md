# How to Test cub-scout Works

**Last Updated:** 2026-01-12

A practical guide to verifying cub-scout works correctly in your environment.

> **Looking for the full test strategy?** See [TESTING-GUIDE.md](TESTING-GUIDE.md) for comprehensive testing documentation.

---

## Table of Contents

- [Quick Verification (2 minutes)](#quick-verification-2-minutes)
- [Three-Phase Test Suite](#three-phase-test-suite)
- [Demo Scenarios](#demo-scenarios)
- [Connected Mode Testing](#connected-mode-testing)
- [What Gets Tested](#what-gets-tested)
- [CCVE and Remedy Testing (Independent)](#ccve-and-remedy-testing-independent)
- [Common Issues](#common-issues)
- [Related Documentation](#related-documentation)

---

## Quick Verification (2 minutes)

Run these commands to verify your setup works:

```bash
# 1. Build the binary
go build ./cmd/cub-scout

# 2. Check environment
./test/preflight/mini-tck

# 3. See what's running in your cluster
./test/atk/map

# 4. Run a quick demo
./test/atk/demo quick
```

If all four commands succeed, cub-scout is working correctly in standalone mode.

---

## Three-Phase Test Suite

Tests run in three phases, in order:

| Phase | What | Duration | Command |
|-------|------|----------|---------|
| **1. Standard** | Build, unit tests, integration, ownership detection | ~60s | `./test/run-all.sh --phase=1` |
| **2. Demos** | Interactive demos work correctly | ~30s | `./test/run-all.sh --phase=2` |
| **3. Examples** | Third-party integrations work | ~15s | `./test/run-all.sh --phase=3` |

### Run All Phases

```bash
# Full test suite (recommended)
./test/run-all.sh

# With options
./test/run-all.sh --quick      # Skip slow tests
./test/run-all.sh --connected  # Include ConfigHub mode
```

### Test Output

```
================================================================================
PHASE 1: Standard Tests (8 passed)
================================================================================
✓ Pre-flight check (7 checks)
✓ Build
✓ Unit tests
✓ Integration tests (6 tests)
✓ ATK ownership detection (6 scenarios)
✓ ATK map & scan

================================================================================
PHASE 2: Demos (4 passed)
================================================================================
✓ demo quick
✓ demo ccve
✓ demo healthy
✓ demo unhealthy

================================================================================
PHASE 3: Examples (5 passed)
================================================================================
✓ ArgoCD extension
✓ Flux operator
✓ Impressive demo
✓ Standard examples

STATUS: ALL TESTS PASSED ✓
```

---

## Demo Scenarios

Beyond unit tests, narrative demos prove the system solves real problems:

### Core Demos

```bash
# 30-second cluster overview
./test/atk/demo quick

# CCVE detection story (the 4-hour outage)
./test/atk/demo ccve

# Healthy vs unhealthy cluster states
./test/atk/demo healthy
./test/atk/demo unhealthy
```

### Scenario Demos

```bash
# BIGBANK incident: CCVE-2025-0027 detection
./test/atk/demo scenario bigbank-incident

# Find orphaned resources (13 orphans typical)
./test/atk/demo scenario orphan-hunt

# Monday morning health check workflow
./test/atk/demo scenario monday-morning

# Platform vs app config protection
./test/atk/demo scenario clobber

# Query language syntax
./test/atk/demo scenario query
```

### TUI Demos

```bash
# Fleet query examples
./examples/demos/fleet-queries-demo.sh

# Kyverno policy scan (460 KPOL patterns)
./examples/demos/kyverno-scan-demo.sh

# Resource ownership tracing
./examples/demos/tui-trace-demo.sh
```

---

## Connected Mode Testing

If you use ConfigHub, verify connected mode works:

### Prerequisites

```bash
# 1. Authenticate to ConfigHub
cub auth login

# 2. Start a worker in your space
cub context set space platform-dev
cub worker run cluster-worker

# 3. Verify worker is connected
cub worker list
```

### Run Connected Tests

```bash
# Pre-flight with ConfigHub checks
./test/preflight/mini-tck --connected

# Full test suite with connected mode
./test/run-all.sh --connected

# Verify ConfigHub TUI
./test/atk/map confighub
```

### What Connected Mode Tests

| Test | What It Verifies |
|------|------------------|
| Worker connected | Worker status is "Ready", not "Disconnected" |
| Target exists | Targets have valid slugs, not "null" |
| `map confighub` | Hierarchy displays correctly |
| `map --mode=admin` | Admin view produces valid output |
| `map --mode=fleet` | Fleet aggregation works |
| No null values | Prevents issue #1 (null/unknown in output) |

---

## What Gets Tested

### Ownership Detection (6 Scenarios)

Each ownership type is tested end-to-end:

| Scenario | Owner | What's Deployed | Expected |
|----------|-------|-----------------|----------|
| `argo-basic` | ArgoCD | Guestbook app | 5 resources detected |
| `confighub-basic` | ConfigHub | Backend service | 6 resources with unit labels |
| `confighub-variant` | ConfigHub | Payment service variant | 6 resources |
| `flux-basic` | Flux | Podinfo via Kustomization | 7 resources |
| `flux-helm` | Flux | Podinfo via HelmRelease | 7 resources |
| `native-basic` | Native | kubectl-applied nginx | 6 resources |

### CCVE Scanning

```bash
# Scan for configuration vulnerabilities
./test/atk/scan

# What it checks:
# - 4,500+ CCVE patterns in database
# - 460 Kyverno policy patterns (KPOL)
# - Stuck reconciliation states
# - Timing bombs (expiring certs, quotas)
# - Dangling/orphan resources
```

---

## CCVE and Remedy Testing (Independent)

**IMPORTANT:** CCVE scanning and remedy functionality can and should be tested **separately** from the rest of the confighub-agent project. These components:

- Do NOT require ConfigHub connection or workers
- Do NOT require the `cub` CLI
- Only require a Kubernetes cluster (for remedy E2E tests)
- Can be validated independently during development

### CCVE-Only Testing (No Cluster Needed)

```bash
# Validate CCVE remedy functions exist
./test/validate-functions.sh

# Run CCVE package tests
go test ./pkg/ccve/... -v

# List auto-fixable CCVEs (no cluster)
./cub-scout remedy --list

# Static file scan (no cluster)
./cub-scout scan --file test/fixtures/bad-config.yaml
```

### Remedy-Only Testing (Cluster Required, NOT ConfigHub)

```bash
# Remedy package unit tests
go test ./pkg/remedy/... -v

# Remedy E2E tests (creates test-remedy namespace)
./test/remedy-e2e.sh

# Manual dry-run test
./cub-scout remedy CCVE-2025-0147 --dry-run -n default
```

### Why This Separation Matters

1. **Faster iteration** - Test CCVE patterns without full integration testing
2. **No infrastructure** - Remedy framework works without ConfigHub
3. **CI/CD friendly** - Validate CCVE/remedy in lightweight pipelines
4. **Independent development** - CCVE team can work without ConfigHub setup

---

### Integration Tests

| Test | What It Verifies |
|------|------------------|
| `TestConnectedModePrerequisites` | cub CLI works, space accessible |
| `TestMapConfighubView` | TUI hierarchy renders correctly |
| `TestMapAdminMode` | Admin view produces valid JSON |
| `TestMapFleetMode` | Fleet aggregation works |
| `TestMapJSONOutput` | JSON output is valid |
| `TestNoNullValues` | No null/unknown in output |

---

## Common Issues

### "No cluster accessible"

```bash
# Check kubectl works
kubectl cluster-info

# Check current context
kubectl config current-context
```

### "Flux/ArgoCD CRDs not found"

```bash
# Install Flux CRDs (for testing)
flux install --components-extra=image-reflector-controller,image-automation-controller

# Or use Kind cluster with CRDs
./test/atk/setup-kind
```

### "Worker disconnected"

```bash
# Check worker status
cub worker list

# Restart worker
cub worker run <worker-name>
```

### "cub CLI not authenticated"

```bash
# Login to ConfigHub
cub auth login

# Verify
cub context show
```

---

## Test Logs

Test results are saved automatically:

```bash
# Location
docs/planning/sessions/test-runs/test-run-YYYY-MM-DD_HH-MM-SS.log

# View latest
ls -la docs/planning/sessions/test-runs/

# Last 5 logs retained (older auto-deleted)
```

---

## Quick Reference

### Verify Everything Works

```bash
# Minimum verification
./test/preflight/mini-tck && ./test/atk/demo quick

# Full verification (standalone)
./test/run-all.sh

# Full verification (with ConfigHub)
./test/run-all.sh --connected
```

### Test Counts (Full Suite)

| Category | Tests |
|----------|-------|
| Phase 1: Standard | 8 |
| Phase 2: Demos | 4 |
| Phase 3: Examples | 5 |
| Demo Scenarios | 6 |
| TUI Demo Scripts | 6 |
| Demo YAML Configs | 5 |
| Integrations | 4 |
| App Config Example | 5 |
| **Grand Total** | **43** |

### Summary

| Question | Answer |
|----------|--------|
| How do I quickly verify it works? | `./test/preflight/mini-tck && ./test/atk/demo quick` |
| How do I run all tests? | `./test/run-all.sh` |
| How do I test connected mode? | `./test/run-all.sh --connected` |
| How do I validate expected outputs? | `./test/validate-expected-outputs.sh` |
| Where are test logs? | `docs/planning/sessions/test-runs/` |
| How do I test ownership detection? | `./test/atk/verify` |
| How do I test CCVE scanning? | `./test/atk/scan` |
| How do I test remedy framework? | `go test ./pkg/remedy/... -v && ./test/remedy-e2e.sh` |
| Where are expected outputs defined? | `test/expected-outputs/` (31 files) |

---

## Expected Outputs (31 files)

Every CLI command, demo, and example has documented expected output in `test/expected-outputs/`. This serves as:
1. Documentation - what to expect
2. Automated verification - test assertions
3. Actions foundation - future `assert:` statements

```bash
# Validate all expected outputs match actual
./test/validate-expected-outputs.sh

# Validate specific category
./test/validate-expected-outputs.sh --category=cli
./test/validate-expected-outputs.sh --category=demos
./test/validate-expected-outputs.sh --category=examples
```

See: [test/expected-outputs/README.md](../test/expected-outputs/README.md) for full inventory.

---

## Related Documentation

| Document | Purpose |
|----------|---------|
| [TESTING-GUIDE.md](TESTING-GUIDE.md) | Step-by-step testing with full output examples |
| [test/expected-outputs/](../test/expected-outputs/) | Expected outputs for every command (31 files) |
| [test/atk/DEMO-REQUIREMENTS.yaml](../test/atk/DEMO-REQUIREMENTS.yaml) | Per-demo prerequisites |
| [EXAMPLES-OVERVIEW.md](EXAMPLES-OVERVIEW.md) | Demo and example index |

---

*For questions or issues: https://github.com/confighubai/confighub-agent/issues*
