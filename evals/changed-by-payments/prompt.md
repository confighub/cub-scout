---
name: changed-by-payments
description: "Control: a Helm-delivered Deployment nobody edited by hand."
expected_outcome: "CHANGED_BY: Helm. No hand edit. cub-scout explain: mutationCause controller-drift, mutationManager helm (only helm and kube-controller-manager in managedFields)."
tags: [attribution, negative]
max_turns: 20
timeout_seconds: 600
allowed_tools: [Read, Glob, Grep, Skill]
---

I exported my cluster's state with `kubectl get -o yaml` into the `cluster` directory in the current working directory (namespaces, deployments, replicasets, pods, services, configmaps, events). Use whatever tools you have available. Has anyone changed the `payments-api` Deployment in the `payments` namespace by hand, outside Helm? Finish with one line `CHANGED_BY: <Helm if it made the most recent change, the command a person used if someone did, or UNKNOWN>`.
