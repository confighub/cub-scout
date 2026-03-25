# Copyable Prompts

## 1. Orient Me First

Read this example and do not mutate anything yet.

Explain:

- what the "Banko" pattern is (Flux monorepo with Kustomize overlays)
- what the Git repo structure contains
- what the cluster fixtures simulate
- what the three alignment states mean (aligned, git-only, cluster-only)
- what success looks like

Then run only:

```bash
../../cub-scout combined \
  --git-path git-repo \
  --from-bundle cluster-fixtures/ \
  --suggest --json
```

## 2. Safe Walkthrough

Guide me through `combined-git-live` step by step.

Before each command:

- explain what it does
- confirm it is read-only
- tell me what alignment logic applies
- tell me what variant inference rules apply (Kustomize overlay paths)

Use this path:

```bash
# ASCII view
../../cub-scout combined \
  --git-path git-repo \
  --from-bundle cluster-fixtures/ \
  --suggest

# JSON for scripting
../../cub-scout combined \
  --git-path git-repo \
  --from-bundle cluster-fixtures/ \
  --suggest --json
```

## 3. Verify The Alignment

After the combined alignment runs, verify:

- 4 aligned workloads (Flux-managed, both Git and cluster agree)
- 1 git-only (notifications-service — defined but not deployed)
- 1 cluster-only (cache-warmer — running but not in Git, Native)
- JSON output matches `expected-output/alignment.json`

```bash
diff <(../../cub-scout combined --git-path git-repo --from-bundle cluster-fixtures/ --suggest --json | jq -S .) \
     <(jq -S . expected-output/alignment.json)
```

## 4. Call Out The Remaining Gap

Evaluate this example honestly.

Say whether:

- the three alignment states are exhaustive and unambiguous
- variant inference from Kustomize overlay paths is deterministic
- the example avoids guessing whether git-only or cluster-only is a problem
