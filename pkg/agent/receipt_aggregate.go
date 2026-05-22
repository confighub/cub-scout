// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

// receipt_aggregate.go — the aggregate-with-discovery half of #448.
//
// An aggregate receipt is a Statement that composes N per-resource
// receipts into a single fingerprinted evidence artifact. The subject
// is `synthetic-aggregate://sha256/<short-id>` (deterministically
// derived from the inputs); inputAttestations[] references each
// per-resource receipt by digest; the verdict is synthesized
// deterministically over the input verdicts via a configurable policy
// (max-severity by default).
//
// Use cases (per #448):
//   - `cub-scout receipt verify --scope namespace/<ns>` — auto-discover
//     all workloads in the namespace, build N per-resource receipts,
//     emit one aggregate over the set.
//   - `cub-scout receipt verify <a>,<b>,<c> -n <ns>` — comma-list batch
//     for an explicit set of resources.
//   - Multi-stage delivery chains: aggregate the per-stage receipts.
//
// The CLI half lives in `cmd/cub-scout/receipt_aggregate.go`. This
// file is the pure-construction surface (no cluster reads).
//
// Wire-format note:
//
//   - Subject: `synthetic-aggregate://sha256/<aggregate-id>` (the
//     aggregate-id is the first 16 hex chars of the deterministic
//     digest; the full SHA-256 lives in the subject's digest map).
//   - PredicateName: `aggregate-verdict` (a new predicate distinct
//     from the per-resource predicates).
//   - Verdict: synthesized over the input verdicts per
//     `AggregateVerdictPolicy` (default: max-severity).
//   - inputAttestations[]: VerifiedAttestationRef for each input
//     receipt; each is fingerprint-verified at chain construction
//     time (same API-boundary check as `BuildAttestationRef`).
//   - Omissions: aggregate-level entries (e.g., "1 of N input
//     receipts is INCONCLUSIVE; aggregate verdict may not reflect
//     full coverage").
//   - Scope: a synthetic scope describing what the aggregate covers
//     (namespace-shaped or batch-shaped); the underlying per-resource
//     scopes live on the input attestations.

// PredicateAggregateVerdict is the predicate name for aggregate
// receipts (#448). Distinct from the per-resource predicates
// (`applied-matches-spec`, `source-truth-pass`,
// `no-manual-edits-since`) — the aggregate doesn't evaluate evidence,
// it synthesizes a verdict over input receipts.
const PredicateAggregateVerdict PredicateName = "aggregate-verdict"

// AggregateVerdictPolicy controls how the aggregate verdict is
// synthesized from the input receipts' verdicts. Implementations are
// deterministic — same inputs always yield the same verdict.
//
// v1 ships max-severity only. Future policies (majority, weighted)
// may attach to `cub-scout receipt verify --aggregate-policy <name>`.
type AggregateVerdictPolicy interface {
	// Name returns the policy's stable identifier (lands in the
	// aggregate receipt's `predicate.claim` so consumers know which
	// rule fired).
	Name() string

	// Synthesize returns the aggregate verdict over the inputs.
	// Inputs are expected to be a non-empty slice; an empty slice
	// produces INCONCLUSIVE.
	Synthesize(verdicts []ReceiptVerdict) ReceiptVerdict
}

// MaxSeverityPolicy is the default aggregate-verdict policy. It picks
// the most-severe verdict in the input set:
//
//	BLOCK > INCONCLUSIVE > WATCH > PASS
//
// Rationale: any BLOCK is a hard contradiction — one bad apple
// invalidates the aggregate. INCONCLUSIVE outranks WATCH because
// missing evidence is "louder" than ambiguous evidence (the consumer
// needs to fix the gap before they can claim coverage).
//
// Empty input slice → INCONCLUSIVE (the aggregate can't be evaluated
// without at least one input).
type MaxSeverityPolicy struct{}

// Name returns "max-severity".
func (MaxSeverityPolicy) Name() string { return "max-severity" }

// Synthesize returns the most-severe verdict (BLOCK > INCONCLUSIVE >
// WATCH > PASS). Empty input returns INCONCLUSIVE.
func (MaxSeverityPolicy) Synthesize(verdicts []ReceiptVerdict) ReceiptVerdict {
	if len(verdicts) == 0 {
		return VerdictINCONCLUSIVE
	}
	// Severity rank: higher = more severe. Unknown verdicts default
	// to 0 (treat as PASS so a typo doesn't accidentally upgrade the
	// aggregate to BLOCK).
	rank := map[ReceiptVerdict]int{
		VerdictPASS:         1,
		VerdictWATCH:        2,
		VerdictINCONCLUSIVE: 3,
		VerdictBLOCK:        4,
	}
	worst := VerdictPASS
	worstRank := 0
	for _, v := range verdicts {
		r := rank[v]
		if r > worstRank {
			worst = v
			worstRank = r
		}
	}
	return worst
}

