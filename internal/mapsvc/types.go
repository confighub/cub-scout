// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Package mapsvc provides the core data types and business logic for the cub-scout map command.
// It separates the data model and processing logic from the CLI and TUI rendering.
package mapsvc

import (
	"sort"
	"strings"
	"time"

	"github.com/confighub/cub-scout/v2/pkg/agent"
)

// Entry represents a resource in the fleet map.
// This is the core data type that represents discovered Kubernetes resources.
type Entry struct {
	ID            string                           `json:"id"`
	ClusterName   string                           `json:"clusterName"`
	Namespace     string                           `json:"namespace"`
	Kind          string                           `json:"kind"`
	Name          string                           `json:"name"`
	APIVersion    string                           `json:"apiVersion"`
	Owner         string                           `json:"owner"`
	OwnerDetails  map[string]string                `json:"ownerDetails,omitempty"`
	OwnerEvidence *agent.PlatformSubstrateEvidence `json:"ownerEvidence,omitempty"`
	// OwnershipDetection is populated from the same DetectOwnership result as
	// Owner. It is excluded from legacy JSON and emitted only in the opt-in
	// map-list ownership diagnostics envelope.
	OwnershipDetection *OwnershipDetectionEvidence `json:"-"`
	Observation        *agent.ObservationEvidence  `json:"observation,omitempty"`
	Labels             map[string]string           `json:"labels,omitempty"`
	Status             string                      `json:"status"` // Ready, NotReady, Failed, Pending, Unknown
	CreatedAt          time.Time                   `json:"createdAt"`
	UpdatedAt          time.Time                   `json:"updatedAt"`
}

const (
	OwnershipDetectionStatusDetected      = "detected"
	OwnershipDetectionStatusNoKnownMarker = "no_known_marker"
	OwnershipDetectionStatusSourceMissing = "source_missing"
	OwnershipDetectionReasonNoMarker      = "no_supported_marker_observed_on_returned_object"
	OwnershipDetectionReasonSourceMissing = "detector_source_not_available"
)

