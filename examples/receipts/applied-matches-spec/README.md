# Receipt Example: `applied-matches-spec`

cub-scout receipts are typed, fingerprinted, immutable evidence artifacts
wrapping cub-scout's existing field-level evidence (attribution, gitSource,
sourceTruth, compareThreeWay) into a verifiable record. v1 batch 1 ships
the `applied-matches-spec` predicate.

For the full design, see [`docs/proposals/receipts-way-forward.md`](../../../docs/proposals/receipts-way-forward.md).
For the contract reference, see [`docs/reference/json-contracts.md`](../../../docs/reference/json-contracts.md) § Receipt Contract.

## Quick Start

```bash
# Verify a deployment is applied per its controller-resolved git anchor.
./cub-scout receipt verify deploy/api -n prod

# Same, written to disk as the canonical in-toto Statement v1 envelope.
./cub-scout receipt verify deploy/api -n prod --format json --out api.receipt.json

# Verify against an explicit revision (e.g., the SHA in your release ticket).
./cub-scout receipt verify deploy/api -n prod --at-commit abc123def456
```

## Wire Format

Every receipt is an **in-toto Statement v1** envelope (`_type =
"https://in-toto.io/Statement/v1"`) wrapping a cub-scout-specific predicate
URI `https://cub-scout.dev/receipt/v1`. The same envelope shape that SLSA,
Sigstore, Kyverno, and Tekton Chains use — v2 DSSE signing wraps this
without changing the envelope.

The four files in this directory cover the four possible verdicts.

## Verdicts

### `pass-controller-drift.json` — PASS

The controller (Argo CD here) is reconciling. `managedFields` confirms
`argocd-controller` is the last writer; the live anchor matches the spec
anchor; the cause classifier returns `controller-drift`. The receipt
records PASS with a structured `nextStep` pointing at the read-only
follow-up `cub-scout explain`.

This is the "everything is fine" receipt — but with cryptographic evidence,
so the CI gate or release-promotion check can attach it to the deployment
record.

### `block-manual-edit.json` — BLOCK

Same Argo-owned resource, but `managedFields` shows `kubectl-edit` wrote
the live state after the controller's last apply. The cause classifier
returns `manual-edit` → BLOCK. The receipt's `nextStep` points at the
read-only diagnostic `kubectl get --show-managed-fields`; it deliberately
does **not** suggest a mutating recovery action — receipts are evidence,
never recommendations to mutate.

### `block-anchor-mismatch.json` — BLOCK

The user passed `--at-commit deadbeef...` but the controller-resolved
anchor for the live resource is `abc123...`. The two anchors don't match,
so the receipt records BLOCK regardless of the cause — investigate the
divergence before any apply.

### `inconclusive-no-anchor.json` — INCONCLUSIVE

No GitOps controller has stamped a git anchor on the resource (or the
tracer CLI isn't installed). The receipt cannot prove or refute the
applied-matches-spec claim. INCONCLUSIVE carries an explicit
`OmissionGitSourceAnchor` entry — silent ambiguity is converted into a
visible non-claim. Standalone-mode builds also carry the
`OmissionConfigHubUnitSubject` entry by default.

## What's in Every Receipt

| Field | Source |
|-------|--------|
| `_type` | Locked at `"https://in-toto.io/Statement/v1"` |
| `subject[]` | Always includes `k8s-live://<apiVersion>/<kind>/<namespace>/<name>`; connected mode with linkage adds `confighub-unit://<slug>@rev=<n>` |
| `predicateType` | Locked at `"https://cub-scout.dev/receipt/v1"` |
| `predicate.scope` | `kind / name / namespace / cluster` from the CLI |
| `predicate.verifier` | `{ tool: "cub-scout", version: <build tag> }` |
| `predicate.verifiedAt` | RFC 3339 UTC timestamp |
| `predicate.predicateName` | `"applied-matches-spec"` (v1 batch 1) |
| `predicate.spec.anchor` | Git repo + revision + path (the desired-state anchor) |
| `predicate.verdict` | `PASS \| WATCH \| BLOCK \| INCONCLUSIVE` |
| `predicate.evidence` | Attribution + gitSource (passed through from the existing #435 attribution layer) |
| `predicate.omissions[]` | Explicit non-claims — always present; empty means comprehensive coverage |
| `predicate.inputAttestations[]` | Reserved for chain composition; v1 emits `[]` |
| `predicate.nextSteps[]` | Read-only follow-up hints; mutating `actionType` or `nextCommand` is rejected at receipt-emit time |
| `predicate.fingerprint` | `sha256:` SHA-256 over RFC 8785 canonical JSON of the full Statement minus only this field |

## Read-Only Triad Lock

The receipt invariant from `#410` / `#428` / `#446`:

- cub-scout emits artifacts; it never mutates the cluster, ConfigHub, or
  any external store.
- The receipt package uses only `Get` / `List` / `Watch` on the K8s API.
  A static guard (`TestReceiptPackageReadOnlyClient` in
  `cmd/cub-scout/`) fails the build if any mutating client method
  appears in the receipt source files.
- `FilterNextSteps` drops mutating `actionType` (`"mutating"`) and
  mutating `nextCommand` substrings (`apply`, `edit`, `patch`, `delete`,
  `sync`, `create`, `update`, `replace`, `scale`, `rollout`,
  `reconcile`) before the receipt is fingerprinted. Defense in depth.

## Verifying the Fingerprint

The fingerprint covers the full Statement minus only the fingerprint
field itself. Tampering with any other field — subject digest, verdict,
predicate type, even the `_type` envelope — will fail verification.

```go
import "github.com/confighub/cub-scout/pkg/agent"

if err := agent.VerifyStatementFingerprint(stmt); err != nil {
    // The receipt has been tampered with or was malformed at emit time.
}
```

## Regenerating These Examples

The four JSON files are generated deterministically by the helper at
[`tools/gen-receipt-examples/`](../../../tools/gen-receipt-examples/).
The CI test `TestReceiptExamplesAreFresh` (in
`cmd/cub-scout/receipt_examples_test.go`) re-runs the generator and
fails if the committed files have drifted from what the generator
produces — the examples are a contract surface, not loose
documentation.

```bash
go run ./tools/gen-receipt-examples examples/receipts/applied-matches-spec
```

## References

- Parent issue: [`confighub/cub-scout#446`](https://github.com/confighub/cub-scout/issues/446) — Receipt capability
- Locked design: [`docs/proposals/receipts-way-forward.md`](../../../docs/proposals/receipts-way-forward.md)
- Contract: [`docs/reference/json-contracts.md`](../../../docs/reference/json-contracts.md) § Receipt Contract
- Implementation: `pkg/agent/receipt*.go`, `cmd/cub-scout/receipt*.go`
- Read-only triad: `#410` / `#428` (cub-scout = evidence; ConfigHub = authority; Pilot = judge)
