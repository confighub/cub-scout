# Copyable Prompts

## 1. Orient Me First

Read this example and do not mutate anything yet.

Explain:

- what this example deploys (Flux + podinfo + orphan fixtures)
- what `setup.sh` creates (kind cluster, Flux, GitRepository, Kustomization, orphans)
- what `cleanup.sh` removes (kind cluster)
- which steps require a live cluster
- what success looks like

Then review only:

```bash
cat setup.sh
cat orphans.yaml
```

## 2. Safe Walkthrough

Guide me through `platform-example` step by step.

Before each command:

- explain what it does
- say whether it mutates cluster state
- tell me what to look for in the output
- tell me what evidence surface it affects

Use this path:

```bash
./setup.sh
../../cub-scout map list
../../cub-scout map orphans
../../cub-scout trace deploy/podinfo -n podinfo
../../cub-scout gitops status
```

## 3. Verify The Deployment

After setup, verify:

- Flux controllers are running in `flux-system`
- podinfo Kustomization is reconciled
- podinfo deployment is ready
- orphan resources are present and detected as Native
- `map orphans` shows resources from `orphans.yaml`
- `trace` shows the full GitRepository → Kustomization → Deployment chain

## 4. Call Out The Remaining Gap

Evaluate this example honestly.

Say whether:

- the orphan fixtures are realistic
- the Flux setup is reproducible and deterministic
- the example works on a fresh kind cluster without external dependencies
- scan evidence is included or missing from this example
