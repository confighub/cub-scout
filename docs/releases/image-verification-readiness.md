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

## Remaining Gate Execution

The approved immediate target is the v2.12.x readiness patch, **v2.12.1**.
v2.12.0 is already published; preserve its tag and artifacts. Broader image
features belong to future [#584](https://github.com/confighub/cub-scout/issues/584),
not this gate. Execution stays tracked in
[#582](https://github.com/confighub/cub-scout/issues/582).

### 1. Authenticated Publication And Registry

- Obtain explicit approval for a disposable ConfigHub space/target, delivery
  controller and Kubernetes context. Publication may trigger event consumers;
  do not reuse an existing application or infer authorization from a valid login.
- Record tool/server versions and non-secret scope identities. Supply literal
  digest-pinned Deployment configuration and publish with the current `cub`
  CLI. Preserve the publication result and independently obtained OCI manifest
  digest; do not derive intended identity from the live controller.
- Check the actual HTTPS registry reference **without `--oci-layout`** using
  existing scoped credentials and normal TLS verification. Record how registry
  credentials are supplied without recording their values. A public anonymous
  pull alone does not prove authenticated registry access.
- Observe controller delivery and run the read-only acceptance harness with
  that exact reference, controller and context. Require overall PASS and
  complete Deployment image evidence in standalone and plugin modes.
- In isolated credential configurations, exercise missing/invalid registry
  credentials where access is required. Failures must be explicit non-PASS;
  do not revoke shared credentials or mutate production to create a negative.
- Preserve redacted dated reports, exit codes, binary checksum and request
  counts. Record controller versions and clean up only approved test resources.
  HTTP-only local publication is partial evidence, not this HTTPS gate.

### 2. Independent Clean-Machine Walkthrough

- A second operator uses the public guide, a clean machine/VM and a release
  candidate, without the implementer's checkout, preinstalled Scout or hidden
  environment. A fresh container run by the implementer is useful packaging
  evidence but is not independent operator sign-off.
- Record OS/architecture, installation route, exact version/checksum and any
  guide corrections. Keep Kubernetes and registry credentials scoped to the
  approved test environment, never embedded in the evidence.
- Run headless standalone and plugin checks from independently supplied intent;
  verify JSON/stdout/file agreement and exit 0 for PASS. With `--max-pods 1`
  against the two-replica fixture, require INCONCLUSIVE and exit 2.
- Record who verified the run privately, and publish an anonymous pass/fail
  summary with explicit omissions. The implementer cannot self-certify this gate.

### 3. Publish And Check The Distributed Binary

- Confirm the patch version, merge fixes and pass CI. Publish a candidate first
  if external walkthrough evidence is not yet available; do not label it stable
  readiness proof. Do not move an existing published tag.
- On the approved stable release, verify archive/checksum assets and the
  Homebrew update. Download into a new directory, verify `checksums.txt` and
  version output, and test the actual archive's standalone and plugin binaries,
  not a local rebuild or a renamed substitute.
- Repeat the authenticated two-replica PASS and capped INCONCLUSIVE checks
  with the downloaded binaries. Reconfirm installed plugin behavior in an
  isolated `CUB_CONFIG`, leaving the user's installation untouched.
- Record results per tested platform; compilation or archive existence is not
  runtime proof for untested platforms. Update the guide's UNRELEASED labels
  only after publication, with the exact first fixed version.
- Close #582 only when all three gates have evidence, or retain explicit
  outstanding gates. A successful release workflow alone is insufficient.
