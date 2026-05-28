// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Package-level receipt types implementing the design locked in #446.
//
// A receipt is the typed, fingerprinted, immutable evidence object cub-scout
// emits to prove "what was true at time T about this resource." See
// docs/proposals/receipts-way-forward.md for the full design synthesis.
//
// Wire format is in-toto Statement v1 (the universal envelope used by SLSA,
// Sigstore, Kyverno, Tekton Chains, etc.) wrapping a cub-scout-specific
// predicate. v1 ships fingerprint-only; DSSE signing + Sigstore Bundle v0.3
// are v2 work that is purely additive on this envelope.
//
// Read-only triad lock (#410/#428): nothing in this package mutates the
// cluster, ConfigHub, or any external store. Receipts emit; consumers
// persist.

package agent

const (
	// StatementType is the in-toto Statement v1 envelope type URI. Locked
	// at v1 so v2 signing can wrap this in DSSE without changing the
	// envelope. See https://github.com/in-toto/attestation/blob/main/spec/v1/statement.md
	StatementType = "https://in-toto.io/Statement/v1"

	// PredicateTypeReceiptV1 is the cub-scout predicate URI. Resolved per
	// the locked design decision: cub-scout owns this URI; upstream
	// in-toto vetted-predicate contribution is a v2+ ambition.
	PredicateTypeReceiptV1 = "https://cub-scout.dev/receipt/v1"

	// PredicateVersion is the cub-scout predicate body schema version.
	// Bump on breaking changes; additive changes do not bump.
	PredicateVersion = "v1"

	// SubjectSchemeK8sLive is the URI scheme for the live K8s resource
	// subject. Names look like "k8s-live://apps/v1/Deployment/prod/api".
	SubjectSchemeK8sLive = "k8s-live://"

	// SubjectSchemeConfigHubUnit is the URI scheme for the ConfigHub Unit
	// subject. Names look like "confighub-unit://payments-api@rev=42".
	// Standalone-mode receipts omit this subject with an omissions[] entry.
	SubjectSchemeConfigHubUnit = "confighub-unit://"

	// SubjectSchemeSyntheticAggregate is the URI scheme for the aggregate-
	// receipt subject (#448). The aggregate receipt is a Statement over a
	// synthetic subject computed deterministically from the SHA-256
	// digests of its input attestations (the per-resource receipts the
	// aggregate composes). Names look like
	//   synthetic-aggregate://sha256/<aggregate-id>
	// where <aggregate-id> is the first 16 hex chars of the digest, and
	// the full SHA-256 lives in the subject's digest map.
	//
	// The aggregate receipt's verdict is synthesized deterministically
	// over the input verdicts (max-severity by default; #448's
	// `--aggregate-policy` flag may add alternative synthesis rules in
	// future). See `BuildSyntheticAggregateSubject` in
	// `pkg/agent/receipt_aggregate.go`.
	SubjectSchemeSyntheticAggregate = "synthetic-aggregate://"

	// SubjectSchemeRenderedObjectSet identifies the desired rendered
	// manifest set that an object-set receipt evaluated. Names look like
	//   rendered-object-set://sha256/<short-digest>
	// with the full SHA-256 in the subject digest map.
	SubjectSchemeRenderedObjectSet = "rendered-object-set://"

	// SubjectSchemeK8sLiveObjectSet identifies the live projection of
	// the Kubernetes objects cub-scout observed for an object-set
	// receipt. Names look like
	//   k8s-live-object-set://namespace/redis
	// or k8s-live-object-set://cluster.
	SubjectSchemeK8sLiveObjectSet = "k8s-live-object-set://"
)

// Statement is the in-toto Statement v1 envelope wrapping a cub-scout
// receipt predicate. The struct field order follows the canonical JSON
// shape consumers expect; field NAMES are what matters for canonical
// serialization, not declaration order.
type Statement struct {
	Type          string    `json:"_type"`
	Subject       []Subject `json:"subject"`
	PredicateType string    `json:"predicateType"`
	Predicate     Predicate `json:"predicate"`
}

// Subject identifies what the receipt is about. cub-scout receipts have
// either one subject (standalone) or two (connected — live + ConfigHub
// unit). Digest is SHA-256 over the canonical representation of the
// subject (see CanonicalSubjectDigest in receipt_subject.go).
type Subject struct {
	Name   string            `json:"name"`
	Digest map[string]string `json:"digest"`
}

