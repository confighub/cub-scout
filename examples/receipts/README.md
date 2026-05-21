# Receipts

cub-scout receipts are typed, fingerprinted, immutable evidence artifacts
that wrap cub-scout's existing field-level evidence into a verifiable
record. CI/CD gates, audit trails, postmortems, and acceptance-judge
tooling can attach a receipt to a decision and later prove the inputs
were what they claim to be.

Wire format: **in-toto Statement v1** envelope (`_type =
"https://in-toto.io/Statement/v1"`) wrapping a cub-scout predicate URI
`https://cub-scout.dev/receipt/v1`. v1 ships fingerprint-only; v2 adds
DSSE signing wrapped in Sigstore Bundle v0.3 (purely additive — no
envelope change).

## v1 Batch 1: `applied-matches-spec`

See [`applied-matches-spec/`](./applied-matches-spec/) for the four
canonical example receipts (PASS, BLOCK on manual-edit, BLOCK on anchor
mismatch, INCONCLUSIVE) and the contract reference.

## Generating Your Own

```bash
# Standalone-mode receipt — works without ConfigHub auth.
./cub-scout receipt verify deploy/api -n prod

# Pin to an explicit revision (e.g., the SHA in a release ticket).
./cub-scout receipt verify deploy/api -n prod --at-commit abc123def456

# Write the canonical JSON form to disk for audit attachment.
./cub-scout receipt verify deploy/api -n prod --format json --out api.receipt.json
```

## Read-Only Triad Lock

Receipts emit artifacts; they never mutate. Static guards in
`cmd/cub-scout/receipt_readonly_test.go` and the `FilterNextSteps`
filter in `pkg/agent/receipt_predicates.go` enforce this at build
time — no mutating K8s client calls in receipt source files, no
mutating `actionType` or `nextCommand` in emitted next-step hints.

## References

- Parent issue: [`confighub/cub-scout#446`](https://github.com/confighub/cub-scout/issues/446)
- Locked design: [`docs/proposals/receipts-way-forward.md`](../../docs/proposals/receipts-way-forward.md)
- Contract: [`docs/reference/json-contracts.md`](../../docs/reference/json-contracts.md) § Receipt Contract
