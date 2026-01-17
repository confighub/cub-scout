# cub-scout

Explore and map GitOps in your clusters.

## Build & Test

```bash
go build ./cmd/cub-scout        # Build
go test ./... -v                # Test (500+ tests)
./cub-scout version             # Verify (ALWAYS use ./ prefix)
```

**IMPORTANT:** Always use `./cub-scout`, not `cub-scout`. The binary is local, not in PATH.

## Commands

| Category | Commands |
|----------|----------|
| **Discovery** | `discover`, `trace`, `tree` |
| **Mapping** | `map`, `list`, `fleet` |
| **Inspection** | `health`, `issues`, `orphans`, `drift` |
| **Export** | `snapshot`, `record` |
| **Meta** | `config`, `demo`, `query`, `version` |

## Key Directories

| Path | Purpose |
|------|---------|
| `cmd/cub-scout/` | CLI commands, TUI |
| `pkg/agent/` | K8s watcher, ownership detection |
| `pkg/gitops/` | GitOps-specific logic |
| `test/` | Tests and fixtures |
| `docs/` | Documentation |
| `examples/` | Demos and examples |

## Ownership Detection

Deterministic label lookups (no AI):

| Owner | Detection |
|-------|-----------|
| Flux | `kustomize.toolkit.fluxcd.io/*` or `helm.toolkit.fluxcd.io/*` |
| Argo CD | `app.kubernetes.io/instance` + `argocd.argoproj.io/instance` |
| Helm | `app.kubernetes.io/managed-by: Helm` |
| ConfigHub | `confighub.com/UnitSlug` |
| Native | None of the above |

## Operating Modes

| Mode | Features |
|------|----------|
| **Standalone** | All discovery/mapping verbs work offline |
| **Connected** | + scan, record --hub (requires confighub.com auth) |

## Rules

1. **Read-only by default** — no write operations without explicit flags
2. **Deterministic** — same input = same output, no AI/ML
3. **Test everything** — `go test ./... -v` must pass
4. **Graceful degradation** — work offline, work without ConfigHub

---

## Vibe Coding Testing Requirements

**CRITICAL:** When using AI-assisted "vibe coding", 100% test coverage is non-negotiable. AI can hallucinate code that looks correct but doesn't work. Tests are the only proof.

### The 100% Proof Principle

> "If you can't prove it works, it doesn't work."

Every feature, command, and code path MUST be verified by tests. The goal is **100% PROOF** across all test categories.

### Four Main Test Groups (25% each)

| Test Group | Weight | What It Proves |
|------------|--------|----------------|
| **Unit Tests** | 25% | Ownership detection, query parsing, CCVE patterns |
| **Integration Tests** | 25% | CLI commands work, JSON output valid |
| **GitOps E2E** | 25% | Flux + ArgoCD ownership, trace, deep-dive |
| **Connected Mode** | 25% | ConfigHub worker, import, app-space list |

**Target: >90% score across all groups**

### Seven Test Levels

```yaml
# test/test-levels.yaml
levels:
  smoke:      go build && ./cub-scout version
  unit:       go test ./...
  integration: go test -tags=integration ./test/integration/...
  gitops:     ./test/prove-it-works.sh --level=gitops
  demos:      ./test/prove-it-works.sh --level=demos
  examples:   ./test/prove-it-works.sh --level=examples
  connected:  ./test/prove-it-works.sh --level=connected
  full:       ./test/prove-it-works.sh --level=full
```

### What Each Level Verifies

| Level | Time | Cluster | ConfigHub | What It Tests |
|-------|------|---------|-----------|---------------|
| smoke | 10s | No | No | Build + version |
| unit | 30s | No | No | All `go test ./...` (500+ tests) |
| integration | 2m | Yes | No | CLI commands work |
| gitops | 5m | Yes | No | Flux + ArgoCD ownership, trace |
| demos | 10m | Yes | No | Demo scripts run |
| examples | 15m | Yes | No | Example apps deploy |
| connected | 20m | Yes | Yes | Worker, import, app-space |
| **full** | 30m | Yes | Yes | EVERYTHING |

