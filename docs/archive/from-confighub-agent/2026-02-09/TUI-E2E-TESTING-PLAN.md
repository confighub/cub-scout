# TUI E2E Testing Plan

## Goal

Comprehensive TUI testing that covers:
- Many real apps & hierarchies
- Multiple tools (Flux, Argo CD, Helm, Native)
- User journeys (standalone + connected modes)
- Real kind cluster with realistic workloads

## Current State

### What We Have
- **77 TUI tests** in `cmd/cub-agent/*_test.go`
- **14 views** tested for key switching
- **Basic ownership fixtures** (5 patterns)
- **RM examples** in `~/Desktop/examples-internal/rendered-manifest/`

### Key Gaps
1. No multi-tool test cluster (Flux + Argo + Helm together)
2. No E2E demos for drift, crashes, cross-cluster
3. Import wizard Steps 1-2 skipped (requires k8s)
4. No realistic hierarchies (just mock data)
5. No user journey automation

---

## Phase 1: Test Infrastructure

### 1.1 Multi-Tool Test Cluster

Create a kind cluster with ALL tools installed:

```bash
# New script: test/e2e/setup-multi-tool-cluster.sh
kind create cluster --name tui-e2e

# Install Flux
flux install

# Install Argo CD
kubectl create namespace argocd
kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml

# Install Helm (already available via kubectl)
```

### 1.2 Realistic Workload Fixtures

Deploy workloads from multiple sources:

| Source | Workloads | Owner Type |
|--------|-----------|------------|
| RM Flux examples | cert-manager, traefik, prometheus | Flux |
| RM Argo examples | grafana, loki, keda | ArgoCD |
| Direct Helm | redis, postgresql | Helm |
| kubectl apply | nginx, custom-app | Native |
| ConfigHub labels | payment-api, orders-api | ConfigHub |

### 1.3 Test Cluster States

Create snapshots for different scenarios:

| State | Description | For Testing |
|-------|-------------|-------------|
| `healthy` | All workloads Ready | Dashboard, status |
| `degraded` | 2 pods CrashLoopBackOff | Crashes view |
| `drifted` | 3 resources out of sync | Drift view |
| `orphans` | 5 Native resources | Orphans view |
| `mixed` | Combination of above | Real-world scenario |

---

## Phase 2: User Journey Tests

### Journey 1: "What's running?" (Standalone)

```
Start: cub-agent map
Steps:
  1. See dashboard (s) - verify health %
  2. Switch to workloads (w) - verify owner breakdown
  3. Filter to Flux (Q → flux) - verify filter works
  4. Search for "cert" (/) - verify search highlights
  5. View pipelines (p) - verify GitOps chain
Exit: q
```

**Assertions:**
- Dashboard shows correct health percentage
- Workload count matches kubectl
- Filter reduces visible workloads
- Search finds matching resources

### Journey 2: "Find the problem" (Standalone)

```
Start: cub-agent map (with degraded cluster)
Steps:
  1. See dashboard - notice red health bar
  2. Switch to crashes (c) - see failing pods
  3. Select a crash, press Enter - see details
  4. Switch to issues (i) - see all problems
  5. Trace ownership (T) - see who owns the failing resource
Exit: q
```

**Assertions:**
- Dashboard health < 100%
- Crashes view lists CrashLoopBackOff pods
- Issues view aggregates all problems
- Trace shows ownership chain

### Journey 3: "Audit GitOps coverage" (Standalone)

```
Start: cub-agent map
Steps:
  1. View workloads (w) - see owner breakdown
  2. Filter to Native (Q → orphans) - see unmanaged
  3. Switch to orphans view (o) - dedicated view
  4. Check sprawl (x) - see tool sprawl
  5. View cluster data (4) - see data sources
Exit: q
```

**Assertions:**
- Native count matches unmanaged resources
- Orphans view shows same resources
- Sprawl shows multiple deployers
- Cluster data shows Flux, Argo, Helm sources

### Journey 4: "Import to ConfigHub" (Connected)

```
Start: cub-agent map --hub
Steps:
  1. Press i to start import
  2. Select namespace (production)
  3. Select workloads to import
  4. Configure unit structure
  5. Wait for import completion
  6. Verify in hub view
Exit: q
```

**Assertions:**
- Import wizard shows namespace list
- Workload selection works
- Unit creation succeeds
- Hub view shows new unit

### Journey 5: "Check drift" (Standalone)

```
Start: cub-agent map (with drifted cluster)
Steps:
  1. View dashboard - see drift warning
  2. Switch to drift (d) - see out-of-sync resources
  3. View details on drifted resource
  4. Check pipelines (p) - see which deployer is affected
Exit: q
```

**Assertions:**
- Dashboard shows drift indicator
- Drift view lists out-of-sync resources
- Details show expected vs actual

### Journey 6: "Hub hierarchy navigation" (Connected)

