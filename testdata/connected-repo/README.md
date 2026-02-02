# Connected Mode Test Fixture

This directory contains a minimal repository fixture for testing connected mode
(v0.11) git-aware patterns.

## Structure

```
connected-repo/
├── .git/.keep                      # Makes this a valid git root
├── graph.json                      # Graph with ApplicationSet + Kustomization nodes
├── argocd/
│   └── applicationset.yaml         # ApplicationSet with list + git generators
├── flux/
│   └── kustomization.yaml          # Kustomization with spec.path
└── clusters/
    └── prod/
        └── kustomization.yaml      # Target path for Flux Kustomization
```

## Usage

### Local mode (--git-root)
```bash
./cub-scout patterns detect --git-root testdata/connected-repo
```

### Connected mode (--git-url + --git-ref)
Used in integration tests with httptest server providing the tarball.

## Env vars for testing

- `CUB_SCOUT_TEST_GRAPH_JSON=testdata/connected-repo/graph.json` - Load graph from JSON
- `CUB_SCOUT_TEST_TIME=2026-01-01T00:00:00Z` - Deterministic timestamps
- `CUB_SCOUT_GITHUB_API_BASE=http://localhost:PORT` - Override GitHub API (httptest)
