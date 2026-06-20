// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"fmt"
	"sort"
	"time"
)

// ObjectSetDiffEvidence is attached under predicate.evidence.objectSetDiff for
// object-set-diff receipts (#496, Issue A). It is the set-level delta between a
// rendered (desired) object set and live cluster state: per-object authored
// field deltas plus closure deltas (objects added or removed relative to the
// desired set). Both a dry-run ("what would this change touch across the set?")
// and a drift check ("what differs across the set now?") emit this shape — the
// only difference is which side is the "current render".
type ObjectSetDiffEvidence struct {
	DesiredSource  ObjectSetSource       `json:"desiredSource"`
	Scope          ObjectSetScope        `json:"scope"`
	DesiredDigest  string                `json:"desiredDigest"`
	LiveDigest     string                `json:"liveDigest"`
	ChangedObjects []ObjectSetDiffObject `json:"changedObjects,omitempty"`
	AddedObjects   []ObjectSetObjectID   `json:"addedObjects,omitempty"`
	RemovedObjects []ObjectSetObjectID   `json:"removedObjects,omitempty"`
	Summary        ObjectSetDiffSummary  `json:"summary"`

	// NormalizationProfile names the server-normalization profile applied
	// symmetrically to the desired and live objects before projection and
	// digesting (e.g. k8s-zero-defaults/v1). Empty means raw comparison.
	// Recording it makes the normalization assumption part of the claim.
	NormalizationProfile string `json:"normalizationProfile,omitempty"`
}

// ObjectSetDiffObject is one desired object whose authored fields differ from
// live, with the field-level deltas that drove the difference.
type ObjectSetDiffObject struct {
	ID            ObjectSetObjectID    `json:"id"`
	DesiredDigest string               `json:"desiredDigest,omitempty"`
	LiveDigest    string               `json:"liveDigest,omitempty"`
	Differences   []ObjectSetFieldDiff `json:"differences,omitempty"`
}

// ObjectSetFieldDiff is one authored-field delta on a changed object. Cause and
// ManagerHint carry the writing field-manager attribution when it is available
// (drift attributed to a controller/tool); they are optional.
type ObjectSetFieldDiff struct {
	Field   string `json:"field"`
	Desired string `json:"desired"`
	Live    string `json:"live"`
	// Cause classifies the mutation source for this field delta (e.g.
	// controller-drift, manual-edit). Optional; empty when no managedFields
	// attribution mapped to this field.
	Cause string `json:"cause,omitempty"`
	// ManagerHint is a representative field-manager string for transparency.
	// Optional; empty when unknown.
	ManagerHint string `json:"managerHint,omitempty"`
}

// ObjectSetDiffSummary is the set-level roll-up: how many desired objects were
// compared, how many changed (field deltas), and the closure deltas.
type ObjectSetDiffSummary struct {
	Total   int `json:"total"`
	Changed int `json:"changed"`
	Added   int `json:"added"`
	Removed int `json:"removed"`
}

// BuildObjectSetDiffReceiptInput is the pure input to BuildObjectSetDiffReceipt.
// Callers do file and cluster reads, then assemble the evidence, before
// invoking this function.
type BuildObjectSetDiffReceiptInput struct {
	Evidence          ObjectSetDiffEvidence
	Verifier          Verifier
	VerifiedAt        time.Time
	InputAttestations []VerifiedAttestationRef
}

