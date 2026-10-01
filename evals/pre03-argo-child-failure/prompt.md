---
name: pre03-argo-child-failure
description: "Trace the supplied historical Argo parent to an observed failing descendant."
expected_outcome: "Keep parent and child health distinct; report the exact linked child failure and source scope."
tags: [benchmark-v1, PRE-03, argo, child-chain, recorded]
max_turns: 6
timeout_seconds: 120
allowed_tools: [Read, Grep]
---

Use only the seven files in `cluster/`: six retained observations and the
neutral capture metadata. Do not use network or live cluster access. These are
historical retained observations; the reads are sequential, not atomic, and do
not establish current state. The evidence covers an Argo-only branch; do not
infer a Crossplane Claim/XR chain or a complete Kubernetes object graph.

Trace the root Argo Application to its tracked child Application using the
parent's resource reference and the child's tracking annotation. Follow the
child's recorded tree to the StatefulSet and Pod. Report the parent and child
sync/health independently and the Pod's recorded state, image, failure class,
and exact failure message from the supplied diagnostics. State whether the
parent's status by itself establishes descendant health, what Crossplane
coverage is present in these files, the observation timing/scope, and the
cleanup limitation from neutral metadata. Use literal identifiers, statuses,
and failure text where present. Do not infer a UID foreign key between the two
Application objects, image digest identity, a current state, or extra runtime
relationships.

Return exactly one minified, single-line JSON object, with no Markdown fences,
leading/trailing text, or whitespace outside strings. Use exactly these
string-valued keys once each in this order:
`parent_application`, `parent_sync_status`, `parent_health_status`,
`parent_tracked_child`, `child_application`, `child_tracking_id`,
`child_sync_status`, `child_health_status`, `failing_statefulset`, `failing_pod`,
`pod_state`, `pod_image`, `pod_failure_class`, `pod_failure_message`,
`parent_status_proves_child_health`, `crossplane_evidence`,
`observation_scope`, `capture_timing`, `cleanup_status`, `evidence_files`.

Encoding vocabulary:
- Sync status: `Synced`, `OutOfSync`, or `UNKNOWN`.
- Application health: `Healthy`, `Progressing`, `Degraded`, or `UNKNOWN`.
- Resource references, names, tracking IDs, image, and failure message: exact
  literal text from the supplied files; use `MISSING` if absent.
- Pod state: `WAITING_IMAGE_PULL_BACK_OFF`, `RUNNING_READY`, `TERMINATED`, or
  `UNKNOWN`.
- Pod failure class: `ERRIMAGEPULL_AND_IMAGEPULLBACKOFF`, `OTHER_FAILURE`,
  `NO_FAILURE`, or `UNKNOWN`.
- `parent_status_proves_child_health`: `YES`, `NO`, or `UNKNOWN`.
- `crossplane_evidence`: `CROSSPLANE_CHAIN_OBSERVED`,
  `NO_CROSSPLANE_CHAIN_IN_PROVIDED_FILES`, or `UNKNOWN`.
- `observation_scope`: `HISTORICAL_SEQUENTIAL_NON_ATOMIC_NOT_CURRENT`,
  `ATOMIC_CURRENT`, or `UNKNOWN`.
- `capture_timing`: `SOURCE_OBJECT_TIMES_ONLY`,
  `PER_RESPONSE_CAPTURE_TIMES`, or `UNKNOWN`.
- `cleanup_status`: `CLEANUP_CONFIRMED`, `CLEANUP_UNCONFIRMED`,
  `UNCONFIRMED_CONFLICTING_CLEANUP_FIELDS`, or `UNKNOWN`.
- `evidence_files`: `capture-scope.json+argocd-core-root.json+argocd-core-child.json+argocd-core-root-tree.txt+argocd-core-child-tree.txt+pod-describe.txt+events.txt`.

Do not add properties, rephrase the failure message, or explain outside JSON.
