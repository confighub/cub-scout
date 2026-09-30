# OCI identity lifecycle receipt case

This is an offline, prepared-not-run candidate for DEL-04. It is not mapped
into `benchmark-v1`, is not admitted to the paid baseline, and does not make
the 24-case benchmark executable. It is a receipt projection, not a raw
Kubernetes snapshot or raw OCI/bundle bytes.

The fixture preserves a complete public evidence record and four complete
public YAML receipts byte-for-byte from `confighub/helm-expt` revision
`9ab4c753a888dc305a3c07956c9f8f5a19eb70a0`:

| Fixture | Pinned source path | SHA-256 |
| --- | --- | --- |
| `oci-evidence-chain.yaml` | `data/oci-evidence-chains/records/cub-installer-nginx-three-consumers.yaml` | `e9816912866968552b93e31ea6ac7b919bdea0536c05d8ba72ff32bf91f5f43e` |
| `installer-publication-receipt.yaml` | `runs/installer-oci/bitnami-nginx/24.0.2/installer-package-publication-receipt.yaml` | `ccf8623f8d5328461e909448865e918ac01454bbf99225714672bf23eec6c0d3` |
| `render-intent.yaml` | `data/helm-render-intents/intents/bitnami-nginx-24-0-2-http-clusterip.yaml` | `4e88c279923b1ff1d025e4f1c2f4f8eae572e70363d4bf211498bdf35de1c21a` |
| `render-receipt.yaml` | `recipes/bitnami/nginx/24.0.2/revisions/http-clusterip/r001/receipts/render-receipt.yaml` | `a67ac02c888bcf56b4adac6a6cf481c001bc59e26d8618cfba1af8988bb0fd09` |
| `catalog-delivery-proof.yaml` | `runs/catalog-oci-delivery-proof/bitnami-nginx-24-0-2-http-clusterip.yaml` | `0734c0be8c931e96fee18917b66aaae619954a3c467a384e8cbc60e002da4d38` |

The public receipts contain no credential payloads. The scaffold copies these
same bytes to each arm. It does not include the package tarball, OCI manifests,
or bundle bytes, and it does not make network or cluster calls.

The image field in the delivery receipt is a recorded image **reference**;
raw runtime image identity is outside this fixture's evidence scope. The
observation is historical, not current-state evidence. The receipts describe
render policy and lifecycle coverage, but do not establish hook execution.
The delivery receipt limits its claim to delivery, not policy execution.

The fixture includes receipts from the source, render, release and delivery
stages, without interpreting their result values in provenance metadata. It
does not include rendered-manifest bytes or output OCI/bundle bytes, so
independent digest recomputation is outside the fixture. Do not claim raw
runtime identity, current health, artifact integrity by recomputation, hook
execution or policy execution.

No prompt evaluation has been run. This case is a receipt-backed product
contract, not a raw-runtime snapshot, full source/build proof, live run, or
benchmark result.
