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

`verify-live-flux.py` creates one owned kind cluster with real Flux and has Flux
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
python3 examples/source-agreement/verify-live-flux.py
```

It needs `kind`, `kubectl` and the `flux` CLI, and network access for the Flux
images, the repositories and the chart. It writes a private kubeconfig, never
the shared one, and removes its cluster.

## Result

The [retained proof](live-flux-proof.json) passed on Kubernetes 1.35: 40 source
observations (2 workloads, 2 layouts, 10 surfaces), all naming the workload's
own source, plus 4 owner observations. The
[first attempt](live-flux-attempt-1.json) is retained: a workload rollout timed out
before any surface was asked, an environment failure.

## Argo CD

`verify-live-argo.py` does the same with real Argo CD (v3.5.3, from the pinned
and checksummed manifest the project's CI installs). Argo CD delivers the
guestbook example through an Application. The two layouts are that Application
as a root object, and the same Application carrying the tracking id of a parent
Application whose source is the fleet repository (app-of-apps).

It runs with **no `argocd` CLI on PATH**, only `kubectl`. That is deliberate:
the CLI is optional, and an agent sandbox or a CI job often has none.

```bash
python3 examples/source-agreement/verify-live-argo.py
```

Its [first run](live-argo-attempt-1.json) failed on the product. Without the
CLI, `trace` and every `--kube-context` path named the source, because they
read the Application through the Kubernetes API, while the default paths of
`explain`, `receipt verify` and `compare` could not: they used a tracer that
needs the `argocd` binary. `explain` said `source: unknown` where `trace`
named the repository, and passing `--kube-context` for the context that was
already current changed the answer. Those paths now fall back to the
Kubernetes API when the CLI is absent.

The [retained proof](live-argo-proof.json) passed on Kubernetes 1.35: 20
source observations (1 workload, 2 layouts, 10 surfaces), all naming the
guestbook repository, and owner ArgoCD from `map list`.

## Limits

- The parent layouts are made with labels and tracking ids on a parent that is
  suspended or never synced, not by a real `flux bootstrap` or a real parent
  sync.
- They check the source and the owner. They do not check that the surfaces
  agree on revision, drift, freshness or health.
- The Flux check has the `flux` CLI on PATH; Flux evidence still needs it. The
  Argo check has no `argocd` CLI; behaviour with the CLI is covered by a
  deterministic test, not this harness.
- Multi-source Applications, ApplicationSets, the plugin form, `watch`, `bot`
  and the TUI are not covered.
- These are conformance checks, not measurements: they say nothing about
  whether cub-scout saves an agent time or cost.
