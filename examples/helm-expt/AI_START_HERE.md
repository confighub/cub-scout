# AI Start Here

Use this page to drive the `helm-expt` example safely with Claude, Codex, Cursor,
or another AI assistant.

## What this example is for

It wires cub-scout's `object-set-matches` install receipt end-to-end against a
real cluster, and then **shows where it falls short today** for the
`confighub/helm-expt` use case. It is a self-contained reproduction of helm-expt
finding **F3** (a workload whose required Secret is not in the shipped object set).

It is a demonstration of a scope gap, not a passing demo. `verify.sh` is expected
to report a "false green".

## Self-contained — does NOT touch helm-expt

This example ships its own fixture (`fixtures/release-objects.yaml`). It never
reads, writes, or depends on a `helm-expt` checkout. `setup.sh` creates its own
kind cluster named `cub-scout-helm-expt` and only `cleanup.sh` deletes it.

## Read-only first

```bash
cd examples/helm-expt
cat fixtures/release-objects.yaml   # read the fixture and its inline F3 comment
./setup.sh --help                   # read what setup will do before running it
```

## Recommended path

```bash
./setup.sh      # creates kind cluster, applies the object set (no Secret -> F3)
./verify.sh     # builds the object-set receipt, contrasts it with reality
./cleanup.sh    # deletes the kind cluster
```

To run against your current context instead of a new kind cluster:

```bash
./setup.sh --no-cluster
./verify.sh --no-cluster
./cleanup.sh --no-cluster
```

## What to expect

`verify.sh` prints a scorecard for the same install:

- `object-set-matches` → **PASS** (present + match) — a false green on its own
- `prerequisites-met` (#477) → **BLOCK** — the required Secret is absent (pre-flight)
- `workloads-converged` (#476) → **BLOCK** — the pod is in `CreateContainerConfigError` (runtime)
- receipt freshness/TTL (#478) → **ABSENT** — still open

The two BLOCKs catching the same F3 install from both ends is the point. The full
set maps to
[`docs/proposals/helm-expt-driven-gaps.md`](../../docs/proposals/helm-expt-driven-gaps.md).

## Important boundaries

- `verify.sh` is read-only against the cluster; it writes only into `./out/`.
- cub-scout never mutates the cluster.
- The receipt PASS is correct *for the predicate's deliberate scope* (presence +
  authored-field match). The example documents what that scope omits.

## Related files

- [README.md](./README.md) — the full integration narrative and runtime menu
- [contracts.md](./contracts.md) — what each artifact does and does not prove
- [prompts.md](./prompts.md) — copy-paste prompts for an AI assistant
- [setup.sh](./setup.sh) / [verify.sh](./verify.sh) / [cleanup.sh](./cleanup.sh)
- [fixtures/release-objects.yaml](./fixtures/release-objects.yaml)
