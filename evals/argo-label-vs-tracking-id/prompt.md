---
name: argo-label-vs-tracking-id
description: "Argo CD annotation tracking: the tracking-id names payments; a copied instance label names storefront."
expected_outcome: "APPLICATION: payments. ledger's argocd.argoproj.io/tracking-id is payments:apps/Deployment:shop/ledger, and argocd-cm sets application.resourceTrackingMethod: annotation (Argo CD's default), so the tracking-id decides; the argocd.argoproj.io/instance: storefront label is stale."
tags: [ownership, pitfall]
max_turns: 20
timeout_seconds: 600
allowed_tools: [Read, Glob, Grep, Skill]
---

I exported my cluster's state with `kubectl get -o yaml --show-managed-fields` into the `cluster` directory in the current working directory (namespaces, deployments, replicasets, pods, services, configmaps, events). Use whatever tools you have available. Which Argo CD Application owns the `ledger` Deployment in the `shop` namespace? Finish with one line `APPLICATION: <name, or UNKNOWN>`.
