# Reference: Argo CD ApplicationSet handling

cub-scout's coverage of **ApplicationSet** — the Argo CD CRD that generates Applications dynamically from templates + generators. This reference documents which generators cub-scout parses, how generator origin is preserved in import proposals, and the edge cases (matrix nesting, exclude patterns, full-path slugs).

Source of truth: `cub-scout/#363` (the parsing landing), `pkg/agent/argo_appset.go` (the parser), and `examples/argo-import-confighub-demo/` (the worked end-to-end flow).

## What ApplicationSet is

Argo CD's `ApplicationSet` is a CRD whose **template** is an Application spec and whose **generators** produce parameter sets (e.g., `{cluster, path, name}` triples) that the template is rendered against. The result: N Applications materialized from one ApplicationSet definition.

The generators cub-scout recognizes:

| Generator | What it produces | cub-scout coverage |
|---|---|---|
| `list` | Static enumeration of parameter sets | Fully parsed |
| `clusters` | Per-cluster fan-out based on Argo CD's registered clusters | Parsed; cluster identity propagates to the proposal's `targets[]` |
| `git` — `directories` | One Application per matching subdirectory in a repo | Fully parsed including `path` patterns and `!exclude` |
| `git` — `files` | One Application per matching file in a repo | Parsed; file path becomes the slug source |
| `matrix` | Cartesian product of nested generators | Parsed with **nested git generators** in particular |
| `merge` | Merged parameter sets from nested generators | Parsed |
| `pullRequest` | One Application per open PR (GitHub/GitLab/Bitbucket) | Detected but not currently exported into import proposals |
| `scmProvider` | One Application per repo in an org/group | Detected but not currently exported into import proposals |

The first six produce **structural** Applications that cub-scout can import. The last two (`pullRequest`, `scmProvider`) are dynamic by design and don't map cleanly to ConfigHub units; cub-scout detects them and surfaces them in `unsupported[]` on the import proposal.

## Worked example — git generator with `directories`

```yaml
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: platform-prod
spec:
  generators:
    - git:
        repoURL: https://github.com/org/platform-config
        revision: main
        directories:
          - path: apps/prod/*
          - path: apps/prod/legacy/*
            exclude: true
  template:
    metadata:
      name: '{{path.basename}}'
    spec:
      source:
        repoURL: https://github.com/org/platform-config
        path: '{{path}}'
        targetRevision: main
      destination:
        server: https://kubernetes.default.svc
        namespace: prod
```

Repo layout:

```
apps/prod/api/        ← matches apps/prod/*
apps/prod/worker/     ← matches apps/prod/*
apps/prod/frontend/   ← matches apps/prod/*
apps/prod/legacy/old/ ← matches apps/prod/legacy/* (excluded)
apps/prod/legacy/v1/  ← matches apps/prod/legacy/* (excluded)
```

cub-scout produces Applications for `api`, `worker`, `frontend` (3 of 5). The two `legacy/*` subdirectories are excluded by the `exclude: true` rule.

Import proposal (`cub-scout import argocd --space platform-prod --format json`):

```json
{
  "units": [
    {"slug": "apps-prod-api", "appsetOrigin": "platform-prod", "path": "apps/prod/api", "kind": "Deployment"},
    {"slug": "apps-prod-worker", "appsetOrigin": "platform-prod", "path": "apps/prod/worker", "kind": "Deployment"},
    {"slug": "apps-prod-frontend", "appsetOrigin": "platform-prod", "path": "apps/prod/frontend", "kind": "Deployment"}
  ],
  "spaces": ["platform-prod"],
  "targets": ["Kubernetes:in-cluster"]
}
```

The `appsetOrigin` field on each unit records the parent ApplicationSet. Useful when the ApplicationSet itself is being adopted into ConfigHub — preserves the parent-child hierarchy.

## Full-path slugs (the duplicate-basename fix)

The `#363` work added **full-path slugs** to disambiguate Applications that would otherwise collide by basename.

Without full-path slugs:

```
apps/team-a/api      → slug "api"
services/team-a/api  → slug "api"   ← collision
```

The two paths get the same slug → the second silently overwrites the first in any deterministic registry.

With full-path slugs (current behavior):

```
apps/team-a/api      → slug "apps-team-a-api"
services/team-a/api  → slug "services-team-a-api"
```

Full-path slugs are the default for git-generator imports. They're path-centric — the path is the unique identifier, not the basename. This matches how Argo CD treats them internally (Application names default to `{{path.basename}}` but the metadata includes the full path).

## Matrix generator with nested git

```yaml
spec:
  generators:
    - matrix:
        generators:
          - git:
              repoURL: https://github.com/org/platform-config
              directories:
                - path: apps/*
          - clusters: {}
```

A matrix generator produces the cartesian product of its nested generators. The example above: every app × every cluster. If the repo has 3 apps and Argo CD has 2 registered clusters, the matrix produces 6 Applications.

cub-scout's parser walks the matrix:

1. Extract the nested git generator's directories → `{api, worker, frontend}`
2. Extract the nested clusters generator's clusters → `{prod-use2, prod-euw1}`
3. Combine → 6 Applications: `api@prod-use2`, `api@prod-euw1`, `worker@prod-use2`, etc.

