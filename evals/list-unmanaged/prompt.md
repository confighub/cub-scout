---
name: list-unmanaged
description: "Exactly the two unlabelled Deployments are unmanaged; the ConfigHub one is managed."
expected_outcome: "UNMANAGED: hotfix-worker, debug-nginx. The other four carry kustomize.toolkit.fluxcd.io/* (checkout), argocd.argoproj.io/instance plus tracking-id (cart), app.kubernetes.io/managed-by=Helm (payments-api) and confighub.com/UnitSlug (inventory). cub-scout: map list owner=Native."
tags: [ownership, inventory]
max_turns: 20
timeout_seconds: 300
allowed_tools: [Read, Glob, Grep, Skill]
---

I exported my cluster's state with `kubectl get -o yaml` into the `cluster` directory you have read access to (namespaces, deployments, replicasets, pods, services, configmaps, events). Which Deployments in this cluster (outside kube-system and local-path-storage) are not managed by any GitOps tool or by ConfigHub? Finish with one line `UNMANAGED: <comma-separated Deployment names>`.