// Predicate is the cub-scout receipt body. See
// docs/reference/json-contracts.md § Receipt Contract for the full field
// reference.
type Predicate struct {
	// Version is the predicate body schema version (currently "v1").
	Version string `json:"version"`

	// Claim is a free-text human-readable assertion (e.g., "applied
	// matches spec at apps/prod/api/deployment.yaml").
	Claim string `json:"claim"`

	// Scope identifies the resource the predicate was evaluated against.
	Scope Scope `json:"scope"`

	// Verifier identifies the tool that produced the receipt.
	Verifier Verifier `json:"verifier"`

	// VerifiedAt is the ISO 8601 timestamp of receipt creation.
	VerifiedAt string `json:"verifiedAt"`

	// PredicateName is the predicate evaluated (e.g.,
	// "applied-matches-spec"). See AllPredicates() in
	// receipt_predicates.go for the v1 list.
	PredicateName string `json:"predicateName"`

	// Spec is the desired-state anchor the predicate evaluated against,
	// when one applies (e.g., git repo + revision + path for applied-
	// matches-spec). Omitted when the predicate is anchor-less.
	Spec *SpecAnchor `json:"spec,omitempty"`

	// Verdict is the receipt-level conclusion. One of PASS, WATCH, BLOCK,
	// INCONCLUSIVE.
	Verdict ReceiptVerdict `json:"verdict"`

	// Evidence carries the underlying evidence the predicate evaluated.
	// At v1 this is compareThreeWay output + attribution + sourceTruth +
	// gitSource. Receipts pass evidence through verbatim; the receipt
	// envelope's stability is about the envelope, not the evidence body
	// shape (which may evolve as cub-scout's evidence surfaces evolve).
	Evidence Evidence `json:"evidence"`

	// Omissions is the explicit list of what the receipt deliberately
	// does NOT claim. Required field. Empty array means comprehensive
	// coverage; non-empty entries mean the verdict was reached with
	// known gaps. See docs/reference/json-contracts.md.
	Omissions []Omission `json:"omissions"`

	// InputAttestations references other receipts by digest for chain
	// semantics (e.g., the upstream layer-1 governance-of-mutation
	// receipt this Runtime-layer receipt depends on). v1 emits [];
	// composition lands in #448 (aggregate / chained receipts v2).
	InputAttestations []AttestationRef `json:"inputAttestations"`

	// NextSteps is structured read-only follow-up hints. Reuses the
	// nextSteps[] shape from #370. Mutating actionType is rejected at
	// receipt-emit time — see RejectMutatingNextSteps in
	// receipt_predicates.go.
	NextSteps []ReceiptNextStep `json:"nextSteps,omitempty"`

	// Fingerprint is SHA-256 over RFC 8785 canonical JSON of the full
	// Statement (_type + subject + predicateType + predicate) minus only
	// this Fingerprint field itself. Hashing predicate-only would leave
	// subject and predicateType unprotected; the full-Statement scope
	// closes that. See receipt_fingerprint.go.
	Fingerprint string `json:"fingerprint"`
}

// Scope identifies the K8s resource the receipt is about.
type Scope struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Cluster   string `json:"cluster,omitempty"`
}

// Verifier identifies the tool that produced the receipt.
type Verifier struct {
	Tool    string `json:"tool"`    // "cub-scout"
	Version string `json:"version"` // e.g. "v2.3.0"
}

// SpecAnchor is the desired-state anchor a predicate evaluated against.
// AnchorType determines which fields are populated (Git, OCI, ConfigHub
// unit revision). At v1 only "git" anchors are populated by the
// applied-matches-spec evaluator; OCI / ConfigHub anchors are valid in
// the schema for forward compat.
type SpecAnchor struct {
	Anchor SpecAnchorBody `json:"anchor"`
}

// SpecAnchorBody is the typed body of a spec anchor. Use one of the
// shapes; the other fields stay zero-valued and serialize as omitted.
type SpecAnchorBody struct {
	Type    string `json:"type"` // "git" | "oci" | "confighub-unit"
	RepoURL string `json:"repoUrl,omitempty"`
	// Revision is a Git SHA (for type=git), an OCI digest (for type=oci),
	// or a ConfigHub unit revision number (for type=confighub-unit).
	Revision string `json:"revision,omitempty"`
	Path     string `json:"path,omitempty"`
	// File + Line populate when stage B back-resolution (#440) finds the
	// specific manifest file line that sets the value.
	File string `json:"file,omitempty"`
	Line int    `json:"line,omitempty"`
}

// ReceiptVerdict is the receipt-level conclusion. cub-scout maps native
// source-truth status + verdict pairs to this four-valued enum; the
// native pair is preserved in Evidence.SourceTruth for consumers that
// need the full fidelity. See receipt_predicates.go MapSourceTruth.
type ReceiptVerdict string

const (
	// VerdictPASS means evidence supports the claim.
	VerdictPASS ReceiptVerdict = "PASS"

	// VerdictWATCH means evidence is ambiguous; situation needs
	// monitoring. Includes the source-truth ASK case at v1.
	VerdictWATCH ReceiptVerdict = "WATCH"

	// VerdictBLOCK means evidence contradicts the claim.
	VerdictBLOCK ReceiptVerdict = "BLOCK"

	// VerdictINCONCLUSIVE means evidence is missing or unavailable
	// (source-truth INCOMPLETE/BLOCKED/UNKNOWN, managedFields stripped,
	// ConfigHub offline in standalone mode, etc.). INCONCLUSIVE always
	// carries one or more Omissions entries explaining what's missing.
	VerdictINCONCLUSIVE ReceiptVerdict = "INCONCLUSIVE"
)