// AggregateScopeSpec describes what the aggregate receipt covers. The
// CLI translates `--scope namespace/<ns>` and `<a>,<b>,<c>` into one
// of these shapes; the aggregate's predicate.scope carries the
// derived summary.
type AggregateScopeSpec struct {
	// Kind is one of "namespace" or "batch". Other shapes (cluster,
	// view) may land in future v2 work.
	Kind string

	// Namespace is set when Kind is "namespace"; the namespace the
	// aggregate was discovered from.
	Namespace string

	// MemberCount is the number of input attestations the aggregate
	// composes. Used in the derived claim.
	MemberCount int
}

// BuildSyntheticAggregateSubject constructs the synthetic-aggregate
// subject for an aggregate receipt. The subject's name and digest are
// both derived from the input attestations' digests:
//
//   - Concatenate each input's sha256 digest hex (without the
//     "sha256:" prefix), in sorted order for determinism, separated
//     by newlines.
//   - SHA-256 over the concatenated bytes is the aggregate digest.
//   - The first 16 hex chars of the digest are the aggregate-id in
//     the subject URI.
//
// Sorting the inputs by digest hex makes the aggregate subject
// order-independent — re-running the verify with the inputs in a
// different order produces the same subject (the aggregate is a set,
// not a list).
//
// Returns an error if refs is empty (an aggregate over zero inputs
// is meaningless) or if any ref has an empty sha256 digest.
func BuildSyntheticAggregateSubject(refs []VerifiedAttestationRef) (Subject, error) {
	if len(refs) == 0 {
		return Subject{}, fmt.Errorf("synthetic-aggregate subject: empty refs (aggregate must compose at least one input)")
	}

	digests := make([]string, 0, len(refs))
	for i, r := range refs {
		raw := r.Ref()
		hex := strings.TrimSpace(raw.Digest["sha256"])
		if hex == "" {
			return Subject{}, fmt.Errorf("synthetic-aggregate subject: input attestation %d has empty sha256 digest", i)
		}
		digests = append(digests, hex)
	}
	sort.Strings(digests)

	canon := strings.Join(digests, "\n")
	sum := sha256.Sum256([]byte(canon))
	fullDigest := hex.EncodeToString(sum[:])
	aggregateID := fullDigest[:16]

	subjectName := fmt.Sprintf("%ssha256/%s", SubjectSchemeSyntheticAggregate, aggregateID)

	return Subject{
		Name:   subjectName,
		Digest: map[string]string{"sha256": fullDigest},
	}, nil
}

// BuildAggregateReceiptInput is the construction input for
// BuildAggregateReceipt.
type BuildAggregateReceiptInput struct {
	// Inputs are the per-resource VerifiedAttestationRefs to compose.
	// Required (at least one). Each must be a wrapper produced via
	// BuildAttestationRef (i.e., the source Statement's fingerprint
	// was verified at construction time).
	Inputs []VerifiedAttestationRef

	// InputVerdicts mirrors Inputs — the verdict from each input
	// receipt, used by the synthesis Policy. Same order as Inputs;
	// caller is responsible for keeping them aligned. Length mismatch
	// is an error.
	//
	// We don't read the verdict from the Inputs themselves because
	// VerifiedAttestationRef wraps only the URI + digest — the full
	// Statement isn't reachable from a ref alone. The caller (which
	// has both the loaded Statement and the constructed ref) supplies
	// both.
	InputVerdicts []ReceiptVerdict

	// Scope describes what the aggregate covers (namespace, batch).
	// Required.
	Scope AggregateScopeSpec

	// Policy is the verdict-synthesis policy. Defaults to
	// MaxSeverityPolicy when nil.
	Policy AggregateVerdictPolicy

	// Verifier identifies the tool building the receipt. Required.
	Verifier Verifier

	// VerifiedAt is the receipt-build timestamp. Zero value uses
	// time.Now().UTC().
	VerifiedAt time.Time

	// Connected is the connected-mode signal. Aggregate receipts
	// don't add a confighub-unit:// subject (the underlying inputs
	// carry their own). Used here only to populate Predicate.
	Connected bool

	// Omissions is any caller-supplied aggregate-level omission set.
	// BuildAggregateReceipt may append further entries (e.g., "1 of N
	// inputs is INCONCLUSIVE") but never overwrites this slice.
	Omissions []Omission
}

