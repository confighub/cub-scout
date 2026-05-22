# Receipt Example: Aggregate Receipts with `--scope`

This example walks through using `cub-scout receipt verify --scope namespace/<ns>` to compose **N per-resource receipts + 1 aggregate** over them. The aggregate's subject is a deterministic `synthetic-aggregate://sha256/<id>`; its verdict is synthesized over the input verdicts via the configured policy (default: max-severity).

Issue: [`#448`](https://github.com/confighub/cub-scout/issues/448) aggregate half.
Parent capability: [`#446`](https://github.com/confighub/cub-scout/issues/446).
Shipped in: [`#469`](https://github.com/confighub/cub-scout/pull/469).

## Two CLI shapes

```bash
# Namespace auto-discovery: walk Deployment / StatefulSet / DaemonSet /
# CronJob / Job in the namespace, build a per-resource receipt for each,
# then build the aggregate over them.
cub-scout receipt verify --scope namespace/<ns> --strategy <s> --save

# Comma-list batch: explicit set of resources (kinds normalized per the
# single-resource positional rules).
cub-scout receipt verify <kind>/<name>,<kind>/<name>,... -n <ns> --strategy <s>
```

In both shapes the CLI emits N per-resource receipts (JSONL lines on stdout, optionally `--save`d) followed by 1 aggregate receipt (pretty-printed JSON). `--fail-on` applies to the **aggregate verdict**.

## The aggregate receipt

```json
{
  "_type": "https://in-toto.io/Statement/v1",
  "subject": [
    {
      "name": "synthetic-aggregate://sha256/1a2b3c4d5e6f7890",
      "digest": {"sha256": "1a2b3c4d5e6f7890abcdef..."}
    }
  ],
  "predicateType": "https://cub-scout.dev/receipt/v1",
  "predicate": {
    "version": "v1",
    "predicateName": "aggregate-verdict",
    "verdict": "BLOCK",
    "claim": "aggregate verdict BLOCK over 12 receipt(s) in namespace prod (policy=max-severity)",
    "scope": {"kind": "namespace", "namespace": "prod"},
    "evidence": {},
    "omissions": [
      {
        "missing": "aggregate-partial-coverage",
        "reason": "2 of 12 input attestation(s) carried verdict INCONCLUSIVE; aggregate verdict may not reflect full coverage",
        "severity": "warning"
      }
    ],
    "inputAttestations": [
      {"uri": "cub-scout-receipt://abc123def456", "digest": {"sha256": "abc123def456..."}},
      {"uri": "cub-scout-receipt://9e7d12fa11b2", "digest": {"sha256": "9e7d12fa11b2..."}}
    ],
    "nextSteps": [],
    "verifier": {"tool": "cub-scout", "version": "v2.2.1"},
    "verifiedAt": "2026-05-22T22:00:00Z",
    "fingerprint": "sha256:..."
  }
}
```

Key shape rules:

- **Subject scheme** is `synthetic-aggregate://sha256/<aggregate-id>` where `<aggregate-id>` is the first 16 hex chars of the SHA-256 over the deterministically-sorted concatenation of input digests. Reordering the inputs produces the **same** subject — the aggregate is a set, not a list.
- **`predicateName`** is `aggregate-verdict` (a new predicate distinct from `applied-matches-spec` / `source-truth-pass` / `no-manual-edits-since`).
- **Verdict** is synthesized over the input verdicts via the configured `AggregateVerdictPolicy`. v1 ships `max-severity` (default): `BLOCK > INCONCLUSIVE > WATCH > PASS`.
- **`omissions[]`** includes an `aggregate-partial-coverage` entry when any input was INCONCLUSIVE or any per-resource verify failed during discovery.
- **`inputAttestations[]`** carries one entry per per-resource receipt. Each entry is fingerprint-verified at chain-construction time via the same `VerifiedAttestationRef` typed wrapper the chained-half flow uses (Codex round-6 P1 `#463` fix).
- **Fingerprint** covers the full Statement (including `inputAttestations[]`) minus only `predicate.fingerprint`.

## Worked example — fleet conformance audit

A quarterly compliance run over `view:prod-baseline-q2-2026`:

```bash
$ cub-scout receipt verify \
    --scope namespace/prod \
    --strategy git-argo \
    --save \
    --aggregate-policy max-severity

{"_type":"https://in-toto.io/Statement/v1",...}   # per-resource: deploy/api
{"_type":"https://in-toto.io/Statement/v1",...}   # per-resource: deploy/worker
{"_type":"https://in-toto.io/Statement/v1",...}   # per-resource: deploy/cron
... (12 per-resource receipts, one JSONL line each)

{
  "_type": "https://in-toto.io/Statement/v1",
  "subject": [...],
  "predicateType": "https://cub-scout.dev/receipt/v1",
  "predicate": {
    "predicateName": "aggregate-verdict",
    "verdict": "BLOCK",
    "claim": "aggregate verdict BLOCK over 12 receipt(s) in namespace prod (policy=max-severity)",
    ...
  }
}

saved: $XDG_DATA_HOME/cub-scout/receipts/2026-05-22T22-00-00Z__aggregate-verdict__-__1a2b3c4d.receipt.json
```

The aggregate's `--fail-on` exits 2 if the synthesized verdict matches:

```bash
$ cub-scout receipt verify \
    --scope namespace/prod \
    --strategy git-argo \
    --fail-on any-non-pass \
    --save
# (per-resource receipts emit to stdout + saved to store)
# (aggregate emits + saves)
# exit 2 — aggregate verdict matched the gate set
```

## Comma-list batch

For an explicit set of resources (not auto-discovered):

```bash
$ cub-scout receipt verify \
    deploy/api,deploy/worker,statefulset/db \
    -n prod \
    --strategy git-argo \
    --save \
    --out aggregate.receipt.json
```

The same JSONL-then-aggregate output shape; the aggregate's `predicate.scope.kind` is `"batch"` rather than `"namespace"`.

## Verdict-synthesis policies

v1 ships `max-severity` only: `BLOCK > INCONCLUSIVE > WATCH > PASS`.

INCONCLUSIVE outranks WATCH because **missing evidence is "louder" than ambiguous evidence** — the consumer needs to fix the gap before they can claim coverage. The aggregate-partial-coverage omission entry makes this explicit.

Future policies (`majority`, weighted) wire through `--aggregate-policy <name>` without breaking the surface:

```bash
$ cub-scout receipt verify --scope namespace/prod --strategy git-argo \
    --aggregate-policy max-severity   # default; explicit for clarity
```

Unknown policies are rejected upfront.

## Per-resource failure handling

A per-resource verify can fail (404 on the resource, load error, etc.). The CLI **does not abort** on per-resource failures:

- Failed verifies emit a stderr warning
- They contribute to an `aggregate-partial-coverage` omission entry on the aggregate
- The aggregate is still built from the **successful** subset

Example with 1 failure in a 12-resource namespace:

```bash
$ cub-scout receipt verify --scope namespace/prod --strategy git-argo
Warning: per-resource verify failed for Deployment/legacy-tool in prod: ...
... (11 per-resource receipts emit successfully)
{
  ...
  "omissions": [
    {
      "missing": "aggregate-partial-coverage",
      "reason": "1 of 12 resources in scope failed per-resource verify; aggregate composed from 11 successful verifies",
      "severity": "warning"
    }
  ],
  "inputAttestations": [11 entries],
  ...
}
```

## Validating later

Six months after the audit, an auditor can validate any individual per-resource receipt OR the aggregate:

```bash
$ cub-scout receipt validate aggregate.receipt.json
# exit 0 = fingerprint intact; 1 = mismatch; 2 = I/O

$ cub-scout receipt show aggregate.receipt.json --format ascii
# Renders the same evidence (verdict, claim, omissions, input count)
```

The aggregate's fingerprint covers `inputAttestations[]` by construction; tampering with the digest of any input invalidates the aggregate's recomputed fingerprint.

## See also

- [`../README.md`](../README.md) — receipt v1 + v2 overview
- [`../chained/README.md`](../chained/README.md) — chained receipts via `--input-attestation`
- [`../ci-gate/README.md`](../ci-gate/README.md) — CI gating with `--fail-on`
- [`../watch-emit/README.md`](../watch-emit/README.md) — real-time emission via `watch --emit-receipt-on`
- [`docs/reference/json-contracts.md`](../../../docs/reference/json-contracts.md) § "--scope aggregate-with-discovery" — full wire format
- [`docs/reference/commands.md`](../../../docs/reference/commands.md) § `receipt verify` — `--scope` and `--aggregate-policy` flags
- Pilot consumer skill: [`pilot-fleet-conformance`](../../../skills/pilot-fleet-conformance/SKILL.md) (uses aggregate receipts in the operational verdict path)
- Pilot consumer skill: [`pilot-compliance-audit`](../../../skills/pilot-compliance-audit/SKILL.md) (uses aggregate receipts in the periodic-compliance path)
- Issue: [`#448`](https://github.com/confighub/cub-scout/issues/448); aggregate half shipped in [`#469`](https://github.com/confighub/cub-scout/pull/469)
