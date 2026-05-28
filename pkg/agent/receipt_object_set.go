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

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	ObjectSetMatchModeAuthoredFields = "authored-fields"

	ObjectSetObjectMatched      = "matched"
	ObjectSetObjectMissing      = "missing"
	ObjectSetObjectMismatched   = "mismatched"
	ObjectSetObjectInconclusive = "inconclusive"
)

// ObjectSetSource identifies the desired rendered manifest input used by an
// object-set receipt.
type ObjectSetSource struct {
	Type        string `json:"type"` // "file" | "directory"
	Ref         string `json:"ref"`
	Digest      string `json:"digest,omitempty"`
	ObjectCount int    `json:"objectCount"`
}

// ObjectSetScope records the live scope used to fetch objects. The namespace
// may be empty when the set contains only cluster-scoped objects.
type ObjectSetScope struct {
	Kind      string `json:"kind"` // "namespace" | "cluster" | "mixed"
	Namespace string `json:"namespace,omitempty"`
}

// ObjectSetEvidence is attached under predicate.evidence.objectSet for
// object-set-matches receipts.
type ObjectSetEvidence struct {
	DesiredSource ObjectSetSource          `json:"desiredSource"`
	Scope         ObjectSetScope           `json:"scope"`
	MatchMode     string                   `json:"matchMode"`
	DesiredDigest string                   `json:"desiredDigest"`
	LiveDigest    string                   `json:"liveDigest"`
	Summary       ObjectSetSummary         `json:"summary"`
	Objects       []ObjectSetObjectSummary `json:"objects"`
}

type ObjectSetSummary struct {
	Desired      int `json:"desired"`
	Matched      int `json:"matched"`
	Missing      int `json:"missing"`
	Mismatched   int `json:"mismatched"`
	Inconclusive int `json:"inconclusive"`
}

type ObjectSetObjectSummary struct {
	ID            ObjectSetObjectID `json:"id"`
	Status        string            `json:"status"`
	DesiredDigest string            `json:"desiredDigest,omitempty"`
	LiveDigest    string            `json:"liveDigest,omitempty"`
	Error         string            `json:"error,omitempty"`
	Differences   []ObjectSetDiff   `json:"differences,omitempty"`
}

type ObjectSetObjectID struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name"`
}

func (id ObjectSetObjectID) Key() string {
	return id.APIVersion + "|" + id.Kind + "|" + id.Namespace + "|" + id.Name
}

func (id ObjectSetObjectID) String() string {
	if id.Namespace == "" {
		return id.APIVersion + "|" + id.Kind + "||" + id.Name
	}
	return id.APIVersion + "|" + id.Kind + "|" + id.Namespace + "|" + id.Name
}

type ObjectSetDiff struct {
	Path    string      `json:"path"`
	Desired interface{} `json:"desired,omitempty"`
	Live    interface{} `json:"live,omitempty"`
	Reason  string      `json:"reason,omitempty"`
}

// ObjectSetObservedObject is one desired object plus the live result observed
// for that identity. Error is interpreted as missing unless Inconclusive is
// true, in which case the object could not be checked.
type ObjectSetObservedObject struct {
	Desired      *unstructured.Unstructured
	Live         *unstructured.Unstructured
	Error        string
	Inconclusive bool
}

type BuildObjectSetReceiptInput struct {
	Evidence          ObjectSetEvidence
	Verifier          Verifier
	VerifiedAt        time.Time
	InputAttestations []VerifiedAttestationRef
}

