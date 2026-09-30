# Agent Evals

Does an agent answer operators' questions better with cub-scout than without it,
and at what cost? An *agentic* GitOps explorer is one that gives agents a cost
and time advantage, not only better answers; this suite is where cub-scout
has to show it.
This suite measures that with
[`claude plugin eval`](https://code.claude.com/docs/en/plugin-evals): each case
runs with the cub-scout plugin loaded and again without it, and the difference
(`Δ`) is what cub-scout contributed. Tracking issue:
[#603](https://github.com/confighub/cub-scout/issues/603).

This is the pilot: thirteen cases on one recorded scenario, and three on a 300-Deployment scale scenario. The first suite of 20–30
cases, a scheduled CI run and published results come next.

## Main scenario evidence refresh (2026-09-30)

The main scenario was re-recorded in an isolated disposable cluster after the
recorder gained `--show-managed-fields`. Both arms now receive the full field
manager evidence for attribution. Source revision and file hashes are in
[the recording manifest](fixtures/recording-2026-09-30.json). The old result
tables below describe their original evidence; they are not results on these
new fixtures. The scale raw exports were also refreshed read-only from the
existing named scale context; [their manifest](fixtures/scale/recording-2026-09-30.json)
records provenance. Its scout arm remains live, so runtime observations can
change after capture.

The scenario uses representative field-manager names and controller labels;
real GitOps controllers are not installed. The raw dump and MCP capture occur
sequentially, so timestamps and runtime status can change during recording.
The [first paired smoke](reports/2026-09-30-fair-attribution-smoke.md) answered
correctly in both arms and cost more with scout; no benefit is assumed.

The `changed-by-checkout` and attribution-contract smokes were file-only:
ordinary tools were Read, Glob and Grep, plus harness task, skill and search
tools; Bash, kubectl, Helm, jq and Python were unavailable. Neither satisfies
roadmap Experiment A. Before a further paid attribution campaign, preflight
safe, equivalent ordinary command access for both arms.

## Run

Needs Claude Code 2.1.269 or later, logged in. Runs count against that account.

```bash
claude plugin eval . --scaffold --runs 3 --max-cost-usd 10
```

`--scaffold` is required: each case's `scaffold.sh` writes the cluster export
into the run's workspace. A single case, one run per arm (cheapest while
editing graders):

```bash
claude plugin eval . --scaffold --case owner-unlabelled --runs 1 --keep-temp
```

Recorded runs are sandboxed: no home directory, no kubeconfig, no network.
They do not touch a cluster or ConfigHub; the scale cases below explicitly
use a live cluster.

## Cost and speed

Correctness is half the question; the other half is what an answer costs. Run
with `--json`, then:

```bash
evals/scripts/report.py evals/results/<run>.json [more.json ...] --by-tag
```

It prints the legacy harness score and cost tables, plus a binary verified
answer table when the result contains grader definitions. The legacy `$/score`
is total run cost divided by the fractional harness score. It is retained for
diagnostics and continuity; it is not a binary correctness rate or evidence of
general savings. The binary table checks positive-weight graders embedded in
the result, excluding every `tool_used` grader even if it contributed to the
harness score. It cannot verify that those embedded patterns are the current
case graders; use the offline regrader below for that. Cost includes agent,
judge grader and mock spend, including runs that ended in errors or hit a turn
limit. Schema-v1 `costUsd` is already inclusive: `judgeCostUsd` and
`mocks.calls.costUsd` are breakdowns, not extra amounts to add. This is verified
against Claude Code 2.1.274 and its saved primary/mock cost records; see the
[producer's result documentation](https://code.claude.com/docs/en/plugin-evals#json-result).
Missing, invalid or inconsistent spend is unknown, never free. Input completeness is reported before the tables. Use
`--require-complete` for a gate: partial or unverified completeness exits
non-zero while still printing the observed costs and scores. A completed run
is not necessarily a passing run; correctness remains a separate metric.
Completeness uses the producer's `partial` flag and per-case planned run counts;
it cannot discover a case omitted entirely from a falsely complete artifact.

```bash
evals/scripts/report.py evals/results/<run>.json --require-complete
```

Do not pool different models, versions, evidence conditions or grader revisions
into one headline number. The reporter combines observations; it does not prove
that the input experiments are comparable.

### Regrade saved transcripts against current graders

The report's binary table uses grader definitions embedded in the result. To
audit saved runs against the regex graders currently in each case's
`graders/` directory, run the offline transcript regrader:

```bash
evals/scripts/regrade.py evals/results/<run>.json \
  --out evals/results/<run>.regraded-current.json
```

It reads the terminal `type: result` text from each saved transcript, checks
all current regex graders, and excludes `tool_used` indicators entirely. An
error or interrupted run fails; missing traces, unsupported grader formats,
malformed metadata and paths outside allowed directories are unknown. The new
audit is separate from the source result and records its hash, current grader
hashes, run indexes, outcomes and spend/model metadata. The command does not
rerun agents or judges. By default traces must resolve inside the input result
file's directory. Some harness artifacts keep traces elsewhere; allow only a
reviewed directory explicitly with repeatable `--trace-root` flags:

```bash
evals/scripts/regrade.py evals/results/<run>.json \
  --trace-root /path/to/reviewed/traces \
  --out evals/results/<run>.regraded-current.json
```

The current execution priority and evidence rules are in
[`docs/roadmap-3.0-execution.md`](../docs/roadmap-3.0-execution.md) and
[#645](https://github.com/confighub/cub-scout/issues/645).

Historical pilot table (one run per case per arm, 2026-09-26/27):

**Superseded accounting:** the original reporter added judge/mock breakdowns
twice. These recorded-suite dollar columns preserve the old published figures
for audit only; regenerate from the original JSON with the corrected reporter
before using them. Live scale runs with mocks disabled are unaffected.

| Group | Arm | Historical harness score | $/run | Historical $/score | Turns |
|---|---|---|---|---|---|
| All 13 cases | with | 0.97 | $0.70 | $0.72 | 15.2 |
| | without | 0.67 | $0.52 | $0.78 | 14.5 |
| Attribution | with | 1.00 | $0.83 | $0.83 | 18.0 |
| | without | 0.25 | $0.73 | $2.93 | 22.0 |
| Pitfalls | with | 1.00 | $0.68 | $0.68 | 13.5 |
| | without | 0.75 | $0.32 | $0.42 | 9.8 |

These are exploratory historical harness results, not current strict transcript
regrades or a general savings claim. The attribution rows compare unequal
evidence: the original baseline export lacked `metadata.managedFields`, while cub-scout
could read it. On this small scenario the plugin costs more per run, partly
because its skill descriptions and tool schemas are added to every session.
The scale tables below also retain their original historical scores and cost
ratios; they do not establish general savings.

## Design

- **Both arms receive the same full raw export.** Each main-scenario case's
  `scaffold.sh` writes `kubectl get -o yaml --show-managed-fields` evidence into
  `./cluster/`. The baseline reads this export; the scout arm also receives
  skills and MCP recordings from the scenario. Prompts declare that these are
  recorded responses, not independent live confirmation. The historical
  attribution runs omitted managedFields from the baseline; their reported
  advantage must not be transferred to this refreshed design.
- **MCP answers are recordings.** `mocks/cub-scout/` answers `doctor`, `map`,
  `scan`, `gitops_status`, `trace` and `explain` with what a standalone
  `cub-scout mcp serve` returned for the scenario. `trace` and `explain` are
  agent mocks: a small model returns the recording for the call's `resource`,
  accepting the same spellings as the real tool (`deploy|deployment|deployments`
  in any case, then `/NAME`) and returning the real tool's error for a bare
  name or an unknown Deployment. The harness cannot key a fixed mock on a value
  containing `/`. `_tools.json` is the real `tools/list`, so the agent sees
  the real tool descriptions. Tools without a recording (`release_check`) are
  not offered.
- **Deterministic graders.** Every prompt asks for a final line (`OWNER:`,
  `UNMANAGED:`, `CAUSE:`, `REVISION:`), and regex graders check it. Each
  grader also fails an empty answer. `skill-fired` and `used-cub-scout-mcp` are
  indicators only: they show whether cub-scout produced the answer and are left
  out of the score.
- **Negative cases.** `owner-unlabelled` and `git-revision-unknown` pass only if
  the agent declines to name an owner or commit that the cluster does not show,
  and `changed-by-payments` only if it does not invent a hand edit. Honest
  omission is part of what is measured.
- **Traps.** `rollout-stuck-looks-healthy` (Available=True while the new
  version never started), `flux-helm-not-plain-helm` (Helm's labels on a
  Flux-managed chart), `flux-installed-but-not-working` (controller pods but no
  Flux objects) and `argo-label-vs-tracking-id` (a copied label contradicting
  the tracking-id; this case found #628).
- **Attribution is now answerable from both sources.** The four
  `changed-by-*` cases ask about the most recent non-status writer. The export
  includes `metadata.managedFields`; explain reports `mutationCause` and
  `mutationManager`. The measurement question is whether scout saves work when
  both arms have those facts, not whether access to extra facts helps.
- **Reference answers.** Each `prompt.md` has an `expected_outcome` saying where
  in the export, and in cub-scout's output, the answer comes from.

## Scenario and recording

`fixtures/scenario.yaml` is nine Deployments whose ownership is given by labels
(Flux, Argo CD, Helm, ConfigHub, a Helm chart installed by Flux, two
unmanaged, and one with a stale Argo CD label), an image that does not exist,
and an `argocd-cm` that sets annotation tracking. No controllers are installed. `fixtures/setup.sh` applies each labelled
workload server-side under its controller's real field manager
(`kustomize-controller`, `argocd-controller`, `helm`), as the controller would.
`fixtures/incident.sh` then has Flux roll out an `orders` image that does not
exist (the old pods keep serving), and edits three workloads by hand:
`kubectl set image` on `checkout`, `kubectl scale` on `cart`, `kubectl patch`
on `inventory`.

To re-record after a change to cub-scout's output:

```bash
kind create cluster --name scout-evals
evals/fixtures/setup.sh kind-scout-evals
# wait until payments-api is ImagePullBackOff and the rest are Running
evals/fixtures/incident.sh kind-scout-evals
go build ./cmd/cub-scout
evals/scripts/record.py kind-scout-evals
kind delete cluster --name scout-evals
```

`kind create cluster` switches your current kubectl context; switch it back.
`record.py` reads only the named context, hides `cub` from PATH so the server
is recorded in standalone mode, and regenerates every case's `scaffold.sh`
(`record.py --scaffolds-only` does only that, from the committed export).
`test/unit/evals_fixtures_test.go` fails if a scaffold drifts from the
recording.

The `owner-unlabelled` case reuses the recorded `hotfix-worker` export and MCP
answer, where legacy `health` is `Unavailable` while `currentChange.verdict` is
`PASS`. Its new `healthMeasurement` field is a deterministic offline extension
for #620, derived from the recorded partial-trace answer and the current
contract, not a fresh live recording; the remaining answer fields retain their
recorded provenance. This case exercises the meaning of that distinction but
does not replace live end-to-end verification of the updated tool response.

### The scale scenario

`fixtures/scale/generate.py` writes `fixtures/scale/scenario.yaml`: 300
Deployments in 30 `team-*` namespaces, seeded so the expected answers are
fixed (Flux 120, Argo CD 90, Helm 45, ConfigHub 33, unmanaged 12, and two with
an image that does not exist). The rest are scaled to zero, which keeps one
kind node under its pod limit while every object stays in the export (about
1 MB of YAML). Its cases live in `evals/scale/` and ask cluster-wide questions:
owner counts, the unmanaged list, what is failing.

The scale cases run **live**: the with-cub-scout arm talks to a real
`cub-scout mcp serve` on the recording cluster, because a recording cannot
answer every combination of the MCP `map` tool's filters and modes. The
harness gives plugin servers a sandboxed HOME and does not pass KUBECONFIG, so
`evals/scripts/live-path.sh` builds a `cub-scout` wrapper that sets it, and
prints a PATH with the wrapper first and `cub` left out (standalone). The
baseline arm still reads the recorded export. Each scale case carries guard
mocks that fail loudly if it is run without `--mocks off`.

```bash
PATH="$(evals/scripts/live-path.sh kind-scout-evals-scale)" \
  claude plugin eval . --scaffold --tag scale --runs 3 --mocks off \
  --allow-tools "mcp__plugin_cub-scout_cub-scout__*" --max-cost-usd 30
```

```bash
kind create cluster --name scout-evals-scale
evals/fixtures/setup.sh kind-scout-evals-scale evals/fixtures/scale/scenario.yaml
# wait until the two failing pods are ImagePullBackOff and doctor's rollouts
# show no WATCH: recording while 300 Deployments settle captures them
# mid-rollout
go build ./cmd/cub-scout
evals/scripts/record.py kind-scout-evals-scale --scenario scale
# keep the cluster while running the live cases; delete it afterwards
kind delete cluster --name scout-evals-scale
```

This scenario explores agent work on a large cluster; it does not establish a
general cost advantage. It also exposed #633: before that fix, `doctor` reported 302
warnings here, one per idle Deployment and DaemonSet. `map` for the whole
cluster is about 270 KB, more than Claude Code accepts from one MCP call by
default; the MCP `map` tool filters only by namespace.

### Scale results, pilot (2026-09-27, superseded by the 3-run results below)

One run per case per arm, same model; recorded with #634, before #637.

| Case | With | Without | Historical $/score with | Historical $/score without | Turns with / without |
|---|---|---|---|---|---|
| scale-ownership-counts | 1.00 | 0.00 (turn cap) | $1.27 | n/a | 43 / 31 |
| scale-unmanaged | 1.00 | 0.00 (turn cap) | $1.51 | n/a | 36 / 31 |
| scale-whats-failing | 1.00 | 1.00 | $1.03 | $0.94 | 26 / 32 |
| **All three** | **1.00** | **0.33** | **$1.27** | **$4.35** | 35 / 31 |

This one-run pilot is exploratory and superseded by the three-run data below.
Its historical harness scores gave a $/score ratio, not a validated binary
correctness comparison. The two baseline errors hit the 30-turn budget while
counting and classifying by grep; their answers were not verified successes.
`scale-whats-failing` scored evenly: `doctor` answered it in one call, and the
baseline found the two failing pods by grep.

cub-scout's own weak point here: its whole-cluster `map` answer (268 KB) is
larger than an MCP result may be, so Claude Code saved it to a file and the
agent grepped that. Filters and a count mode on the MCP `map` tool (#635)
should cut both turns and cost.

### Scale results, 3 runs, live (2026-09-27)

Three runs per case per arm; the with-cub-scout arm against a live
`cub-scout mcp serve` with #637's `map` filters and modes; $22.13 in total.

| Case | Arm | Historical harness passes | $/run | Historical $/score | Turns | Seconds |
|---|---|---|---|---|---|---|
| scale-ownership-counts | with | 3/3 | $0.85 | $0.85 | 23.3 | 190 |
| | without | 3/3 | $1.33 | $1.33 | 39.7 | 265 |
| scale-unmanaged | with | 3/3 | $1.83 | $1.83 | 34.3 | 438 |
| | without | 2/3 | $1.47 | $2.20 | 31.7 | 250 |
| scale-whats-failing | with | 3/3 | $1.02 | $1.02 | 23.0 | 135 |
| | without | 3/3 | $0.88 | $0.88 | 25.7 | 172 |
| **All** | with | **9/9** | $1.23 | **$1.23** | 26.9 | 254 |
| | without | 8/9 | $1.23 | $1.38 | 32.3 | 229 |

The table preserves the historical harness scores and ratios from this
three-run pilot. Those figures are exploratory and do not establish general
savings; use the binary transcript regrader for current-pattern correctness.
The recorded harness reported all nine cub-scout runs and eight baseline runs
passing its checks. It was slower and dearer on the unmanaged list, and the
transcripts say why: each run had the right list within two `map` calls, then
kept verifying it against the export (16 to 27 greps, or five `trace` and four
`explain` calls and a sub-agent). The arms had different evidence access: the
cub-scout arm had both live tool evidence and the export, and cross-checked
them.

Two follow-ups: a live-only variant for the scale cases (cub-scout against a
live cluster, no export, versus the export alone), which is how agents meet
cub-scout and a separate workflow comparison of cost and time; and evidence
on each `names_only` entry (why it counts as unmanaged), in case the verification is a
trust gap.

### Live-only scale cases: prepared, results pending

The three `*-live` cases have no export scaffold. Run only their plugin arm
(`--ablation none`); a without-plugin arm would have neither an export nor live
tools and is not a valid baseline. Compare each case with the **without** arm
of the corresponding unsuffixed scale case, keeping the model/version, fixture
state and grader revision aligned. Report the three pairs separately; do not
combine all six case names into an all-cases headline. This measures different
operator workflows, not an equal-information experiment.

The interrupted September 27 artifact, when available locally, is partial and
must not be presented as the completed three-case result. Preserve it for cost
accounting. The exact-identity list graders were tightened on September 30;
older scores need transcript regrading or a rerun before comparison with new
scores. The historical tables above retain their original grader results.

For a fresh run from a terminal with Claude Code login and access to the
existing `kind-scout-evals-scale` context:

```bash
go build ./cmd/cub-scout
kubectl --context kind-scout-evals-scale get deployments -A
# Stop if the named context is unavailable or the scenario has changed.
# Compute PATH separately so a failed preflight cannot start a paid eval.
eval_path="$(evals/scripts/live-path.sh kind-scout-evals-scale)" &&
  PATH="$eval_path" claude plugin eval . --tag scale-live --ablation none \
    --runs 3 --mocks off --allow-tools "mcp__plugin_cub-scout_cub-scout__*" \
    --max-cost-usd 10 --json evals/results/scale-live-only-runs3-rerun.json

evals/scripts/report.py evals/results/scale-live-only-runs3-rerun.json \
  --require-complete
```

The handover estimated about $8; the command requests a $10 harness budget.
That is an estimate and a harness limit, not a guaranteed bill. Keep the old
artifact and use a fresh output filename for each attempt. If the budget or
session ends early, publish the partial status and cost, not a benefit claim.
Do not recreate or delete the shared cluster merely to run this command.
Paid execution, a pinned-model repeat, and a complete comparison are pending.

Offline checks exercise the actual list-grader regexes against exact,
reordered, missing, duplicate, extra and suffixed resource identities. Reporting
checks use synthetic result files under `test/fixtures/evals-report/`, outside
the harness's case discovery path. These ownership and diagnosis questions
extend the existing [AI-agent quest](../examples/ai-agent-quest/); the scale
scenario above supplies their deterministic inputs. Run checks without model
charges:

```bash
go test ./test/unit -run 'TestEval' -count=1
```

## What the recordings show today

Recorded with v2.12.3 plus #625 (hand-edit attribution), #629 (Argo CD
tracking-id first) and #630 (MCP keeps a command's JSON when it exits
non-zero). Before #630, MCP `trace` returned only `exit status 1` for every
Deployment no controller could trace (#619); it now returns the JSON answer
with `isError: true` and a note. `trace` on an Argo CD workload still fails
honestly: the scenario has no Argo CD server to ask.

## Growing the suite

The suite grows with the product. New cases come from four places:

1. **Agent-facing bugs.** Reproduce the failure as a question and keep it as a
   regression case. `owner-unlabelled` guards #617 and `changed-by-checkout`
   guards #624.
2. **New evidence kinds.** A change that adds evidence an agent should use
   (Attestations #591, ChangeWorkflow state #597, Crossplane v2 #601, Argo
   Rollouts #602) adds the question it answers, in the same PR.
3. **Reported failures.** An "AI capability gap" issue becomes a case before
   it becomes a fix.
4. **More scenarios.** One scenario today. The Argo CD and Crossplane stacks
   get their own under `fixtures/<scenario>/`, each with a setup script and a
   recording; `record.py` will take a scenario name.

Adding a case:

1. Extend the scenario (`fixtures/scenario.yaml`, `setup.sh`, `incident.sh`).
2. Re-record from a throwaway kind cluster (see below).
3. Write `prompt.md` (the question, a required final line, and
   `expected_outcome`), `case.yaml`, and graders. Each grader must fail an
   empty answer.
4. Check cub-scout's recorded answer by hand. This step found #624.
5. Run it once per arm and read both transcripts, not just the score.

Target cases the export cannot answer, or makes easy to get wrong; a case both
arms pass by reading labels measures nothing about cub-scout. Keep negative
cases, where the right answer is "none" or "unknown".

Cases start as capability cases. One that passes reliably across runs becomes
a regression case, run for every release, with results in the release notes.

### Recorded, live and refreshed

- **Scoring runs use recordings.** No cluster is involved, so runs are
  repeatable, safe and can run in CI. A user's own cluster is never used.
- **Recordings need a cluster briefly.** A throwaway kind cluster, recorded
  and deleted. When cub-scout's output changes, the recordings must be
  refreshed, or the suite keeps measuring the old behaviour. A change to MCP
  tool output or a skill re-records in the same PR.
- **A few live runs catch what recordings hide.** Mocks stand in for the
  server, so they cannot catch transport or wiring faults; the MCP framing bug
  (#616) was of that kind. A small live check runs chosen cases against a real
  `cub-scout mcp serve` on kind (`--mocks off`), weekly or before a release.

Planned in #603: a scheduled CI run, a CI check that re-records and fails on a
changed recording (ignoring pod names and timestamps), the live check, and
later a pack users can run against their own agent and cluster.

## First results (2026-09-26)

One run per case per arm, Claude Code 2.1.274 with its default model
(`claude-opus-5` in the transcripts), cub-scout v2.12.3 plus #625. $11.78 for 18
runs. One run each is enough to see where the difference is, not to measure
its size; the 3-run suite comes next.

| Case | With | Without | Historical harness score Δ | cub-scout used |
|---|---|---|---|---|
| changed-by-checkout | 1.00 | 0.00 | +1.00 | skill, `explain` |
| changed-by-cart | 1.00 | 0.00 | +1.00 | skills, `explain` |
| changed-by-inventory | 1.00 | 0.00 | +1.00 | `explain` |
| changed-by-payments (control) | 1.00 | 1.00 | 0 | skills |
| git-revision-unknown | 1.00 | 1.00 | 0 | `gitops_status` |
| list-unmanaged | 0.67 | 0.67 | 0 | `map` |
| owner-confighub | 1.00 | 1.00 | 0 | skills |
| owner-unlabelled | 1.00 | 1.00 | 0 | `trace` |
| why-payments-broken | 1.00 | 1.00 | 0 | none |

These are original exploratory harness scores, not current strict transcript
regrades. Mean historical score Δ was +0.33. Where the answer is in labels or
status, both arms scored as right. For hand-edit attribution, the baseline
export omitted `metadata.managedFields` while the cub-scout arm could inspect
it; every baseline said UNKNOWN rather than guessing. That is an unequal
evidence result, not an equal-information comparison. The control and both
negative cases passed in both arms: no invented hand edit, owner or commit.

`list-unmanaged` asked about "any GitOps tool or ConfigHub", and both arms
reasonably counted a Helm release as unmanaged. The question now names Flux,
Argo CD, Helm and ConfigHub; a re-run passed in both arms ($1.54).

Observed on the way, for follow-up:

- `scout-ingest`, the adopt/import skill, fired first on three of the four
  attribution questions, probably on the prompt's "I exported my cluster's
  state". The right skills (`investigate-drift`, `scout-attribute`) fired
  second or not at all.
- In `list-unmanaged` an agent read `gitops_status`'s "no Flux CRDs" as a
  limit of the export, not a fact about the cluster.
- The `explain` agent mock once wrapped its answer in a code fence; the
  content was the recording byte for byte.

### Pitfall cases (2026-09-27)

One run per arm, same model, v2.12.3 plus #625 and #629. $6.46 including a
rerun.

| Case | With | Without | Historical harness score Δ | cub-scout used |
|---|---|---|---|---|
| rollout-stuck-looks-healthy | 1.00 | 1.00 | 0 | a skill |
| flux-helm-not-plain-helm | 1.00 | 1.00 | 0 | `trace` |
| flux-installed-but-not-working | 1.00 | 1.00 | 0 | `gitops_status`, `doctor`, `explain` |
| argo-label-vs-tracking-id | 1.00 | 1.00 | 0 | `trace`, the `observe-argocd` skill |

In this historical pilot, where the evidence is in the export, this model read
it carefully enough to avoid the traps: `Available=True` beside `ProgressDeadlineExceeded`, Flux's
labels beside Helm's, the tracking-id beside a copied label. The first run of
`flux-installed-but-not-working` showed Δ +1.00 only because the baseline hit
the 300-second limit mid-investigation; with 600 seconds it answered correctly,
so that Δ is not counted. `argo-label-vs-tracking-id` found #628: before #629,
cub-scout named the Application from the copied label, so its answer would have
been wrong while the baseline's was right.

In these historical exploratory runs, the clearest difference was attribution
evidence missing from the baseline export (`managedFields`). Because the arms
had unequal evidence, this does not establish an equal-information advantage.
Next candidates: evidence spread across many objects, and questions where
cub-scout's verdicts save an agent from reading thousands of lines, measured by
turns and cost as well as score.

Two earlier attempts are not counted: the harness could not find the export
(`add_dirs`), and the fixed mocks could not key on a resource containing
`/`; both are fixed above. Before the stand-in controllers were added, the
baseline deduced "manual" from the absence of any controller and guessed
right on `changed-by-checkout`; the stand-ins remove that shortcut.

### Health measurement contract fixture

`health-measurement-contract` is a product-contract-only check outside
`benchmark-v1`. Its exact-object and MCP evidence is a genuine, sequential,
read-only before/after observation recorded on 2026-09-30 for
`Deployment/team-02/auth`. The object UID and resourceVersion match across
observations; the calls were sequential, not atomic. `proof.json` carries source
IDs and SHA-256 values. The case-scoped export contains only the matching
Deployment, and the fixture-owned scaffold writes the accompanying proof files.
The prompt instructs the model to read saved evidence and avoid MCP calls;
this is not an enforced tool-isolation boundary. The regex grader checks only
the final answer, so any future run must also audit its trace for unexpected
tool use before claiming compliance with that instruction. This recorded-input
product-contract check is intended to run with `--ablation none`; it
is not a paired quality or savings measurement. `record.py --scaffolds-only`
prints when it preserves this custom scaffold; the unit test validates its
export and proof output. No eval run or paid grader has been run for this case.
