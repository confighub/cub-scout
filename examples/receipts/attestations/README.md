# Direct ConfigHub revision claims

Inspect the exact revision named by a live object's combined
`confighub.com/origin` annotation:

```sh
./cub-scout explain configmap/contract-config -n test --with-confighub --format json
./cub-scout explain configmap/contract-config -n test --with-confighub --tui
./cub-scout trace configmap/contract-config -n test --with-confighub --format md
./cub-scout receipt verify configmap/contract-config -n test --with-confighub --format json
```

`explain.json` and `receipt.json` are actual isolated-server acceptance outputs
for a disposable source-annotation fixture. They demonstrate the read adapter,
not equivalence between intended configuration and the live ConfigMap. The
receipt's fingerprint validates and its verdict matches the observation without
the supporting claim block. Setup creates a disposable namespace outside Scout;
Scout only observes it. The namespace is deleted after inspection.

The ConfigHub Attestation is an unsigned server entity about intended revisions;
the Scout receipt is an in-toto Statement about live observations. Direct Pass
and Fail claims stay visible, including expired and observably revoked claims.
An omitted `revoked` means unknown. Neither a missing claim nor a restored
DataHash proves effective approval or absence of revocation. Workflow decisions
remain ConfigHub's responsibility.

The adapter uses revision GET plus one space-scoped attestation list. Both reads
share a 15-second deadline and have independent 1 MiB output limits. It rejects
conflicting identities and ambiguous responses; missing fields/references produce
omissions. Receipts additionally read the exact revision's data and recompute its
SHA-256 before adding `confighub-data-sha256` to the ConfigHub subject. Its
separate `sha256` hashes canonical JSON containing exact space/unit/revision IDs
and the strictly parsed objects in served order. Unsupported manifests, denied
reads and mismatched hashes leave the subject omitted. The data read has its own
15-second deadline and 1 MiB limit. Effective coverage and viewer RBAC gates
remain open. Standalone commands do not perform these reads.

Reproduce the adapter against a disposable ConfigHub v0.8.3 server with matching
cub, an exact revision and Pass/Fail/expired/revocation entities. Create an owned
ConfigMap whose combined origin supplies spaceId, unitSlug, unitId and
revisionNum; run the commands above and connected MCP `confighub_attestations`.
Use `./cub-scout receipt validate receipt.json --format json` to verify integrity.
Never reuse a production namespace for fixture setup.
