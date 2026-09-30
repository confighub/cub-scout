---
name: changed-by-checkout
description: "Identify the recorded field manager for the checkout Deployment image and state the evidence boundary."
expected_outcome: "The checkout metadata.managedFields entry whose fieldsV1 owns spec.template.spec.containers[name=checkout].image has manager kubectl-set. This supports the kubectl-set manager attribution but does not recover the exact argv or prove a human actor. The export has no Git desired state; MCP output is recorded, not live."
tags: [attribution]
max_turns: 20
timeout_seconds: 600
allowed_tools: [Read, Glob, Grep, Skill]
---

I exported cluster state with `kubectl get -o yaml --show-managed-fields` into the `cluster` directory in the current working directory (namespaces, deployments, replicasets, pods, services, configmaps, events). Any available cub-scout MCP responses are recordings from this scenario, not independent live confirmation. Git desired state is not included. Which field manager owns the `checkout` Deployment's image field in this recording, and does the available evidence support attributing a manual-tool edit? Do not infer a human actor or literal command line from the manager name alone. If the field evidence is absent or ambiguous, use UNKNOWN for the affected values.

This case measures a narrow answer contract, not freeform prose safety. Output exactly one line and no surrounding explanation, using this schema (replace each placeholder with your evidence-based value):
`CHANGED_BY: <manager-based edit attribution or UNKNOWN> | MANAGER: <metadata.managedFields manager or UNKNOWN> | FIELD_PATH: <metadata.managedFields image field path or UNKNOWN> | SCOPE: recorded evidence only; no live confirmation; no Git desired state provided`
