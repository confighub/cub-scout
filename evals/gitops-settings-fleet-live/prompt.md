---
name: gitops-settings-fleet-live
description: "Live only: 300 deployers, cub-scout and no export. Run with --ablation none and compare with the export-only baseline of gitops-settings-fleet."
expected_outcome: "The two lists in ../gitops-settings-fleet/expected.json. cub-scout gitops_settings with setting [\"self-heal=off\"] returns the first and with [\"suspend=on\"] the second; the default summary lists both under self-heal off and suspend on."
tags: [gitops-settings-fleet-live, live-only, inventory, argo, flux]
max_turns: 30
timeout_seconds: 900
allowed_tools: [Read, Glob, Grep, Skill]
---

Use whatever tools you have available to look at my Kubernetes cluster's GitOps objects: 240 Argo CD Applications and 60 Flux Kustomizations and HelmReleases. No GitOps controller is running in this cluster, so no object has a status; answer from what each object's spec declares.

I want to know which deployers would leave a manual change to their workloads in place. List:

1. every Argo CD Application that syncs automatically but does not self-heal. An Application that does not sync automatically belongs in neither list.
2. every Flux Kustomization or HelmRelease that is suspended.

Finish with exactly these two lines, each list comma-separated and sorted alphabetically, using `none` for an empty list:

`NO_SELF_HEAL: <namespace/name>, ...`
`SUSPENDED: <Kind/namespace/name>, ...`
