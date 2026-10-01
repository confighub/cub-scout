---
name: kubara-hub-spoke-placement
description: "Recorded-only Kubara intended placement with live observations deliberately absent."
expected_outcome: "Separate config/render-derived hub and spoke placement from Unknown live version, sync, health, and readiness; do not turn centralized Argo placement into a spoke installation claim."
tags: [benchmark-v1, PRE-04, prerequisites, kubara, placement]
max_turns: 8
timeout_seconds: 120
allowed_tools: [Read, Grep]
---

Use only the supplied files under `cluster/`. They are a pinned Kubara source
config and a desired-matrix projection; neither is a live cluster capture.

For the `cert-manager` component, report its desired selected version and
intended placement across the hub and all spokes. State whether observed
version, Argo sync, health, and workload readiness are known for those cells.
Also report how the matrix classifies Argo CD on the spokes and whether this
evidence establishes its live installation there. Treat missing observations
as unknown; do not infer disabled, unmanaged, unhealthy, or a live installation
from desired state.

Return one bare JSON object with exactly these string-valued keys, each once,
in any order and with no surrounding or contradictory prose:

`component`, `selected_version`, `hub_cluster`, `spoke_clusters`,
`cert_manager_placement`, `cert_manager_live_observation_fields`,
`spoke_argo_placement`, `spoke_argo_live_observation`, `unknown_interpretation`,
`evidence`.

Use the literal `UNKNOWN` when an observation is not established. The final
field should name the supplied files that support the answer, not a live source.
