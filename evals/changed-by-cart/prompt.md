---
name: changed-by-cart
description: "An Argo CD-delivered Deployment scaled by hand with kubectl scale."
expected_outcome: "CHANGED_BY: kubectl (scale). The export shows 3 replicas and includes field managers in metadata.managedFields. cub-scout explain: mutationCause manual-edit, mutationManager kubectl (the scale subresource)."
tags: [attribution]
max_turns: 20
timeout_seconds: 600
allowed_tools: [Read, Glob, Grep, Skill]
---

I exported my cluster's state with `kubectl get -o yaml --show-managed-fields` into the `cluster` directory in the current working directory (namespaces, deployments, replicasets, pods, services, configmaps, events). This is recorded evidence: any available cub-scout MCP responses are recordings from this scenario, not independent live confirmation. Use whatever tools you have available. Has anyone changed the `cart` Deployment in the `shop` namespace by hand, outside Argo CD? Finish with one line `CHANGED_BY: <Argo CD if it made the most recent change, the command a person used if someone did, or UNKNOWN>`.
