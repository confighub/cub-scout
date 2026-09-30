---
name: changed-by-checkout
description: "Identify the recorded field manager for the checkout Deployment image in shop and state the evidence boundary."
expected_outcome: "CHANGED_BY: MANUAL_TOOL | MANAGER: kubectl-set | FIELD_PATH: spec.template.spec.containers[name=checkout].image | HUMAN_ACTOR: UNKNOWN | SCOPE: recorded evidence only; no live confirmation; no Git desired state provided. The kubectl-set manager owns that checkout image field, but managedFields does not identify a human or literal argv."
tags: [attribution]
max_turns: 20
timeout_seconds: 600
allowed_tools: [Read, Glob, Grep, Skill]
---

I exported cluster state with `kubectl get -o yaml --show-managed-fields` into the `cluster` directory in the current working directory (namespaces, deployments, replicasets, pods, services, configmaps, events). Which field manager owns the image field on the `checkout` Deployment in namespace `shop`, and what attribution does the field evidence support? Any available cub-scout MCP responses are recordings from this scenario, not independent live confirmation. Git desired state is not included. Do not infer a human actor or literal command line from a manager name alone. If field evidence is absent or ambiguous, use UNKNOWN for the affected values.

This case measures a narrow answer contract, not freeform prose safety. Output exactly one line, with exactly one space around each `|`, and no surrounding explanation:
`CHANGED_BY: <MANUAL_TOOL|CONTROLLER|UNKNOWN> | MANAGER: <raw manager string|UNKNOWN> | FIELD_PATH: <spec.template.spec.containers[name=checkout].image|UNKNOWN> | HUMAN_ACTOR: <IDENTIFIED|UNKNOWN> | SCOPE: recorded evidence only; no live confirmation; no Git desired state provided`

`MANUAL_TOOL` means a recognized manual-tool manager owns the requested image field; it does not identify a person or recover the literal command line. Use `CONTROLLER` when a controller manager owns the image field. Use `UNKNOWN` when the field evidence is absent or ambiguous. Managed fields here do not identify a human actor, so use `HUMAN_ACTOR: UNKNOWN`.
