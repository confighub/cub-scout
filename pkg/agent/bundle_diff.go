// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Package agent provides Bundle Diff v1 for pairwise bundle comparison.
//
// A bundle diff compares two bundles and produces a new JSON artifact
// (bundle-diff.v1) describing what changed between them.
//
// Contract:
// - Diffs are new facts in new schemas (never modify bundles)
// - Continuity requires explicit join mode
// - Ambiguous/unjoinable are first-class statuses
// - Output ordering is deterministic (by identity, then section)
package agent

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// BundleDiffSchemaVersion is the schema version for bundle diffs.
const BundleDiffSchemaVersion = "bundle-diff.v1"

// JoinMode specifies how objects are matched across bundles.
type JoinMode string

const (
	// JoinObjectID joins on the object_id field (default)
	JoinObjectID JoinMode = "object_id"

	// JoinComposite joins on (kind, namespace, name) - not yet implemented
	JoinComposite JoinMode = "composite"

	// JoinNone performs no joining - not yet implemented
	JoinNone JoinMode = "none"
)

// BundleDiff is the top-level diff result.
// Schema: bundle-diff.v1
type BundleDiff struct {
	// SchemaVersion identifies the diff format
	SchemaVersion string `json:"schema_version"`

	// From identifies the source bundle
	From BundleRef `json:"from"`

	// To identifies the target bundle
	To BundleRef `json:"to"`

	// JoinMode specifies how objects were matched
	JoinMode JoinMode `json:"join_mode"`

	// Summary provides aggregate counts by status
	Summary DiffSummary `json:"summary"`

	// Objects lists per-object diffs, sorted by identity
	Objects []ObjectDiff `json:"objects"`
}

// BundleRef identifies a bundle in a diff.
type BundleRef struct {
	// ID is the bundle identifier (from catalog or path)
	ID string `json:"id"`

	// CreatedAt is when the bundle was created
	CreatedAt time.Time `json:"created_at"`

	// Digest is the optional content hash
	Digest string `json:"digest,omitempty"`
}

// DiffSummary provides counts by object status.
type DiffSummary struct {
	Added      int `json:"added"`
	Removed    int `json:"removed"`
	Changed    int `json:"changed"`
	Unchanged  int `json:"unchanged"`
	Ambiguous  int `json:"ambiguous"`
	Unjoinable int `json:"unjoinable"`
}

// ObjectStatus describes an object's diff status.
type ObjectStatus string

const (
	StatusAdded      ObjectStatus = "added"
	StatusRemoved    ObjectStatus = "removed"
	StatusChanged    ObjectStatus = "changed"
	StatusUnchanged  ObjectStatus = "unchanged"
	StatusAmbiguous  ObjectStatus = "ambiguous"
	StatusUnjoinable ObjectStatus = "unjoinable"
)

// ObjectDiff describes the diff for a single object.
type ObjectDiff struct {
	// Identity is the join key (object_id for now)
	Identity string `json:"identity"`

	// Status is the overall diff status for this object
	Status ObjectStatus `json:"status"`

	// Sections lists per-section diffs in fixed order
	Sections []SectionDiff `json:"sections"`
}

// SectionStatus describes a section's diff status.
type SectionStatus string

const (
	SectionChanged    SectionStatus = "changed"
	SectionUnchanged  SectionStatus = "unchanged"
	SectionMissingFrom SectionStatus = "missing_from"
	SectionMissingTo  SectionStatus = "missing_to"
)

// SectionDiff describes the diff for a single section.
type SectionDiff struct {
	// Name identifies the section (drift, correlation)
	Name string `json:"name"`

	// Status describes how the section differs
	Status SectionStatus `json:"status"`

	// Counts provides optional numeric summaries
	Counts *SectionCounts `json:"counts,omitempty"`
}

// SectionCounts provides numeric summaries for a section.
type SectionCounts struct {
	Before int `json:"before"`
	After  int `json:"after"`
	Delta  int `json:"delta"`
}

