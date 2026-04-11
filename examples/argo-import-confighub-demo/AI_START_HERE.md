# AI Start Here

Use this page when you want to drive `argo-import-confighub-demo` safely with
Codex, Claude, Cursor, or another AI assistant.

## What This Example Is For

This example is for the current GitHub + ArgoCD + AI/CLI + ConfigHub wedge.

It shows three different Argo-shaped import lenses on one cluster:

- `cub gitops import` for rendered dry/wet import into ConfigHub
- `cub-scout import argocd` for per-Application detail
- `cub-scout import` for broad ownership discovery, including Helm and Native

## Read-Only First

Start with preview commands only:

```bash
cd examples/argo-import-confighub-demo
./setup.sh --explain
./setup.sh --explain-json
```

These commands do not mutate ConfigHub and do not create a cluster.

## Recommended Path

Cluster plus human-demo walkthrough, while keeping the cluster up for follow-up checks:

```bash
./setup.sh
./verify.sh
```

With ConfigHub worker and rendered import path:

```bash
cub auth login
./setup.sh --with-worker
./verify.sh
```

With optional synthetic history for demo storytelling:

```bash
./setup.sh --with-worker --seed-history
./verify.sh
```

When you are done:

```bash
./cleanup.sh
```

## Important Boundaries

- `./setup.sh --explain` and `./setup.sh --explain-json` are read-only
- `./setup.sh` delegates to `./demo.sh --keep`
- `./setup.sh --with-worker` delegates to `./demo.sh --keep --live`
- `./verify.sh` is read-only and checks three evidence surfaces plus `cub-scout scan`
- `./cleanup.sh` deletes the local kind cluster and stops the detached discovery worker if present
- `./cleanup.sh` does not remove ConfigHub units or synthetic ChangeSets

## What To Verify

Cluster side:

```bash
kubectl --context kind-argo-import-demo get applications -n argocd
kubectl --context kind-argo-import-demo get all -A
```

ConfigHub side when `--with-worker` was used:

```bash
cub target list --space argo-import-demo
cub unit list --space argo-import-demo
```

cub-scout side:

```bash
../../cub-scout gitops status
../../cub-scout map list
../../cub-scout scan --state --json
```

Use the evidence like this:

- `kubectl` proves raw cluster facts
- `cub` proves imported state and reports connected-readiness status
- `cub-scout` proves live ownership and GitOps context facts

Important: import/render evidence is not the same thing as proving live workload
reconciliation. Compare all three surfaces when runtime truth matters.

## Current Boundary

This Argo Slice 2 path now includes `cub-scout scan` evidence in `./verify.sh`.

Important:

- scan/finding evidence is still separate from ConfigHub import/render proof
- the current scripted scan evidence is local to the Argo demo
- parity across Flux, the how-to docs, and the published doc is still a follow-on slice

## Related Files

- [README.md](./README.md)
- [prompts.md](./prompts.md)
- [contracts.md](./contracts.md)
- [setup.sh](./setup.sh)
- [verify.sh](./verify.sh)
- [cleanup.sh](./cleanup.sh)
