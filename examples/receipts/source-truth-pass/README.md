# Receipt Example: `source-truth-pass`

`source-truth-pass` wraps cub-scout's existing `compare source-truth`
derivation into a typed, fingerprinted receipt. The predicate verifies
that the three observed surfaces (ConfigHub / controller / runtime) agree
under an explicitly-declared delivery strategy.

cub-scout **never infers a strategy** — you must pass `--strategy`. The
nine v1 strategies (see [source-truth strategies reference](../../../docs/reference/json-contracts.md))
cover the common Argo / Flux / Helm / OCI / Kustomize combinations.

## Quick Start

```bash
# Vanilla Git -> Argo -> Kubernetes.
./cub-scout receipt verify deploy/api -n prod --strategy git-argo

# ConfigHub -> OCI -> Argo -> Kubernetes.
./cub-scout receipt verify deploy/api -n prod --strategy confighub-oci-argo

# Write the canonical JSON form to disk for audit attachment.
./cub-scout receipt verify deploy/api -n prod --strategy git-argo --format json --out api.source-truth.receipt.json
```

## Connected-Mode Required

`source-truth-pass` reads the ConfigHub surface via the `cub` CLI. The
predicate requires connected mode (`cub auth login` or
`CONFIGHUB_API_KEY`). Standalone-mode runs decline with an INCONCLUSIVE
verdict + the source-truth proof gaps mirrored into `omissions[]`.

## Verdict Mapping

The source-truth derivation produces a four-valued **Status** (PASS /
WATCH / BLOCK / ASK). The receipt maps these directly to receipt-level
verdicts:

| `compare source-truth` Status | Receipt Verdict |
|-------------------------------|-----------------|
| `PASS`  | `PASS` |
| `WATCH` | `WATCH` |
| `BLOCK` | `BLOCK` |
| `ASK`   | `INCONCLUSIVE` (with the evidence's `proof_gaps` mirrored into `omissions[]`) |

The receipt always carries the full `SourceTruthEvidence` body
(surfaces, proof gaps, outlier, native verdict) in
`predicate.evidence.sourceTruth` so consumers needing the unflattened
detail can read it directly.

## Strategy Mismatch Is BLOCK

If the receipt is asked to verify under one strategy but the underlying
source-truth derivation runs under a different one, the predicate emits
**BLOCK** with an `OmissionStrategyMismatch` entry. We never silently
honor the caller's strategy over what the evidence claims to be under
— that would let a CI gate paper over a real strategy disagreement.

## Example Files

### `pass-agreed.json` — PASS

Vanilla `git-argo` strategy. All three surfaces agree, `Status: PASS`,
`SourceTruth: AGREED`, no proof gaps. Receipt verdict: PASS.

### `block-mismatch.json` — BLOCK

Same `git-argo` strategy, but the controller's observed revision
(`feedface...`) doesn't match the runtime's expectations. The
source-truth derivation emits `Status: BLOCK` with controller as the
outlier. Receipt verdict: BLOCK.

## References

- Parent issue: [`confighub/cub-scout#446`](https://github.com/confighub/cub-scout/issues/446) — Receipt capability
- Source-truth contract: [`docs/reference/json-contracts.md`](../../../docs/reference/json-contracts.md) § Source-Truth Evidence Contract
- Strategy enumeration: `pkg/agent/source_truth.go` `AllStrategies()`
- Implementation: `pkg/agent/receipt_predicates.go` `EvaluateSourceTruthPass`
