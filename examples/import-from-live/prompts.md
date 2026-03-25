# Copyable Prompts

## 1. Orient Me First

Read this example and do not mutate anything yet.

Explain:

- what the "Arnie" pattern is
- what fixtures simulate (13 resources, 3 namespaces, 3 owners)
- what `--dry-run` does vs real import
- what success looks like

Then run only:

```bash
../../cub-scout import --dry-run --from-bundle fixtures/ --json
```

## 2. Safe Walkthrough

Guide me through `import-from-live` step by step.

Before each command:

- explain what it does
- confirm it is read-only (dry-run)
- tell me what ownership detection rules apply
- tell me what variant inference rules apply

Use this path:

```bash
# Preview
../../cub-scout import --dry-run --from-bundle fixtures/

# JSON for verification
../../cub-scout import --dry-run --from-bundle fixtures/ --json
```

## 3. Verify The Proposal

After the dry-run, verify:

- 9 workloads discovered (6 ArgoCD + 3 Helm)
- 1 Native resource detected and skipped
- App structure has 3 components (api, worker, redis)
- Each component has 3 variants (dev, staging, prod)
- JSON output matches `expected-output/suggestion.json`

```bash
diff <(../../cub-scout import --dry-run --from-bundle fixtures/ --json | jq -S .) \
     <(jq -S . expected-output/suggestion.json)
```

## 4. Call Out The Remaining Gap

Evaluate this example honestly.

Say whether:

- the fixture data covers ArgoCD, Helm, and Native detection correctly
- variant inference from ArgoCD paths is deterministic
- the proposal handles the Native/orphan case conservatively (skip, not guess)
