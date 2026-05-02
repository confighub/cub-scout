# Onboard Existing Flux — Worked Example (Planned)

> **Status:** Outline only. Scripts (`setup.sh`, `verify.sh`,
> `cleanup.sh`) are not yet implemented. See
> [docs/specs/pattern-1-takeover-v1.md](../../docs/specs/pattern-1-takeover-v1.md)
> for the underlying design.

This worked example demonstrates Pattern 1 takeover for an existing
Flux installation: ConfigHub becomes the source of truth, OCI is
published, and the Flux Kustomization's `spec.sourceRef` is repointed
from a `GitRepository` to an `OCIRepository`. The cluster shape does
not change.

## What This Example Will Demonstrate

1. **Pre-onboard state.** A kind cluster with Flux installed and a
   sample workload (`payments`) reconciled from a `GitRepository` via
   a `Kustomization`.
2. **Dry-run discovery.** `cub-scout onboard --controller flux --dry-run`
   shows the proposed Component / Base Variant / Deployable Variant /
   Target, plus the render diff.
3. **Takeover.** `cub-scout onboard --controller flux` creates the
   ConfigHub state, publishes OCI, creates an `OCIRepository` in the
   namespace, and updates the Kustomization's `sourceRef` to point at
   it.
4. **Verification.** `cub-scout compare three-way` reports green.
5. **A change through ConfigHub.** A reviewed change is applied through
   ConfigHub. Flux reconciles from the new OCI artifact. Three-way
   compare confirms.
6. **Rollback (optional path).** `cub-scout onboard --rollback`
   restores the Kustomization's `sourceRef` back to the original
   `GitRepository`.

## Prerequisites

- `kind` and `kubectl`.
- `flux` CLI.
- `cub-scout` binary built locally (`go build ./cmd/cub-scout`).
- `cub` CLI installed and authenticated (`cub auth login`).
- Network access to ConfigHub OCI from inside the kind cluster.
- Flux 2.0+ (for `OCIRepository` support).

## Step Sequence (Planned)

```bash
# 1. Bring up kind cluster, install Flux, deploy sample workload from Git
./setup.sh

# 2. Verify pre-onboard baseline
./verify.sh --phase pre-onboard

# 3. Dry-run the takeover
cub-scout onboard --controller flux -n payments --dry-run

# 4. Execute the takeover
cub-scout onboard --controller flux -n payments

# 5. Verify
./verify.sh --phase post-onboard

# 6. Apply a reviewed change through ConfigHub
cub unit update payments-api --space payments-prod \
  --image acme/payments-api:v1.2.3
./verify.sh --phase post-change

# 7. (Optional) Rollback
cub-scout onboard --rollback --controller flux -n payments
./verify.sh --phase post-rollback

# 8. Cleanup
./cleanup.sh
```

## Verification at Each Step

| Phase | What `verify.sh` will prove |
|-------|-----------------------------|
| pre-onboard | Flux Kustomization references a `GitRepository`, workloads ready. |
| post-onboard | Kustomization references an `OCIRepository`. ConfigHub Component / Variant exist. `compare three-way` green. Audit record present. |
| post-change | ConfigHub holds the new intent. OCI artifact updated. Flux has reconciled. `compare three-way` green. |
| post-rollback | Kustomization references the original `GitRepository`. ConfigHub state retained (or removed if `--purge`). |

## Files (To Be Added)

- `setup.sh` — bring up kind + Flux + sample workload
- `verify.sh` — phase-aware verification
- `cleanup.sh` — tear down kind cluster
- `fixtures/` — Git repo content + `GitRepository` / `Kustomization` manifests
- `contracts.md` — what each verification phase asserts and why

## HelmRelease Variant (Future)

The example above covers the Kustomization case. A follow-on example
should cover HelmRelease backed by an OCI HelmRepository. HelmReleases
backed by non-OCI HelmRepositories are out of v1 scope and should be
called out as such in any HelmRelease example.

## Open Questions Tracked Here

Example-specific instances of the open questions in the
[Pattern 1 spec](../../docs/specs/pattern-1-takeover-v1.md):

- **OCI auth from inside kind for Flux.** Flux uses a `Secret`
  referenced by the `OCIRepository`. The setup should provision this
  cleanly.
- **Render byte-equivalence.** Same question as the Argo example —
  does the demo workload's Kustomize render match ConfigHub's OCI
  render exactly?

## See Also

- [Onboard Existing](../../docs/howto/onboard-existing.md) — User-facing flow
- [Pattern 1 Takeover Spec](../../docs/specs/pattern-1-takeover-v1.md) — Design
- [Flux Import Demo](../flux-import-confighub-demo/) — The discovery-only sibling demo
- [Onboard Existing ArgoCD](../onboard-existing-argocd/) — Argo equivalent