// Evidence is the typed evidence body the predicate evaluated against.
// Fields are optional; receipts include only the evidence shapes the
// specific predicate consumed. CompareThreeWay is opaque at the
// pkg/agent layer (the structured compareResourceResult lives in
// cmd/cub-scout) so it's an interface{}; the CLI populates it.
type Evidence struct {
	CompareThreeWay interface{}               `json:"compareThreeWay,omitempty"`
	Attribution     *FieldMutationAttribution `json:"attribution,omitempty"`
	SourceTruth     *SourceTruthEvidence      `json:"sourceTruth,omitempty"`
	GitSource       *GitSourceAnchor          `json:"gitSource,omitempty"`
	ObjectSet       *ObjectSetEvidence        `json:"objectSet,omitempty"`
}

// Omission is one structured gap the receipt does not claim. The
// required field is Missing (what's missing); Reason is human-readable
// context; Severity is optional ("info" / "warning") for downstream
// triage.
type Omission struct {
	Missing  string `json:"missing"`
	Reason   string `json:"reason,omitempty"`
	Severity string `json:"severity,omitempty"`
}

// AttestationRef is a reference to another receipt by digest, for chain
// semantics. v1 emits [] for InputAttestations; composition lands in
// #448.
type AttestationRef struct {
	URI    string            `json:"uri"`
	Digest map[string]string `json:"digest"`
}

// ReceiptNextStep is a structured read-only hint for the consumer.
// ActionType is restricted to non-mutating values at receipt-emit time
// (see receipt_predicates.go RejectMutatingNextSteps). Mirrors the
// shape introduced in #370 for explain / doctor / compare three-way.
type ReceiptNextStep struct {
	// ActionType is one of "read-only", "waiting", "human-decision".
	// "mutating" is rejected at receipt-emit time.
	ActionType string `json:"actionType"`
	// Reason is one-line human-readable rationale.
	Reason string `json:"reason"`
	// NextCommand is the suggested next shell invocation. Must be
	// non-mutating per the receipt invariant.
	NextCommand string `json:"nextCommand,omitempty"`
	// NextSurface is the surface the next command lives on (e.g.,
	// "cub-scout", "kubectl", "cub").
	NextSurface string `json:"nextSurface,omitempty"`
}

// Common omission Missing values, exposed as constants so callers and
// tests can reference them by name rather than free string.
const (
	OmissionConfigHubUnitSubject  = "confighub-unit-subject"
	OmissionManagedFields         = "managedFields"
	OmissionGitSourceAnchor       = "git-source-anchor"
	OmissionSourceTruthComplete   = "source-truth-complete"
	OmissionAutoDetectedPredicate = "auto-detected-predicate"
	OmissionFieldCoverage         = "field-coverage"

	// OmissionAggregatePartialCoverage is recorded by aggregate-verdict
	// receipts (#448) when one or more input attestations carried an
	// INCONCLUSIVE verdict. The aggregate verdict still synthesizes
	// (max-severity by default), but the consumer should know it may
	// not reflect full coverage — at least one underlying per-resource
	// receipt itself couldn't be built.
	OmissionAggregatePartialCoverage = "aggregate-partial-coverage"

	// OmissionStrategyMissing is recorded by the source-truth-pass
	// predicate when no --strategy was passed. cub-scout never infers a
	// strategy — the receipt either records the declared strategy or
	// declines to claim.
	OmissionStrategyMissing = "strategy"

	// OmissionStrategyMismatch is recorded when the receipt was asked to
	// verify under a strategy that disagrees with what the source-truth
	// evidence in the live cluster actually carries.
	OmissionStrategyMismatch = "strategy-mismatch"

	// OmissionSourceTruthEvidence is recorded by the source-truth-pass
	// predicate when the evidence body has no source-truth surface to
	// evaluate.
	OmissionSourceTruthEvidence = "source-truth-evidence"

	// OmissionSinceMissing is recorded by the no-manual-edits-since
	// predicate when no --since cutoff was passed. cub-scout will not
	// invent a cutoff.
	OmissionSinceMissing = "since"

	// OmissionManagedFieldsTime is recorded by the no-manual-edits-since
	// predicate when a managedFields entry has no Time stamp. Old K8s
	// versions or replayed objects can have entries with nil Time;
	// cub-scout cannot claim "no manual edits since T" if it cannot place
	// each entry on the timeline.
	OmissionManagedFieldsTime = "managedFields-time"

	// OmissionExtraLiveObjectCoverage is recorded by object-set receipts
	// when cub-scout verified every desired object from the rendered
	// manifest set but did not enumerate unexpected live resources that
	// are outside the desired object identity set.
	OmissionExtraLiveObjectCoverage = "extra-live-object-coverage"

	// OmissionObjectSetCoverage is recorded by object-set receipts when
	// one or more desired objects could not be checked because the API
	// mapping or live read was inconclusive.
	OmissionObjectSetCoverage = "object-set-coverage"
)
