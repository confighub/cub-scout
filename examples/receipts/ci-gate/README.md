# Receipt Example: CI Gating with `--fail-on`

This example walks through using `cub-scout receipt verify --fail-on` as
a CI-pipeline gate. The use case: after a release, the pipeline runs
`receipt verify` against the deployed resource; the receipt's verdict
becomes the pipeline's gate signal, and the receipt itself is preserved
as a release artifact for audit / postmortem.

Issue: [`#451`](https://github.com/confighub/cub-scout/issues/451).
Parent capability: [`#446`](https://github.com/confighub/cub-scout/issues/446).

## The flow

```
   release pipeline
        │
        ├─ deploy (Argo / Flux / cub apply)
        │
        ├─ wait for reconciliation
        │
        ├─ ./cub-scout receipt verify deploy/api -n prod \
        │     --strategy git-argo \
        │     --fail-on any-non-pass \
        │     --save \
        │     --out api.receipt.json
        │     │
        │     ├─ exit 0 → PASS verdict → promote / continue
        │     │
        │     └─ exit 2 → WATCH / BLOCK / INCONCLUSIVE → halt
        │
        ├─ upload api.receipt.json to the release artifact store
        │     (the receipt is durable regardless of which branch fired)
        │
        └─ post the verdict + receipt-fingerprint to Slack / GitHub PR
```

Key property: the receipt is **printed to stdout / written to `--out` /
saved to `--save` BEFORE the gate evaluates the verdict**. CI gets both
the failure exit code AND the durable evidence artifact. Codex round-6
P2 (`#463`) tightened this so a bad `--fail-on` value rejects the
command upfront — there's no path where the artifact escapes with a
misconfigured gate.

## Exit semantics

| Exit | Meaning | What CI should do |
|------|---------|-------------------|
| 0    | PASS, or verdict not in the `--fail-on` set | Continue the pipeline |
| 2    | Verdict matched `--fail-on` | Halt; preserve `api.receipt.json` for review |
| 1    | Operational error (cluster unreachable, bad subject, invalid flag value) | Retry or fail the pipeline; the gate did NOT fire |

`--fail-on PASS` is rejected upfront — gating on a passing receipt is
a workflow bug; the CLI errors out with exit 1.

## `--fail-on` values

| Value | Meaning |
|-------|---------|
| `WATCH` | Exit 2 when the receipt verdict is `WATCH` |
| `BLOCK` | Exit 2 when the receipt verdict is `BLOCK` |
| `INCONCLUSIVE` | Exit 2 when the receipt verdict is `INCONCLUSIVE` (evidence missing or unavailable) |
| `WATCH,BLOCK` | Comma-separated; exit 2 on either |
| `any-non-pass` | Sugar for `WATCH,BLOCK,INCONCLUSIVE` |

Case-insensitive. Use the sugar form `any-non-pass` if you want the
strictest gate (only PASS receipts pass the gate); use the explicit
list when you want to tolerate, say, INCONCLUSIVE while still blocking
on WATCH and BLOCK.

## GitHub Actions

```yaml
name: Deploy and verify
on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Install cub-scout
        run: |
          curl -sSL https://github.com/confighub/cub-scout/releases/latest/download/cub-scout_linux_amd64 \
            -o /usr/local/bin/cub-scout
          chmod +x /usr/local/bin/cub-scout

      - name: Deploy via Argo
        run: argocd app sync payments-api --prune

      - name: Wait for reconciliation
        run: argocd app wait payments-api --health --timeout 300

      - name: Verify deployment matches Git
        id: verify
        run: |
          cub-scout receipt verify deploy/payments-api \
            -n prod \
            --strategy git-argo \
            --fail-on any-non-pass \
            --save \
            --out ./payments-api.receipt.json
        # exit 0 → success, exit 2 → gate fired (verdict is non-PASS),
        # exit 1 → operational error

      - name: Upload receipt as release artifact (always)
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: payments-api-receipt
          path: ./payments-api.receipt.json
```

The `if: always()` on the upload step is important — when the gate
fires (exit 2), `verify` step's `continue-on-error` is false (default),
so the workflow fails. But the receipt was written to `--out` BEFORE
the gate evaluated, so it's already on disk and the upload still
captures it.

## GitLab CI

```yaml
verify-deploy:
  stage: verify
  image: confighub/cub-scout:latest
  needs: [deploy, wait-reconciliation]
  script:
    - cub-scout receipt verify deploy/payments-api
        -n prod
        --strategy git-argo
        --fail-on any-non-pass
        --save
        --out payments-api.receipt.json
  artifacts:
    when: always   # keep the artifact even when the job fails
    paths:
      - payments-api.receipt.json
    expire_in: 30 days
```

## Jenkins

```groovy
pipeline {
  agent any
  stages {
    stage('Verify deploy') {
      steps {
        script {
          def rc = sh(
            script: '''
              cub-scout receipt verify deploy/payments-api -n prod \
                --strategy git-argo \
                --fail-on any-non-pass \
                --save \
                --out payments-api.receipt.json
            ''',
            returnStatus: true
          )
          // archive regardless of exit code
          archiveArtifacts artifacts: 'payments-api.receipt.json'
          if (rc == 2) {
            error("receipt verdict is non-PASS; promotion blocked")
          } else if (rc != 0) {
            error("operational error during receipt verification (rc=${rc})")
          }
        }
      }
    }
  }
}
```

## Local verification

You don't need a CI pipeline to test the gate semantics — run it
locally first:

```bash
# Force an INCONCLUSIVE verdict by asking for a predicate with missing inputs.
./cub-scout receipt verify deploy/api -n prod \
  --predicate no-manual-edits-since \
  --since 2026-05-22T00:00:00Z \
  --fail-on any-non-pass
echo "exit: $?"   # expect 2 if managedFields are missing → INCONCLUSIVE

# Bad --fail-on value — rejected upfront, no artifact produced.
./cub-scout receipt verify deploy/api -n prod \
  --fail-on GARBAGE \
  --out /tmp/should-not-exist.receipt.json
echo "exit: $?"   # expect 1
test -f /tmp/should-not-exist.receipt.json && echo "BUG: artifact leaked" \
                                            || echo "good: no artifact"
```

## Chaining receipts in CI

For multi-stage pipelines (e.g., deploy → smoke-test → promote), each
stage can produce a receipt and the next stage chains via
`--input-attestation`:

```bash
# Stage 1: deploy + verify
cub-scout receipt verify deploy/api -n staging \
  --strategy git-argo \
  --fail-on any-non-pass \
  --out stage1.receipt.json

# Stage 2: promote + verify, chained to stage 1
cub-scout receipt verify deploy/api -n prod \
  --strategy git-argo \
  --input-attestation ./stage1.receipt.json \
  --fail-on any-non-pass \
  --out stage2.receipt.json
```

Each referenced receipt's fingerprint is verified at chain construction
time — a tampered `stage1.receipt.json` would refuse to chain into
`stage2`, and the failure is upfront (exit 1) not silent.

## See also

- [`../README.md`](../README.md) — receipt v1 + v2 overview
- [`docs/reference/cli-contract.md`](../../../docs/reference/cli-contract.md) § `cub-scout receipt` — exit codes table
- [`docs/reference/json-contracts.md`](../../../docs/reference/json-contracts.md) § Receipt Contract — wire format
- Issues: [`#446`](https://github.com/confighub/cub-scout/issues/446) (v1 foundation), [`#451`](https://github.com/confighub/cub-scout/issues/451) (`--fail-on`)
