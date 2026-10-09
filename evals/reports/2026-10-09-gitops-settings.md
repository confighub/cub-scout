# gitops_settings: eval runs — 2026-10-09

One question, asked three ways: which Argo CD Applications sync automatically
without self-heal, and which Flux Kustomizations and HelmReleases are suspended?
Three runs per arm each. Total reported spend **$7.74**: $7.11 for the runs
below and a wiring check, and $0.63 to rerun the live-only case on the final
code.

**Summary.** When the agent already has the raw export on disk, `gitops_settings`
adds nothing: it was never called, and on the small case the cub-scout arm cost
more. When the agent has cub-scout and no export, it answered correctly in four
turns for about $0.21, against a median $0.41 (mean $0.84) for an agent reading
the export. Three runs per arm on generated scenarios do not establish a
general saving.

## Results

| Case | Arm | Evidence the arm had | Both lists correct | $/run (mean; each) | Turns | Seconds | Called `gitops_settings` |
|---|---|---|---:|---|---:|---:|---|
| `gitops-settings-no-self-heal` (30 deployers, 42 KB export) | with | export + recorded MCP | 3/3 | $0.32 (0.32, 0.33, 0.32) | 7, 7, 7 | 21, 20, 23 | never |
| | without | export | 3/3 | $0.24 (0.25, 0.23, 0.23) | 7, 6, 6 | 21, 20, 18 | n/a |
| `gitops-settings-fleet` (300 deployers, 338 KB export) | with | export + live MCP | 2/3 | $0.57 (0.54, 0.55, 0.63) | 13, 19, 16 | 57, 79, 88 | never |
| | without | export | 3/3 | $0.84 (1.73, 0.41, 0.37); median $0.41 | 17, 19, 15 | 312, 70, 59 | n/a |
| `gitops-settings-fleet-live` (same 300 deployers), final code | with | live MCP, no export | 3/3 | $0.21 (0.21, 0.21, 0.21) | 4, 4, 4 | 19, 19, 21 | twice per run |
| the same, first run, before the review fixes | with | live MCP, no export | 3/3 | $0.21 (0.21, 0.21, 0.22) | 4, 4, 4 | 19, 18, 21 | twice per run |

"Both lists correct" counts the two answer graders only. The harness score also
counts indicator graders under `--ablation none`, so it reports 0.75 for the
live-only runs although both answers passed in all of them.

## What the transcripts show

- **With the export present, the tool is not used.** In all six cub-scout-arm
  runs of the two paired cases the agent read or grepped the export and never
  called `gitops_settings`, although it was offered (recorded in the small
  case, live and connected in the fleet case). No skill was loaded in any run.
- **On the small case cub-scout cost more and changed nothing.** The same
  answers in about the same turns (7 against 6 to 7), for about $0.09 more per
  run. The cub-scout arm's context was larger: 33 to 34 thousand
  cache-creation tokens per run against 23 to 25 thousand, and about 81
  thousand cache-read tokens against 52 thousand. What in the plugin accounts
  for that was not measured.
- **The one wrong answer was a case the tool distinguishes, and that run did
  not ask it.** One cub-scout-arm fleet run added six Applications that have
  `selfHeal: false` under `automated.enabled: false`; those do not sync
  automatically. Asked for `self-heal=off`, the tool leaves them out (the
  recorder asserts this against the expected list), and unfiltered it reports
  their `self-heal` as `n/a`. The run made no tool call, so nothing measured
  here shows the tool would have prevented the error.
- **One baseline fleet run was an outlier.** It ran 65 greps and two sub-agents
  and cost $1.73; the other two cost $0.37 and $0.41. With three runs the mean
  is dominated by it, so the median is given too.
- **Live-only, the agent went straight to the tool.** Each run made two calls,
  `setting ["auto-sync=on", "self-heal=off"]` and `setting ["suspend=true"]`.
  On the final code the answers were 6.8 KB and 2.0 KB of text (3.0 KB when
  the agent asked for the `deployers` view).

## What this does and does not support

Supported, on these scenarios:

- For this question, an agent with cub-scout and no export was correct in fewer
  turns and at lower reported cost than an agent reading the export of the same
  cluster: 4 turns against 15 to 19, and $0.21 against a median $0.41 and a
  mean $0.84.
