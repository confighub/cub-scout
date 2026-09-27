# Agent Evals

Does an agent answer operators' questions better with cub-scout than without it?
This suite measures that with
[`claude plugin eval`](https://code.claude.com/docs/en/plugin-evals): each case
runs with the cub-scout plugin loaded and again without it, and the difference
(`Δ`) is what cub-scout contributed. Tracking issue:
[#603](https://github.com/confighub/cub-scout/issues/603).

This is the pilot: thirteen cases on one recorded scenario. The first suite of 20–30
cases, a scheduled CI run and published results come next.

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

Runs are sandboxed: no home directory, no kubeconfig, no network. Nothing touches
a cluster or ConfigHub.

## Cost and speed

Correctness is half the question; the other half is what an answer costs. Run
with `--json`, then:

```bash
evals/scripts/report.py evals/results/<run>.json [more.json ...] --by-tag
```

It prints, per case and per tag, for each arm: mean score, cost per run, cost
per correct answer, turns and seconds. Cost per run includes the agent, judge
graders and agent mocks. **Cost per correct answer** (total cost divided by
total score) is the headline: an arm that is cheap per run but rarely right is
expensive per correct answer. Runs that ended in an error are counted and
listed, because their cost was spent.

From the pilot (one run per case per arm, 2026-09-26/27):

| Group | Arm | Score | $/run | $/correct | Turns |
|---|---|---|---|---|---|
| All 13 cases | with | 0.97 | $0.70 | $0.72 | 15.2 |
| | without | 0.67 | $0.52 | $0.78 | 14.5 |
| Attribution | with | 1.00 | $0.83 | $0.83 | 18.0 |
| | without | 0.25 | $0.73 | $2.93 | 22.0 |
| Pitfalls | with | 1.00 | $0.68 | $0.68 | 13.5 |
| | without | 0.75 | $0.32 | $0.42 | 9.8 |

On this small scenario cub-scout costs more per run: the plugin adds its skill
descriptions and tool schemas to every session. It pays for itself where the
export cannot answer. The hypothesis to test next is scale: with hundreds of
workloads, reading raw YAML should cost an agent far more than asking
cub-scout, which is the claim the `scale` scenario exists to measure rather
than assert.

## Design

- **Both arms see the same evidence.** Each case's `scaffold.sh` writes a
  `kubectl get -o yaml` export of the scenario into the run's workspace as
  `./cluster/`, with the files embedded (a first attempt with `add_dirs` left
  the agent unable to find the directory). The without-cub-scout arm answers
  from that export alone. The with-cub-scout arm also gets the plugin's skills
  and cub-scout's MCP tools. So `Δ` measures what
  cub-scout adds on the same evidence, not the value of having any data at all.
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
- **Cases the export cannot answer.** The five ownership and diagnosis cases
  can be answered from labels and status in the export; the first run showed
  agents grep the export and never call cub-scout there, so Δ is about 0 by
  design. The four `changed-by-*` cases ask who made the most recent change.
  That lives in `metadata.managedFields`, which `kubectl get -o yaml` omits and
  cub-scout's `explain` reports as `mutationCause` / `mutationManager`. This
  is where Δ should show.
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

| Case | With | Without | Δ | cub-scout used |
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

Mean Δ +0.33. Where the answer is in labels or status, both arms are right and
cub-scout adds nothing. Where it is in managedFields, only the cub-scout arm
answers; every baseline said UNKNOWN (honest) rather than guessing. The
control and both negative cases passed in both arms: no invented hand edit,
owner or commit.

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

| Case | With | Without | Δ | cub-scout used |
|---|---|---|---|---|
| rollout-stuck-looks-healthy | 1.00 | 1.00 | 0 | a skill |
| flux-helm-not-plain-helm | 1.00 | 1.00 | 0 | `trace` |
| flux-installed-but-not-working | 1.00 | 1.00 | 0 | `gitops_status`, `doctor`, `explain` |
| argo-label-vs-tracking-id | 1.00 | 1.00 | 0 | `trace`, the `observe-argocd` skill |

Where the evidence is in the export, this model reads it carefully enough to
avoid the traps: `Available=True` beside `ProgressDeadlineExceeded`, Flux's
labels beside Helm's, the tracking-id beside a copied label. The first run of
`flux-installed-but-not-working` showed Δ +1.00 only because the baseline hit
the 300-second limit mid-investigation; with 600 seconds it answered correctly,
so that Δ is not counted. `argo-label-vs-tracking-id` found #628: before #629,
cub-scout named the Application from the copied label, so its answer would have
been wrong while the baseline's was right.

So far cub-scout's measured advantage is evidence the export lacks
(managedFields). Next candidates: evidence spread across many objects, and
questions where cub-scout's verdicts save an agent from reading thousands of
lines, measured by turns and cost as well as score.

Two earlier attempts are not counted: the harness could not find the export
(`add_dirs`), and the fixed mocks could not key on a resource containing
`/`; both are fixed above. Before the stand-in controllers were added, the
baseline deduced "manual" from the absence of any controller and guessed
right on `changed-by-checkout`; the stand-ins remove that shortcut.
