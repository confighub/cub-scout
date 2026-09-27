---
name: scale-whats-failing-live
description: "Live only: 300 Deployments: exactly the 2 that are failing, and why. The cub-scout arm has the cluster and no export; run with --ablation none and compare with the export-only baseline of scale-whats-failing."
expected_outcome: "FAILING: team-10/cron, team-15/api. Both pods are ImagePullBackOff on images that do not exist; every other Deployment is healthy (most scaled to zero)."
tags: [scale-live, live-only, diagnosis]
max_turns: 30
timeout_seconds: 900
allowed_tools: [Read, Glob, Grep, Skill]
---

Use whatever tools you have available to look at my Kubernetes cluster. The workloads are the Deployments in the `team-*` namespaces. Which of these Deployments are failing, and why? Finish with one line `FAILING: <comma-separated namespace/name>`.
