# cub-scout Complete Guide

**ONE DOCUMENT. EVERYTHING YOU NEED.**

This is the authoritative reference for cub-scout. Read this first.

---

## What is cub-scout?

A read-only Kubernetes observer that answers:
- **What's running?** — discover all resources
- **Who owns it?** — trace ownership to Flux, ArgoCD, Helm, ConfigHub, or Native
- **Is it configured correctly?** — scan for misconfigurations

Part of the [ConfigHub](https://confighub.com) ecosystem.

---

## The Binary

```bash
# Build
go build ./cmd/cub-scout

# Run (ALWAYS use ./ prefix - it's local, not in PATH)
./cub-scout version
./cub-scout map
./cub-scout scan
```

**RULE:** Always `./cub-scout`, never `cub-scout`.

---

## Commands

### Discovery

| Command | What It Does |
|---------|--------------|
| `./cub-scout discover` | What's running? Who owns it? |
| `./cub-scout trace deploy/x -n y` | Look UP: who manages this resource? |
| `./cub-scout trace --app appname` | Trace ArgoCD application |
| `./cub-scout tree deploy/x -n y` | Look DOWN: what does this create? |

### Mapping

| Command | What It Does |
|---------|--------------|
| `./cub-scout map` | Interactive TUI explorer |
| `./cub-scout map list` | Plain text resource inventory |
| `./cub-scout map deep-dive` | ALL cluster data with LiveTree |
| `./cub-scout map app-hierarchy` | Inferred ConfigHub model |
| `./cub-scout map fleet` | Multi-cluster aggregated view |
| `./cub-scout map deployers` | List GitOps deployers |
| `./cub-scout map orphans` | Unmanaged resources |

### Inspection

| Command | What It Does |
|---------|--------------|
| `./cub-scout health` | Cluster health summary |
| `./cub-scout issues` | What needs attention? |
| `./cub-scout orphans` | Unmanaged resources |
| `./cub-scout drift` | Desired vs actual state |

### Scanning

| Command | What It Does |
|---------|--------------|
| `./cub-scout scan` | Scan for misconfigurations |
| `./cub-scout scan --list` | List all CCVE patterns |
| `./cub-scout scan --json` | JSON output |

### Export

| Command | What It Does |
|---------|--------------|
| `./cub-scout snapshot` | Capture state (JSON/YAML) |
| `./cub-scout record --json` | Save discoveries as JSON |
| `./cub-scout record --hub` | Record to ConfigHub (requires auth) |

### ConfigHub Integration

| Command | What It Does |
|---------|--------------|
| `./cub-scout import --namespace x` | Import namespace to ConfigHub |
| `./cub-scout import --dry-run` | Preview import without changes |
| `./cub-scout app-space list` | List ConfigHub spaces |

---

## Ownership Detection

cub-scout detects who manages each resource using **deterministic label lookups** (no AI):

| Owner | Detection Method |
|-------|------------------|
| **Flux** | `kustomize.toolkit.fluxcd.io/*` or `helm.toolkit.fluxcd.io/*` labels |
| **ArgoCD** | `app.kubernetes.io/instance` + `argocd.argoproj.io/instance` labels |
| **Helm** | `app.kubernetes.io/managed-by: Helm` label |
| **ConfigHub** | `confighub.com/UnitSlug` label |
| **Native** | None of the above (kubectl-applied) |

**Priority:** Flux > ArgoCD > Helm > ConfigHub > Native

---

## Operating Modes

| Mode | Features | Requirements |
|------|----------|--------------|
| **Standalone** | All discovery, mapping, tracing | Just kubectl access |
| **Connected** | + scan, import, record --hub | ConfigHub account + worker |

Most features work completely offline without any external connection.

---

## Testing

### Vibe Coding = 100% Test Coverage

**CRITICAL:** When using AI-assisted "vibe coding", 100% test coverage is non-negotiable.

> "If you can't prove it works, it doesn't work."

AI can hallucinate code that looks correct but doesn't work. Tests are the only proof.

### The One Test Script

Use `prove-it-works.sh`. Ignore `run-all.sh` (legacy).

```bash
./test/prove-it-works.sh --level=smoke       # 10 seconds
./test/prove-it-works.sh --level=unit        # 30 seconds
./test/prove-it-works.sh --level=integration # 2 minutes, needs cluster
./test/prove-it-works.sh --level=gitops      # 5 minutes, needs Flux + ArgoCD
./test/prove-it-works.sh --level=connected   # 20 minutes, needs ConfigHub
./test/prove-it-works.sh --level=full        # EVERYTHING
```

### Test Levels

| Level | Time | Cluster | ConfigHub | What It Tests |
|-------|------|---------|-----------|---------------|
| smoke | 10s | No | No | Build + version |
| unit | 30s | No | No | All `go test ./...` (500+ tests) |
| integration | 2m | Yes | No | CLI commands work |
| gitops | 5m | Yes | No | Flux + ArgoCD ownership, trace |
| demos | 10m | Yes | No | Demo scripts run |
| examples | 15m | Yes | No | Example apps deploy |
| connected | 20m | Yes | Yes | Worker, import, app-space |
| full | 30m | Yes | Yes | EVERYTHING |

### Four Test Groups (25% each)

| Group | Weight | Verification |
|-------|--------|--------------|
| Unit Tests | 25% | `go test ./...` |
| Integration | 25% | `./test/prove-it-works.sh --level=integration` |
| GitOps E2E | 25% | `./test/prove-it-works.sh --level=gitops` |
| Connected | 25% | `./test/prove-it-works.sh --level=connected` |

**Target: >90% across all groups**

### Minimum Required Tests

```bash
# Before any PR merge (CI enforces this)
go build ./cmd/cub-scout
go test ./...

# Before any release
./test/prove-it-works.sh --level=gitops
```

### GitOps E2E Requirements

Must verify BOTH Flux and ArgoCD:

```bash
# Flux trace
./cub-scout trace deploy/cart -n boutique

# ArgoCD trace (requires login first)
kubectl port-forward svc/argocd-server -n argocd 8080:443 &
argocd login localhost:8080 --username admin --password $(kubectl get secret argocd-initial-admin-secret -n argocd -o jsonpath="{.data.password}" | base64 -d)
./cub-scout trace --app guestbook

# All owner types
./cub-scout trace deploy/feature-flags -n platform-core  # ConfigHub
./cub-scout trace deploy/inventory-service -n team-inventory  # Helm
./cub-scout trace deploy/legacy-auth -n legacy-apps  # Native (warns)
```

### Deep-Dive and App-Hierarchy

```bash
# Must produce 500+ lines
./cub-scout map deep-dive | wc -l

# Must produce 400+ lines
./cub-scout map app-hierarchy | wc -l
```

### Connected Mode Testing

```bash
# 1. Start worker
cub worker run dev --space tutorial

# 2. Verify Ready
cub worker list

# 3. Test import
./cub-scout import --dry-run --namespace boutique

# 4. Actually import
./cub-scout import --namespace boutique

# 5. Verify
cub unit list
```

### Test Scorecard

After comprehensive testing, create `test/SCORECARD-YYYY-MM-DD.md`:

```markdown
| Test Group | Weight | Score | Status |
|------------|--------|-------|--------|
| Unit Tests | 25% | 100% | PASS |
| Integration | 25% | 100% | PASS |
| GitOps E2E | 25% | 100% | PASS |
| Connected | 25% | 100% | PASS |
| **TOTAL** | 100% | **100%** | **FULLY PROVEN** |
```

---

## Design Principles

### Deterministic, Not AI

All ownership detection uses **deterministic heuristics**:
- Same input = same output, every time
- Fully auditable and explainable
- No machine learning
- Works completely offline

### Read-Only by Default

cub-scout only reads cluster state. It never modifies resources unless you explicitly use write commands.

### Standalone First

Most features work without any external connection:
- No database required
- No server required
- No ConfigHub account required for exploration

---

## Directory Structure

| Path | Purpose |
|------|---------|
| `cmd/cub-scout/` | CLI commands, TUI |
| `pkg/agent/` | K8s watcher, ownership detection |
| `pkg/gitops/` | GitOps-specific logic |
| `test/` | Tests and fixtures |
| `test/prove-it-works.sh` | THE test script |
| `test/SCORECARD-*.md` | Test result scorecards |
| `docs/` | Documentation |
| `examples/` | Demos and examples |

---

## Common Errors

| Error | Fix |
|-------|-----|
| `cub-scout: command not found` | Use `./cub-scout` (with `./` prefix) |
| Cluster tests fail | Check `kubectl cluster-info` |
| GitOps tests fail | Check `flux check` and `argocd version` |
| Connected fails | Check `cub auth status` and `cub worker list` |
| ArgoCD trace fails | Login first: `argocd login localhost:8080` |

---

## MoSCoW Prioritization

| Priority | Requirement |
|----------|-------------|
| **MUST** | Unit tests pass |
| **MUST** | Integration tests pass |
| **MUST** | Flux ownership detection works |
| **MUST** | ArgoCD ownership detection works |
| **SHOULD** | deep-dive shows all data |
| **SHOULD** | trace works for all owner types |
| **COULD** | Connected mode tests pass |
| **WON'T** | ConfigHub-to-cluster sync |

---

## Quick Reference

```bash
# Build
go build ./cmd/cub-scout

# Test
go test ./...                                    # Unit tests
./test/prove-it-works.sh --level=full           # Full E2E

# Explore
./cub-scout map                                  # Interactive TUI
./cub-scout map list                             # Plain text list
./cub-scout map deep-dive                        # All cluster data

# Trace
./cub-scout trace deploy/x -n y                  # Who manages this?
./cub-scout trace --app appname                  # ArgoCD app chain

# Scan
./cub-scout scan                                 # Find misconfigurations

# Import (requires ConfigHub)
./cub-scout import --namespace x --dry-run       # Preview
./cub-scout import --namespace x                 # Actually import
```

---

## Links

- **ConfigHub:** https://confighub.com
- **Discord:** https://discord.gg/confighub
- **Issues:** https://github.com/confighub/cub-scout/issues

---

**Latest scorecard:** `test/SCORECARD-2026-01-17.md`