// OwnershipDetectionEvidence explains only the deterministic ownership
// detector result for an object that was successfully returned by a list.
// It does not prove that the object is orphaned or identify a person.
type OwnershipDetectionEvidence struct {
	Status string `json:"status"`
	Source string `json:"source,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// NewOwnershipDetectionEvidence captures the canonical detector's selected
// source. Unknown ownership is specifically about the successfully returned
// object passed to DetectOwnership, not an unreadable or omitted list result.
func NewOwnershipDetectionEvidence(ownership agent.Ownership) *OwnershipDetectionEvidence {
	if ownership.Type == "" || ownership.Type == agent.OwnerUnknown {
		return &OwnershipDetectionEvidence{
			Status: OwnershipDetectionStatusNoKnownMarker,
			Reason: OwnershipDetectionReasonNoMarker,
		}
	}
	if ownership.Source == "" {
		return &OwnershipDetectionEvidence{
			Status: OwnershipDetectionStatusSourceMissing,
			Reason: OwnershipDetectionReasonSourceMissing,
		}
	}
	return &OwnershipDetectionEvidence{
		Status: OwnershipDetectionStatusDetected,
		Source: ownership.Source,
	}
}

// CollectionOmission records a resource-list request that did not produce an
// entry. Reasons are normalized and do not include raw server errors.
type CollectionOmission struct {
	APIVersion string `json:"apiVersion"`
	Resource   string `json:"resource"`
	Namespace  string `json:"namespace,omitempty"`
	Reason     string `json:"reason"`
}

// OwnershipEvidenceResource is a compact ownership-only projection. The full
// ordinary map output remains available for drilldown; this projection avoids
// repeating labels, status, timestamps, and observation metadata.
type OwnershipEvidenceResource struct {
	ClusterName        string                           `json:"clusterName,omitempty"`
	APIVersion         string                           `json:"apiVersion"`
	Kind               string                           `json:"kind"`
	Namespace          string                           `json:"namespace,omitempty"`
	Name               string                           `json:"name"`
	Owner              string                           `json:"owner"`
	OwnerDetails       map[string]string                `json:"ownerDetails,omitempty"`
	OwnerEvidence      *agent.PlatformSubstrateEvidence `json:"ownerEvidence,omitempty"`
	OwnershipDetection *OwnershipDetectionEvidence      `json:"ownershipDetection"`
}

// OwnershipEvidenceCollection describes completeness for the resource list
// requests that make up this inventory.
type OwnershipEvidenceCollection struct {
	Status    string               `json:"status"`
	Omissions []CollectionOmission `json:"omissions"`
}

// OwnershipEvidenceOutput is the opt-in, versioned per-entry diagnostic
// envelope used by CLI and MCP.
type OwnershipEvidenceOutput struct {
	Schema     string                      `json:"schema"`
	Resources  []OwnershipEvidenceResource `json:"resources"`
	Collection OwnershipEvidenceCollection `json:"collection"`
}

// BuildOwnershipEvidenceOutput builds a deterministic opt-in envelope. The
// input entries and omission list are copied before sorting.
func BuildOwnershipEvidenceOutput(entries []Entry, omissions []CollectionOmission) OwnershipEvidenceOutput {
	resources := make([]OwnershipEvidenceResource, len(entries))
	for i, entry := range entries {
		resources[i] = OwnershipEvidenceResource{
			ClusterName:        entry.ClusterName,
			APIVersion:         entry.APIVersion,
			Kind:               entry.Kind,
			Namespace:          entry.Namespace,
			Name:               entry.Name,
			Owner:              entry.Owner,
			OwnerDetails:       entry.OwnerDetails,
			OwnerEvidence:      entry.OwnerEvidence,
			OwnershipDetection: entry.OwnershipDetection,
		}
	}
	sort.Slice(resources, func(i, j int) bool {
		a, b := resources[i], resources[j]
		if a.Namespace != b.Namespace {
			return a.Namespace < b.Namespace
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Name < b.Name
	})
	omissions = append([]CollectionOmission(nil), omissions...)
	sort.Slice(omissions, func(i, j int) bool {
		a, b := omissions[i], omissions[j]
		if a.APIVersion != b.APIVersion {
			return a.APIVersion < b.APIVersion
		}
		if a.Resource != b.Resource {
			return a.Resource < b.Resource
		}
		if a.Namespace != b.Namespace {
			return a.Namespace < b.Namespace
		}
		return a.Reason < b.Reason
	})
	status := "complete"
	if len(omissions) > 0 {
		status = "partial"
	}
	return OwnershipEvidenceOutput{
		Schema:    "map-list-ownership-evidence.v1",
		Resources: resources,
		Collection: OwnershipEvidenceCollection{
			Status:    status,
			Omissions: omissions,
		},
	}
}

// OwnershipDetectionSummary is the shared compact text projection used by
// ASCII, Markdown and TUI detail views.
func OwnershipDetectionSummary(evidence *OwnershipDetectionEvidence) string {
	if evidence == nil {
		return "ownership detection evidence unavailable"
	}
	switch evidence.Status {
	case OwnershipDetectionStatusDetected:
		if evidence.Source != "" {
			return "detected via " + evidence.Source
		}
	case OwnershipDetectionStatusNoKnownMarker:
		return "no known marker: no supported ownership marker observed on this returned object"
	case OwnershipDetectionStatusSourceMissing:
		return "ownership source unavailable: " + evidence.Reason
	}
	return "ownership detection evidence unavailable"
}

// CollectionOmissionSummary renders a sanitized, deterministic list omission.
func CollectionOmissionSummary(omission CollectionOmission) string {
	scope := omission.APIVersion + "/" + omission.Resource
	if omission.Namespace != "" {
		scope += " namespace=" + omission.Namespace
	}
	return scope + " not returned (" + omission.Reason + ")"
}

// GetField implements query.Matchable for Entry.
// This enables flexible querying of entry fields.
func (e Entry) GetField(field string) (string, bool) {
	// Handle labels[key] syntax
	if len(field) > 7 && field[:7] == "labels[" && field[len(field)-1] == ']' {
		key := field[7 : len(field)-1]
		if e.Labels == nil {
			return "", false
		}
		v, ok := e.Labels[key]
		return v, ok
	}
	switch field {
	case "kind":
		return e.Kind, true
	case "namespace":
		return e.Namespace, true
	case "name":
		return e.Name, true
	case "owner":
		return e.Owner, true
	case "status":
		return e.Status, true
	case "cluster", "clusterName":
		return e.ClusterName, true
	case "apiVersion":
		return e.APIVersion, true
	default:
		return "", false
	}
}

// DisplayOwner returns the canonical display name for an owner type.
// Internal names are lowercase (flux, argo, helm, etc.) but display names are capitalized.
func DisplayOwner(owner string) string {
	switch strings.ToLower(owner) {
	case "flux":
		return "Flux"
	case "argo":
		return "ArgoCD"
	case "helm":
		return "Helm"
	case "terraform":
		return "Terraform"
	case "sveltos":
		return "Sveltos"
	case "modelplane":
		return "Modelplane"
	case "crossplane":
		return "Crossplane"
	case "kro":
		return "kro"
	case "confighub":
		return "ConfigHub"
	case "k8s", "native", "unknown", "":
		return "Native"
	default:
		return owner
	}
}

// OwnerStats tracks counts by owner type.
type OwnerStats struct {
	ByOwner  map[string]int
	ByKind   map[string]int
	ByStatus map[string]int
	Total    int
}

// NewOwnerStats creates an initialized OwnerStats.
func NewOwnerStats() *OwnerStats {
	return &OwnerStats{
		ByOwner:  make(map[string]int),
		ByKind:   make(map[string]int),
		ByStatus: make(map[string]int),
	}
}

// Add records an entry in the stats.
func (s *OwnerStats) Add(e Entry) {
	s.Total++
	s.ByOwner[e.Owner]++
	s.ByKind[e.Kind]++
	s.ByStatus[e.Status]++
}
