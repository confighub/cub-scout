# cub-scout

Explore and map GitOps in your clusters.

**Full documentation:** [GUIDE.md](GUIDE.md)

---

## Build & Test

```bash
go build ./cmd/cub-scout        # Build
go test ./...                   # Test
./cub-scout version             # Verify (ALWAYS use ./ prefix)
```

**IMPORTANT:** Always use `./cub-scout`, not `cub-scout`. The binary is local, not in PATH.

---

## Two CLIs

| CLI | Purpose |
|-----|---------|
| `./cub-scout` | This tool (cluster observer) |
| `cub` | ConfigHub CLI (account management, workers) |

See [GUIDE.md](GUIDE.md) for full command reference.

---

## Commands (Quick Reference)

```bash
./cub-scout map                    # Interactive TUI
./cub-scout map list               # Plain text list
./cub-scout map deep-dive          # All cluster data
./cub-scout map orphans            # Unmanaged resources
./cub-scout trace deploy/x -n y    # Who manages this?
./cub-scout trace --app name       # ArgoCD app trace
./cub-scout scan                   # Find misconfigurations
./cub-scout import -n namespace    # Import to ConfigHub
```

---

## Ownership Detection

| Owner | Detection |
|-------|-----------|
| Flux | `kustomize.toolkit.fluxcd.io/*` or `helm.toolkit.fluxcd.io/*` |
| ArgoCD | `app.kubernetes.io/instance` + `argocd.argoproj.io/instance` |
| Helm | `app.kubernetes.io/managed-by: Helm` |
| ConfigHub | `confighub.com/UnitSlug` |
| Native | None of the above |

---

## Rules

1. **Read-only by default** — no write operations without explicit flags
2. **Deterministic** — same input = same output, no AI/ML
3. **Test everything** — `go test ./...` must pass
4. **Graceful degradation** — work offline, work without ConfigHub

---

## Testing Requirements

**AI-assisted development requires 100% test coverage.** Tests are the only proof code works.

### The One Test Script

```bash
./test/prove-it-works.sh --level=unit        # 30 seconds
./test/prove-it-works.sh --level=gitops      # 5 minutes
./test/prove-it-works.sh --level=full        # EVERYTHING
```

### Four Test Groups (25% each)

| Group | Verification |
|-------|--------------|
| Unit Tests | `go test ./...` |
| Integration | `--level=integration` |
| GitOps E2E | `--level=gitops` |
| Connected | `--level=connected` |

**Target: >90% across all groups**

### Before PR Merge

```bash
go build ./cmd/cub-scout
go test ./...
```

### Before Release

```bash
./test/prove-it-works.sh --level=gitops
```

---

## Key Directories

| Path | Purpose |
|------|---------|
| `cmd/cub-scout/` | CLI commands, TUI |
| `pkg/agent/` | K8s watcher, ownership detection |
| `test/` | Tests and fixtures |
| `test/prove-it-works.sh` | THE test script |

---

## Full Documentation

See [GUIDE.md](GUIDE.md) for:
- Complete command reference
- All map subcommands
- GitOps E2E test requirements
- Connected mode testing
- Troubleshooting guide
