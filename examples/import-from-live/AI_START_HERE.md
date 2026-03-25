# AI Start Here

Use this page when you want to drive `import-from-live` safely with
Codex, Claude, Cursor, or another AI assistant.

## What This Example Is For

This example shows the "Arnie" pattern: discovering workloads from a
running Kubernetes cluster (ArgoCD + Helm + Native), proposing a ConfigHub
App structure, and optionally importing immediately.

It uses fixture manifests in `fixtures/` to simulate a cluster with 13
resources across three namespaces.

## Read-Only First

Start with preview commands only:

```bash
cd examples/import-from-live
../../cub-scout import --dry-run --from-bundle fixtures/
```

For machine-readable output:

```bash
../../cub-scout import --dry-run --from-bundle fixtures/ --json
```

These commands do not mutate ConfigHub or modify any cluster state.

## Recommended Path

```bash
# Preview what would be proposed
../../cub-scout import --dry-run --from-bundle fixtures/

# JSON output for scripting or verification
../../cub-scout import --dry-run --from-bundle fixtures/ --json

# Compare against expected output
diff <(../../cub-scout import --dry-run --from-bundle fixtures/ --json | jq -S .) \
     <(jq -S . expected-output/suggestion.json)
```

## Important Boundaries

- `--dry-run` never writes to ConfigHub
- `--from-bundle` reads fixture files instead of connecting to a live cluster
- The `demo.sh` script runs the full narrated human walkthrough
- No commands in the dry-run path mutate anything

## What To Verify

After running the dry-run:

- 9 workloads are discovered across 3 namespaces
- ArgoCD owns 6 Deployments (api + worker x 3 envs)
- Helm owns 3 StatefulSets (redis x 3 envs)
- 1 Native ConfigMap (debug-config) is detected and skipped
- App structure groups by component name (api, worker, redis)
- Variants are inferred from ArgoCD Application paths (dev, staging, prod)

## Artifacts

| File | Purpose |
|------|---------|
| `fixtures/applications.yaml` | 3 ArgoCD Application CRs |
| `fixtures/deployments.yaml` | 6 Deployments with ArgoCD labels |
| `fixtures/statefulsets.yaml` | 3 StatefulSets with Helm labels |
| `fixtures/native.yaml` | 1 unmanaged ConfigMap |
| `expected-output/suggestion.json` | Expected JSON output |

## Related Files

- [README.md](./README.md)
- [prompts.md](./prompts.md)
- [contracts.md](./contracts.md)
