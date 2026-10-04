---
name: recorded-typed-list
description: "Interpret item type identity from an exact recorded DeploymentList API envelope."
expected_outcome: "Preserve recorded type derivation and omit unsupported current-state conclusions."
tags: [product-contract, recorded, typed-list, authored-opt-in, prepared-not-run]
max_turns: 6
timeout_seconds: 120
allowed_tools: [Read, Grep]
---

Use only `cluster/deployments.json`. This is a historical, byte-preserved
apps/v1 DeploymentList response. Its items omit apiVersion and kind, which the
exact Kubernetes typed envelope establishes as apps/v1 Deployment. A generic
v1/List cannot supply omitted item types. No live or network read is allowed.
Do not claim that you ran Scout, inspected a controller or established current
state, capture completeness or ownership authority.

Return one compact JSON object with exactly these keys in this order:
`apiVersion`, `kind`, `namespace`, `name`, `typedListDerivedObjects`, `currentState`.
Use the exact item namespace/name and the normalized item type. The derivation
count is the number of input objects with at least one type field supplied by
the exact typed envelope. Set currentState to UNKNOWN.
