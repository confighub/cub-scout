# cub-scout Test Inventory

**Created:** 2026-01-14
**Purpose:** Complete inventory of all tests, ensuring comprehensive coverage

---

## Test Categories

### A. Go Unit Tests (No Cluster Required)

| File | Tests | What It Proves |
|------|-------|----------------|
| `pkg/agent/ownership_test.go` | 14 | Ownership detection for all 6 types (Flux, ArgoCD, Helm, ConfigHub, Native, Unknown) |
| `pkg/agent/flux_trace_test.go` | 3 | Flux ownership chain tracing |
| `pkg/agent/argo_trace_test.go` | 3 | ArgoCD ownership chain tracing |
| `pkg/agent/trace_test.go` | 5 | General trace functionality |
| `pkg/agent/relationships_test.go` | 4 | Resource relationship detection |
| `pkg/agent/state_scanner_test.go` | 8 | CCVE state scanning |
| `pkg/agent/kyverno_scan_test.go` | 3 | Kyverno policy scanning |
| `pkg/query/query_test.go` | 12 | Query language parsing |
| `pkg/remedy/executor_test.go` | 6 | Remedy execution |
| `test/unit/ownership_test.go` | 6 | Additional ownership edge cases |
| `test/unit/cub_cli_test.go` | 4 | cub CLI JSON parsing (prevents issue #1) |

**Total: ~68 unit tests**

### B. Go TUI Tests (teatest, No Cluster Required)

| File | Tests | What It Proves |
|------|-------|----------------|
| `cmd/cub-scout/localcluster_test.go` | 36 | Local cluster TUI keybindings, views, snapshot |
| `cmd/cub-scout/hierarchy_test.go` | 27 | Hub TUI navigation, search, snapshot |
| `cmd/cub-scout/import_wizard_test.go` | 8 | Import wizard flow |
| `cmd/cub-scout/suggest_test.go` | 4 | Suggestion logic |
| `cmd/cub-scout/logger_test.go` | 2 | Logger functionality |

**Total: ~77 TUI tests**

### C. Go Integration Tests (Requires Cluster)

| File | Tests | What It Proves |
|------|-------|----------------|
| `test/integration/connected_test.go` | 12 | Full CLI commands work against real cluster |

**Tests cover:**
- `cub-scout map status` - Status output format
- `cub-scout map list` - Resource listing
- `cub-scout map list --json` - JSON output valid
- `cub-scout map deployers` - GitOps deployer listing
- `cub-scout map orphans` - Orphan detection
- `cub-scout map issues` - Issue listing
- `cub-scout scan` - CCVE scanning
- `cub-scout scan --json` - Scan JSON output
- `cub-scout trace` - Ownership tracing
- Query language filters
- Fleet view
- ConfigHub prerequisites (worker/target)

### D. Bash E2E Demos (Requires Cluster)

| Demo | What It Proves |
|------|----------------|
| `demo quick` | Apply fixtures → map status/list/issues works |
| `demo ccve` | CCVE-2025-0027 detection scenario |
| `demo healthy` | Enterprise healthy cluster view |
| `demo unhealthy` | Enterprise problem detection |
| `demo connected` | ConfigHub connected mode (requires auth) |
| `demo query` | Query language filtering |
| `demo import` | Import workflow preview |
| `scenario bigbank-incident` | Full BIGBANK narrative |
| `scenario orphan-hunt` | Orphan detection workflow |
| `scenario monday-morning` | Health check workflow |
| `scenario clobber` | Platform clobber protection |
| `scenario break-glass` | Break-glass accept/reject workflow |

### E. Example Validation

| Example | Validation |
|---------|------------|
| `argocd-extension/extension.js` | JavaScript syntax check |
| `argocd-extension/scanner-cronjob.yaml` | kubectl dry-run |
| `flux-operator/ccve-exporter.yaml` | kubectl dry-run |
| `impressive-demo/bad-configs/` | kubectl dry-run |
| `impressive-demo/fixed-configs/` | kubectl dry-run |
| `impressive-demo/demo-script.sh` | Executable check |

---

## Scenario Coverage Matrix

| Scenario | Unit | TUI | Integration | E2E Demo |
|----------|------|-----|-------------|----------|
| **Disconnected (no ConfigHub)** | ✓ | ✓ | ✓ | ✓ |
| **Connected (ConfigHub auth)** | - | ✓ | ✓ | ✓ |
| **Fleet mode** | - | ✓ | ✓ | - |

## Feature Coverage Matrix

| Feature | Unit | TUI | Integration | E2E Demo |
|---------|------|-----|-------------|----------|
| `map status` | - | ✓ | ✓ | ✓ |
| `map list` | - | ✓ | ✓ | ✓ |
| `map deployers` | - | ✓ | ✓ | - |
| `map orphans` | - | ✓ | ✓ | ✓ |
| `map issues` | - | ✓ | ✓ | ✓ |
| `map drift` | - | ✓ | - | - |
| `map crashes` | - | ✓ | - | - |
| `map workloads` | - | ✓ | - | - |
| `map fleet` | - | ✓ | ✓ | - |
| `map --hub` (TUI) | - | ✓ | - | - |
| `scan` | ✓ | ✓ | ✓ | ✓ |
| `scan --json` | - | - | ✓ | - |
| `trace` | ✓ | - | ✓ | - |
| `import` | - | ✓ | - | ✓ |
| Query language | ✓ | ✓ | ✓ | ✓ |
| Ownership (6 types) | ✓ | - | - | ✓ |

## GitOps Tool Coverage

| Tool | Unit | TUI | Integration | E2E Demo |
|------|------|-----|-------------|----------|
| Flux Kustomization | ✓ | - | ✓ | ✓ |
| Flux HelmRelease | ✓ | - | ✓ | ✓ |
| ArgoCD Application | ✓ | - | ✓ | ✓ |
| Helm Release | ✓ | - | - | - |
| Native/kubectl | ✓ | - | ✓ | ✓ |
| ConfigHub Unit | ✓ | - | - | - |

---

## Gaps Identified

### HIGH Priority

1. **No TUI integration test for Hub mode in CI** - Hub TUI requires TTY
2. **No E2E demo for `map drift`** - Drift view not exercised
3. **No E2E demo for `map crashes`** - Crashes view not exercised
4. **Flux-operator YAML validation failing** - Needs fix

### MEDIUM Priority

1. **No trace E2E demo** - Only integration test
2. **No scan --ccve E2E demo** - Specific CCVE filtering
3. **Import wizard not tested E2E** - Only TUI teatest

### LOW Priority

1. **Helm Release E2E** - Only unit tested
2. **ConfigHub Unit E2E** - Only unit tested

---

## Running Full Test Suite

```bash
# COMPREHENSIVE TEST (proves all scenarios)

# 1. Build
go build ./cmd/cub-scout

# 2. Unit + TUI tests (no cluster)
go test ./...

# 3. Integration tests (requires cluster)
go test -tags=integration ./test/integration/...

# 4. E2E demos (requires cluster)
./test/run-all.sh

# 5. Validate functions
./test/validate-functions.sh
```

---

## Test Results Template

```
Date: YYYY-MM-DD
Time: HH:MM

Unit Tests:     X passed / Y skipped / Z failed
TUI Tests:      X passed / Y skipped / Z failed
Integration:    X passed / Y skipped / Z failed
E2E Demos:      X passed / Y skipped / Z failed
Examples:       X passed / Y skipped / Z failed

Total: XXX passed

Gaps verified:
- [ ] Disconnected scenario works
- [ ] Connected scenario works
- [ ] Fleet scenario works
- [ ] All 6 ownership types detected
- [ ] Flux fixtures work
- [ ] ArgoCD fixtures work
```