- `gitops_settings` gave no benefit to an agent that already held the raw
  export, and the cub-scout arm cost more on the small case.

Not supported:

- **A general cost or time saving.** Three runs per arm, one question, generated
  objects, no controllers, one model.
- **A like-for-like comparison in the live-only case.** The arms hold different
  evidence. The baseline had a prepared export; it was not an agent running
  `kubectl` against the cluster, which the harness did not offer. An agent with
  `kubectl` and `jq` might do better or worse than grep over files; that was not
  measured.
- **Any claim about accuracy.** The baseline was 6/6 on the paired cases; the
  cub-scout arm 5/6.

## Follow-ups this suggests

- The tool was used when it was the only way to see the cluster and ignored
  when files were present. Whether the plugin's skills should say when to
  prefer it over reading an export is an open question; forcing it was not
  tried.
- The cub-scout arm carried a larger context on every run of the small case,
  whether or not a tool was used.

## How it was run

Claude Code 2.1.285, primary model `claude-opus-5-5`, fast mode off, model not
pinned by flag. `--no-publish --trust-plugin --keep-temp`, concurrency 1.

| Run | Source | Command | Cost |
|---|---|---|---:|
| Wiring check | `086b292b` | `--scaffold --case gitops-settings-no-self-heal --runs 1` | $0.5787 |
| Small | `bcb10022` | `--scaffold --case gitops-settings-no-self-heal --runs 3 --max-cost-usd 6` | $1.6733 |
| Fleet | `dcc8278f` | `--scaffold --case gitops-settings-fleet --runs 3 --mocks off --allow-tools "mcp__plugin_cub-scout_cub-scout__*" --max-cost-usd 12`, PATH from `evals/scripts/live-path.sh` | $4.2231 |
| Fleet, live only | `caf79399` | `--case gitops-settings-fleet-live --runs 3 --mocks off --ablation none --allow-tools "mcp__plugin_cub-scout_cub-scout__*" --max-cost-usd 6`, same PATH | $0.6343 |
| Fleet, live only, final code | `2e2b055e` | the same | $0.6332 |

**What changed between the runs and the final code.** An independent review
after the first runs led to these changes: the summary view separates
`policies` from `options`; the MCP tool refuses a misspelt, wrong-typed or
empty filter where it used to drop it; the tool description says "with no
arguments" before "one call covers every"; and the graders now require the
exact set. The live-only case, the only one in which the tool was called, was
rerun on the final code with the same result. The two paired cases were not
rerun: the tool was never called in them, though its description, which the
agent sees, changed by two sentences. Every saved run was regraded offline
against the final graders and no outcome changed.

**The wiring check** ran on an earlier scenario that had one Application outside
the Argo CD namespace. Both arms left it out, reasoning that this Argo CD was
not configured to manage Applications there, and both therefore failed the
grader. The object was moved and the limit recorded: `gitops_settings` lists an
Application by its spec and cannot tell whether an Argo CD instance manages it.
The wiring check is not counted in the results above.

**Skills.** The paired runs were made with five skill files edited to name the
tool. No skill was loaded in any run, and the edits did not touch any skill's
name or description, which is the part an agent sees without loading one. Those
edits were then reverted: `benchmark-v1` pins the exact contents of the skills
tree, and changing it is not part of this work. The skills therefore do not
mention `gitops_settings`.

**The small case's prompt names cub-scout** ("any available cub-scout MCP
responses are recordings from this scenario"), as the main suite's prompts do.
It does not tell the agent to use it, and it is the same text in both arms.

Costs are the harness's reported list-price figures, not account credits. Each
run's `costUsd` already includes mock and judge spend. No mock was called in the
small case (the tool was never used), and the fleet cases ran with mocks off, so
no mock model spend is in these numbers. Result files are under
`evals/results/` and are not committed; SHA-256 prefixes:
`d635ac6944e6174e` (small), `79587707f1f64b09` (fleet), `da6d8ac952c0bf72`
(fleet, live only), `54c01628e95859bd` (fleet, live only, final code).

A second agent recomputed every figure in the results table from the result
files and transcripts and found them correct; its other findings are the
corrections described above.

The harness also reported nine other case files in the suite that fail to load
with this Claude Code version (`case.yaml must be a YAML object`, and two
graders without `type:`). They are not part of this work and were not run.