// BuildObjectSetDiffReceipt builds a single set-level diff receipt for a
// rendered manifest set compared against live cluster state. It mirrors
// BuildObjectSetReceipt: same subject schemes (rendered-object-set:// desired +
// k8s-live-object-set:// live), the object-set-diff predicate, and the shared
// fingerprint stamper so the receipt is signed and chain-walkable. It is pure.
//
// Verdict rule (#496, Issue A):
//   - BLOCK         any object has field-level Differences (real authored drift).
//   - WATCH         only closure deltas (addedObjects / removedObjects) and no
//     changed objects — the set's membership differs but every
//     shared object's authored fields match.
//   - PASS          no changed, added, or removed objects.
//
// INCONCLUSIVE is reserved for load/build errors surfaced by the caller before
// this function (e.g. an inconclusive live lookup), which the caller maps to an
// error return; this builder itself only enumerates PASS / WATCH / BLOCK over
// well-formed evidence.
func BuildObjectSetDiffReceipt(in BuildObjectSetDiffReceiptInput) (Statement, error) {
	if in.Evidence.DesiredDigest == "" {
		return Statement{}, fmt.Errorf("object-set-diff receipt: missing desired digest")
	}
	if in.Evidence.LiveDigest == "" {
		return Statement{}, fmt.Errorf("object-set-diff receipt: missing live digest")
	}

	verifiedAt := in.VerifiedAt
	if verifiedAt.IsZero() {
		verifiedAt = time.Now().UTC()
	}

	// Sort the evidence collections deterministically so the canonical JSON
	// (and therefore the fingerprint) is stable regardless of input order.
	sort.Slice(in.Evidence.ChangedObjects, func(i, j int) bool {
		return in.Evidence.ChangedObjects[i].ID.Key() < in.Evidence.ChangedObjects[j].ID.Key()
	})
	sort.Slice(in.Evidence.AddedObjects, func(i, j int) bool {
		return in.Evidence.AddedObjects[i].Key() < in.Evidence.AddedObjects[j].Key()
	})
	sort.Slice(in.Evidence.RemovedObjects, func(i, j int) bool {
		return in.Evidence.RemovedObjects[i].Key() < in.Evidence.RemovedObjects[j].Key()
	})

	changed := len(in.Evidence.ChangedObjects)
	added := len(in.Evidence.AddedObjects)
	removed := len(in.Evidence.RemovedObjects)
	in.Evidence.Summary.Changed = changed
	in.Evidence.Summary.Added = added
	in.Evidence.Summary.Removed = removed

	verdict := VerdictPASS
	switch {
	case changed > 0:
		verdict = VerdictBLOCK
	case added > 0 || removed > 0:
		verdict = VerdictWATCH
	}

	omissions := []Omission{}
	// The diff is over authored fields of the desired set; it never claims
	// the live cluster contains nothing else unless closure deltas were
	// computed (--diff). Record the boundary honestly.
	if added == 0 && removed == 0 {
		omissions = append(omissions, Omission{
			Missing:  OmissionObjectSetDiffClosure,
			Reason:   "object-set-diff compares authored fields of the desired set against live; addedObjects/removedObjects closure deltas are only computed when the closed-world diff (--diff) is requested",
			Severity: "info",
		})
	}

	nextSteps := []ReceiptNextStep{}
	switch verdict {
	case VerdictPASS:
		nextSteps = append(nextSteps, ReceiptNextStep{
			ActionType:  "read-only",
			Reason:      "no authored-field, added, or removed deltas between the desired object set and live — the change touches nothing across the set",
			NextCommand: "cub-scout doctor",
			NextSurface: "cub-scout",
		})
	case VerdictWATCH:
		nextSteps = append(nextSteps, ReceiptNextStep{
			ActionType:  "read-only",
			Reason:      fmt.Sprintf("no shared object's authored fields changed, but the set membership differs (%d added, %d removed) — review the closure deltas before accepting", added, removed),
			NextCommand: "cub-scout map list",
			NextSurface: "cub-scout",
		})
	case VerdictBLOCK:
		nextSteps = append(nextSteps, ReceiptNextStep{
			ActionType:  "read-only",
			Reason:      fmt.Sprintf("%d object(s) have authored-field deltas between the desired set and live; inspect changedObjects before applying or accepting the drift", changed),
			NextCommand: "cub-scout compare three-way",
			NextSurface: "cub-scout",
		})
	}
	filteredSteps, stepOmissions := FilterNextSteps(nextSteps)
	omissions = append(omissions, stepOmissions...)

	inputAttestations := make([]AttestationRef, 0, len(in.InputAttestations))
	for i, v := range in.InputAttestations {
		if v.IsZero() {
			return Statement{}, fmt.Errorf("object-set-diff receipt: inputAttestations[%d] is a zero-value VerifiedAttestationRef; construct via BuildAttestationRef or BuildAttestationRefsFromPaths", i)
		}
		inputAttestations = append(inputAttestations, v.Ref())
	}

	scope := Scope{
		Kind:      "ObjectSet",
		Name:      "rendered-install",
		Namespace: in.Evidence.Scope.Namespace,
	}
	claim := "object-set diff between rendered manifests and live"
	if in.Evidence.DesiredSource.Ref != "" {
		claim = fmt.Sprintf("object-set diff between rendered manifests from %s and live", in.Evidence.DesiredSource.Ref)
	}

	stmt := Statement{
		Type: StatementType,
		Subject: []Subject{
			BuildRenderedObjectSetSubject(in.Evidence.DesiredDigest),
			BuildLiveObjectSetSubject(in.Evidence.Scope, in.Evidence.LiveDigest),
		},
		PredicateType: PredicateTypeReceiptV1,
		Predicate: Predicate{
			Version:           PredicateVersion,
			Claim:             claim,
			Scope:             scope,
			Verifier:          in.Verifier,
			VerifiedAt:        verifiedAt.UTC().Format(time.RFC3339),
			PredicateName:     string(PredicateObjectSetDiff),
			Verdict:           verdict,
			Evidence:          Evidence{ObjectSetDiff: &in.Evidence},
			Omissions:         omissions,
			InputAttestations: inputAttestations,
			NextSteps:         filteredSteps,
		},
	}

	if err := StampFingerprint(&stmt); err != nil {
		return Statement{}, fmt.Errorf("object-set-diff receipt: stamp fingerprint: %w", err)
	}
	return stmt, nil
}
