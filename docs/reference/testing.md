# cub-scout Testing Guide

> Canonical source: `docs/testing/README.md`.
>
> This guide is the operator playbook for running the full suite locally and in CI.

## Quick Start

```bash
# Fast check (no cluster)
go build ./cmd/cub-scout && go test ./...

# Full verification (cluster + demos + examples)
./test/prove-it-works.sh --level=full
```

Always run the local binary as `./cub-scout`.

## Test Groups (Quality Bar)

| Group | Weight | Command | Proof |
|---|---:|---|---|
| Unit | 20% | `go test ./...` | Core logic and CLI/TUI behavior |
| Integration | 20% | `go test -tags=integration ./test/integration/...` | Commands and JSON contracts |
| GitOps E2E | 20% | `./test/prove-it-works.sh --level=gitops` | Flux/Argo ownership + trace behavior |
| Attribution contract | 20% | `go test ./pkg/agent/... -run Attribution` | Deterministic evidence + scoring |
| Connected | 20% | `./test/prove-it-works.sh --level=connected` | `cub` auth/context-dependent flows |

Target: `>90%` across groups before release.

## Test Levels (`prove-it-works.sh`)

| Level | Cluster | ConfigHub | Scope |
|---|---|---|---|
| `smoke` | no | no | build/version/help sanity |
| `unit` | no | no | all `go test ./...` |
| `integration` | yes | no | local map/scan/trace command checks |
| `gitops` | yes | no | Flux + Argo deploy/trace path |
| `demos` | yes | no | `cub-scout demo` command suite |
| `examples` | yes | no | real examples catalog + example checks |
| `connected` | yes | yes | worker/import/app-space paths |
| `full` | yes | yes | all levels |

Run with:

```bash
./test/prove-it-works.sh --level=<level>
```

## Real Examples Strategy (Pre-1.0)

Real examples are now a first-class gate.

### Catalog Source

- `test/examples/real-examples-catalog.yaml`
- `test/examples/README.md`

### Gate Test

```bash
go test -tags=integration ./test/integration/... -run '^TestRealExamplesCatalog$' -count=1
```

This enforces:

- skeleton coverage for key repo patterns
- scenario coverage for incident-style demos
- required-path existence for required examples
- optional validation of sibling repos when present locally

## ATK Deprecation Plan (Pre-1.0)

Goal: move testing and demos to `cub-scout` commands and Go tests, keeping legacy scripts only as migration scaffolding.

### Phase 1: Default path switched (done)

- `test/prove-it-works.sh` demos run via `./cub-scout demo ...`
- CI demos run via `./cub-scout demo ...`
- examples gate uses `TestRealExamplesCatalog`

### Phase 2: Legacy references cleanup (in progress)

- remove `test/atk/*` references from active docs and user-facing examples
- keep `test/atk/` marked as legacy/manual until parity is complete
- block new active-doc references to legacy wrappers

### Phase 3: Removal readiness (before 1.0)

- ensure all required fixtures are reachable without wrapper scripts
- ensure regression coverage exists in Go/integration tests
- delete or archive legacy wrappers that are no longer needed

## Connected Mode Test Boundary

Connected tests must verify only the supported interface boundary:

- authentication and context via `cub auth ...`
- data/actions via `cub` command contracts consumed by `cub-scout`

Do not add direct ConfigHub HTTP calls in `cub-scout` tests.

## Lifecycle Hazard Coverage

Risk scanning includes lifecycle hazard paths.

### Static manifest scan

```bash
./cub-scout scan --lifecycle-hazards --file <manifest.yaml>
```

### Live cluster scan

```bash
./cub-scout scan --lifecycle-hazards
```

The live scan is best-effort and reads hook-annotated objects across common resource types.

## Release Minimum

Before tagging a release candidate:

1. `go test ./...`
2. `go test -tags=integration ./test/integration/...`
3. `go test -tags=integration ./test/integration/... -run '^TestRealExamplesCatalog$' -count=1`
4. `./test/prove-it-works.sh --level=full`

## Related Docs

- `docs/testing/README.md`
- `test/README.md`
- `test/examples/README.md`
- `docs/roadmap-1x-connected-upsell.md`