```
Start: cub-agent map --hub
Steps:
  1. See org/space/unit tree
  2. Expand a space (→)
  3. View unit details (Enter)
  4. Toggle cluster filter (a)
  5. Switch to panel view (P)
  6. View suggest (g)
Exit: q
```

**Assertions:**
- Tree shows correct hierarchy
- Details pane updates on selection
- Filter toggles between all/cluster
- Panel shows WET↔LIVE correlation

---

## Phase 3: Test Implementation

### 3.1 New Test Files

```
cmd/cub-agent/
├── journey_test.go          # User journey tests
├── e2e_test.go              # E2E with real cluster
└── testdata/
    ├── cluster-healthy/     # Golden files for healthy state
    ├── cluster-degraded/    # Golden files for degraded state
    └── cluster-drifted/     # Golden files for drift state
```

### 3.2 Test Helpers

```go
// setupTestCluster creates a kind cluster with all tools
func setupTestCluster(t *testing.T) *TestCluster

// applyFixtures deploys workloads for a specific scenario
func applyFixtures(t *testing.T, scenario string)

// runJourney executes a user journey and captures output
func runJourney(t *testing.T, journey Journey) *JourneyResult

// assertView checks that a view matches expected output
func assertView(t *testing.T, view string, expected string)
```

### 3.3 CI Integration

```yaml
# .github/workflows/e2e-tui.yml
name: TUI E2E Tests
on: [push, pull_request]
jobs:
  e2e:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: engineerd/setup-kind@v0.5.0
      - name: Install tools
        run: |
          flux install
          kubectl apply -f argocd-install.yaml
      - name: Deploy fixtures
        run: ./test/e2e/deploy-fixtures.sh
      - name: Run TUI journeys
        run: go test ./cmd/cub-agent/... -tags=e2e -v
```

---

## Phase 4: RM Integration

### 4.1 Use RM Examples as Fixtures

```bash
# Copy RM patterns to test fixtures
cp -r ~/Desktop/examples-internal/rendered-manifest/flux-helm-kustomize \
      test/e2e/fixtures/flux-rm/

cp -r ~/Desktop/examples-internal/rendered-manifest/argo-umbrella-charts \
      test/e2e/fixtures/argo-rm/
```

### 4.2 RM-Specific Tests

| Test | What It Verifies |
|------|------------------|
| `TestRMFluxOwnership` | Flux HelmRelease detection |
| `TestRMArgoOwnership` | Argo ApplicationSet detection |
| `TestRMRenderedMode` | Plain YAML still shows correct owner |
| `TestRMCRDOrdering` | CRDs visible in cluster data |
| `TestRMMultiEnv` | dev/staging/prod environments |

### 4.3 Expected Outputs

Create expected outputs for RM patterns:

```
test/expected-outputs/examples/
├── rm-flux-dev.yaml
├── rm-flux-staging.yaml
├── rm-flux-production.yaml
├── rm-argo-dev.yaml
├── rm-argo-staging.yaml
└── rm-argo-production.yaml
```

---

## Phase 5: Apptique Examples Integration

### 5.1 Use Apptique Patterns

Already in repo at `examples/apptique-examples/`:
- `flux-monorepo/`
- `argo-applicationset/`
- `argo-app-of-apps/`

### 5.2 Apptique-Specific Tests

| Pattern | Test |
|---------|------|
| Flux Monorepo | Multi-namespace Kustomize overlays |
| Argo AppSet | Directory generator pattern |
| App of Apps | Parent/child Application detection |

---

## Test Matrix

| Journey | Standalone | Connected | Cluster Required |
|---------|------------|-----------|------------------|
| What's running? | ✓ | ✓ | Yes |
| Find the problem | ✓ | ✓ | Yes |
| Audit GitOps | ✓ | ✓ | Yes |
| Import to ConfigHub | - | ✓ | Yes + Auth |
| Check drift | ✓ | ✓ | Yes |
| Hub navigation | - | ✓ | No (mock) |

## Success Criteria

- [ ] Multi-tool cluster script works on CI
- [ ] All 6 user journeys pass
- [ ] RM examples integrated as fixtures
- [ ] Expected outputs for all patterns
- [ ] CI runs E2E tests on every PR
- [ ] < 10 min total E2E runtime

## Timeline

| Week | Milestone |
|------|-----------|
| 1 | Multi-tool cluster setup script |
| 2 | User journey test framework |
| 3 | RM fixture integration |
| 4 | CI integration + documentation |

## Related Documents

- [TESTING-GUIDE.md](historical/2026-01-07-before-reorg/docs/TESTING-GUIDE.md) - Current testing guide
- [TUI-PRD.md](TUI-PRD.md) - TUI product requirements
- [UXBOW-TESTING-STRATEGY.md](UXBOW-TESTING-STRATEGY.md) - UX testing approach
