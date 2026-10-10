# Delivery tree: Application/delivery-tree

Context: `kind-gitops-cluster`

1 root, 8 deployers, 8 resources reported.

> Each deployer's resources are what it reports about itself. They are not checked against the cluster.

- **1 reported but not found in the cluster**
- **1 with no status: not reconciled**
- **2 report nothing about what they applied**

- **Application argocd/delivery-tree** · Synced · Healthy · auto-sync on, self-heal on, prune on
  - **Application argocd/delivery-tree-broken** · Unknown · Healthy \(reported without a comparison\) · auto-sync on, self-heal off, prune off
    - _parent ignores differences at /spec/syncPolicy, when comparing only_
    - _ComparisonError: Failed to load target state: failed to generate manifest for source 1 of 1: rpc error: code = Unknown desc = examples/delivery-tree/argocd/no-such-path: app path does not exist_
    - _reports nothing about what it applied: the Application has no status.resources_
  - **Application argocd/delivery-tree-team-a** · Synced · Healthy · auto-sync on, self-heal off, prune on
    - _parent ignores differences at /spec/syncPolicy, when comparing only_
    - **Application argocd/delivery-tree-jobs** · Synced · Healthy · auto-sync on, self-heal off, prune off
      - ConfigMap delivery-tree-jobs/nightly-schedule  ·  Synced
      - ConfigMap delivery-tree-jobs/weekly-schedule  ·  Synced
    - **Application argocd/delivery-tree-web** · Synced · Healthy · auto-sync on, self-heal on, prune on
      - ConfigMap delivery-tree-web/web-settings  ·  Synced
      - Deployment delivery-tree-web/web  ·  Synced
      - Service delivery-tree-web/web  ·  Synced
  - **Application argocd/delivery-tree-team-b** · OutOfSync · Missing · auto-sync off, self-heal n/a, prune n/a
    - _parent ignores differences at /spec/syncPolicy, when comparing only_
    - **Application argocd/delivery-tree-api**
      - _not found in the cluster_
      - _its parent reports it OutOfSync_
  - **Application delivery-tree-elsewhere-apps/delivery-tree-elsewhere** · auto-sync on, self-heal off, prune off
    - _not reconciled: the object has no status_
    - _parent ignores differences at /spec/syncPolicy, when comparing only_
    - _reports nothing about what it applied: the Application has no status.resources_
  - ConfigMap delivery-tree-platform/platform-settings  ·  Synced
  - Namespace delivery-tree-elsewhere-apps  ·  Synced
  - Namespace delivery-tree-platform  ·  Synced

Note: No resource health is reported: Argo CD keeps it in its own tree, not on the Application. A resource with no health shown is not known to be healthy.

## Reads

| Controller | Kind | Status | Count |
|---|---|---|---|
| Argo CD | Application | read | 8 |
| Flux | Kustomization | read | 0 |
| Flux | HelmRelease | read | 0 |
