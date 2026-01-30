// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"fmt"
	"time"
)

// TraceResult represents the full ownership chain for a resource
type TraceResult struct {
	// Object is the resource being traced
	Object ResourceRef `json:"object"`

	// Chain is the ownership chain from source to resource
	// Ordered from root (GitRepository) to leaf (the traced resource)
	Chain []ChainLink `json:"chain"`

	// FullyManaged indicates if the resource is fully managed by GitOps
	FullyManaged bool `json:"fullyManaged"`

	// Tool indicates which GitOps tool manages this resource
	Tool string `json:"tool"` // "flux", "argocd", or ""

	// Error contains any error encountered during tracing
	Error string `json:"error,omitempty"`

	// TracedAt is when the trace was performed
	TracedAt time.Time `json:"tracedAt"`

	// ConfigHub contains ConfigHub-specific metadata if the resource is managed by ConfigHub
	ConfigHub *TraceConfigHub `json:"confighub,omitempty"`

	// History contains deployment/reconciliation history entries
	// Ordered from most recent to oldest
	History []HistoryEntry `json:"history,omitempty"`

	// CrossReferences contains resources referenced by this resource that have different owners
	// For example, a Flux-managed Deployment referencing a Crossplane-created Secret
	CrossReferences []CrossReference `json:"crossReferences,omitempty"`
}

// CrossReference represents a reference to a resource with a different owner
type CrossReference struct {
	// Ref is the referenced resource (e.g., Secret, ConfigMap)
	Ref ResourceRef `json:"ref"`

	// RefType describes how the resource is referenced (e.g., "secretRef", "configMapRef", "envFrom", "volume")
	RefType string `json:"refType"`

	// Owner is the ownership information for the referenced resource
	Owner *Ownership `json:"owner,omitempty"`

	// Status is the current status of the referenced resource (e.g., "exists", "missing", "pending")
	Status string `json:"status"`

	// Message provides additional context (e.g., why it's missing)
	Message string `json:"message,omitempty"`
}

// HistoryEntry represents a single deployment or reconciliation event
// This is a universal format that works across all GitOps tools
type HistoryEntry struct {
	// Timestamp is when this event occurred
	Timestamp time.Time `json:"timestamp"`

	// Revision is the version/commit that was deployed (e.g., "v1.2.3@abc123", "main@sha1:def456")
	Revision string `json:"revision"`

	// Status is the outcome (e.g., "deployed", "ReconciliationSucceeded", "failed")
	Status string `json:"status"`

	// Source describes what/who triggered the event (e.g., "manual sync by alice@example.com", "auto-sync")
	Source string `json:"source,omitempty"`

	// Message provides additional context about the event
	Message string `json:"message,omitempty"`

	// Duration is how long the operation took (for Flux reconciliations)
	Duration string `json:"duration,omitempty"`
}

// TraceConfigHub contains ConfigHub integration data for a trace
type TraceConfigHub struct {
	// UnitSlug is the ConfigHub unit that owns this resource
	UnitSlug string `json:"unitSlug,omitempty"`

	// SpaceID is the ConfigHub space ID
	SpaceID string `json:"spaceId,omitempty"`

	// SpaceName is the human-readable space name
	SpaceName string `json:"spaceName,omitempty"`

	// TargetID is the ConfigHub target (cluster) ID
	TargetID string `json:"targetId,omitempty"`

	// RevisionNum is the ConfigHub revision number deployed
	RevisionNum string `json:"revisionNum,omitempty"`

	// LiveRevisionNum is the live/latest revision in ConfigHub
	LiveRevisionNum string `json:"liveRevisionNum,omitempty"`

	// DriftDetected indicates if ConfigHub detected drift
	DriftDetected bool `json:"driftDetected,omitempty"`

	// RemediationURL is the URL to remediate issues in ConfigHub
	RemediationURL string `json:"remediationUrl,omitempty"`
}

// ResourceRef identifies a Kubernetes resource
type ResourceRef struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Group     string `json:"group,omitempty"`
	Version   string `json:"version,omitempty"`
}

// String returns a human-readable resource reference
func (r ResourceRef) String() string {
	if r.Namespace != "" {
		return fmt.Sprintf("%s/%s in %s", r.Kind, r.Name, r.Namespace)
	}
	return fmt.Sprintf("%s/%s", r.Kind, r.Name)
}

