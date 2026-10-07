# Every surface names the same source

A workload's source is one fact. `trace`, `explain`, `receipt verify`,
`compare` and the MCP tools each state it, and they must state the same one.

This example exists because they once did not. For a workload delivered by
Flux, `receipt verify` and the session-bound `compare` asked `flux trace` about
the workload's owning Kustomization instead of the workload. Under a parent
Kustomization, which is how `flux bootstrap` lays a cluster out, that returns
the parent's "fleet" repository. A receipt for the Deployment then passed
`applied-matches-spec` against a repository the Deployment did not come from,
while `trace` and `explain` named the right one. Nothing compared them.

## What the check does

`verify-live.py` creates one owned kind cluster with real Flux and has Flux
deliver podinfo twice:

| Workload | Delivered by | Its source |
|---|---|---|
| `team-a/podinfo` | Kustomization | Git repository `https://github.com/stefanprodan/podinfo` |
| `team-b/podinfo-helm` | HelmRelease | chart repository `https://stefanprodan.github.io/podinfo` |

It runs in two layouts: the owners as root objects, and the owners labelled as
managed by a parent Kustomization whose own source is a different "fleet"
repository.

For each workload and layout it asks ten surfaces where the workload came from:

- `trace`, `explain`, `receipt verify` and `compare`, each with and without
  `--kube-context`;
- the MCP `trace` and `explain` tools, over stdio.

Each must name the workload's own source and only that. None may name the
fleet repository. `map list` must report the owner as Flux.

```bash
python3 examples/flux-source-agreement/verify-live.py
```

It needs `kind`, `kubectl` and the `flux` CLI, and network access for the Flux
images, the repositories and the chart. It writes a private kubeconfig, never
the shared one, and removes its cluster.

## Result

The [retained proof](live-proof.json) passed on Kubernetes 1.35: 40 source
observations (2 workloads, 2 layouts, 10 surfaces), all naming the workload's
own source, plus 4 owner observations. The
[first attempt](live-attempt-1.json) is retained: a workload rollout timed out
before any surface was asked, an environment failure.

## Limits

- The parent layout is made by labelling the owners as managed by a suspended
  parent Kustomization, not by a real `flux bootstrap`.
- It checks the source and the owner. It does not check that the surfaces
  agree on revision, drift, freshness or health.
- Argo CD, the plugin form, `watch`, `bot` and the TUI are not covered.
- It is a conformance check, not a measurement: it says nothing about whether
  cub-scout saves an agent time or cost.
