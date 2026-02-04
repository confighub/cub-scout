# Exit Codes Reference

**Version:** v0.14.4
**Purpose:** Define CI-friendly exit codes for automation

---

## Drift Command Exit Codes

The `cub-scout drift` command uses exit codes to signal findings to CI/CD systems.

| Code | Name | Meaning |
|------|------|---------|
| 0 | OK | No failure triggered (or no `--fail-on` specified) |
| 1 | Error | Operational error (bad arguments, file read failure, cluster access) |
| 2 | Failure | Findings met the `--fail-on` severity threshold |

### Usage

```bash
# Pure reporting mode (always exits 0 unless error)
cub-scout drift --file manifests/

# Fail on warning or critical findings
cub-scout drift --file manifests/ --fail-on warning

# Fail only on critical findings
cub-scout drift --file manifests/ --fail-on critical

# Fail on any finding (including info)
cub-scout drift --file manifests/ --fail-on info
```

### CI Integration Example

```yaml
# GitHub Actions
- name: Check for drift
  run: |
    cub-scout drift --file manifests/ --fail-on warning --format json > drift.json
  continue-on-error: true

- name: Upload drift report
  if: failure()
  uses: actions/upload-artifact@v3
  with:
    name: drift-report
    path: drift.json
```

### Semantic Contract

Exit codes are determined **solely by JSON facts** (severity field) and the `--fail-on` flag.

- ASCII output does **not** affect exit behavior
- The Leak Test invariant is enforced: removing ASCII would not change exit code

See `docs/semantic-contract.md` for the full model.

---

## General Exit Codes

Other cub-scout commands follow standard Unix conventions:

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |

---

## Design Principles

1. **Deterministic** — Same input + same flags = same exit code
2. **JSON-driven** — Exit logic uses structured facts, not rendered output
3. **Explicit thresholds** — User must opt-in to failure behavior with `--fail-on`
4. **No silent failures** — Operational errors always exit 1
