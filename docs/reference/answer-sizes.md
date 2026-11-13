# How much an agent reads: cub-scout against raw kubectl

Measured on 2026-11-13 in the `live-status` CI lane
([run 43491874238](https://github.com/confighub/cub-scout/actions/runs/43491874238)),
on kind clusters made by `cub cluster up` with one delivered workload. The
lane writes `size-comparison.json` on every run; these are that file's
numbers. Package C of the
[agent plan](../roadmap-3.0-execution.md#agent-plan--2026-11-13).

This is a measurement of bytes. It is not a measurement of whether an agent
answers correctly, or for less money: no model was run.

## Result

Bytes of JSON on stdout. "Raw" is `kubectl get ... -o json` for the objects
that hold the same facts.

| Question | cub-scout command | Flux: cub-scout / raw | Argo CD: cub-scout / raw |
|---|---|---|---|
| Which source and revision delivered this workload, and is the chain ready? | `trace deploy/argobot --format json` | 2,734 / 12,679 (22%) | 4,767 / 15,690 (30%) |
| Do ConfigHub, the controller and the workload agree? | `compare source-truth deploy/argobot --format json` | 909 / 12,679 (7%) | 902 / 15,690 (6%) |
| What do the delivery controllers report, cluster-wide? | `gitops status --format json` | 3,787 / 16,549 (23%) | 2,911 / 17,904 (16%) |
| Who owns this resource? (one with no owner marker) | `trace deploy/coredns --format json` | 847 / 8,432 (10%) | 847 / 8,432 (10%) |

## After `trace` gained the `answer` block

`trace` now carries the [answer envelope](../proposals/answer-envelope.md),
beside the fields it had. That made its answers larger
([run 43520381877](https://github.com/confighub/cub-scout/actions/runs/43520381877)):

| `trace` of | Flux: before, after / raw | Argo CD: before, after / raw |
|---|---|---|
| the delivered workload | 2,734, then 4,658 / 12,678 (22%, then 37%) | 4,767, then 6,477 / 14,088 (30%, then 46%) |
| a resource with no owner marker | 847, then 1,670 / 8,432 (10%, then 20%) | 847, then 1,670 / 8,432 (10%, then 20%) |

The block says things the answer did not say before (a state to decide on,
whose word each claim rests on, what was not read, what the reads cost), and
it repeats some that it did. Until 3.0 removes the fields it supersedes, a
trace costs an agent about 1.9 KB more to read. The other three commands in
the table above are unchanged.

The same run is the first with a measured cost: the Flux trace made 12
Kubernetes requests and read 41,465 bytes to produce its 4,658-byte answer;
the Argo CD trace made 9 and read 41,817.

## What the numbers do and do not show

- **They favour raw kubectl in one way.** The raw side counts only the
  objects that matter, as if the agent already knew which to fetch. Finding
  them (which Kustomization delivers this Deployment?) costs further reads
  that are not counted. For the second question the ConfigHub unit read is
  not counted either.
- **They favour cub-scout in another.** The raw side is the default
  `-o json`. An agent that knows exactly which fields it needs can ask for
  them with `-o jsonpath` and read far less. That takes knowing the answer's
  shape in advance.
- **The catalog is not in them.** An agent pays for cub-scout's tool catalog
  once a session: 15,120 bytes for the standalone tools
  (`cmd/cub-scout/mcp_catalog_size_test.go`). On these numbers the first
  question does not repay that; the second usually does. An agent that asks
  one question a session is better served by `kubectl`.
- **The cluster is tiny.** One delivered workload, two or three delivery
  objects. `gitops status` grows with the cluster and so does the raw side;
  their ratio at scale is not measured here.
- **With ConfigHub evidence, `gitops status` is much larger.** The same
  command with `--with-confighub --confighub-space '*'` was 13,545 bytes on
  the Flux cluster in an earlier run. That form is not in the table.

## Reproducing it

```bash
gh workflow run CI --ref main -f level=live-status
```

Each `live-status-proof-<controller>` artifact holds `size-comparison.json`.
The numbers are recorded and nothing is asserted on them: a question where
cub-scout is the larger is a finding to read, not a failure to hide.
