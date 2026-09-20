# Image Verification Tests

Published release: **v2.12.1** (2026-09-20). Tracking: [#582](https://github.com/confighub/cub-scout/issues/582).
The published v2.12.0 tag and files remain unchanged.

## Supported Check

Scout checks a digest-pinned OCI configuration bundle against an Argo Application
or a Flux Kustomization with an OCIRepository. Complete Deployment image evidence
requires current ownership, replica counts, running and ready containers, and
matching image digests. Always use the overall verdict: matching images do not
override a configuration difference.

This is a dated observation, not an application test or a continuous guarantee.
Other workload adapters, multi-architecture digest resolution and image-only
fleet search are deferred to [#584](https://github.com/confighub/cub-scout/issues/584).

## Results

Tests ran on OrbStack with Kubernetes v1.35.0, Argo CD v3.4.4 and Flux v2.8.6.

| Test | Result |
|---|---|
| Two running, ready replicas | PASS for Argo and Flux |
| One-Pod observation limit | INCONCLUSIVE for Argo and Flux |
| Replica count differs from configuration | BLOCK for Argo and Flux |
| Authenticated ConfigHub publication and HTTPS OCI pull | PASS for Argo and Flux, standalone and plugin |
| Missing or invalid registry credentials | INCONCLUSIVE |
| Worker lacks required target permission | INCONCLUSIVE |
| Registry certificate is not trusted | INCONCLUSIVE |

The authenticated test used ConfigHub v0.5.1 and cub v0.5.3. A temporary TLS
proxy forwarded requests to the local HTTP OCI listener. Clients verified the
test CA; authentication remained enforced by ConfigHub. A test worker had
access only to the disposable space/target. Scout fetched from the registry,
not an OCI layout. This tests the local installation, not a hosted service.

Recorded reports:
- [Local OCI delivery](../../examples/oci-release-check/live-proof.json)
- [Authenticated publication and registry](../../examples/oci-release-check/authenticated-live-proof.json)
- [Published binary and plugin checks](../../examples/oci-release-check/published-binary-proof.json)

Reports include dates, binary checksums, request counts and results. The first
live build predates the HTTP error-body cap; that fix has separate deterministic
tests for 401, 403, 429, 500 and 503 responses. The authenticated run includes it.

Unit, integration and GitOps E2E CI passed for the merged implementation.
Focused race tests passed. Credential-helper tests ran on macOS and Linux;
Windows was compile-tested, not runtime-tested.

## Release Checks

- [x] Authenticated publication and HTTPS registry validation.
- [x] Clean-container test of downloaded release binaries and plugin installation.
- [x] Published v2.12.1 archives, checksums and Homebrew update verified.

Published Linux arm64 standalone and plugin checks passed against Argo in a
fresh Alpine container without the source checkout. Both returned INCONCLUSIVE
with exit 2 for capped reads; missing credentials and an untrusted CA also
returned INCONCLUSIVE. stdout matched the saved reports. The plugin installed
with `cub plugin install confighub/cub-scout@v2.12.1` and matched the archive binary.
Published macOS arm64 passed version/help smoke checks. Both downloaded archives
matched release checksums. The Homebrew cask version and hashes matched; a
Homebrew installation was not tested. Flux runtime evidence above uses the
source build of the same implementation, not the downloaded release.

The maintainer waived independent teammate sign-off on 2026-09-20. The separate
AI walkthrough did not complete because its agent hit a usage limit; it is not
counted as passing evidence. The clean-container checks above completed the
remaining release gate.

## Reproduce

For an existing target, use the [read-only runner](../../examples/oci-release-check/LIVE-VALIDATION.md).
Supply the intended manifest digest from the publication record, not the cluster.
The runner records the binary checksum, output and exit code.

For disposable local controller tests:

```bash
bash test/e2e/image-delivery-live.sh
```

That script creates and removes its own cluster. It uses an HTTP registry and
an OCI layout, so it does not reproduce the authenticated HTTPS test above.
