# AI Start Here

Use this page when you want to drive `combined-git-live` safely with
Codex, Claude, Cursor, or another AI assistant.

## What This Example Is For

This example shows the "Banko" pattern: aligning a Flux monorepo Git
structure with live cluster state to find what's aligned, what's in Git
but not deployed, and what's running but not in Git.

It uses a simulated Git repo (`git-repo/`) and cluster fixtures
(`cluster-fixtures/`) — no live cluster is required for the fixture path.

## Read-Only First

Start with the fixture-only path:

```bash
cd examples/combined-git-live
../../cub-scout combined \
  --git-path git-repo \
  --from-bundle cluster-fixtures/ \
  --suggest --json
```

This reads local files only. No cluster connection or ConfigHub mutation.

## Recommended Path

```bash
# ASCII alignment view
../../cub-scout combined \
  --git-path git-repo \
  --from-bundle cluster-fixtures/ \
  --suggest

# JSON for verification
../../cub-scout combined \
  --git-path git-repo \
  --from-bundle cluster-fixtures/ \
  --suggest --json

# Compare against expected output
diff <(../../cub-scout combined --git-path git-repo --from-bundle cluster-fixtures/ --suggest --json | jq -S .) \
     <(jq -S . expected-output/alignment.json)
```

## Important Boundaries

- `--from-bundle` reads fixture files instead of connecting to a live cluster
- `--suggest` produces a proposal, not an action
- No commands in the fixture path mutate anything
- Reconciliation rules in the proposal are suggested defaults, not applied policy

## What To Verify

After running the combined alignment:

- 4 workloads are `aligned` (payment-api + payment-worker x dev + prod)
- 1 app is `git-only` (notifications-service — in Git, not deployed)
- 1 workload is `cluster-only` (cache-warmer — running, not in Git)
- Variant inference comes from Kustomize overlay paths
- JSON output matches `expected-output/alignment.json`

## Artifacts

| File | Purpose |
|------|---------|
| `git-repo/` | Simulated Flux monorepo with base/overlay Kustomize structure |
| `cluster-fixtures/deployments.yaml` | Cluster Deployments with Flux labels + one orphan |
| `expected-output/alignment.json` | Expected JSON output from `combined --suggest --json` |

## Related Files

- [README.md](./README.md)
- [prompts.md](./prompts.md)
- [contracts.md](./contracts.md)
