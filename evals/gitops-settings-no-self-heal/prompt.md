---
name: gitops-settings-no-self-heal
description: "Which deployers would leave a manual change in place: Applications that sync automatically without self-heal, and suspended Flux objects."
expected_outcome: "NO_SELF_HEAL: argocd/etl-nightly, argocd/feature-store, argocd/fx-rates, argocd/ingress-nginx, argocd/settlement-worker, team-sandbox/playground. SUSPENDED: HelmRelease/team-h/redis, Kustomization/flux-system/legacy-migration, Kustomization/flux-system/monitoring. In the export: an Application is in the first list when spec.syncPolicy.automated is present, automated.enabled is not false, and automated.selfHeal is not true. Three of the six never mention selfHeal (fx-rates, feature-store, playground) and one has `automated: {}` (ingress-nginx), so searching for `selfHeal: false` finds only two, plus argocd/dns-migration, which must be left out because automated.enabled is false. legacy-billing, cluster-bootstrap and backfill-2025 have no automated block and do not sync automatically. cub-scout gitops_settings with setting [\"self-heal=off\"] returns the six, and with [\"suspend=on\"] the three; the default summary lists them under self-heal off and suspend on."
tags: [gitops-settings, inventory, argo, flux]
max_turns: 20
timeout_seconds: 600
allowed_tools: [Read, Glob, Grep, Skill]
---

I exported my cluster's GitOps objects with `kubectl get -o yaml --show-managed-fields` into the `cluster` directory in the current working directory (namespaces, Argo CD applications and appprojects, Flux gitrepositories, helmrepositories, kustomizations and helmreleases, and the configmaps in `argocd`). This is recorded evidence: any available cub-scout MCP responses are recordings from this scenario, not independent live confirmation. No GitOps controller is running in this cluster, so no object has a status; answer from what each object's spec declares. Use whatever tools you have available.

I want to know which deployers would leave a manual change to their workloads in place. List:

1. every Argo CD Application that syncs automatically but does not self-heal. An Application that does not sync automatically belongs in neither list.
2. every Flux Kustomization or HelmRelease that is suspended.

Finish with exactly these two lines, each list comma-separated and sorted alphabetically, using `none` for an empty list:

`NO_SELF_HEAL: <namespace/name>, ...`
`SUSPENDED: <Kind/namespace/name>, ...`
