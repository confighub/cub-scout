---
name: scale-unmanaged-live
description: "Live only: 300 Deployments: exactly the 12 that no tool manages. The cub-scout arm has the cluster and no export; run with --ablation none and compare with the export-only baseline of scale-unmanaged."
expected_outcome: "UNMANAGED: team-02/auth, team-03/auth, team-05/cron, team-05/auth, team-05/notify, team-11/api, team-11/notify, team-12/web, team-18/web, team-19/cron, team-25/search, team-30/cache. These twelve carry only the app label."
tags: [scale-live, live-only, ownership]
max_turns: 30
timeout_seconds: 900
allowed_tools: [Read, Glob, Grep, Skill]
---

Use whatever tools you have available to look at my Kubernetes cluster. The workloads are the Deployments in the `team-*` namespaces. Which of these Deployments are managed by none of Flux, Argo CD, Helm or ConfigHub? Finish with one line `UNMANAGED: <comma-separated namespace/name>`.
