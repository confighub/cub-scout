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

`component`, `selected_version`, `hub_cluster`, `hub_intent`, `spoke_clusters`,
`spoke_intent`, `cert_manager_observed_version`, `cert_manager_argo_sync`,
`cert_manager_health`, `cert_manager_readiness`, `spoke_argo_placement`,
`spoke_argo_live_observation`, `unknown_interpretation`, `evidence`.

Use these output formats and enum values; they define the answer encoding, not
the factual answers:

- `selected_version`: `<chart>@<version>`.
- `spoke_clusters`: comma-separated names in the order their cert-manager rows
  appear in `desired-matrix.json`.
- `hub_intent`, `spoke_intent`: `selected`, `not_selected`, or `UNKNOWN`.
- `cert_manager_observed_version`: the observed version or `UNKNOWN`.
- `cert_manager_argo_sync`: `Synced`, `OutOfSync`, or `UNKNOWN`.
- `cert_manager_health`: `Healthy`, `Progressing`, `Degraded`, or `UNKNOWN`.
- `cert_manager_readiness`: `READY`, `NOT_READY`, or `UNKNOWN`.
  These four cert-manager observation fields each summarize all four selected
  cells; use `UNKNOWN` if the source does not establish that observation.
- `spoke_argo_placement`: `hub-managed`, `selected-locally`, `not-selected`,
  or `UNKNOWN`.
- `spoke_argo_live_observation`: `OBSERVED` or `UNKNOWN`.
- `unknown_interpretation`: `not_disabled_unmanaged_or_unhealthy`, `disabled`,
  `unmanaged`, or `unhealthy`.
- `evidence`: supplied filenames joined with `+`, in this order:
  `desired-matrix.json+config.yaml`.
