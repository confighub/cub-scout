# gitops-settings-fleet-live

The live-only half of [gitops-settings-fleet](../gitops-settings-fleet/): the
same 300 deployers and the same question, but the agent has **no export**, only
the plugin's skills and a live `cub-scout mcp serve` against the recording
cluster. It is how an agent normally meets cub-scout.

Run it with `--ablation none` (there is no meaningful no-plugin arm: without the
plugin the agent has nothing to read) and compare it with the **without** arm of
`gitops-settings-fleet`, which has the export and no cub-scout. The two arms
differ in what evidence they hold, so this compares two ways of working, not
one tool added to a fixed setup.

Result (2026-10-09, three runs): both lists correct 3/3, 4 turns, about $0.21
and 19 seconds per run, two `gitops_settings` calls each. The export-only
baseline it is compared with was 3/3 at a median $0.41 and 15 to 19 turns. See
the [report](../reports/2026-10-09-gitops-settings.md) for why that is not a
general saving.

Graders and guard mocks are copies of the fleet case's and are rewritten by
`../gitops-settings-fleet/record.py up`. The expected answer is
`../gitops-settings-fleet/expected.json`.

```bash
python3 evals/gitops-settings-fleet/record.py up
PATH="$(KUBECONFIG=<kubeconfig> evals/scripts/live-path.sh <context>)" \
  claude plugin eval . --case gitops-settings-fleet-live --runs 3 --mocks off --ablation none \
  --allow-tools "mcp__plugin_cub-scout_cub-scout__*" --max-cost-usd 6 \
  --json evals/results/gitops-settings-fleet-live.json
python3 evals/gitops-settings-fleet/record.py down
```
