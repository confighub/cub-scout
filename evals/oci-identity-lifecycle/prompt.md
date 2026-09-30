---
name: oci-identity-lifecycle
description: "Receipt-backed OCI identity chain; prepared, not run."
expected_outcome: "Separate recorded identities and delivery observations from missing runtime, current-state, recomputation, and lifecycle proof."
tags: [product-contract, DEL-04, OCI, recorded-receipts]
max_turns: 8
timeout_seconds: 120
allowed_tools: [Read, Grep]
---

Trace the package identity through the reviewed render, ConfigHub release, and
the three consumers using only the complete receipts under
`evidence/oci-identity-lifecycle/`. They are committed public source receipts,
not raw OCI bundle bytes or a live cluster snapshot. Keep similarly named
digests distinct. Report only recorded delivery and readiness. Distinguish the
recorded image reference from a runtime image ID. Mark current state,
independent OCI/bundle verification, runtime image ID, hook execution, and
policy execution `UNKNOWN` when the receipts do not establish them. A recorded
`--no-hooks` policy is not evidence of hook execution.

Return one bare JSON object with exactly these keys, in any order, and no
surrounding prose. Every value must be a string:

`package_oci_reference`, `package_manifest_digest`, `package_layer_digest`,
`rendered_manifest_sha256`, `rendered_object_set_sha256`,
`confighub_release_id`, `output_oci_digest`, `bundle_digest`,
`consumer_digests_match`, `recorded_consumer_results`,
`recorded_image_reference`, `runtime_image_id`, `current_cluster_state`,
`independent_bundle_verification`, `recorded_hook_policy`,
`no_hooks_render_flag`, `lifecycle_observed`, `hook_execution`,
`policy_execution`, and `observation_time`.

Copy exact identities and timestamps only where a receipt records them. For
`consumer_digests_match`, use `YES`, `NO`, or `UNKNOWN`. Format
`recorded_consumer_results` in the delivery receipt's leg order:
`Argo CD=<fraction>;Flux=<fraction>;Direct apply=<fraction>`.
`observation_time` must be copied from `catalog-delivery-proof.yaml` at
`spec.observedAt`. For `current_cluster_state`,
`independent_bundle_verification`, `runtime_image_id`, and `hook_execution`,
use `UNKNOWN` when evidence is absent. The delivery receipt says the scratch
organization lacked apply-policy Triggers and limits its proof to delivery;
it does not establish whether policy execution occurred. Therefore
`policy_execution` is `UNKNOWN`, not `NOT_RUN`. Do not copy the recorded image
reference into `runtime_image_id`. Copy the `recorded_hook_policy`, the
specific no-hooks flag, and `lifecycle_observed` exactly. Do not substitute a
similar digest, infer runtime state from the receipt's observation, or claim
independent verification without artifact bytes. Do not run historical
commands or use a live cluster, registry, or ConfigHub service.
