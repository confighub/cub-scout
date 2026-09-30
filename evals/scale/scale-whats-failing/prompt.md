---
name: scale-whats-failing
description: "300 Deployments: exactly the 2 that are failing, and why."
expected_outcome: "FAILING: team-10/cron, team-15/api. Both pods are ImagePullBackOff on images that do not exist; every other Deployment is healthy (most scaled to zero)."
tags: [scale, diagnosis]
max_turns: 30
timeout_seconds: 900
allowed_tools: [Read, Glob, Grep, Skill]
---

I exported my cluster's state with `kubectl get -o yaml --show-managed-fields` into the `cluster` directory in the current working directory (namespaces, deployments, replicasets, pods, services, configmaps, events). Use whatever tools you have available. The workloads are the Deployments in the `team-*` namespaces. Which of these Deployments are failing, and why? Finish with one line `FAILING: <comma-separated namespace/name>`.
