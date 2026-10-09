---
name: gitops-settings-fleet
description: "300 deployers: the Applications that sync automatically without self-heal, and the suspended Flux objects."
expected_outcome: "The two lists in this case's expected.json (noSelfHeal and suspended), which the generator wrote from its own table before the cluster existed. In the export: an Application is in the first list when spec.syncPolicy.automated is present, automated.enabled is not false, and automated.selfHeal is not true; many of them never mention selfHeal, and Applications with enabled: false or no automated block belong in neither list. cub-scout gitops_settings with setting [\"self-heal=off\"] returns the first list and with [\"suspend=on\"] the second."
tags: [gitops-settings-fleet, inventory, argo, flux, live]
max_turns: 30
timeout_seconds: 900
allowed_tools: [Read, Glob, Grep, Skill]
---

I exported my cluster's GitOps objects with `kubectl get -o yaml --show-managed-fields` into the `cluster` directory in the current working directory (namespaces, Argo CD applications and appprojects, Flux gitrepositories, helmrepositories, kustomizations and helmreleases, and the configmaps in `argocd`). There are 240 Applications and 60 Flux objects. No GitOps controller is running in this cluster, so no object has a status; answer from what each object's spec declares. Use whatever tools you have available.

I want to know which deployers would leave a manual change to their workloads in place. List:

1. every Argo CD Application that syncs automatically but does not self-heal. An Application that does not sync automatically belongs in neither list.
2. every Flux Kustomization or HelmRelease that is suspended.

Finish with exactly these two lines, each list comma-separated and sorted alphabetically, using `none` for an empty list:

`NO_SELF_HEAL: <namespace/name>, ...`
`SUSPENDED: <Kind/namespace/name>, ...`
