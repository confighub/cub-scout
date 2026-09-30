---
name: scale-unmanaged
description: "300 Deployments: exactly the 12 that no tool manages."
expected_outcome: "UNMANAGED: team-02/auth, team-03/auth, team-05/cron, team-05/auth, team-05/notify, team-11/api, team-11/notify, team-12/web, team-18/web, team-19/cron, team-25/search, team-30/cache. These twelve carry only the app label."
tags: [scale, ownership]
max_turns: 30
timeout_seconds: 900
allowed_tools: [Read, Glob, Grep, Skill]
---

I exported my cluster's state with `kubectl get -o yaml --show-managed-fields` into the `cluster` directory in the current working directory (namespaces, deployments, replicasets, pods, services, configmaps, events). Use whatever tools you have available. The workloads are the Deployments in the `team-*` namespaces. Which of these Deployments are managed by none of Flux, Argo CD, Helm or ConfigHub? Finish with one line `UNMANAGED: <comma-separated namespace/name>`.
