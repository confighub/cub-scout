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

## As released in v2.16.0

With the full `answer` on `trace` and the lean one on `compare source-truth`
([run 43836709515](https://github.com/confighub/cub-scout/actions/runs/43836709515)):

| Question | Flux: cub-scout / raw | Argo CD: cub-scout / raw |
|---|---|---|
| Which source and revision delivered this workload? | 4,564 / 12,679 (36%) | 6,383 / 15,690 (41%) |
| Do ConfigHub, the controller and the workload agree? | 1,239 / 12,679 (10%) | 1,232 / 15,690 (8%) |
| What do the delivery controllers report, cluster-wide? | 3,787 / 16,549 (23%) | 2,911 / 17,671 (16%) |
| Who owns this resource? (one with no owner marker) | 1,576 / 8,432 (19%) | 1,576 / 8,432 (19%) |

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

## 24-case deterministic bytes-and-tokens benchmark (no model, no credits)

Measured on 2026-11-24 with `PYTHONDONTWRITEBYTECODE=1 python3 evals/scripts/measure_bytes_tokens.py --format md`
over the 24 frozen cases in `evals/benchmark-v1.json` (Package C / Measurement
rung 1). Zero model calls, zero credits, zero live cluster access.

**Method:**
- **Raw bytes:** exact UTF-8 byte sum of all evidence files materialized by
  each case's `scaffold.sh` (in a fresh temp dir) or stored in `<case>/fixtures/`.
- **Answer ceiling:** declared compact-JSON byte ceiling of the `answer` block
  for the case's primary cub-scout tool (`cmd/cub-scout/answer_size_ceilings_test.go`).
- **Token estimate:** `ceil(utf8_bytes / 4)`, matching `cmd/cub-scout/mcp_catalog_size_test.go`.
- **MCP catalog overhead (paid once per session, not in per-case rows):**
  15,218 bytes (~3,805 tokens) standalone all-tools; 9,390 bytes (~2,348 tokens)
  standalone `--tools core`; 31,224 bytes (~7,806 tokens) connected all-tools;
  12,337 bytes (~3,085 tokens) connected `--tools core`.

**Caveats:**
- This measures evidence size only. It does **not** prove an agent answers
  correctly or spends less money; that requires a paid paired run (not approved).
- On multi-file `scaffold.sh` cases (`INV-01`–`INV-03`, `ATR-01`–`ATR-04`), raw
  bytes count the full exported cluster snapshot; an agent that greps or runs
  targeted queries reads less than the full snapshot.
- The `answer` column counts the declared `answer` block ceiling, not the
  surrounding command-specific JSON fields.

| Case | Group | Raw files | Raw bytes | Raw ~tokens | Primary tool | Answer ceiling (bytes / ~tokens) | Answer % of raw |
|---|---|---:|---:|---:|---|---:|---:|
| INV-01 | inventory | 7 | 2,226,060 | 556,515 | `map` | 400 / 100 | 0.02% |
| INV-02 | inventory | 7 | 2,226,060 | 556,515 | `map` | 400 / 100 | 0.02% |
| INV-03 | inventory | 7 | 654,473 | 163,619 | `explain` | 900 / 225 | 0.14% |
| INV-04 | inventory | 4 | 10,671 | 2,668 | `map` | 400 / 100 | 3.75% |
| ATR-01 | attribution | 7 | 654,473 | 163,619 | `explain` | 900 / 225 | 0.14% |
| ATR-02 | attribution | 7 | 654,473 | 163,619 | `explain` | 900 / 225 | 0.14% |
| ATR-03 | attribution | 7 | 654,473 | 163,619 | `explain` | 900 / 225 | 0.14% |
| ATR-04 | attribution | 7 | 654,473 | 163,619 | `trace` | 900 / 225 | 0.14% |
| DEL-01 | delivery_identity | 1 | 3,456 | 864 | `trace` | 900 / 225 | 26.04% |
| DEL-02 | delivery_identity | 1 | 2,119 | 530 | `compare_source_truth` | 400 / 100 | 18.88% |
| DEL-03 | delivery_identity | 3 | 33,668 | 8,417 | `trace` | 900 / 225 | 2.67% |
| DEL-04 | delivery_identity | 5 | 17,515 | 4,379 | `release_check` | 1,600 / 400 | 9.14% |
| HLT-01 | health | 1 | 4,845 | 1,212 | `gitops_status` | 900 / 225 | 18.58% |
| HLT-02 | health | 16 | 116,356 | 29,089 | `gitops_status` | 900 / 225 | 0.77% |
| HLT-03 | health | 3 | 189,665 | 47,417 | `doctor` | 500 / 125 | 0.26% |
| HLT-04 | health | 3 | 42,587 | 10,647 | `gitops_status` | 900 / 225 | 2.11% |
| PRE-01 | prerequisites_graph | 15 | 60,017 | 15,005 | `doctor` | 500 / 125 | 0.83% |
| PRE-02 | prerequisites_graph | 7 | 28,759 | 7,190 | `doctor` | 500 / 125 | 1.74% |
| PRE-03 | prerequisites_graph | 7 | 41,298 | 10,325 | `trace` | 900 / 225 | 2.18% |
| PRE-04 | prerequisites_graph | 3 | 81,213 | 20,304 | `map` | 400 / 100 | 0.49% |
| RUL-01 | reuse_limits | 3 | 6,846 | 1,712 | `explain` | 900 / 225 | 13.15% |
| RUL-02 | reuse_limits | 2 | 17,263 | 4,316 | `explain` | 900 / 225 | 5.21% |
| RUL-03 | reuse_limits | 4 | 5,991 | 1,498 | `map` | 400 / 100 | 6.68% |
| RUL-04 | reuse_limits | 4 | 11,390 | 2,848 | `release_check` | 1,600 / 400 | 14.05% |
| **Total (24 cases)** | **6 groups** | — | **8,398,144** | **2,099,546** | — | **18,800 / 4,700** | **0.22%** |

Reproduce:

```bash
PYTHONDONTWRITEBYTECODE=1 python3 evals/scripts/measure_bytes_tokens.py --format md
PYTHONDONTWRITEBYTECODE=1 python3 evals/scripts/test_measure_bytes_tokens.py
```