// DiffBundles compares two bundles and returns a diff.
// Only JoinObjectID is supported in PR2.
func DiffBundles(from, to *DebugBundle, joinMode JoinMode) (*BundleDiff, error) {
	// Validate join mode
	if joinMode == JoinComposite {
		return nil, fmt.Errorf("join mode 'composite' not yet implemented")
	}
	if joinMode == JoinNone {
		return nil, fmt.Errorf("join mode 'none' not yet implemented")
	}
	if joinMode == "" {
		joinMode = JoinObjectID
	}

	diff := &BundleDiff{
		SchemaVersion: BundleDiffSchemaVersion,
		From:          bundleToRef(from),
		To:            bundleToRef(to),
		JoinMode:      joinMode,
	}

	// Build object maps by identity
	fromObjects := buildObjectMap(from)
	toObjects := buildObjectMap(to)

	// Collect all identities
	allIdentities := make(map[string]bool)
	for id := range fromObjects {
		allIdentities[id] = true
	}
	for id := range toObjects {
		allIdentities[id] = true
	}

	// Sort identities for deterministic output
	sortedIdentities := make([]string, 0, len(allIdentities))
	for id := range allIdentities {
		sortedIdentities = append(sortedIdentities, id)
	}
	sort.Strings(sortedIdentities)

	// Compute per-object diffs
	for _, identity := range sortedIdentities {
		fromObj, inFrom := fromObjects[identity]
		toObj, inTo := toObjects[identity]

		objDiff := computeObjectDiff(identity, fromObj, toObj, inFrom, inTo)
		diff.Objects = append(diff.Objects, objDiff)

		// Update summary
		switch objDiff.Status {
		case StatusAdded:
			diff.Summary.Added++
		case StatusRemoved:
			diff.Summary.Removed++
		case StatusChanged:
			diff.Summary.Changed++
		case StatusUnchanged:
			diff.Summary.Unchanged++
		case StatusAmbiguous:
			diff.Summary.Ambiguous++
		case StatusUnjoinable:
			diff.Summary.Unjoinable++
		}
	}

	return diff, nil
}

// bundleToRef creates a BundleRef from a bundle.
func bundleToRef(bundle *DebugBundle) BundleRef {
	id := fmt.Sprintf("%s/%s", bundle.Metadata.Target.Kind, bundle.Metadata.Target.Name)
	if bundle.Metadata.Target.Namespace != "" {
		id = fmt.Sprintf("%s:%s/%s",
			bundle.Metadata.Target.Kind,
			bundle.Metadata.Target.Namespace,
			bundle.Metadata.Target.Name)
	}
	if bundle.Metadata.Label != "" {
		id = bundle.Metadata.Label
	}

	return BundleRef{
		ID:        id,
		CreatedAt: bundle.Metadata.CreatedAt,
	}
}

// objectData holds the sections for a single object.
type objectData struct {
	driftFindings []DriftFinding
	hasCorrelation bool
	correlation   *RootCauseSnapshot
}

// buildObjectMap groups bundle contents by object_id.
func buildObjectMap(bundle *DebugBundle) map[string]*objectData {
	objects := make(map[string]*objectData)

	// Group drift findings by object_id
	for _, df := range bundle.DriftFindings {
		id := df.ObjectID
		if id == "" {
			// Generate object_id if missing
			id = fmt.Sprintf("%s:%s/%s",
				df.Object.Kind,
				df.Object.Namespace,
				df.Object.Name)
		}
		if objects[id] == nil {
			objects[id] = &objectData{}
		}
		objects[id].driftFindings = append(objects[id].driftFindings, df)
	}

	// Add the target object from session if present
	if bundle.Session != nil {
		targetID := fmt.Sprintf("%s:%s/%s",
			bundle.Session.Target.Kind,
			bundle.Session.Target.Namespace,
			bundle.Session.Target.Name)
		if objects[targetID] == nil {
			objects[targetID] = &objectData{}
		}
		if bundle.Session.RootCause != nil {
			objects[targetID].hasCorrelation = true
			objects[targetID].correlation = bundle.Session.RootCause
		}
	}

	return objects
}

