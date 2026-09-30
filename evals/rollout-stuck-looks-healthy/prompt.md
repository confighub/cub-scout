---
name: rollout-stuck-looks-healthy
description: "A Flux rollout of orders stalled on an image pull while the old pods keep it Available."
expected_outcome: "ROLLED_OUT: no. The Deployment is Available=True (the old ReplicaSet's 2 pods serve) but Progressing=False with ProgressDeadlineExceeded; the new ReplicaSet's pod is ImagePullBackOff on registry.k8s.io/pause:9.9.9-orders. cub-scout explain: currentChange verdict BLOCK, reason runtime_failed, podReasons ImagePullBackOff."
tags: [rollout, pitfall]
max_turns: 20
timeout_seconds: 600
allowed_tools: [Read, Glob, Grep, Skill]
---

I exported my cluster's state with `kubectl get -o yaml --show-managed-fields` into the `cluster` directory in the current working directory (namespaces, deployments, replicasets, pods, services, configmaps, events). Use whatever tools you have available. We just deployed a new version of the `orders` Deployment in the `shop` namespace. Did the new version go out? Finish with one line `ROLLED_OUT: <yes|no>`.
