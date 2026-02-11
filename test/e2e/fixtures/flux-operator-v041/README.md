# Flux Operator v0.41 Interop Fixture

This fixture set captures a minimal, read-only scenario used by the `v0.20.0`
Flux Operator interop slice:

- CronJob + Job visibility
- Flux source artifact provenance
- activity timeline signals (Flux conditions + Events)
- preview-environment heuristics (PR namespace metadata)

## Contents

- `resources.yaml`:
  - Flux `GitRepository` + `Kustomization`
  - `CronJob` and `Job`
  - preview namespace labels/annotations (Forgejo/Gitea style)
  - sample warning `Event`

## Intended Use

Apply into an ephemeral cluster for manual/e2e checks:

```bash
kubectl apply -f test/e2e/fixtures/flux-operator-v041/resources.yaml
```

Then exercise:

```bash
cub-scout map cronjobs
cub-scout map jobs
cub-scout map activity --since 24h
cub-scout map previews --stale-after 72h
cub-scout trace deployment/payment-api -n ecommerce --artifacts
```
