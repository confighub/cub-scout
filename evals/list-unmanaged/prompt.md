---
name: list-unmanaged
description: "Exactly the two unlabelled Deployments are unmanaged; the ConfigHub one is managed."
expected_outcome: "UNMANAGED: hotfix-worker, debug-nginx. The others carry kustomize.toolkit.fluxcd.io/* (checkout), argocd.argoproj.io/instance plus tracking-id (cart), app.kubernetes.io/managed-by=Helm (payments-api) and confighub.com/UnitSlug (inventory). cub-scout: map list owner=Native."
tags: [ownership, inventory]
max_turns: 20
timeout_seconds: 600
allowed_tools: [Read, Glob, Grep, Skill]
---

I exported my cluster's state with `kubectl get -o yaml --show-managed-fields` into the `cluster` directory in the current working directory (namespaces, deployments, replicasets, pods, services, configmaps, events). Use whatever tools you have available. Which Deployments in this cluster (outside kube-system, local-path-storage, flux-system and argocd) are not managed by any of Flux, Argo CD, Helm or ConfigHub? Finish with one line `UNMANAGED: <comma-separated Deployment names>`.
