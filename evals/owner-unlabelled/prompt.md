---
name: owner-unlabelled
description: "Native workload: the owner is nobody; no GitOps tool may be claimed."
expected_outcome: "OWNER: none. deployments.yaml: default/hotfix-worker has only the label app=hotfix-worker, no Flux, Argo, Helm or ConfigHub labels or annotations and no ownerReferences. cub-scout: map list and explain report Native, and explain adds mutationManager kubectl-client-side-apply (from managedFields, which the plain export omits)."
tags: [ownership, negative]
max_turns: 20
timeout_seconds: 600
allowed_tools: [Read, Glob, Grep, Skill]
---

I exported my cluster's state with `kubectl get -o yaml` into the `cluster` directory in the current working directory (namespaces, deployments, replicasets, pods, services, configmaps, events). Use whatever tools you have available. Who manages the `hotfix-worker` Deployment in the `default` namespace, and how do you know? Finish with one line `OWNER: <Flux|ArgoCD|Helm|ConfigHub|none>`.