// BuildAggregateReceipt composes N input attestations into a single
// aggregate receipt with a synthesized verdict.
//
// Errors:
//   - len(Inputs) == 0 (the aggregate must compose at least one input)
//   - len(InputVerdicts) != len(Inputs)
//   - any input is the zero value (forgery attempt; same API-boundary
//     check as BuildReceipt's VerifiedAttestationRef handling)
//   - Scope.Kind is empty or invalid
//   - Verifier is empty (Tool or Version unset)
func BuildAggregateReceipt(in BuildAggregateReceiptInput) (Statement, error) {
	if len(in.Inputs) == 0 {
		return Statement{}, fmt.Errorf("build-aggregate-receipt: empty Inputs")
	}
	if len(in.InputVerdicts) != len(in.Inputs) {
		return Statement{}, fmt.Errorf(
			"build-aggregate-receipt: InputVerdicts length %d does not match Inputs length %d",
			len(in.InputVerdicts), len(in.Inputs),
		)
	}
	for i, v := range in.Inputs {
		if v.IsZero() {
			return Statement{}, fmt.Errorf(
				"build-aggregate-receipt: Inputs[%d] is a zero-value VerifiedAttestationRef; construct via BuildAttestationRef so the source receipt's fingerprint is verified before chaining",
				i,
			)
		}
	}
	if in.Scope.Kind == "" {
		return Statement{}, fmt.Errorf("build-aggregate-receipt: empty Scope.Kind (expected \"namespace\" or \"batch\")")
	}
	if in.Verifier.Tool == "" {
		return Statement{}, fmt.Errorf("build-aggregate-receipt: empty Verifier.Tool")
	}

	// Resolve policy default.
	policy := in.Policy
	if policy == nil {
		policy = MaxSeverityPolicy{}
	}

	// Build the synthetic subject.
	subject, err := BuildSyntheticAggregateSubject(in.Inputs)
	if err != nil {
		return Statement{}, fmt.Errorf("build-aggregate-receipt: %w", err)
	}

	// Synthesize the aggregate verdict.
	verdict := policy.Synthesize(in.InputVerdicts)

	// Append aggregate-level omissions: when any input is
	// INCONCLUSIVE, the consumer should know the aggregate verdict
	// may not reflect full coverage (an INCONCLUSIVE input means the
	// receipt itself couldn't be built; the aggregate covers it but
	// without a real per-resource verdict).
	omissions := append([]Omission{}, in.Omissions...)
	inconclusiveCount := 0
	for _, v := range in.InputVerdicts {
		if v == VerdictINCONCLUSIVE {
			inconclusiveCount++
		}
	}
	if inconclusiveCount > 0 {
		omissions = append(omissions, Omission{
			Missing: OmissionAggregatePartialCoverage,
			Reason: fmt.Sprintf(
				"%d of %d input attestation(s) carried verdict INCONCLUSIVE; aggregate verdict may not reflect full coverage",
				inconclusiveCount, len(in.InputVerdicts),
			),
			Severity: "warning",
		})
	}

	// Build the claim string.
	claim := deriveAggregateClaim(in.Scope, verdict, policy.Name(), len(in.Inputs))

	// Derive scope summary for the aggregate's predicate.scope. The
	// per-resource scopes live on the input attestations themselves.
	scope := Scope{
		Kind:      in.Scope.Kind,
		Namespace: in.Scope.Namespace,
		// Name is left empty — the aggregate doesn't have a single
		// resource name; the member count is the meaningful signal.
	}

	// Timestamp.
	verifiedAt := in.VerifiedAt
	if verifiedAt.IsZero() {
		verifiedAt = time.Now().UTC()
	}

	// Unwrap input attestations to the wire shape.
	inputAttestations := make([]AttestationRef, 0, len(in.Inputs))
	for _, v := range in.Inputs {
		inputAttestations = append(inputAttestations, v.Ref())
	}

	pred := Predicate{
		Version:           PredicateVersion,
		Claim:             claim,
		Scope:             scope,
		Verifier:          in.Verifier,
		VerifiedAt:        verifiedAt.UTC().Format(time.RFC3339),
		PredicateName:     string(PredicateAggregateVerdict),
		Verdict:           verdict,
		Evidence:          Evidence{}, // aggregates don't carry per-resource evidence; it lives on inputs
		Omissions:         omissions,
		InputAttestations: inputAttestations,
		NextSteps:         []ReceiptNextStep{}, // aggregates don't recommend; consumers drill into inputs
	}

	stmt := Statement{
		Type:          StatementType,
		Subject:       []Subject{subject},
		PredicateType: PredicateTypeReceiptV1,
		Predicate:     pred,
	}

	if err := StampFingerprint(&stmt); err != nil {
		return Statement{}, fmt.Errorf("build-aggregate-receipt: stamp fingerprint: %w", err)
	}

	return stmt, nil
}

// deriveAggregateClaim builds the human-readable claim for the
// aggregate receipt's `predicate.claim`. Examples:
//
//	"aggregate verdict PASS over 12 receipt(s) in namespace prod (policy=max-severity)"
//	"aggregate verdict BLOCK over 3 receipt(s) (batch; policy=max-severity)"
func deriveAggregateClaim(scope AggregateScopeSpec, verdict ReceiptVerdict, policyName string, memberCount int) string {
	switch scope.Kind {
	case "namespace":
		return fmt.Sprintf(
			"aggregate verdict %s over %d receipt(s) in namespace %s (policy=%s)",
			verdict, memberCount, scope.Namespace, policyName,
		)
	case "batch":
		return fmt.Sprintf(
			"aggregate verdict %s over %d receipt(s) (batch; policy=%s)",
			verdict, memberCount, policyName,
		)
	default:
		return fmt.Sprintf(
			"aggregate verdict %s over %d receipt(s) (scope=%s; policy=%s)",
			verdict, memberCount, scope.Kind, policyName,
		)
	}
}
