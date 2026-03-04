# cub-scout

Read-only Kubernetes observer. Detects ownership (Flux, ArgoCD, Helm, Crossplane, ConfigHub, Native).

## Build & Run

```bash
go build ./cmd/cub-scout
./cub-scout map              # Interactive TUI
./cub-scout map list         # Plain text
./cub-scout trace deploy/x -n y
./cub-scout scan
./cub-scout gitops status    # GitOps pipeline health (v0.14)
./cub-scout graph export     # Resource graph (v0.6)
./cub-scout patterns detect  # Pattern detection (v0.7)
```

**Always use `./cub-scout`** (local binary), not `cub-scout`.

**Output formats (v0.14):** Most commands support `--format ascii|json|md`.

## Claude Capability-Assistant Mode

For demo flow "Can I do X with cub-scout or ConfigHub?":

- Start from shared skill profile: [docs/ai/cub-scout-skill.md](docs/ai/cub-scout-skill.md)
- Use [docs/howto/claude-capability-assistant.md](docs/howto/claude-capability-assistant.md) as the operating playbook.
- If a request is not currently supported, file using:
  - `./scripts/create-ai-capability-issue.sh ...`
  - or issue template: `AI capability gap`

## Documentation

| File | Purpose |
|------|---------|
| [README.md](README.md) | Project overview, install, quick start |
| [CLI-GUIDE.md](CLI-GUIDE.md) | Complete CLI reference with examples |
| [CONTRIBUTING.md](CONTRIBUTING.md) | How to contribute |
| [docs/semantic-contract.md](docs/semantic-contract.md) | ASCII vs JSON meaning contract (R1-R6) |

## Key Principles

1. **Single cluster** — standalone mode inspects one kubectl context; multi-cluster only via connected mode
2. **Read-only by default** — never modifies cluster state
3. **Deterministic** — same input = same output, no AI/ML
4. **Parse, don't guess** — ownership from actual labels, not heuristics
5. **Complement GitOps** — works alongside Flux, Argo, Helm
6. **Graceful degradation** — works without cluster, ConfigHub, or internet
7. **Test everything** — `go test ./...` must pass
8. **CLI/TUI parity** — CLI and TUI are two renderings of one model. Every feature must have both a CLI command (with `--format ascii|json|md`) and a TUI equivalent. CLI is not a second-class citizen.

## Directory Structure

| Path | Purpose |
|------|---------|
| `cmd/cub-scout/` | CLI commands, TUI |
| `pkg/agent/` | K8s watcher, ownership detection |
| `test/` | Tests |

## Ownership Detection

| Owner | Detection |
|-------|-----------|
| Flux | `kustomize.toolkit.fluxcd.io/*` or `helm.toolkit.fluxcd.io/*` labels |
| ArgoCD | `argocd.argoproj.io/instance` label or tracking-id annotation |
| Helm | `app.kubernetes.io/managed-by: Helm` |
| Terraform | `app.terraform.io/run-id` annotation or managed label |
| Crossplane | `crossplane.io/claim-name` label *(experimental)* |
| ConfigHub | `confighub.com/UnitSlug` label |
| Native | None of the above |

## Testing

```bash
go build ./cmd/cub-scout
go test ./...
```

## Backlog Tracking

Future ideas live in supplemental planning docs (`docs/roadmap-*.md`, `docs/reference/*-product-guide.md`).
These are non-authoritative and easy to forget during sprint planning.

Rules:
- `docs/roadmap.md` contains an **Untracked Backlog Checklist** at the top
- When a new idea appears in any planning doc, add it to the checklist
- When an item graduates to real work, file a GitHub issue and remove the checklist entry
- The master tracking issue is **#154** — keep it current

This prevents ideas from being trapped in prose nobody re-reads.

---

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

---

## cub-scout v0.5 Delivery Addendum

This addendum defines how the principles in this document are applied during the
**v0.5 delivery phase**.

v0.5 is a **trust and utility milestone**, not a completeness milestone.

The goal is to ship a version that users can rely on daily, with clearly bounded
capabilities and explicitly documented limitations.

---

## v0.5 Priorities (In Order)

During v0.5 delivery, decisions MUST be evaluated against these priorities,
in descending order:

1. **Correctness of ownership and provenance**
   - cub-scout must not make false claims about where resources came from.
   - Unknown or ambiguous states are always preferable to incorrect certainty.

2. **Honest, explainable output**
   - Partial results are acceptable if clearly explained.
   - Errors must explain what failed and why, not just that something failed.

3. **Stability of the TUI and CLI**
   - No panics, crashes, or undefined navigation behavior.
   - Predictable keybindings and command behavior.

4. **Scope discipline**
   - Features that dilute trust or stability are out of scope.
   - v0.5 is intentionally narrow.

Completeness, performance optimizations, and breadth of controller support are
explicitly secondary during this phase.

---

## Core Principles (Never Waived)

The following principles are non-negotiable for v0.5:

- Deterministic behavior
- Parse, don't guess
- Read-only by default
- No AI/ML inference
- Avoid false ownership or orphan claims
- Graceful degradation in the presence of missing or partial data

Any change that violates these principles MUST NOT ship.

---

## Strong Defaults (May Be Deferred with Explicit Waiver)

The following expectations remain strong defaults but may be deferred for v0.5
*only if a waiver is explicitly recorded*:

- Full end-to-end (E2E) test coverage
- Exhaustive controller support
- Complete example coverage for all edge cases
- Fleet-scale or connected-mode parity

Deferral is acceptable only when the failure mode is safe and documented.

---

## Waivers (v0.5 Only)

A requirement may be waived during v0.5 delivery **only if all conditions below
are met**:

1. The limitation is clearly documented (code comment, issue, or docs).
2. The failure mode is safe:
   - No false positives
   - No misleading ownership or health claims
3. A follow-up issue is filed and labeled appropriately.

Waivers MUST be visible and intentional.
Silent corner-cutting is not acceptable.

---

## Testing Expectations for v0.5

For v0.5:

- Logic affecting ownership, provenance, or reconciliation reasoning MUST have
  deterministic tests.
- UI and CLI changes MUST be exercised against at least one real cluster.
- E2E tests are strongly encouraged but may be deferred via waiver.

The absence of a test is acceptable only if:
- The behavior is explicitly documented as provisional, AND
- The risk surface is understood and bounded.

---

## Release Bar for v0.5

v0.5 is considered shippable only if:

- Ownership claims are reproducible and conservative.
- The TUI does not panic on malformed or minimal clusters.
- Known limitations are explicitly documented.
- The tool can be recommended to another operator with clear caveats.

---

## Post–v0.5 Expectation

This addendum is temporary.

Future releases may:
- Remove waiver allowances
- Tighten testing requirements
- Raise the completeness bar

All work done under v0.5 constraints should assume later hardening.
