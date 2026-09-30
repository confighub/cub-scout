---
name: owner-confighub
description: "ConfigHub-labelled workload: the owner is ConfigHub."
expected_outcome: "OWNER: ConfigHub. deployments.yaml: inventory/inventory carries confighub.com/UnitSlug=inventory and no other ownership labels. cub-scout: explain and map list report ConfigHub."
tags: [ownership]
max_turns: 20
timeout_seconds: 600
allowed_tools: [Read, Glob, Grep, Skill]
---

I exported my cluster's state with `kubectl get -o yaml --show-managed-fields` into the `cluster` directory in the current working directory (namespaces, deployments, replicasets, pods, services, configmaps, events). This is recorded evidence: any available cub-scout MCP responses are recordings from this scenario, not independent live confirmation. Use whatever tools you have available. Who manages the `inventory` Deployment in the `inventory` namespace, and how do you know? Finish with one line `OWNER: <Flux|ArgoCD|Helm|ConfigHub|none>`.
