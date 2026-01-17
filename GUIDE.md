# cub-scout Complete Guide

**ONE DOCUMENT. EVERYTHING YOU NEED.**

This is the authoritative reference for cub-scout. Read this first.

---

## What is cub-scout?

A read-only Kubernetes observer that answers:
- **What's running?** — map all resources
- **Who owns it?** — trace ownership to Flux, ArgoCD, Helm, ConfigHub, or Native
- **Is it configured correctly?** — scan for misconfigurations

Part of the [ConfigHub](https://confighub.com) ecosystem.

---

## Two CLIs: `cub-scout` vs `cub`

| CLI | What It Is | Install |
|-----|------------|---------|
| **`./cub-scout`** | This tool. Cluster observer. | `go build ./cmd/cub-scout` |
| **`cub`** | ConfigHub CLI. Account management. | [confighub.com/docs](https://confighub.com/docs) |

**`cub-scout`** works standalone. **`cub`** is only needed for connected mode (import, workers).

```bash
# cub-scout - always use ./ prefix (local binary)
./cub-scout map
./cub-scout trace deploy/x -n y
./cub-scout scan

# cub - separate CLI, installed globally
cub auth login
cub worker run dev
cub unit list
```

---

## Commands

### Top-Level Commands

| Command | What It Does |
|---------|--------------|
| `./cub-scout map` | Interactive TUI explorer |
| `./cub-scout trace deploy/x -n y` | Who manages this resource? |
| `./cub-scout trace --app name` | Trace ArgoCD application |
| `./cub-scout scan` | Scan for misconfigurations |
| `./cub-scout snapshot` | Dump cluster state as JSON |
| `./cub-scout import` | Import namespace to ConfigHub |
| `./cub-scout version` | Print version |

### Map Subcommands

| Command | What It Does |
|---------|--------------|
| `./cub-scout map` | Interactive TUI (default) |
| `./cub-scout map list` | Plain text resource list |
| `./cub-scout map status` | One-line health check |
| `./cub-scout map deep-dive` | ALL cluster data with LiveTree |
| `./cub-scout map app-hierarchy` | Inferred ConfigHub model |
| `./cub-scout map deployers` | List GitOps deployers |
| `./cub-scout map workloads` | List workloads by owner |
| `./cub-scout map orphans` | Unmanaged resources |
| `./cub-scout map drift` | Desired vs actual state |
| `./cub-scout map issues` | Resources with problems |
| `./cub-scout map crashes` | Failing pods/deployments |
| `./cub-scout map fleet` | Multi-cluster view (requires labels) |
| `./cub-scout map hub` | ConfigHub hierarchy (requires cub auth) |

### Scan Subcommands

| Command | What It Does |
|---------|--------------|
| `./cub-scout scan` | Scan for CCVEs |
| `./cub-scout scan --list` | List all CCVE patterns |
| `./cub-scout scan --json` | JSON output |

### Import Commands

| Command | What It Does |
|---------|--------------|
| `./cub-scout import -n namespace` | Import namespace to ConfigHub |
| `./cub-scout import --dry-run` | Preview without changes |
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
| **Standalone** | map, trace, scan, snapshot | Just kubectl access |
| **Connected** | + import, app-space, map hub | ConfigHub account + cub CLI |

Most features work completely offline without any external connection.

---

## Testing

### AI-Assisted Development Requires 100% Test Coverage

When using AI to write code, tests are the only proof it works.

> "If you can't prove it works, it doesn't work."

AI can generate code that looks correct but doesn't function. Every feature must be verified.

### The One Test Script

```bash
./test/prove-it-works.sh --level=smoke       # 10 seconds
./test/prove-it-works.sh --level=unit        # 30 seconds
./test/prove-it-works.sh --level=integration # 2 minutes
./test/prove-it-works.sh --level=gitops      # 5 minutes
./test/prove-it-works.sh --level=connected   # 20 minutes
./test/prove-it-works.sh --level=full        # EVERYTHING
```

Ignore `run-all.sh` (legacy).

### Test Levels

| Level | Time | Cluster | ConfigHub | What It Tests |
|-------|------|---------|-----------|---------------|
| smoke | 10s | No | No | Build + version |
| unit | 30s | No | No | All `go test ./...` |
| integration | 2m | Yes | No | CLI commands work |
| gitops | 5m | Yes | No | Flux + ArgoCD ownership |
| connected | 20m | Yes | Yes | Worker, import |
| full | 30m | Yes | Yes | EVERYTHING |

### Four Test Groups (25% each)

| Group | Weight | Verification |
|-------|--------|--------------|
| Unit Tests | 25% | `go test ./...` |
| Integration | 25% | `--level=integration` |
| GitOps E2E | 25% | `--level=gitops` |
| Connected | 25% | `--level=connected` |

**Target: >90% across all groups**

### Minimum Required

```bash
# Before any PR merge
go build ./cmd/cub-scout
go test ./...

# Before any release
./test/prove-it-works.sh --level=gitops
```

---

## Design Principles

1. **Read-only by default** — never modifies cluster without explicit flags
2. **Deterministic** — same input = same output, no AI/ML
3. **Standalone first** — works without ConfigHub, database, or server
4. **Graceful degradation** — works offline, works without auth

---

## Directory Structure

| Path | Purpose |
|------|---------|
| `cmd/cub-scout/` | CLI commands, TUI |
| `pkg/agent/` | K8s watcher, ownership detection |
| `pkg/gitops/` | GitOps-specific logic |
| `test/` | Tests and fixtures |
| `test/prove-it-works.sh` | THE test script |
| `docs/` | Documentation |
| `examples/` | Demos and examples |

---

## Common Errors

| Error | Fix |
|-------|-----|
| `cub-scout: command not found` | Use `./cub-scout` (with `./` prefix) |
| `cub: command not found` | Install ConfigHub CLI from confighub.com |
| Cluster tests fail | Check `kubectl cluster-info` |
| GitOps tests fail | Check `flux check` and `argocd version` |
| Connected fails | Check `cub auth status` and `cub worker list` |
| ArgoCD trace fails | Login first: `argocd login localhost:8080` |

---

## Quick Reference

```bash
# Build
go build ./cmd/cub-scout

# Test
go test ./...
./test/prove-it-works.sh --level=full

# Explore
./cub-scout map                    # Interactive TUI
./cub-scout map list               # Plain text
./cub-scout map deep-dive          # All data

# Trace
./cub-scout trace deploy/x -n y    # Who manages this?
./cub-scout trace --app name       # ArgoCD app

# Scan
./cub-scout scan                   # Find problems

# Import (requires cub CLI)
cub auth login
cub worker run dev
./cub-scout import -n namespace
```

---

## Links

- **ConfigHub:** https://confighub.com
- **Discord:** https://discord.gg/confighub
- **Issues:** https://github.com/confighub/cub-scout/issues

---

**Latest scorecard:** `test/SCORECARD-2026-01-17.md`
