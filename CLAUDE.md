# cub-scout

Read-only Kubernetes observer. Detects ownership (Flux, ArgoCD, Helm, Crossplane, ConfigHub, Native).

## Build & Run

```bash
go build ./cmd/cub-scout
./cub-scout map              # Interactive TUI
./cub-scout map list         # Plain text
./cub-scout trace deploy/x -n y
./cub-scout scan
```

**Always use `./cub-scout`** (local binary), not `cub-scout`.

## Documentation

| File | Purpose |
|------|---------|
| [README.md](README.md) | Project overview, install, quick start |
| [CLI-GUIDE.md](CLI-GUIDE.md) | Complete CLI reference with examples |
| [CONTRIBUTING.md](CONTRIBUTING.md) | How to contribute |

## Key Principles

1. **Single cluster** — standalone mode inspects one kubectl context; multi-cluster only via connected mode
2. **Read-only by default** — never modifies cluster state
3. **Deterministic** — same input = same output, no AI/ML
4. **Parse, don't guess** — ownership from actual labels, not heuristics
5. **Complement GitOps** — works alongside Flux, Argo, Helm
6. **Graceful degradation** — works without cluster, ConfigHub, or internet
7. **Test everything** — `go test ./...` must pass

## Directory Structure

| Path | Purpose |
|------|---------|
| `cmd/cub-scout/` | CLI commands, TUI |
| `pkg/agent/` | K8s watcher, ownership detection |
| `test/` | Tests |

## Ownership Detection

| Owner | Detection |
|-------|-----------|
| Flux | `kustomize.toolkit.fluxcd.io/*` labels |
| ArgoCD | `argocd.argoproj.io/instance` label |
| Helm | `app.kubernetes.io/managed-by: Helm` |
| Crossplane | `crossplane.io/claim-name` label *(experimental)* |
| ConfigHub | `confighub.com/UnitSlug` label |
| Native | None of the above |

## Testing

```bash
go build ./cmd/cub-scout
go test ./...
```
