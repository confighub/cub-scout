# Delivery tree: what is under a deployer, to any depth

`gitops tree` starts at a GitOps deployer and walks down: the deployers it
delivers, the deployers those deliver, and the resources each one reports.
This example is an Argo CD app-of-apps three levels deep, and what the command
prints for it. Issue: [#855](https://github.com/confighub/cub-scout/issues/855).

```bash
cub scout gitops tree app/delivery-tree
./cub-scout gitops tree app/delivery-tree
```

## The example

[`argocd/`](argocd/) is one root Application whose source is a chart of more
Applications. Every level reads the same commit: the revision is passed down
as a chart value.

| Application | What it is there to show |
|---|---|
| `delivery-tree` | The root. Auto-sync, self-heal and prune on, and an ignore rule for its child Applications' `/spec/syncPolicy` |
| `delivery-tree-team-a` | A middle level. Self-heal off |
| `delivery-tree-web` | A leaf with a Deployment, a Service and a ConfigMap |
| `delivery-tree-jobs` | A leaf that never prunes |
| `delivery-tree-team-b` | No automated sync, so the Application it would create does not exist |
| `delivery-tree-broken` | A path that does not exist, so Argo CD cannot compare it |
| `delivery-tree-elsewhere` | An Application in a namespace this Argo CD does not read |

To deploy it on a cluster with Argo CD, from a commit that has these files:

```bash
sed -e "s|REPO_URL|https://github.com/confighub/cub-scout.git|" -e "s|REVISION|<commit>|" \
  examples/delivery-tree/argocd/root-application.yaml | kubectl apply -f -
```

## What the command prints

This is [`expected/tree.txt`](expected/tree.txt): the command's output for the
Applications a real Argo CD v3.5.3 left behind
([the recording](../../test/fixtures/delivery-tree-argocd-v353-recorded/)). A
test holds every file under `expected/` to the command.

```text
DELIVERY TREE  Application/delivery-tree  (context kind-gitops-cluster)
1 root, 8 deployers, 8 resources reported
Each deployer's resources are what it reports about itself. They are not checked against the cluster.
  ! 1 reported but not found in the cluster
  ! 1 with no status: not reconciled
  ! 2 report nothing about what they applied

Application argocd/delivery-tree  ·  Synced / Healthy  ·  auto-sync on, self-heal on, prune on
├─ Application argocd/delivery-tree-broken  ·  Unknown / Healthy  ·  auto-sync on, self-heal off, prune off
│     ! parent ignores differences at /spec/syncPolicy
│     ! ComparisonError: Failed to load target state: [...] examples/delivery-tree/argocd/no-such-path: app path does not exist
│     ! reports nothing about what it applied: the Application has no status.resources
├─ Application argocd/delivery-tree-team-a  ·  Synced / Healthy  ·  auto-sync on, self-heal off, prune on
│     ! parent ignores differences at /spec/syncPolicy
│  ├─ Application argocd/delivery-tree-jobs  ·  Synced / Healthy  ·  auto-sync on, self-heal off, prune off
│  │  ├─ ConfigMap delivery-tree-jobs/nightly-schedule  ·  Synced
│  │  └─ ConfigMap delivery-tree-jobs/weekly-schedule  ·  Synced
│  └─ Application argocd/delivery-tree-web  ·  Synced / Healthy  ·  auto-sync on, self-heal on, prune on
│     ├─ ConfigMap delivery-tree-web/web-settings  ·  Synced
│     ├─ Deployment delivery-tree-web/web  ·  Synced
│     └─ Service delivery-tree-web/web  ·  Synced
├─ Application argocd/delivery-tree-team-b  ·  OutOfSync / Missing  ·  auto-sync off, self-heal n/a, prune n/a
│     ! parent ignores differences at /spec/syncPolicy
│  └─ Application argocd/delivery-tree-api
│        ! not found in the cluster
│        ! its parent reports it OutOfSync
├─ Application delivery-tree-elsewhere-apps/delivery-tree-elsewhere  ·  auto-sync on, self-heal off, prune off
│     ! not reconciled: the object has no status
│     ! parent ignores differences at /spec/syncPolicy
│     ! reports nothing about what it applied: the Application has no status.resources
├─ ConfigMap delivery-tree-platform/platform-settings  ·  Synced
├─ Namespace delivery-tree-elsewhere-apps  ·  Synced
└─ Namespace delivery-tree-platform  ·  Synced
```

(The `ComparisonError` line is shortened here; the file has all of it.)

## Reading it

- **Settings and state, per level.** `delivery-tree-web` self-heals and its
  parent `delivery-tree-team-a` does not. The flat `gitops settings` list has
  both rows and cannot show that one is under the other.
- **A parent's settings apply to the child object.** The root self-heals, so a
  change to a child Application made in the cluster is put back, except where
  the root ignores it. It ignores `/spec/syncPolicy`, and that is shown on each
  child the rule names.
- **A resource is what the deployer reports.** `delivery-tree-api` is listed
  under `delivery-tree-team-b`, which has never synced. It is the Application
  team-b would create. It is not in the cluster, and the tree says so.
- **Nothing reported is not nothing there.** `delivery-tree-broken` could not
  be compared and `delivery-tree-elsewhere` is not reconciled. Neither says
  what it applied, and neither is shown as empty.
- **No health on a resource.** Argo CD 3 keeps resource health in its own
  tree, not on the Application. A resource shows its sync status only.

## Other questions

| Question | Command | Output |
|---|---|---|
| Where are its Deployments, and under which Application? | `gitops tree app/delivery-tree --kind Deployment` | [`tree-deployments.txt`](expected/tree-deployments.txt) |
| What is out of sync under it? | `gitops tree app/delivery-tree --sync OutOfSync` | [`tree-out-of-sync.txt`](expected/tree-out-of-sync.txt) |
| Only the first level | `gitops tree app/delivery-tree --depth 1` | [`tree-depth-1.txt`](expected/tree-depth-1.txt) |
| Every root in the cluster | `gitops tree` | [`tree-every-root.txt`](expected/tree-every-root.txt) |
| For a ticket | `gitops tree app/delivery-tree --format md` | [`tree.md`](expected/tree.md) |
| For a program | `gitops tree app/delivery-tree --format json` | [`tree.json`](expected/tree.json) |
| Compact, for an agent | `gitops tree app/delivery-tree --format json --view summary` | [`tree-summary.json`](expected/tree-summary.json) |

## How this is checked

- The recording is from the GitOps E2E lane, which deploys this example on a
  real Argo CD at the commit under test.
- The same lane then runs `gitops tree` against that cluster and checks the
  tree: the children at each level, the Application that is not there, the two
  that report nothing, and that no resource has a health.
- Unit tests build the same tree from the recording with no cluster.

## Limits

- Argo CD Applications only. A Flux Kustomization or HelmRelease appears with
  its own state and is marked as not read for its children
  ([#856](https://github.com/confighub/cub-scout/issues/856)). Other tools:
  the table on [#855](https://github.com/confighub/cub-scout/issues/855#issuecomment-6095173023).
- Reported resources are not checked against the cluster, and the tree stops
  at them: it does not go on to ReplicaSets and Pods. `tree runtime` does.
- Not tried: an Application with very many resources, an ApplicationSet above
  its Applications, and a resource that needs pruning.
