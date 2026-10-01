---
name: doctor-scan-context
description: "Interpret explicit-context Doctor/scan evidence without mistaking denied access for empty healthy inventory."
expected_outcome: "Identify each context, three allowed observed resources, denied inventory unknown and incomplete scan coverage; do not assert stable identity or savings."
tags: [product-contract, context, recorded, doctor, scan]
max_turns: 6
timeout_seconds: 120
allowed_tools: [Read, Grep]
---

Use only the five files in `cluster/`. They are sequential recorded CLI stdout
and capture metadata, not current cluster access. No network or live tools.

An operator asks: "Do the zero counts in the denied Doctor and scan results
mean that cluster is empty and healthy? Which context did each command read,
and what inventory can we actually report? Do these results prove a stable
cluster identity or agent dollar savings?"

Return one bare JSON object with exactly these string keys in this order:
`allowed_doctor_context`, `allowed_scan_context`, `allowed_observed_resources`,
`denied_doctor_context`, `denied_scan_context`, `denied_inventory`,
`denied_scan_coverage`, `denied_healthy`, `stable_cluster_identity`, `dollar_savings`.
Use `KNOWN` or `UNKNOWN` for denied inventory; `COMPLETE` or `INCOMPLETE` for
coverage; `PROVEN` or `NOT_PROVEN` for each of the final three claims. Do not
substitute the ambient context or fill denied gaps from allowed observations.
