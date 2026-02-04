// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Package agent provides drift detection types for v0.14.3.
//
// This file defines the JSON schema for drift reports.
// JSON is authoritative for structural facts; ASCII renders f(JSON)+g.
// See docs/semantic-contract.md for the full model.
//
// Contract: R1 (facts in JSON), R5 (stable identity), R6 (ordering semantic via severity field).
package agent

import (
	"sort"
)

// DriftReport is the v0.14.3 JSON schema for drift detection output.
// All structural facts live here; ASCII rendering adds narrative only.
type DriftReport struct {
	// Command identifies the output type for tooling
	Command string `json:"command"`

	// Context captures the query parameters
	Context DriftContext `json:"context"`

	// Findings is the list of detected drift items
	// Ordering is semantic: by severity (desc), then object_id (asc), then path (asc)
	Findings []DriftFinding `json:"findings"`

	// Summary provides aggregate counts for quick assessment
	Summary DriftSummary `json:"summary"`
}

// DriftContext captures the query parameters for reproducibility.
type DriftContext struct {
	// Cluster is the kubectl context name
	Cluster string `json:"cluster"`

	// Namespace filter (null = all namespaces)
	Namespace *string `json:"namespace"`

	// DesiredSource describes where "desired" state came from
	DesiredSource DriftSource `json:"desiredSource"`

	// LiveSource describes where "live" state came from
	LiveSource DriftSource `json:"liveSource"`
}

// DriftSource identifies the origin of a state snapshot.
type DriftSource struct {
	// Type is one of: git, oci, file, cluster
	Type string `json:"type"`

	// Ref is the reference (commit SHA, OCI tag, file path, context name)
	Ref string `json:"ref"`

	// URL is the source URL (git repo, OCI registry) if applicable
	URL string `json:"url,omitempty"`

	// Subpath within the source (for git/oci)
	Subpath string `json:"subpath,omitempty"`
}

// DriftFinding represents a single drift item.
// All fields are structural facts (machine-relevant).
type DriftFinding struct {
	// ID is a stable identifier for this finding
	// Format: "drift:{object_id}:{path}" for deduplication and joins
	ID string `json:"id"`

	// ObjectID is a stable identifier for the Kubernetes object
	// Format: "{kind}:{namespace}/{name}" or "{kind}:{name}" for cluster-scoped
	ObjectID string `json:"object_id"`

	// Object provides structured identity for cross-schema joins
	Object DriftObjectID `json:"object"`

	// Path is the JSON path to the drifted field (e.g., "spec.replicas")
	Path string `json:"path"`

	// Desired is the expected value from the source of truth
	Desired interface{} `json:"desired"`

	// Live is the actual value observed in the cluster
	Live interface{} `json:"live"`

	// Classification categorizes the type of drift for filtering/grouping
	// Values: capacity, image, config, rollout, resource, health, annotation, label, other
	Classification DriftClassification `json:"classification"`

	// Severity indicates the impact level for CI gating and alerting
	// Values: info, warning, critical
	Severity DriftSeverity `json:"severity"`
}

// DriftObjectID provides structured identity for the drifted object.
type DriftObjectID struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"` // empty for cluster-scoped
	Name      string `json:"name"`
}

// DriftClassification categorizes the type of drift.
// This is a structural fact used for filtering and CI policies.
type DriftClassification string

const (
	// DriftCapacity indicates replica count or scaling drift
	DriftCapacity DriftClassification = "capacity"

	// DriftImage indicates container image drift
	DriftImage DriftClassification = "image"

	// DriftConfig indicates configuration drift (env vars, configmap refs, etc.)
	DriftConfig DriftClassification = "config"

	// DriftRollout indicates deployment strategy or rollout drift
	DriftRollout DriftClassification = "rollout"

	// DriftResource indicates resource requests/limits drift
	DriftResource DriftClassification = "resource"

	// DriftHealth indicates health check/probe drift
	DriftHealth DriftClassification = "health"

	// DriftAnnotation indicates annotation drift
	DriftAnnotation DriftClassification = "annotation"

	// DriftLabel indicates label drift
	DriftLabel DriftClassification = "label"

	// DriftOther indicates unclassified drift
	DriftOther DriftClassification = "other"
)

// DriftSeverity indicates the impact level of a finding.
// This is a structural fact used for CI exit codes and alerting.
type DriftSeverity string

const (
	// DriftSeverityInfo is informational (no action required)
	DriftSeverityInfo DriftSeverity = "info"

	// DriftSeverityWarning requires attention but not immediate
	DriftSeverityWarning DriftSeverity = "warning"

	// DriftSeverityCritical requires immediate attention
	DriftSeverityCritical DriftSeverity = "critical"
)

