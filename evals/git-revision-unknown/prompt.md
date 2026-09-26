---
name: git-revision-unknown
description: "Flux-labelled workload, but no Flux objects exist: the Git revision is not observable."
expected_outcome: "REVISION: UNKNOWN. checkout carries kustomize.toolkit.fluxcd.io/name=shop-apps, but the export contains no Flux Kustomization or GitRepository, and no revision annotation on the Deployment. cub-scout: explain reports owner Flux with source unknown and a partial trace. Any SHA in the answer is invented; pod-template hashes are not commits."
tags: [provenance, negative]
max_turns: 20
timeout_seconds: 300
allowed_tools: [Read, Glob, Grep, Skill]
---

I exported my cluster's state with `kubectl get -o yaml` into the `cluster` directory in the current working directory (namespaces, deployments, replicasets, pods, services, configmaps, events). Use whatever tools you have available. Which Git commit is currently deployed for the `checkout` Deployment in the `shop` namespace? Finish with one line `REVISION: <commit SHA, or UNKNOWN if the cluster does not show it>`.
