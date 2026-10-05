# Genuine pinned Helm lifecycle matrix

The opt-in existing harness completed on 2026-10-05 from clean source
`a561d46fe5e4dee1a4c575febfd2492843404c78`, with Helm 3.22.0, Helm 4.1.4,
kind 0.31.0 and the digest-pinned Kubernetes 1.35.0 node image named in
`summary.json`. The owned cluster was deleted and the final exit was zero.

This directory retains 89 unedited raw outputs with byte counts and hashes in
`capture-manifest.json`. The full original run is retained privately; the
manifest also hashes that private inventory. Command/tool logs with local paths
are omitted from the public projection, not rewritten. No credentials or Helm
Secret payloads are included. These are actual toy-chart Kubernetes/controller
observations, not authored JSON responses or current cluster state.

Both versions show successful hook completion, a distinct failed post-upgrade
Job, retained failed Helm history, rollback restoring configuration and the same
Deployment UID with ready replicas, and a deleted hook with an empty exact-name
list. CRD identity/version remains distinct from its changed source file and
ordinary dependent-resource updates. The server-side transition fails on the
actual foreign `.spec.replicas` owner; the explicit force-conflicts recovery
preserves Deployment UID and converges. Nine standalone/plugin trace projections
agree. The test validates retained hashes and these cross-response relationships.

Run `python3 scripts/ci/test_helm_lifecycle_capture.py` for seven offline packet
checks. Reproduce using `run-helm-version-matrix.sh EMPTY_DIR --lifecycle` and the
README's pinned assets. This is Secret-storage tracing/lifecycle compatibility
on these versions and inputs, not application-functionality, migration safety,
connected governance, MCP/TUI live parity, SQL storage or a published release.
Manager fields alone do not identify apply methods. Historical hook definitions
are not proof of a current Job; deleted execution evidence remains unavailable.

The acceptance harness makes scoped fixture changes. Scout's standalone/plugin
observations never apply, upgrade, rollback or delete Kubernetes resources.