// BuildObjectSetReceipt builds a single receipt for a rendered manifest set
// observed in a live cluster. It is pure: callers do file and cluster reads
// before invoking this function.
func BuildObjectSetReceipt(in BuildObjectSetReceiptInput) (Statement, error) {
	if in.Evidence.Summary.Desired == 0 {
		return Statement{}, fmt.Errorf("object-set receipt: no desired objects")
	}
	if in.Evidence.DesiredDigest == "" {
		return Statement{}, fmt.Errorf("object-set receipt: missing desired digest")
	}
	if in.Evidence.LiveDigest == "" {
		return Statement{}, fmt.Errorf("object-set receipt: missing live digest")
	}

	verifiedAt := in.VerifiedAt
	if verifiedAt.IsZero() {
		verifiedAt = time.Now().UTC()
	}

	verdict := VerdictPASS
	if in.Evidence.Summary.Missing > 0 || in.Evidence.Summary.Mismatched > 0 {
		verdict = VerdictBLOCK
	} else if in.Evidence.Summary.Inconclusive > 0 {
		verdict = VerdictINCONCLUSIVE
	}

	omissions := []Omission{
		{
			Missing:  OmissionExtraLiveObjectCoverage,
			Reason:   "object-set-matches verifies desired object identities and authored fields from the rendered manifests; it does not prove that no extra live objects exist outside that desired set",
			Severity: "info",
		},
	}
	if in.Evidence.Summary.Inconclusive > 0 {
		omissions = append(omissions, Omission{
			Missing:  OmissionObjectSetCoverage,
			Reason:   fmt.Sprintf("%d desired object(s) could not be checked because live lookup or API mapping was inconclusive", in.Evidence.Summary.Inconclusive),
			Severity: "warning",
		})
	}

	nextSteps := []ReceiptNextStep{}
	switch verdict {
	case VerdictPASS:
		nextSteps = append(nextSteps, ReceiptNextStep{
			ActionType:  "read-only",
			Reason:      "every desired rendered object was present and all authored fields matched live",
			NextCommand: "cub-scout doctor",
			NextSurface: "cub-scout",
		})
	case VerdictBLOCK:
		nextSteps = append(nextSteps, ReceiptNextStep{
			ActionType:  "read-only",
			Reason:      "one or more rendered objects are missing or differ from live; inspect object-set evidence before accepting the install",
			NextCommand: "cub-scout map list",
			NextSurface: "cub-scout",
		})
	case VerdictINCONCLUSIVE:
		nextSteps = append(nextSteps, ReceiptNextStep{
			ActionType:  "read-only",
			Reason:      "one or more rendered objects could not be checked; verify API availability and object identity",
			NextCommand: "cub-scout map list",
			NextSurface: "cub-scout",
		})
	}
	filteredSteps, stepOmissions := FilterNextSteps(nextSteps)
	omissions = append(omissions, stepOmissions...)

	inputAttestations := make([]AttestationRef, 0, len(in.InputAttestations))
	for i, v := range in.InputAttestations {
		if v.IsZero() {
			return Statement{}, fmt.Errorf("object-set receipt: inputAttestations[%d] is a zero-value VerifiedAttestationRef; construct via BuildAttestationRef or BuildAttestationRefsFromPaths", i)
		}
		inputAttestations = append(inputAttestations, v.Ref())
	}

	scope := Scope{
		Kind:      "ObjectSet",
		Name:      "rendered-install",
		Namespace: in.Evidence.Scope.Namespace,
	}
	claim := "live object set matches rendered manifests"
	if in.Evidence.DesiredSource.Ref != "" {
		claim = fmt.Sprintf("live object set matches rendered manifests from %s", in.Evidence.DesiredSource.Ref)
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
			PredicateName:     string(PredicateObjectSetMatches),
			Verdict:           verdict,
			Evidence:          Evidence{ObjectSet: &in.Evidence},
			Omissions:         omissions,
			InputAttestations: inputAttestations,
			NextSteps:         filteredSteps,
		},
	}

	if err := StampFingerprint(&stmt); err != nil {
		return Statement{}, fmt.Errorf("object-set receipt: stamp fingerprint: %w", err)
	}
	return stmt, nil
}

func BuildRenderedObjectSetSubject(digest string) Subject {
	short := digest
	if len(short) > 16 {
		short = short[:16]
	}
	return Subject{
		Name:   SubjectSchemeRenderedObjectSet + "sha256/" + short,
		Digest: map[string]string{"sha256": digest},
	}
}

func BuildLiveObjectSetSubject(scope ObjectSetScope, digest string) Subject {
	name := SubjectSchemeK8sLiveObjectSet + "cluster"
	if scope.Namespace != "" {
		name = SubjectSchemeK8sLiveObjectSet + "namespace/" + scope.Namespace
	}
	return Subject{
		Name:   name,
		Digest: map[string]string{"sha256": digest},
	}
}

