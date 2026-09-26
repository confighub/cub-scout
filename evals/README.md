# Agent Evals

Does an agent answer operators' questions better with cub-scout than without it?
This suite measures that with
[`claude plugin eval`](https://code.claude.com/docs/en/plugin-evals): each case
runs with the cub-scout plugin loaded and again without it, and the difference
(`Δ`) is what cub-scout contributed. Tracking issue:
[#603](https://github.com/confighub/cub-scout/issues/603).

This is the pilot: five cases on one recorded scenario. The first suite of 20–30
cases, a scheduled CI run and published results come next.

## Run

Needs Claude Code 2.1.269 or later, logged in. Runs count against that account.

```bash
claude plugin eval . --runs 3 --max-cost-usd 10
```

A single case, one run, no baseline (cheapest while editing graders):

```bash
claude plugin eval . --case owner-unlabelled --runs 1 --ablation none
```

Runs are sandboxed: no home directory, no kubeconfig, no network. Nothing touches
a cluster or ConfigHub.

## Design

- **Both arms see the same evidence.** Each case's `cluster/` directory is a
  `kubectl get -o yaml` export of the scenario (`add_dirs` in `case.yaml`). The
  without-cub-scout arm answers from that export alone. The with-cub-scout arm
  also gets the plugin's skills and cub-scout's MCP tools. So `Δ` measures what
  cub-scout adds on the same evidence, not the value of having any data at all.
- **MCP answers are recordings.** `mocks/cub-scout/` answers `doctor`, `map`,
  `scan`, `gitops_status`, `trace` and `explain` with what a standalone
  `cub-scout mcp serve` returned for the scenario. `trace` and `explain` pick
  the recording for the `resource` argument; `_tools.json` is the real
  `tools/list`, so the agent sees the real tool descriptions. Tools without a
  recording (`release_check`) are not offered.
- **Deterministic graders.** Every prompt asks for a final line (`OWNER:`,
  `UNMANAGED:`, `CAUSE:`, `REVISION:`), and regex graders check it. Each
  grader also fails an empty answer. `skill-fired` and `used-cub-scout-mcp` are
  indicators only: they show whether cub-scout produced the answer and are left
  out of the score.
- **Negative cases.** `owner-unlabelled` and `git-revision-unknown` pass only if
  the agent declines to name an owner or commit that the cluster does not show.
  Honest omission is part of what is measured.
- **Reference answers.** Each `prompt.md` has an `expected_outcome` saying where
  in the export, and in cub-scout's output, the answer comes from.

## Scenario and recording

`fixtures/scenario.yaml` is six Deployments whose ownership is given by labels
alone (Flux, Argo CD, Helm, ConfigHub, two unmanaged) and one image that does
not exist. No controllers are installed.

To re-record after a change to cub-scout's output:

```bash
kind create cluster --name scout-evals
kubectl --context kind-scout-evals apply -f evals/fixtures/scenario.yaml
# wait until payments-api is ImagePullBackOff and the rest are Running
go build ./cmd/cub-scout
evals/scripts/record.py kind-scout-evals
kind delete cluster --name scout-evals
```

`kind create cluster` switches your current kubectl context; switch it back.
`record.py` reads only the named context, hides `cub` from PATH so the server
is recorded in standalone mode, and copies the export into every case that
reads it.

## What the recordings show today

Recorded with v2.12.3 code. MCP `trace` returns only `exit status 1` for all
six Deployments ([#619](https://github.com/confighub/cub-scout/issues/619)), so
agents must rely on `explain` and `map`. The recordings keep that on purpose:
the suite measures cub-scout as shipped.
