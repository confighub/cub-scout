# cub-scout Test Inventory

> **Authoritative testing reference:** [docs/testing/README.md](../docs/testing/README.md)

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
| `pkg/agent/state_scanner_test.go` | 8 | risk issue state scanning |
| `pkg/agent/kyverno_scan_test.go` | 3 | Kyverno policy scanning |
| `pkg/query/query_test.go` | 12 | Query language parsing |
| `pkg/remedy/executor_test.go` | 6 | Remedy execution |
| `test/unit/ownership_test.go` | 6 | Additional ownership edge cases |
| `test/unit/cub_cli_test.go` | 4 | cub CLI JSON parsing (prevents issue #1) |

**Total: ~68 unit tests**

### A2. Pattern Ownership Tests (No Cluster Required)

| File | Tests | What It Proves |
|------|-------|----------------|
| `test/fixtures/patterns/patterns_test.go` | 22 | Complex GitOps pattern ownership + bridge detection |

**Patterns tested:**
- 3-level App-of-Apps (ArgoCD: root → 2 intermediates → 4 leaves → 4 Deployments)
- Multi-generator ApplicationSet (ArgoCD: 2 list generators → 5 Applications → 5 Deployments)
- Flux multi-tenant (platform ks + 3 tenant kustomizations + 4 Deployments)
- Mixed-tool cluster (all 7 ownership types: Flux, ArgoCD, Helm, Terraform, Crossplane, ConfigHub, Native)
- Determinism verification (same input → same ownership classification)

### A2b. Bridge Pattern Tests (No Cluster Required)

| File | Tests | What It Proves |
|------|-------|----------------|
| `internal/patterns/pattern_bridge_test.go` | 13 | Bridge pattern detection with programmatic graphs |
| `test/fixtures/patterns/patterns_test.go` | 4 (bridge) | Bridge pattern fixture integration tests |

**Bridge patterns tested:**
- Git → Flux delivery bridge (GitRepository + Kustomization + Flux-labeled workloads)
- Git → ArgoCD delivery bridge (Application + ArgoCD-labeled workloads)
- ConfigHub → OCI delivery bridge (OCIRepository with ConfigHub origin + dual-managed workloads)
- Live import (ConfigHub labels without GitOps deployer labels)

**Unit tests cover:** detection, skip (prerequisites unmet), and edge cases (no workloads, non-ConfigHub OCI)

**Fixture YAMLs:**
- `test/fixtures/patterns/bridge-git-flux/bridge-git-flux.yaml`
- `test/fixtures/patterns/bridge-git-argocd/bridge-git-argocd.yaml`
- `test/fixtures/patterns/bridge-confighub-oci/bridge-confighub-oci.yaml`
- `test/fixtures/patterns/bridge-live-import/bridge-live-import.yaml`

### A3. Scale & Performance Tests (No Cluster Required)

| File | Tests | What It Proves |
|------|-------|----------------|
| `test/scale/scale_smoke_test.go` | 5 | scan --file at 200/500/1000/2000 resources within time gates |
| `pkg/agent/attribution_bench_test.go` | 4 CI gates + 8 benchmarks | Attribution graph build + ownership detection at 500/1000/2000 nodes |
| `cmd/cub-scout/tui_bench_test.go` | 3 CI gates + 4 benchmarks | TUI View() render at 500/1000 entries within 3s gate |
| `cmd/cub-scout/tui_memory_test.go` | 2 | TUI memory < 200MB/500MB at 500/1000 resources |
| `cmd/cub-scout/tui_profile_test.go` | 2 (gated) | CPU + heap profiling (CUB_SCOUT_PROFILE=1) |

**CI gate tests (must pass):**
- `TestScaleScanFile_1000Resources` — < 10s
- `TestScaleScanFile_2000Resources` — < 20s
- `TestAttributionGraphBuild_1000Nodes_Within3s`
- `TestAttributionGraphBuild_2000Nodes_Within5s`
- `TestOwnershipDetection_1000_Within2s`
- `TestOwnershipDetection_2000_Within3s`
- `TestTUIRender_Dashboard_500_Within3s`
- `TestTUIRender_AllViews_500_Within3s`
- `TestTUIMemory_500Resources_Under200MB`
- `TestTUIMemory_1000Resources_Under500MB`

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
- `cub-scout scan` - risk issue scanning
- `cub-scout scan --json` - Scan JSON output
- `cub-scout trace` - Ownership tracing
- Query language filters
- Fleet view
- ConfigHub prerequisites (worker/target)

### D. Bash E2E Demos (Requires Cluster)

| Demo | What It Proves |
|------|----------------|
| `demo quick` | Apply fixtures → map status/list/issues works |
| `demo risk` | RISK-2025-0027 detection scenario |
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
| `flux-operator/risk-exporter.yaml` | kubectl dry-run |
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
2. **No scan --risk E2E demo** - Specific risk issue filtering
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
./test/prove-it-works.sh --level=full

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
