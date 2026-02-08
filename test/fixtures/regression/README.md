# Regression Fixtures

Fixtures in this folder are stable, local-only manifests for version regression audits.

## Argo hierarchy fixtures (issue #125)

Files:
- `argo-minimal-crds.yaml` - Minimal local CRDs for `Application` and `ApplicationSet`
- `argo-app-of-apps.yaml` - App-of-Apps sample with parent + child Applications
- `argo-applicationset.yaml` - ApplicationSet sample with generated Applications

Design goals:
- No dependency on downloading Argo controller manifests
- Deterministic names and labels
- CI-friendly with kind clusters