// computeObjectDiff computes the diff for a single object.
func computeObjectDiff(identity string, fromObj, toObj *objectData, inFrom, inTo bool) ObjectDiff {
	objDiff := ObjectDiff{
		Identity: identity,
		Sections: make([]SectionDiff, 0, 2),
	}

	// Determine object status
	if !inFrom && inTo {
		objDiff.Status = StatusAdded
	} else if inFrom && !inTo {
		objDiff.Status = StatusRemoved
	} else {
		// Both present - compare sections
		objDiff.Status = StatusUnchanged
	}

	// Compute drift section diff
	driftDiff := computeDriftSectionDiff(fromObj, toObj, inFrom, inTo)
	objDiff.Sections = append(objDiff.Sections, driftDiff)

	// Compute correlation section diff
	corrDiff := computeCorrelationSectionDiff(fromObj, toObj, inFrom, inTo)
	objDiff.Sections = append(objDiff.Sections, corrDiff)

	// Object is changed if any section changed
	if objDiff.Status == StatusUnchanged {
		for _, section := range objDiff.Sections {
			if section.Status == SectionChanged {
				objDiff.Status = StatusChanged
				break
			}
		}
	}

	return objDiff
}

// computeDriftSectionDiff computes the drift section diff.
func computeDriftSectionDiff(fromObj, toObj *objectData, inFrom, inTo bool) SectionDiff {
	section := SectionDiff{Name: "drift"}

	fromCount := 0
	toCount := 0
	if inFrom && fromObj != nil {
		fromCount = len(fromObj.driftFindings)
	}
	if inTo && toObj != nil {
		toCount = len(toObj.driftFindings)
	}

	section.Counts = &SectionCounts{
		Before: fromCount,
		After:  toCount,
		Delta:  toCount - fromCount,
	}

	// Determine section status
	if !inFrom {
		section.Status = SectionMissingFrom
	} else if !inTo {
		section.Status = SectionMissingTo
	} else {
		// Compare drift findings canonically
		if driftEqual(fromObj, toObj) {
			section.Status = SectionUnchanged
		} else {
			section.Status = SectionChanged
		}
	}

	return section
}

// computeCorrelationSectionDiff computes the correlation section diff.
func computeCorrelationSectionDiff(fromObj, toObj *objectData, inFrom, inTo bool) SectionDiff {
	section := SectionDiff{Name: "correlation"}

	// Determine section status
	if !inFrom {
		section.Status = SectionMissingFrom
	} else if !inTo {
		section.Status = SectionMissingTo
	} else {
		// Compare correlation
		fromHas := fromObj != nil && fromObj.hasCorrelation
		toHas := toObj != nil && toObj.hasCorrelation

		if !fromHas && !toHas {
			section.Status = SectionUnchanged
		} else if !fromHas {
			section.Status = SectionChanged
		} else if !toHas {
			section.Status = SectionChanged
		} else if correlationEqual(fromObj.correlation, toObj.correlation) {
			section.Status = SectionUnchanged
		} else {
			section.Status = SectionChanged
		}
	}

	return section
}

// driftEqual compares drift findings canonically.
func driftEqual(from, to *objectData) bool {
	if from == nil && to == nil {
		return true
	}
	if from == nil || to == nil {
		return false
	}
	if len(from.driftFindings) != len(to.driftFindings) {
		return false
	}

	// Sort and compare by canonical JSON
	fromJSON := canonicalDriftJSON(from.driftFindings)
	toJSON := canonicalDriftJSON(to.driftFindings)
	return fromJSON == toJSON
}

// canonicalDriftJSON produces deterministic JSON for drift comparison.
func canonicalDriftJSON(findings []DriftFinding) string {
	if len(findings) == 0 {
		return "[]"
	}

	// Sort by ID for deterministic comparison
	sorted := make([]DriftFinding, len(findings))
	copy(sorted, findings)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ID < sorted[j].ID
	})

	data, _ := json.Marshal(sorted)
	return string(data)
}

// correlationEqual compares correlation snapshots.
func correlationEqual(from, to *RootCauseSnapshot) bool {
	if from == nil && to == nil {
		return true
	}
	if from == nil || to == nil {
		return false
	}

	// Compare by canonical JSON
	fromJSON, _ := json.Marshal(from)
	toJSON, _ := json.Marshal(to)
	return string(fromJSON) == string(toJSON)
}