**IMPORTANT:** Always use `./cub-scout` (with `./` prefix) and `prove-it-works.sh`.
See `test/TESTING-QUICKSTART.md` for the one-page quick reference.

### GitOps E2E Requirements

Must verify BOTH Flux and ArgoCD:

```bash
# Flux trace (forward)
./cub-scout trace deploy/cart -n boutique

# ArgoCD trace (requires login)
kubectl port-forward svc/argocd-server -n argocd 8080:443 &
argocd login localhost:8080 --username admin --password $(kubectl get secret argocd-initial-admin-secret -n argocd -o jsonpath="{.data.password}" | base64 -d)
./cub-scout trace --app guestbook

# Reverse trace (all owner types)
./cub-scout trace deploy/feature-flags -n platform-core  # ConfigHub
./cub-scout trace deploy/inventory-service -n team-inventory  # Helm
./cub-scout trace deploy/legacy-auth -n legacy-apps  # Native (warns)
```

### Deep-Dive Verification

`map deep-dive` must show ALL cluster data sources:

```bash
./cub-scout map deep-dive | wc -l  # Should be 500+ lines

# Must include:
# - Flux GitRepositories
# - Flux HelmRepositories
# - Flux Kustomizations
# - Flux HelmReleases
# - ArgoCD Applications
# - Workloads by owner
# - LiveTree (Deployment → ReplicaSet → Pod)
```

### App-Hierarchy Verification

`map app-hierarchy` must show inferred ConfigHub model:

```bash
./cub-scout map app-hierarchy | wc -l  # Should be 400+ lines

# Must include:
# - Units tree with workload expansion
# - Namespace → AppSpace inference
# - Ownership graph
# - Label analysis
# - ConfigHub mapping suggestions
```

### Connected Mode Requirements

Before any release, connected mode MUST work:

```bash
# 1. Start worker
cub worker run dev --space tutorial

# 2. Verify connection
cub worker list  # Should show "Ready"

# 3. Test import
./cub-scout import --dry-run --namespace boutique

# 4. Actually import
./cub-scout import --namespace boutique

# 5. Verify units created
cub unit list
```

### MoSCoW Prioritization

| Priority | Requirement | Status |
|----------|-------------|--------|
| **MUST** | Unit tests pass | Required |
| **MUST** | Integration tests pass | Required |
| **MUST** | Flux ownership detection | Required |
| **MUST** | ArgoCD ownership detection | Required |
| **SHOULD** | deep-dive shows all data | Expected |
| **SHOULD** | trace works for all owners | Expected |
| **COULD** | Connected mode tests | Nice to have |
| **WON'T** | ConfigHub-to-cluster sync | Out of scope |

### Test Scorecard

After comprehensive testing sessions, create a scorecard in `test/SCORECARD-YYYY-MM-DD.md`:

```markdown
| Test Group | Weight | Score | Status |
|------------|--------|-------|--------|
| Unit Tests | 25% | 100% | PASS |
| Integration | 25% | 100% | PASS |
| GitOps E2E | 25% | 100% | PASS |
| Connected | 25% | 100% | PASS |
| **TOTAL** | 100% | **100%** | **FULLY PROVEN** |
```

**Latest:** See `test/SCORECARD-2026-01-17.md`

### Quick Verification Commands

```bash
# Full proof (run before any release)
./test/prove-it-works.sh --level=full

# Quick proof (run after changes)
go test ./... && ./cub-scout map deep-dive | head -50

# Connected proof (requires worker)
./test/prove-it-works.sh --level=connected
```

### Session End Checklist

At the end of every coding session:

1. Run `go test ./... -v` and save log
2. Run `./cub-scout map deep-dive` to verify TUI
3. If connected mode changed, run connected tests
4. Update scorecard if comprehensive testing done
5. Commit with test results summary
