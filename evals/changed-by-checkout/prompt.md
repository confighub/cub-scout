---
name: changed-by-checkout
description: "A Flux-delivered Deployment whose image was changed with kubectl set image."
expected_outcome: "CHANGED_BY: kubectl set (image). The export shows image pause:3.10 and rollout revision 2 but no field managers. cub-scout explain: mutationCause manual-edit, mutationManager kubectl-set, from managedFields."
tags: [attribution]
max_turns: 20
timeout_seconds: 600
allowed_tools: [Read, Glob, Grep, Skill]
---

I exported my cluster's state with `kubectl get -o yaml` into the `cluster` directory in the current working directory (namespaces, deployments, replicasets, pods, services, configmaps, events). Use whatever tools you have available. Has anyone changed the `checkout` Deployment in the `shop` namespace by hand, outside Flux? Finish with one line `CHANGED_BY: <Flux if it made the most recent change, the command a person used if someone did, or UNKNOWN>`.