// ChainLink represents one level in the ownership chain
type ChainLink struct {
	// Kind is the Kubernetes resource kind (GitRepository, Kustomization, Deployment, etc.)
	Kind string `json:"kind"`

	// Name is the resource name
	Name string `json:"name"`

	// Namespace is the resource namespace
	Namespace string `json:"namespace"`

	// Ready indicates if this link is healthy
	Ready bool `json:"ready"`

	// Status is the human-readable status
	Status string `json:"status"`

	// StatusReason provides additional detail about the status
	StatusReason string `json:"statusReason,omitempty"`

	// Revision is the current revision (for sources and deployers)
	Revision string `json:"revision,omitempty"`

	// Path is the path in the repository (for Kustomizations)
	Path string `json:"path,omitempty"`

	// URL is the source URL (for GitRepositories)
	URL string `json:"url,omitempty"`

	// LastTransitionTime is when the status last changed
	LastTransitionTime *time.Time `json:"lastTransitionTime,omitempty"`

	// Message contains any error or status message
	Message string `json:"message,omitempty"`

	// Children lists resources managed by this link (for deployers)
	Children []ResourceRef `json:"children,omitempty"`

	// OCISource contains parsed OCI source information (for OCI-based sources)
	OCISource *OCISourceInfo `json:"ociSource,omitempty"`
}

// IsHealthy returns true if this chain link is in a healthy state
func (c ChainLink) IsHealthy() bool {
	return c.Ready
}

// Tracer provides GitOps trace functionality
type Tracer interface {
	// Trace returns the full ownership chain for a resource
	Trace(ctx context.Context, kind, name, namespace string) (*TraceResult, error)

	// Available returns true if this tracer's tool is available
	Available() bool

	// ToolName returns the name of the GitOps tool this tracer supports
	ToolName() string
}

// MultiTracer combines multiple tracers and auto-detects which to use
type MultiTracer struct {
	tracers []Tracer
}

// NewMultiTracer creates a tracer that tries multiple backends
func NewMultiTracer(tracers ...Tracer) *MultiTracer {
	return &MultiTracer{tracers: tracers}
}

// Trace tries each tracer until one succeeds
func (m *MultiTracer) Trace(ctx context.Context, kind, name, namespace string) (*TraceResult, error) {
	var lastErr error

	for _, t := range m.tracers {
		if !t.Available() {
			continue
		}

		result, err := t.Trace(ctx, kind, name, namespace)
		if err != nil {
			lastErr = err
			continue
		}

		// If we got a result with a chain, return it
		if result != nil && len(result.Chain) > 0 {
			return result, nil
		}
	}

	if lastErr != nil {
		return nil, lastErr
	}

	// Return empty result if no tracer found anything
	return &TraceResult{
		Object: ResourceRef{
			Kind:      kind,
			Name:      name,
			Namespace: namespace,
		},
		FullyManaged: false,
		TracedAt:     time.Now(),
		Error:        "resource not managed by any detected GitOps tool",
	}, nil
}

// AvailableTracers returns list of available tracer tool names
func (m *MultiTracer) AvailableTracers() []string {
	var available []string
	for _, t := range m.tracers {
		if t.Available() {
			available = append(available, t.ToolName())
		}
	}
	return available
}

// EnrichWithConfigHub adds ConfigHub metadata to a trace result from resource annotations
func (r *TraceResult) EnrichWithConfigHub(labels, annotations map[string]string) {
	// Check for ConfigHub ownership
	unitSlug := labels["confighub.com/UnitSlug"]
	if unitSlug == "" {
		unitSlug = annotations["confighub.com/UnitSlug"]
	}

	if unitSlug == "" {
		return // Not managed by ConfigHub
	}

	ch := &TraceConfigHub{
		UnitSlug: unitSlug,
	}

	// Extract other ConfigHub metadata
	if v := annotations["confighub.com/SpaceID"]; v != "" {
		ch.SpaceID = v
	}
	if v := labels["confighub.com/SpaceName"]; v != "" {
		ch.SpaceName = v
	} else if v := annotations["confighub.com/SpaceName"]; v != "" {
		ch.SpaceName = v
	}
	if v := annotations["confighub.com/TargetID"]; v != "" {
		ch.TargetID = v
	}
	if v := annotations["confighub.com/RevisionNum"]; v != "" {
		ch.RevisionNum = v
	}
	if v := annotations["confighub.com/LiveRevisionNum"]; v != "" {
		ch.LiveRevisionNum = v
	}
	if annotations["confighub.com/DriftDetected"] == "true" {
		ch.DriftDetected = true
	}

	// Build remediation URL if we have enough info
	if ch.SpaceID != "" && ch.UnitSlug != "" {
		ch.RemediationURL = fmt.Sprintf("https://confighub.com/spaces/%s/units/%s", ch.SpaceID, ch.UnitSlug)
	}

	r.ConfigHub = ch
}
