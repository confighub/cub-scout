---
name: scale-ownership-counts
description: "300 Deployments: how many does each owner manage?"
expected_outcome: "flux=120 argocd=90 helm=45 confighub=33 unmanaged=12. From labels: kustomize.toolkit.fluxcd.io/* (Flux), argocd tracking-id and instance label (Argo CD), app.kubernetes.io/managed-by=Helm (Helm), confighub.com/UnitSlug (ConfigHub), none (unmanaged)."
tags: [scale]
max_turns: 30
timeout_seconds: 900
allowed_tools: [Read, Glob, Grep, Skill]
---

I exported my cluster's state with `kubectl get -o yaml` into the `cluster` directory in the current working directory (namespaces, deployments, replicasets, pods, services, configmaps, events). Use whatever tools you have available. The workloads are the Deployments in the `team-*` namespaces. How many of these Deployments does each of Flux, Argo CD, Helm and ConfigHub manage, and how many are managed by none of them? Finish with one line exactly in this form: `COUNTS: flux=N argocd=N helm=N confighub=N unmanaged=N`.
