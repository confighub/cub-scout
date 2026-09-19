# Image Verification Readiness

This is a next-release gate, not a release announcement. The published baseline
is v2.12.0; the stronger Deployment and headless-output fixes merged in #579
and #581 are not in that binary. Further hardening is tracked in
[#582](https://github.com/confighub/cub-scout/issues/582).

## Recommendation Boundary

The intended recommendation is: use the non-interactive CLI to check a known,
digest-pinned literal configuration release delivered through a supported Argo
Application or Flux Kustomization/OCIRepository. A complete Deployment result
requires configuration agreement, controller evidence, ownership, replica
coverage, running/ready regular containers and exact image digest agreement.

Always inspect the overall verdict. A running-image `match` can coexist with a
configuration `BLOCK`, for example when live replica count differs from the
intended configuration while every running replica uses the right image.

This is not image-only discovery, application functional success, proof of
publisher authority, an atomic snapshot or continuous monitoring. StatefulSet,
DaemonSet and Job image-completion adapters and index/platform resolution remain
outside the supported complete-proof scope.

## Required Evidence

| Gate | Acceptance |
|---|---|
| Deterministic correctness | Missing, duplicate, malformed, old, foreign, terminating or unreadable evidence cannot produce a complete image match. |
| Headless operation | CLI/plugin/MCP share the provider; JSON, saved reports and gated exits work without a TUI. |
| Credential lifetime | Bounded reads cancel and reap supported exec-auth helpers; static credentials remain unchanged. Platform limits are explicit. |
| Real controllers | Both adapters pull an actual OCI artifact and create real running Deployments; complete, capped and drift outcomes are checked. |
| Operator reproduction | Independently supplied intended identity, exact binary identity and dated reports are recorded, without cluster writes by the observer. |
| Distribution | A published binary containing these changes passes the walkthrough; source-only success is insufficient. |

## Recorded Result: 2026-09-19

The disposable lane completed successfully using a development build, with real
Argo and Flux controller pulls, Deployments and running containers. The
[recorded summary](../../examples/oci-release-check/live-proof.json) includes
observation times, binary checksum, source baseline/diff checksum and coverage.

| Scenario | Argo | Flux | Kubernetes requests (Argo / Flux) |
|---|---|---|---|
| Two complete replicas | PASS | PASS | 11 / 15 |
| One-Pod observation cap | INCONCLUSIVE | INCONCLUSIVE | 7 / 11 |
| Live replicas changed from two to three | BLOCK | BLOCK | 11 / 15 |

The actual `cub` plugin host and standalone binary also passed the complete and
capped scenarios for both adapters. Matching container images did not hide the
configuration drift. An earlier setup attempt observed a changing controller
and correctly returned INCONCLUSIVE; the fixture now waits for Healthy/stable
controller status before its first check, without retrying failed observations.

Additional validation passed: `go test ./...`, focused helper and headless
race tests, helper regressions on macOS and Linux, Windows test compilation,
read-only/doc contract checks, and shell harness tests. Windows compilation is
not Windows runtime proof. Bounded exec authentication is deliberately
non-interactive, even from a terminal.
Final review also reproduced an oversized HTTP error-body bypass, fixed after
the recorded live build. Deterministic discovery and Pod-list tests cover
401/403/429/500/503 responses at the transport boundary; the recorded binary
checksum describes the live build, not the subsequent error-body fix.

Still open: authenticated publication/registry-path validation in an explicitly
scoped environment, an independent clean-machine walkthrough, and publication
and revalidation of a binary containing these changes. v2.12.0 is not that binary.

## Reproduce

For an existing target, the [read-only acceptance runner](../../examples/oci-release-check/LIVE-VALIDATION.md)
records the selected binary, intended identity, dated reports and normal exit
codes. It stages that same binary in a temporary plugin directory when `cub`
is available, leaving the installed plugin untouched. A skipped plugin run is
not parity proof.

The disposable integration lane creates a new kind cluster and a local registry,
never selects an existing cluster, and removes only its own infrastructure:

```bash
bash test/e2e/image-delivery-live.sh
```

Requirements: Docker, kind, kubectl, Flux CLI, ORAS, jq, Go and network access for
pinned controller installations and container images. It uses Kubernetes
v1.35.0, Argo CD v3.4.4 and Flux v2.8.6. The workload image's platform manifest
is resolved from a pinned index independently of pod status. Reports, intended
manifests, source commit/diff and binary checksum remain in the printed evidence
directory. The script does not publish those local artifacts automatically.

This lane uses an unsigned local HTTP registry for the controllers, while Scout
verifies the same artifact digest from an OCI layout. It is real controller and
runtime validation, not authenticated ConfigHub publication or Scout's HTTPS
registry credential validation. Those boundaries must remain visible in release
claims. No script can stand in for an independent teammate's clean-machine
walkthrough.
