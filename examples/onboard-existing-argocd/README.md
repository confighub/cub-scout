# Onboard Existing ArgoCD — Worked Example (Planned)

> **Status:** Outline only. Scripts (`setup.sh`, `verify.sh`,
> `cleanup.sh`) are not yet implemented. See
> [docs/specs/pattern-1-takeover-v1.md](../../docs/specs/pattern-1-takeover-v1.md)
> for the underlying design.

This worked example demonstrates Pattern 1 takeover for an existing
Argo CD installation: ConfigHub becomes the source of truth, OCI is
published, and the Argo Application's `spec.source` is repointed from
Git to ConfigHub OCI. The cluster shape does not change.

The example replays the four-layer proof captured manually for the
2026-05-02 Argo CVE image-pin demonstration.

## What This Example Will Demonstrate

1. **Pre-onboard state.** A kind cluster with Argo CD installed and a
   sample workload (`payments`) reconciled from a Git repo.
2. **Dry-run discovery.** `cub-scout onboard --controller argo --dry-run`
   shows the proposed Component / Base Variant / Deployable Variant /
   Target, plus the render diff between current Git render and
   predicted ConfigHub OCI render.
3. **Takeover.** `cub-scout onboard --controller argo` creates the
   ConfigHub state, publishes OCI, and repoints the Application.
4. **Verification.** `cub-scout compare three-way` reports green:
   intent, render, and observed all match.
5. **A change through ConfigHub.** A reviewed change (e.g. image pin)
   is applied through ConfigHub. Argo reconciles from the new OCI
   artifact. Three-way compare confirms.
6. **Rollback (optional path).** `cub-scout onboard --rollback` restores
   the Application's pre-takeover Git ref.

## Prerequisites

- `kind` and `kubectl`.
- `cub-scout` binary built locally (`go build ./cmd/cub-scout`).
- `cub` CLI installed and authenticated (`cub auth login`).
- Network access to ConfigHub OCI from inside the kind cluster.
- Argo CD 2.6+ (for OCI source support).

## Step Sequence (Planned)

```bash
# 1. Bring up kind cluster, install Argo CD, deploy sample workload from Git
./setup.sh

# 2. Verify pre-onboard baseline
./verify.sh --phase pre-onboard

# 3. Dry-run the takeover
cub-scout onboard --controller argo -n payments --dry-run

# 4. Execute the takeover
cub-scout onboard --controller argo -n payments

# 5. Verify the takeover succeeded
./verify.sh --phase post-onboard

# 6. Apply a reviewed change through ConfigHub
cub unit update payments-api --space payments-prod \
  --image acme/payments-api:v1.2.3
./verify.sh --phase post-change

# 7. (Optional) Rollback
cub-scout onboard --rollback --controller argo -n payments
./verify.sh --phase post-rollback

# 8. Cleanup
./cleanup.sh
```

## Verification at Each Step

| Phase | What `verify.sh` will prove |
|-------|-----------------------------|
| pre-onboard | Argo Application exists, points at Git, workloads ready. |
| post-onboard | Argo Application points at ConfigHub OCI. ConfigHub Component / Variant exist. `compare three-way` green. Audit record present. |
| post-change | ConfigHub holds the new intent. OCI artifact updated. Argo has reconciled. `compare three-way` green. |
| post-rollback | Argo Application points at the original Git ref. ConfigHub state retained (or removed if `--purge`). |

## Files (To Be Added)

- `setup.sh` — bring up kind + Argo + sample workload
- `verify.sh` — phase-aware verification
- `cleanup.sh` — tear down kind cluster
- `fixtures/` — Git repo content for the sample workload
- `contracts.md` — what each verification phase asserts and why

## Open Questions Tracked Here

These are example-specific instances of the open questions in the
[Pattern 1 spec](../../docs/specs/pattern-1-takeover-v1.md):

- **OCI auth from inside kind.** What's the minimum viable auth setup
  for the Argo controller's ServiceAccount to pull from ConfigHub OCI?
  Likely a registry secret created in the `argocd` namespace.
- **Render byte-equivalence.** Does the demo workload's
  Helm/Kustomize render match ConfigHub's OCI render exactly, or do
  we exercise the cosmetic-diff path? Either is a useful demonstration.

## See Also

- [Onboard Existing](../../docs/howto/onboard-existing.md) — User-facing flow
- [Pattern 1 Takeover Spec](../../docs/specs/pattern-1-takeover-v1.md) — Design
- [ArgoCD Import Demo](../argo-import-confighub-demo/) — The discovery-only sibling demo
- [Onboard Existing Flux](../onboard-existing-flux/) — Flux equivalent
