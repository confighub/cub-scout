---
name: why-payments-broken
description: "Broken workload: pods cannot pull a nonexistent image tag."
expected_outcome: "CAUSE: image pull failure. pods.yaml and events.yaml: both payments-api pods are ImagePullBackOff/ErrImagePull for registry.k8s.io/pause:0.0.0-does-not-exist (NotFound). cub-scout: explain currentChange verdict BLOCK, reason runtime_failed, podReasons ImagePullBackOff."
tags: [diagnosis]
max_turns: 20
timeout_seconds: 600
allowed_tools: [Read, Glob, Grep, Skill]
---

I exported my cluster's state with `kubectl get -o yaml --show-managed-fields` into the `cluster` directory in the current working directory (namespaces, deployments, replicasets, pods, services, configmaps, events). Use whatever tools you have available. `payments-api` in the `payments` namespace isn't serving. What is wrong with it? Finish with one line `CAUSE: <short cause>`.
