# Agent Evals

Does an agent answer operators' questions better with cub-scout than without it?
This suite measures that with
[`claude plugin eval`](https://code.claude.com/docs/en/plugin-evals): each case
runs with the cub-scout plugin loaded and again without it, and the difference
(`Δ`) is what cub-scout contributed. Tracking issue:
[#603](https://github.com/confighub/cub-scout/issues/603).

This is the pilot: nine cases on one recorded scenario. The first suite of 20–30
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

`fixtures/scenario.yaml` is six Deployments whose ownership is given by labels
(Flux, Argo CD, Helm, ConfigHub, two unmanaged) and one image that does not
exist. No controllers are installed. `fixtures/setup.sh` applies each labelled
workload server-side under its controller's real field manager
(`kustomize-controller`, `argocd-controller`, `helm`), as the controller would.
`fixtures/incident.sh` then edits three by hand: `kubectl set image` on
`checkout`, `kubectl scale` on `cart`, `kubectl patch` on `inventory`.

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

Recorded with v2.12.3 code plus #625, without which `explain` attributed the
`kubectl set image` on `checkout` to Flux (#624). MCP `trace` returns only
`exit status 1` for all
six Deployments ([#619](https://github.com/confighub/cub-scout/issues/619)), so
agents must rely on `explain` and `map`. The recordings keep that on purpose:
the suite measures cub-scout as shipped.
