---
name: flux-helm-not-plain-helm
description: "A Helm chart installed by Flux's helm-controller: Flux owns it, so a hand-run helm upgrade would be reverted."
expected_outcome: "OWNER: Flux (HelmRelease flux-system/billing); HELM_UPGRADE_SAFE: no. The Deployment carries app.kubernetes.io/managed-by=Helm and helm.toolkit.fluxcd.io/name=billing; helm-controller reconciles the release, so a manual helm upgrade is reverted at the next reconcile. cub-scout map: owner Flux, ownerDetails helmrelease billing in flux-system."
tags: [ownership]
max_turns: 20
timeout_seconds: 300
allowed_tools: [Read, Glob, Grep, Skill]
---

I exported my cluster's state with `kubectl get -o yaml` into the `cluster` directory in the current working directory (namespaces, deployments, replicasets, pods, services, configmaps, events). Use whatever tools you have available. Who manages the `billing` Deployment in the `billing` namespace? I want to fix it with `helm upgrade`; is that safe? Finish with two lines: `OWNER: <Flux|ArgoCD|Helm|ConfigHub|none>` and `HELM_UPGRADE_SAFE: <yes|no>`.
