# cub-scout

Explore and map GitOps in your clusters.

## Build & Test

```bash
go build ./cmd/cub-scout        # Build
go test ./... -v                # Test (179 tests)
./cub-scout version             # Verify
```

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