// BuildObjectSetEvidence compares desired rendered objects against observed
// live objects. The match mode is authored-fields: every field present in the
// desired manifest must match live, while server-added map fields are ignored.
func BuildObjectSetEvidence(source ObjectSetSource, scope ObjectSetScope, observed []ObjectSetObservedObject) (ObjectSetEvidence, error) {
	summaries := make([]ObjectSetObjectSummary, 0, len(observed))
	desiredSet := make([]interface{}, 0, len(observed))
	liveSet := make([]interface{}, 0, len(observed))
	seenIDs := map[string]struct{}{}

	for _, obs := range observed {
		if obs.Desired == nil {
			continue
		}
		id := NewObjectSetObjectID(obs.Desired)
		if _, exists := seenIDs[id.Key()]; exists {
			return ObjectSetEvidence{}, fmt.Errorf("duplicate desired object identity %s", id.String())
		}
		seenIDs[id.Key()] = struct{}{}
		desiredComparable := ComparableObject(obs.Desired)
		desiredDigest, err := digestJSON(desiredComparable)
		if err != nil {
			return ObjectSetEvidence{}, fmt.Errorf("digest desired object %s: %w", id.String(), err)
		}
		desiredEntry := objectSetDigestEntry(id, desiredComparable)
		desiredSet = append(desiredSet, desiredEntry)

		summary := ObjectSetObjectSummary{
			ID:            id,
			DesiredDigest: desiredDigest,
		}

		switch {
		case obs.Inconclusive:
			summary.Status = ObjectSetObjectInconclusive
			summary.Error = obs.Error
			liveSet = append(liveSet, objectSetDigestEntry(id, map[string]interface{}{"inconclusive": obs.Error}))
		case obs.Live == nil:
			summary.Status = ObjectSetObjectMissing
			if obs.Error != "" {
				summary.Error = obs.Error
			} else {
				summary.Error = "live object not found"
			}
			liveSet = append(liveSet, objectSetDigestEntry(id, map[string]interface{}{"missing": true}))
		default:
			liveComparable := ComparableObject(obs.Live)
			liveProjection, diffs := ProjectLiveAuthoredFields(desiredComparable, liveComparable)
			liveDigest, err := digestJSON(liveProjection)
			if err != nil {
				return ObjectSetEvidence{}, fmt.Errorf("digest live object %s: %w", id.String(), err)
			}
			summary.LiveDigest = liveDigest
			if len(diffs) == 0 {
				summary.Status = ObjectSetObjectMatched
			} else {
				summary.Status = ObjectSetObjectMismatched
				summary.Differences = diffs
			}
			liveSet = append(liveSet, objectSetDigestEntry(id, liveProjection))
		}
		summaries = append(summaries, summary)
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].ID.Key() < summaries[j].ID.Key()
	})
	sort.Slice(desiredSet, func(i, j int) bool {
		return digestEntryKey(desiredSet[i]) < digestEntryKey(desiredSet[j])
	})
	sort.Slice(liveSet, func(i, j int) bool {
		return digestEntryKey(liveSet[i]) < digestEntryKey(liveSet[j])
	})

	desiredDigest, err := digestJSON(desiredSet)
	if err != nil {
		return ObjectSetEvidence{}, fmt.Errorf("digest desired object set: %w", err)
	}
	liveDigest, err := digestJSON(liveSet)
	if err != nil {
		return ObjectSetEvidence{}, fmt.Errorf("digest live object set: %w", err)
	}

	summary := ObjectSetSummary{Desired: len(summaries)}
	for _, obj := range summaries {
		switch obj.Status {
		case ObjectSetObjectMatched:
			summary.Matched++
		case ObjectSetObjectMissing:
			summary.Missing++
		case ObjectSetObjectMismatched:
			summary.Mismatched++
		case ObjectSetObjectInconclusive:
			summary.Inconclusive++
		}
	}
	source.ObjectCount = summary.Desired

	return ObjectSetEvidence{
		DesiredSource: source,
		Scope:         scope,
		MatchMode:     ObjectSetMatchModeAuthoredFields,
		DesiredDigest: desiredDigest,
		LiveDigest:    liveDigest,
		Summary:       summary,
		Objects:       summaries,
	}, nil
}

