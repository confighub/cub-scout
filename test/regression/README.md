# Regression Audits

This folder contains release-readiness regression checks that compare behavior across tagged versions.

## Argo tree audit (issue #125)

Compare `v0.4.0` vs `v0.19.6` for:
- `tree ownership`
- `tree git`
- App-of-Apps parent/child visibility
- ApplicationSet -> generated Application visibility

Run:

```bash
./test/regression/argo-version-audit.sh
```

Artifacts are written to `test/regression/output/<timestamp>/`:
- `report.md` (matrix with `intentional` vs `regression` classification)
- `summary.json` (machine-readable)
- raw command JSON outputs for both versions

Notes:
- Requires Docker + kind + kubectl + jq + Go.
- Uses only local fixtures under `test/fixtures/regression` (no external Argo install required).
