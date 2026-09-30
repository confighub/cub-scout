---
name: flux-installed-but-not-working
description: "Flux controller pods run, but no Flux objects exist: nothing reconciles checkout."
expected_outcome: "FLUX_RECONCILING: no. kustomize-controller and source-controller pods are Running in flux-system, but the cluster has no Flux Kustomization, GitRepository or HelmRelease objects, so nothing reconciles checkout (its kustomize.toolkit.fluxcd.io labels name a Kustomization that does not exist). cub-scout gitops_status: Flux controllerCoverage not_found, found 0. The export alone does not show Flux objects either way."
tags: [gitops-health, negative, pitfall]
max_turns: 20
timeout_seconds: 600
allowed_tools: [Read, Glob, Grep, Skill]
---

I exported my cluster's state with `kubectl get -o yaml --show-managed-fields` into the `cluster` directory in the current working directory (namespaces, deployments, replicasets, pods, services, configmaps, events). Use whatever tools you have available. Is Flux working in this cluster? Is it actually reconciling the `checkout` Deployment in `shop`? Finish with one line `FLUX_RECONCILING: <yes|no|unknown>`.
