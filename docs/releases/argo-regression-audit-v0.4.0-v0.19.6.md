# Argo Regression Audit: v0.4.0 vs v0.19.6

Result: no regressions detected.

Generated: 2026-02-08T22:23:24Z
Cluster: kind-cub-scout-regression-argo
Fixtures:
- test/fixtures/regression/argo-minimal-crds.yaml
- test/fixtures/regression/argo-app-of-apps.yaml
- test/fixtures/regression/argo-applicationset.yaml

Expected fixture counts:
- Argo-owned Deployments: 4
- Argo Applications: 5

| Check | v0.4.0 | v0.19.6 | Status | Notes |
|---|---:|---:|---|---|
| tree ownership: Argo-owned deployment visibility | 0 | 4 | intentional improvement | Improved in v0.19.6 |
| tree git: Argo Application visibility | 0 | 0 | intentional gap | No change; tracked as follow-on work |
| App-of-Apps parent/child visibility in tree git | false | false | intentional gap (tracked by #128) | Not yet surfaced in current `tree git` output |
| ApplicationSet -> generated visibility in tree git | false | false | intentional gap (tracked by #132) | Not yet surfaced in current `tree git` output |

## Raw outputs
- v0.4.0 tree ownership JSON:   /Users/alexis/public/github-repos/cub-scout/test/regression/output/20260208-222216/v0.4.0.tree.ownership.json
- v0.19.6 tree ownership JSON:   /Users/alexis/public/github-repos/cub-scout/test/regression/output/20260208-222216/v0.19.6.tree.ownership.json
- v0.4.0 tree git JSON:   /Users/alexis/public/github-repos/cub-scout/test/regression/output/20260208-222216/v0.4.0.tree.git.json
- v0.19.6 tree git JSON:   /Users/alexis/public/github-repos/cub-scout/test/regression/output/20260208-222216/v0.19.6.tree.git.json
