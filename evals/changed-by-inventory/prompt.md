---
name: changed-by-inventory
description: "A ConfigHub-delivered Deployment with an env var added by kubectl patch."
expected_outcome: "CHANGED_BY: kubectl patch. The export shows FEATURE_BULK_IMPORT=true and includes field managers in metadata.managedFields. cub-scout explain: mutationCause manual-edit, mutationManager kubectl-patch."
tags: [attribution]
max_turns: 20
timeout_seconds: 600
allowed_tools: [Read, Glob, Grep, Skill]
---

I exported my cluster's state with `kubectl get -o yaml --show-managed-fields` into the `cluster` directory in the current working directory (namespaces, deployments, replicasets, pods, services, configmaps, events). Use whatever tools you have available. Has anyone changed the `inventory` Deployment in the `inventory` namespace by hand, outside ConfigHub? Finish with one line `CHANGED_BY: <ConfigHub if it made the most recent change, the command a person used if someone did, or UNKNOWN>`.
