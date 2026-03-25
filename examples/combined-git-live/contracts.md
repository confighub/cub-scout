# Contracts

This file documents the stable inspection paths for `combined-git-live`.

## Read-Only Contracts

### `../../cub-scout combined --git-path git-repo --from-bundle cluster-fixtures/ --suggest`

- mutates: no
- output shape: ASCII alignment view + App proposal
- proves:
  - Git repo structure can be parsed for app definitions
  - cluster fixtures are classified by ownership labels
  - alignment sorts every app into aligned, git-only, or cluster-only
  - variant inference uses Kustomize overlay path segments
  - Native/orphan workloads are reported as cluster-only, not guessed
- notes: reconciliation rules in `--suggest` output are proposed defaults, not applied policy

### `../../cub-scout combined --git-path git-repo --from-bundle cluster-fixtures/ --suggest --json`

- mutates: no
- output shape: JSON object
- proves: same as above, in machine-readable form
- expected anchors:
  - `.alignment` array contains entries with `status` field
  - aligned entries have matching Git and cluster evidence
  - git-only entries have Git evidence but no cluster match
  - cluster-only entries have cluster evidence but no Git match

## Evidence Boundary

This example proves only offline alignment evidence:

- Git structure parsing from `git-repo/`
- cluster state from `cluster-fixtures/`
- alignment classification (aligned, git-only, cluster-only)

It does not prove live cluster connectivity, ConfigHub import success,
or runtime reconciliation state. The `--from-bundle` flag means no
cluster connection is attempted.

## Three States, No Ambiguity

- `aligned`: resource exists in both Git and cluster with matching ownership
- `git-only`: resource defined in Git but not deployed anywhere
- `cluster-only`: resource running in cluster with no matching Git source

cub-scout reports these facts. It does not judge whether git-only or
cluster-only is a problem.
