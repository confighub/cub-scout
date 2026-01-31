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

## Pre-Coding Test & Success Proof Requirements

All feature and bugfix issues **must define success before implementation**.

### 1. Deterministic Unit Tests (Required)
- Define exact inputs (fixtures, manifests, objects)
- Define expected outputs (ownership classification, lineage graph, buckets)
- Tests must be: order-independent, K8s-version tolerant, runnable without live cluster

### 2. Example / Full Test Coverage (Required for user-visible behavior)
- Add or extend an example under `examples/`
- Or explicitly reference an existing example it validates against
- Examples serve as: regression protection, documentation, demo artifacts

### 3. E2E / Integration Proof (Required unless explicitly waived)
For features affecting real cluster behavior, define validation in:
- Standalone mode (single cluster, kubectl context)
- Connected mode (mocked or recorded if CI cannot auth)
- Fleet mode (if behavior aggregates across clusters)

If E2E cannot run in CI: provide reproducible local script or contract test with recorded inputs/outputs.

### 4. Graceful Degradation Rules (Required)
Each issue must state:
- What happens when metadata is missing
- How partial results are surfaced
- How false "unmanaged/orphan" states are avoided

### 5. Definition of Done
An issue is complete only when:
- Tests exist and pass
- Examples demonstrate expected behavior
- User-facing output is correct **and explainable**
