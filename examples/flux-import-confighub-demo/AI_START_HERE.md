# AI Start Here

Use this page when you want to drive `flux-import-confighub-demo` safely with
Codex, Claude, Cursor, or another AI assistant.

## What This Example Is For

This example is for the current GitHub + Flux + AI/CLI + ConfigHub wedge.

It shows two complementary Flux paths on one cluster:

- `cub gitops import` for rendered dry/wet import into ConfigHub
- `cub-scout` discovery for broad ownership, D2 pattern detection, and trace

## Read-Only First

Start with preview commands only:

```bash
cd examples/flux-import-confighub-demo
./setup.sh --explain
./setup.sh --explain-json
```

These commands do not mutate ConfigHub and do not create a cluster.

## Recommended Path

Cluster plus walkthrough, while keeping the cluster up for follow-up checks:

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
flux --context kind-flux-import-demo get all -A
kubectl --context kind-flux-import-demo get gitrepositories,kustomizations,helmreleases -A
```

ConfigHub side when `--with-worker` was used:

```bash
cub target list --space flux-import-demo
cub unit list --space flux-import-demo
```

cub-scout side:

```bash
../../cub-scout gitops status
../../cub-scout tree ownership
../../cub-scout scan --state --json
```

Use the evidence like this:

- `flux` and `kubectl` prove raw cluster/controller facts
- `cub` proves imported state and reports connected-readiness status
- `cub-scout` proves live ownership and GitOps context facts

Important: import/render evidence is not the same thing as proving live workload
reconciliation. Compare all three surfaces when runtime truth matters.

## Current Boundary

This Flux Slice 2 path includes `cub-scout scan` evidence in `./verify.sh`.

Important:

- scan/finding evidence is still separate from ConfigHub import/render proof
- `./verify.sh` reports scan evidence; current runs may show findings or a
  no-findings contract depending on fixture and controller state
- live kept-alive Flux verification should still be rerun after any fixture drift

## Related Files

- [README.md](./README.md)
- [prompts.md](./prompts.md)
- [contracts.md](./contracts.md)
- [setup.sh](./setup.sh)
- [verify.sh](./verify.sh)
- [cleanup.sh](./cleanup.sh)
