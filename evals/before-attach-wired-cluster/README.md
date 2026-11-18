# before-attach-wired-cluster

Asks, before `cub cluster attach staging --controller argo --no-install`,
whether attach will go ahead on this cluster and which object decides it. It
is the question `doctor --before-attach` and the MCP `doctor` tool's
`before_attach` exist to answer
([#888](https://github.com/confighub/cub-scout/issues/888)).

## Where the evidence comes from

Everything is recorded from a real cluster in the attach lane
(`scripts/ci/attach-lane.sh`, `-f level=attach`), never written by hand:

- `fixtures/cluster/` is the Argo CD run's cluster after one real attach (run
  44378253850, cub v0.11.4, Argo CD v3.6.1, Flux v2.10.0), the same files as
  `test/e2e/fixtures/attach-lane/recorded/argo/after/`. Secrets are by
  namespace, name, type and labels only.
- `fixtures/cluster/cub-cluster-attach-help.txt` is the long help of
  `cub cluster attach` at cub v0.11.4, identical to cub's main on 2026-11-18.
- `mocks/cub-scout/fixtures/` holds `doctor --format json` and
  `doctor --before-attach --format json` as the lane recorded them on that
  cluster, before the second attach.
- `mocks/cub-scout/_tools.json` is the standalone `tools/list` of this build.

## Expected answer

```
ATTACH: refuses
BLOCKER: argocd/confighub-oci-creds
```

This is what attach did: the lane attached the same cluster a second time and
attach refused, naming that Secret
(`test/e2e/fixtures/attach-lane/recorded/argo/attach-again.log`).

## Why this question

- Argo CD is installed (`argocd/argocd-server`), which on its own means attach
  uses the existing Argo CD and goes ahead. That is the wrong answer.
- The help text states the rule ("attach refuses a cluster whose Argo CD
  already holds ConfigHub registry credentials") but names the Secret only
  further down, under `--existing-secrets`. Both arms have the help text, so
  the answer can be reached without cub-scout.
- `secrets-by-name.txt` lists the Secret among eight others, with the label
  `argocd.argoproj.io/secret-type: repo-creds`.

## What each arm gets

Both arms get the same `./cluster/` export and help text. The cub-scout arm
also gets the plugin's skills and the recorded `doctor` answers; every other
tool answers that it has no recording here.

## Result

Not run. Paid eval runs are not approved, so whether cub-scout saves an agent
turns, time or cost on this question is not known, and nothing here claims it
does. `test_case.py` checks the case offline: the expected answer is derived
again from the raw export and help text, the graders take the expected lines
and refuse wrong ones, the scaffold reproduces the export, and the recordings
say `refuse`.
