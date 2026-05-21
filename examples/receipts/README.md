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

## v1 Predicates

| Predicate | Inputs | Verdict semantics |
|-----------|--------|-------------------|
| [`applied-matches-spec`](./applied-matches-spec/) | Argo / Flux / ConfigHub owner + controller-resolved git anchor, optional `--at-commit` | LIVE matches the controller-resolved git anchor (PASS on controller-drift cause, BLOCK on manual-edit / anchor mismatch, INCONCLUSIVE on missing anchor) |
| [`source-truth-pass`](./source-truth-pass/) | Explicit `--strategy <name>` (one of 9 strategies), connected-mode ConfigHub auth | Mirrors `compare source-truth` Status → Verdict (PASS / WATCH / BLOCK / INCONCLUSIVE). Strategy mismatch between caller and evidence → BLOCK + `OmissionStrategyMismatch`. |
| [`no-manual-edits-since`](./no-manual-edits-since/) | `--since <RFC3339 timestamp>` cutoff | PASS when no interactive (`kubectl-*`) writer touched `metadata.managedFields` after the cutoff; BLOCK on any late interactive write; INCONCLUSIVE on missing managedFields or nil-Time entries. |

Each subdirectory ships canonical example receipts produced
deterministically by [`tools/gen-receipt-examples`](../../tools/gen-receipt-examples/)
and validated by `TestReceiptExamplesAreFresh`.

## Auto-Detection Priority

When `--predicate` is not passed, cub-scout picks one from these signals:

1. Argo / Flux / ConfigHub owner WITH a resolvable git anchor →
   `applied-matches-spec`
2. `--strategy` provided → `source-truth-pass`
3. `--since` provided → `no-manual-edits-since`
4. Otherwise → INCONCLUSIVE + `OmissionAutoDetectedPredicate`

## Generating Your Own

```bash
# applied-matches-spec — standalone-mode receipt (works without ConfigHub auth).
./cub-scout receipt verify deploy/api -n prod

# applied-matches-spec — pin to an explicit revision (e.g., the SHA in a release ticket).
./cub-scout receipt verify deploy/api -n prod --at-commit abc123def456

# source-truth-pass — connected mode, declared strategy.
./cub-scout receipt verify deploy/api -n prod --strategy git-argo

# no-manual-edits-since — cutoff before today.
./cub-scout receipt verify deploy/api -n prod --since 2026-05-22T00:00:00Z

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