func NewObjectSetObjectID(obj *unstructured.Unstructured) ObjectSetObjectID {
	if obj == nil {
		return ObjectSetObjectID{}
	}
	return ObjectSetObjectID{
		APIVersion: obj.GetAPIVersion(),
		Kind:       obj.GetKind(),
		Namespace:  obj.GetNamespace(),
		Name:       obj.GetName(),
	}
}

var objectSetDynamicFieldPaths = [][]string{
	{"status"},
	{"metadata", "managedFields"},
	{"metadata", "resourceVersion"},
	{"metadata", "generation"},
	{"metadata", "uid"},
	{"metadata", "creationTimestamp"},
	{"metadata", "selfLink"},
}

func ComparableObject(obj *unstructured.Unstructured) map[string]interface{} {
	if obj == nil {
		return nil
	}
	cloned := obj.DeepCopy().Object
	for _, path := range objectSetDynamicFieldPaths {
		unstructured.RemoveNestedField(cloned, path...)
	}
	return cloned
}

// ProjectLiveAuthoredFields returns the live object's projection onto the
// desired authored field shape plus field-level differences.
func ProjectLiveAuthoredFields(desired, live interface{}) (interface{}, []ObjectSetDiff) {
	projected, diffs := projectAuthoredValue("", desired, live)
	return projected, diffs
}

func projectAuthoredValue(path string, desired, live interface{}) (interface{}, []ObjectSetDiff) {
	switch d := desired.(type) {
	case map[string]interface{}:
		lm, ok := live.(map[string]interface{})
		if !ok {
			return live, []ObjectSetDiff{{Path: cleanObjectSetPath(path), Desired: desired, Live: live, Reason: "type mismatch"}}
		}
		out := map[string]interface{}{}
		var diffs []ObjectSetDiff
		keys := make([]string, 0, len(d))
		for k := range d {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			nextPath := joinObjectSetPath(path, k)
			lv, exists := lm[k]
			if !exists {
				out[k] = nil
				diffs = append(diffs, ObjectSetDiff{Path: cleanObjectSetPath(nextPath), Desired: d[k], Live: nil, Reason: "missing field in live object"})
				continue
			}
			pv, childDiffs := projectAuthoredValue(nextPath, d[k], lv)
			out[k] = pv
			diffs = append(diffs, childDiffs...)
		}
		return out, diffs
	case []interface{}:
		ls, ok := live.([]interface{})
		if !ok {
			return live, []ObjectSetDiff{{Path: cleanObjectSetPath(path), Desired: desired, Live: live, Reason: "type mismatch"}}
		}
		if len(d) != len(ls) {
			return live, []ObjectSetDiff{{Path: cleanObjectSetPath(path), Desired: len(d), Live: len(ls), Reason: "list length mismatch"}}
		}
		out := make([]interface{}, len(d))
		var diffs []ObjectSetDiff
		for i := range d {
			pv, childDiffs := projectAuthoredValue(fmt.Sprintf("%s[%d]", path, i), d[i], ls[i])
			out[i] = pv
			diffs = append(diffs, childDiffs...)
		}
		return out, diffs
	default:
		if canonicalValuesEqual(desired, live) {
			return live, nil
		}
		return live, []ObjectSetDiff{{Path: cleanObjectSetPath(path), Desired: desired, Live: live, Reason: "value mismatch"}}
	}
}

func canonicalValuesEqual(a, b interface{}) bool {
	ab, aErr := CanonicalJSON(a)
	bb, bErr := CanonicalJSON(b)
	if aErr != nil || bErr != nil {
		return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
	}
	return string(ab) == string(bb)
}

func digestJSON(v interface{}) (string, error) {
	canon, err := CanonicalJSON(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canon)
	return hex.EncodeToString(sum[:]), nil
}

func objectSetDigestEntry(id ObjectSetObjectID, object interface{}) map[string]interface{} {
	return map[string]interface{}{
		"id":     id.String(),
		"object": object,
	}
}

func digestEntryKey(v interface{}) string {
	m, ok := v.(map[string]interface{})
	if !ok {
		return ""
	}
	s, _ := m["id"].(string)
	return s
}

func joinObjectSetPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

func cleanObjectSetPath(path string) string {
	path = strings.TrimPrefix(path, ".")
	if path == "" {
		return "$"
	}
	return path
}
