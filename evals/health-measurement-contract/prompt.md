---
name: health-measurement-contract
description: "Product-contract check for explicit health measurement coverage; fixture-backed from a sequential live read-only observation, outside benchmark-v1. Run with --ablation none."
expected_outcome: "Emit the exact JSON shape requested. Preserve legacy health Unavailable; healthMeasurement.status is unmeasured with scope controller-chain; currentChange.verdict is PASS. Do not infer unhealthy state or application health."
tags: [product-contract, health-measurement, fixture-backed]
max_turns: 10
timeout_seconds: 300
allowed_tools: [Read, Glob, Grep, Skill]
---

Review the recorded, read-only evidence in `evidence/health-measurement-contract/`. The before/after object has the same UID and resourceVersion. The explain calls were sequential observations, not an atomic snapshot. The before call used published v2.12.4; the after call used local source `8637c81`.

Use the after explain result. Return one compact JSON object only, with exactly these keys and values copied from its schema: `health`, `healthMeasurement` containing `status` and `scope`, and `currentChange` containing `verdict`. Do not add a health conclusion beyond what the fields prove.
