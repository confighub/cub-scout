# Contracts

This file documents the stable inspection paths for `platform-example`.

## Mutating Operations

### `./setup.sh`

- mutates: yes (creates kind cluster, installs Flux, applies orphan fixtures)
- output shape: terminal progress text
- proves:
  - a kind cluster named `platform-demo` can be created
  - Flux can be installed via `flux install`
  - podinfo GitRepository and Kustomization can be created
  - orphan fixtures from `orphans.yaml` can be applied
- notes: requires `kind`, `kubectl`, and `flux` CLI tools

### `./cleanup.sh`

- mutates: yes (deletes kind cluster)
- output shape: terminal progress text
- proves: the kind cluster can be cleanly removed

## Read-Only Contracts

### `../../cub-scout map list`

- mutates: no
- output shape: terminal table view
- proves:
  - Flux-managed resources are classified as Flux
  - orphan resources are classified as Native
  - ownership detection uses actual labels, not heuristics

### `../../cub-scout map orphans`

- mutates: no
- output shape: terminal table view
- proves:
  - resources without GitOps ownership labels are surfaced
  - orphan fixtures from `orphans.yaml` appear as Native

### `../../cub-scout trace deploy/podinfo -n podinfo`

- mutates: no
- output shape: terminal trace chain
- proves:
  - trace follows GitRepository → Kustomization → Deployment → ReplicaSet → Pod
  - Flux ownership is derived from actual labels on the Deployment

### `../../cub-scout gitops status`

- mutates: no
- output shape: terminal status view
- proves:
  - Flux controllers are detected and their health is reported
  - Kustomizations and GitRepositories show reconciliation status

## Evidence Boundary

This example proves live cluster evidence only:

- Flux deployment and reconciliation status
- ownership detection from actual labels on live resources
- orphan identification from absence of GitOps labels

It does not prove ConfigHub import success or connected-mode readiness.
This example does not connect to ConfigHub.
