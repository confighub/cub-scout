---
name: sveltos-controller-facts
description: "Keep reported Sveltos delivery and continuous-health facts separate."
expected_outcome: "Retain source reports without inventing workload health or check freshness."
tags: [product-contract, sveltos, authored-opt-in, prepared-not-run]
max_turns: 6
timeout_seconds: 120
allowed_tools: [Read, Grep]
---

Read only `cluster/observations.yaml`, an authored contract fixture. No live,
network or executable use is allowed. The delivery feature and health condition
belong to different source objects. Report the raw feature status and health
condition status. Provisioned does not establish current workload health;
lastTransitionTime is not the underlying check execution time. Matching reported
cluster names do not establish a UID-verified delivery/health join. No release
identity or ConfigHub gate evaluation is provided.

Return exactly these ordered keys in one compact JSON object:
`deliveryStatus`, `conditionStatus`, `workloadHealth`, `checkFreshness`, `joined`.
Use the two reported status strings and UNKNOWN for both unsupported conclusions.
Use false for joined. Do not claim to have run Scout or a model benchmark.