// DriftSummary provides aggregate counts for quick assessment.
type DriftSummary struct {
	// TotalFindings is the total number of drift items
	TotalFindings int `json:"totalFindings"`

	// BySeverity counts findings by severity level
	// Array format for deterministic ordering (not a map)
	BySeverity []DriftSeverityCount `json:"bySeverity"`

	// ByClassification counts findings by classification
	// Array format for deterministic ordering (not a map)
	ByClassification []DriftClassificationCount `json:"byClassification"`

	// AffectedObjects is the count of unique objects with drift
	AffectedObjects int `json:"affectedObjects"`
}

// DriftSeverityCount is a single severity count entry.
type DriftSeverityCount struct {
	Severity DriftSeverity `json:"severity"`
	Count    int           `json:"count"`
}

// DriftClassificationCount is a single classification count entry.
type DriftClassificationCount struct {
	Classification DriftClassification `json:"classification"`
	Count          int                 `json:"count"`
}

// CanonicalSeverityOrder defines the deterministic severity ordering.
// Matches the semantic meaning: most severe first.
var CanonicalSeverityOrder = []DriftSeverity{
	DriftSeverityCritical,
	DriftSeverityWarning,
	DriftSeverityInfo,
}

// CanonicalClassificationOrder defines the deterministic classification ordering.
// Ordered by typical user priority (capacity/image first, labels/annotations last).
var CanonicalClassificationOrder = []DriftClassification{
	DriftCapacity,
	DriftImage,
	DriftConfig,
	DriftResource,
	DriftRollout,
	DriftHealth,
	DriftLabel,
	DriftAnnotation,
	DriftOther,
}

// SortFindings sorts findings in canonical order for deterministic output.
// Order: severity (desc: critical > warning > info), then object_id (asc), then path (asc)
func SortFindings(findings []DriftFinding) {
	severityRank := map[DriftSeverity]int{
		DriftSeverityCritical: 0,
		DriftSeverityWarning:  1,
		DriftSeverityInfo:     2,
	}

	sort.Slice(findings, func(i, j int) bool {
		// Primary: severity (descending - critical first)
		ri, rj := severityRank[findings[i].Severity], severityRank[findings[j].Severity]
		if ri != rj {
			return ri < rj
		}
		// Secondary: object_id (ascending)
		if findings[i].ObjectID != findings[j].ObjectID {
			return findings[i].ObjectID < findings[j].ObjectID
		}
		// Tertiary: path (ascending)
		return findings[i].Path < findings[j].Path
	})
}

// BuildDriftSummary creates a summary from findings.
// Uses canonical ordering for deterministic output.
func BuildDriftSummary(findings []DriftFinding) DriftSummary {
	severityCounts := make(map[DriftSeverity]int)
	classificationCounts := make(map[DriftClassification]int)
	objectSet := make(map[string]struct{})

	for _, f := range findings {
		severityCounts[f.Severity]++
		classificationCounts[f.Classification]++
		objectSet[f.ObjectID] = struct{}{}
	}

	// Build severity counts in canonical order
	bySeverity := make([]DriftSeverityCount, 0, len(CanonicalSeverityOrder))
	for _, sev := range CanonicalSeverityOrder {
		if count, ok := severityCounts[sev]; ok && count > 0 {
			bySeverity = append(bySeverity, DriftSeverityCount{
				Severity: sev,
				Count:    count,
			})
		}
	}

	// Build classification counts in canonical order
	byClassification := make([]DriftClassificationCount, 0, len(CanonicalClassificationOrder))
	for _, cls := range CanonicalClassificationOrder {
		if count, ok := classificationCounts[cls]; ok && count > 0 {
			byClassification = append(byClassification, DriftClassificationCount{
				Classification: cls,
				Count:          count,
			})
		}
	}

	return DriftSummary{
		TotalFindings:    len(findings),
		BySeverity:       bySeverity,
		ByClassification: byClassification,
		AffectedObjects:  len(objectSet),
	}
}

// NewDriftFinding creates a finding with proper ID generation.
func NewDriftFinding(obj DriftObjectID, path string, desired, live interface{}, classification DriftClassification, severity DriftSeverity) DriftFinding {
	objectID := obj.Kind + ":"
	if obj.Namespace != "" {
		objectID += obj.Namespace + "/"
	}
	objectID += obj.Name

	return DriftFinding{
		ID:             "drift:" + objectID + ":" + path,
		ObjectID:       objectID,
		Object:         obj,
		Path:           path,
		Desired:        desired,
		Live:           live,
		Classification: classification,
		Severity:       severity,
	}
}
