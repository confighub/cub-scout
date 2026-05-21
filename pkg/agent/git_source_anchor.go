// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// GitSourceAnchor identifies where a field's value originated in source
// control. It pairs with FieldMutationAttribution to answer two halves of
// the same question:
//
//   - FieldMutationAttribution: which writer last set this field on the cluster
//   - GitSourceAnchor: which git source the controller is reconciling from
//
// At A2 the anchor is resolved at the resource level — every field on a
// given resource shares the same anchor. Stage B will refine to per-field
// anchors via rendering-aware back-resolution (Helm value paths, Kustomize
// patch lineage).
type GitSourceAnchor struct {
	// RepoURL is the source repository URL observed by the GitOps controller.
	// For Flux GitRepository this is spec.url; for Argo Application this is
	// spec.source.repoURL (or spec.sources[0].repoURL).
	RepoURL string `json:"repoUrl,omitempty"`

	// Revision is the observed commit SHA, tag, or OCI digest. May be empty
	// when the controller has not yet observed a revision.
	Revision string `json:"revision,omitempty"`

	// Path is the subdirectory within the repository used by the controller,
	// when known. For Flux Kustomization this is spec.path; for Argo
	// Application this is spec.source.path.
	Path string `json:"path,omitempty"`

	// File is the relative path (within Path) of the specific manifest file
	// that defines the resource. Populated by stage B back-resolution when
	// a local checkout is provided via --git-source-path. Empty otherwise.
	File string `json:"file,omitempty"`

	// Line is the line number within File where the field was set.
	// Populated by stage B back-resolution. Zero when File is empty or
	// when the field path could not be resolved to a single line (e.g.,
	// rendered Helm/Kustomize output).
	Line int `json:"line,omitempty"`
}

// IsEmpty reports whether the anchor carries no useful information.
func (g *GitSourceAnchor) IsEmpty() bool {
	if g == nil {
		return true
	}
	return strings.TrimSpace(g.RepoURL) == "" &&
		strings.TrimSpace(g.Revision) == "" &&
		strings.TrimSpace(g.Path) == "" &&
		strings.TrimSpace(g.File) == "" &&
		g.Line == 0
}

// CollectGitSourceAnchor traces the resource via the appropriate GitOps
// tracer and returns the source anchor at the root of the ownership chain
// (the GitRepository / OCIRepository / Argo Application source). Returns
// nil for any of:
//
//   - resource is nil
//   - resource has no recognized GitOps owner (Argo / Flux / ConfigHub-via-GitOps)
//   - the required tracer CLI is not available
//   - tracing fails or produces an empty chain
//   - the chain root carries no URL, Revision, or Path
//
// Best-effort by design: A2 is an enrichment of evidence, not a hard
// dependency. Callers should treat nil as "no anchor available" rather
// than an error condition.
func CollectGitSourceAnchor(ctx context.Context, obj *unstructured.Unstructured) *GitSourceAnchor {
	if obj == nil {
		return nil
	}
	owner := DetectOwnership(obj)
	switch owner.Type {
	case OwnerArgo, OwnerConfigHub:
		// ConfigHub-managed resources are delivered by Argo (preferred) or
		// Flux. Try Argo first; fall through to Flux if the Argo tracer
		// finds nothing. Treating ConfigHub-via-Flux as a fallback keeps
		// the common case (ConfigHub-via-Argo) on the fast path.
		if anchor := collectArgoGitSource(ctx, obj); anchor != nil {
			return anchor
		}
		if owner.Type == OwnerConfigHub {
			return collectFluxGitSource(ctx, obj)
		}
		return nil
	case OwnerFlux:
		return collectFluxGitSource(ctx, obj)
	default:
		return nil
	}
}

func collectArgoGitSource(ctx context.Context, obj *unstructured.Unstructured) *GitSourceAnchor {
	tr := NewArgoTracer()
	if !tr.Available() {
		return nil
	}
	res, err := tr.Trace(ctx, obj.GetKind(), obj.GetName(), obj.GetNamespace())
	if err != nil || res == nil || len(res.Chain) == 0 {
		return nil
	}
	return anchorFromChainRoot(res.Chain[0])
}

func collectFluxGitSource(ctx context.Context, obj *unstructured.Unstructured) *GitSourceAnchor {
	tr := NewFluxTracer()
	if !tr.Available() {
		return nil
	}
	res, err := tr.Trace(ctx, obj.GetKind(), obj.GetName(), obj.GetNamespace())
	if err != nil || res == nil || len(res.Chain) == 0 {
		return nil
	}
	return anchorFromChainRoot(res.Chain[0])
}

func anchorFromChainRoot(root ChainLink) *GitSourceAnchor {
	anchor := &GitSourceAnchor{
		RepoURL:  strings.TrimSpace(root.URL),
		Revision: strings.TrimSpace(root.Revision),
		Path:     strings.TrimSpace(root.Path),
	}
	if anchor.IsEmpty() {
		return nil
	}
	return anchor
}
