# gitops-settings-fleet

The same question as
[gitops-settings-no-self-heal](../gitops-settings-no-self-heal/), on a fleet:
240 Argo CD Applications in 12 projects and 60 Flux Kustomizations and
HelmReleases in 8 namespaces. Which Applications sync automatically without
self-heal, and which Flux objects are suspended?

The export of those objects is about 338 KB. The `gitops_settings` default
answer on the same cluster is about 37 KB, and the answer to
`setting ["suspend=on"]` about 5 KB.

## How the answer is known

`record.py` generates `scenario.yaml` from a fixed seed and writes
`expected.json` (27 Applications, 5 Flux objects) from its own table before the
cluster exists. At recording time it then asserts that the real MCP tool, asked
for `self-heal=off` and `suspend=on`, returns exactly those lists; a separate
script over the raw export agrees. The graders require every expected entry and
exactly that many entries.

## What each arm gets

Both arms get the export in `./cluster/`. The cub-scout arm also gets the
plugin's skills and a **live** `cub-scout mcp serve` against the recording
cluster, as the suite's other scale cases do, because a recording cannot answer
every combination of the tool's filters. The mocks in this directory are guards
that fail unless the case is run with `--mocks off`.

## Limits, stated before any result

- **No controller is installed.** Real Argo CD v3.5.3 and Flux CRDs, generated
  objects, no reconciliation and no status. The case is about declared
  settings.
- **Generated, not observed.** The names and the mix of policies come from a
  seed, not from anyone's fleet.
- **The arms differ in evidence access**, as in the other scale cases: one has
  the export, the other the export and the live tool.
- **Live, so not runnable in CI** and not part of `benchmark-v1`, whose inputs
  are frozen and unchanged by this case.
- **One question on one fleet.** A result here is not a general claim about
  agent cost or time.

## Running

```bash
go build ./cmd/cub-scout
python3 evals/gitops-settings-fleet/record.py up     # prints the context and kubeconfig
PATH="$(KUBECONFIG=<kubeconfig> evals/scripts/live-path.sh <context>)" \
  claude plugin eval . --scaffold --case gitops-settings-fleet --runs 3 --mocks off \
  --allow-tools "mcp__plugin_cub-scout_cub-scout__*" --max-cost-usd 12 \
  --json evals/results/gitops-settings-fleet.json
python3 evals/gitops-settings-fleet/record.py down
```

`up` creates an owned kind cluster with a private kubeconfig and leaves it
running; `down` deletes it. The shared kubeconfig is never written.