The import proposal records each as a separate unit with cluster-aware `targets[]`. This is how cub-scout supports **fleet-aware** import from a single ApplicationSet.

## Exclude patterns

`exclude: true` on a directory pattern marks it for **exclusion**:

```yaml
directories:
  - path: apps/*
  - path: apps/legacy/*
    exclude: true
```

The parser order matters: include rules accumulate; exclude rules subtract. The above is "every `apps/*` EXCEPT `apps/legacy/*`."

Multiple excludes compose:

```yaml
directories:
  - path: apps/*
  - path: apps/legacy/*
    exclude: true
  - path: apps/staging/*
    exclude: true
```

Result: every `apps/*` except `apps/legacy/*` and `apps/staging/*`.

The pattern syntax follows Argo CD's documented glob rules (a `*` matches any non-slash segment; `**` matches any number of segments).

## Owner classification on the live side

When cub-scout observes a workload **produced** by an ApplicationSet-generated Application, the workload's labels look like a regular Argo-owned resource:

```yaml
metadata:
  labels:
    argocd.argoproj.io/instance: api  # the Application name, not the ApplicationSet name
  annotations:
    argocd.argoproj.io/tracking-id: "api:apps/Deployment:prod/api"
```

`detectArgoOwnership` returns `OwnerArgo` `SubType=application` `Name=api`. The ApplicationSet parent identity is **not** on the workload's labels — Argo CD writes the Application name, not the ApplicationSet name.

To walk back to the ApplicationSet:

1. The Application has owner-reference to the ApplicationSet (`argoproj.io/v1alpha1/ApplicationSet`)
2. `cub-scout trace deploy/api -n prod` reads the Application, then follows the owner-reference to the ApplicationSet, then walks to the git source

This is handled in `pkg/agent/argo_trace.go`. The two-hop walk is transparent to the user; the trace output names both the Application and the ApplicationSet:

```
$ cub-scout trace deploy/api -n prod
ApplicationSet: platform-prod (generators: git[directories])
  Application:    api (generated from apps/prod/api)
    Source:       https://github.com/org/platform-config @abc123 path=apps/prod/api
    Workload:     Deployment/api in prod
```

## Edge cases

### Empty / unmatched directories

A git generator with `directories: [{path: apps/missing/*}]` where the path doesn't match anything produces **zero** Applications. cub-scout's parser records this as an empty generator output — not an error. The proposal's `unsupported[]` may flag it if the proposal's expectation was non-empty.

### Multiple generators (additive)

```yaml
generators:
  - git: {...}
  - list:
      elements:
        - {name: special, ...}
```

Multiple top-level generators are **additive** — the result is the union of all generators' outputs. cub-scout walks each and concatenates the results. Be aware: this can produce slug collisions if the generators independently propose the same slug — currently surfaces as a `conflict` entry on the proposal.

### Plugin generators

Argo CD supports custom plugin generators (`spec.generators[].plugin`). cub-scout does NOT parse these — they're opaque by design. The proposal records them in `unsupported[]` with a clear reason.

### Sync windows + auto-sync settings

ApplicationSets can configure `syncPolicy.automated.prune` and similar. cub-scout records these as evidence on the trace output but does not enforce or modify them — they're Argo's behavior, not cub-scout's.

### Server-side apply vs client-side apply

ApplicationSet-generated Applications inherit the parent's SSA/CSA setting. The `kubectl-client-side-apply` manager string disambiguation rule (in [`observe-argocd`](../observe-argocd/SKILL.md)) applies the same way regardless of whether the Application was generated by an ApplicationSet or hand-authored.

## Skills that consume this reference

- [`observe-argocd`](../observe-argocd/SKILL.md) — names ApplicationSet as the parent in the owner chain
- [`scout-ingest`](../scout-ingest/SKILL.md) — `import argocd` preserves ApplicationSet origin in proposals
- [`prepare-for-confighub`](../prepare-for-confighub/SKILL.md) — the import-preview workflow leans on full-path slugs to avoid collisions
- [`scout-observe`](../scout-observe/SKILL.md) — `trace` walks the two-hop Application → ApplicationSet chain

## References

- Code:
  - `pkg/agent/argo_appset.go` — the generator parser
  - `pkg/agent/argo_trace.go` — the two-hop trace (Application → ApplicationSet → git source)
  - `cmd/cub-scout/import_argocd.go` — the `import argocd` proposal generator
- Issue: [`#363` — Enhanced Git parser for ArgoCD ApplicationSet git generators](https://github.com/confighub/cub-scout/issues/363) (parent + the landing scope)
- Upstream: [Argo CD ApplicationSet generators](https://argo-cd.readthedocs.io/en/stable/operator-manual/applicationset/Generators/)
- Example: [`examples/argo-import-confighub-demo/`](../../examples/argo-import-confighub-demo/)
- Related: ConfigHub-managed delivery via Argo — see [`observe-confighub-managed`](../observe-confighub-managed/SKILL.md) for the dual-label case
