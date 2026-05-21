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
//
// Implementation note (Codex #446 round-4 fix): the per-controller
// helpers below pass the *owner* (Argo Application / Flux Kustomization
// or HelmRelease) to the tracer, not the workload itself. The Argo
// tracer rejects non-Application kinds; calling it with a Deployment
// fails with `gitSource=nil` for the entire receipt UX. The trace must
// be owner-aware, which is exactly what TraceByOwnership on each tracer
// already implements.
func CollectGitSourceAnchor(ctx context.Context, obj *unstructured.Unstructured) *GitSourceAnchor {
	if obj == nil {
		return nil
	}
	owner := DetectOwnership(obj)
	return CollectGitSourceAnchorForOwner(ctx, obj, owner)
}

// CollectGitSourceAnchorForOwner is the owner-aware variant used by
// callers (like the receipt command) that have already detected
// ownership and want to avoid re-running detection. The tracing branch
// matches the existing CollectGitSourceAnchor behavior: Argo first for
// Argo/ConfigHub, Flux fallback for ConfigHub-via-Flux, Flux for Flux.
func CollectGitSourceAnchorForOwner(ctx context.Context, obj *unstructured.Unstructured, owner Ownership) *GitSourceAnchor {
	if obj == nil {
		return nil
	}
	switch owner.Type {
	case OwnerArgo, OwnerConfigHub:
		// ConfigHub-managed resources are delivered by Argo (preferred) or
		// Flux. Try Argo first; fall through to Flux if the Argo tracer
		// finds nothing. Treating ConfigHub-via-Flux as a fallback keeps
		// the common case (ConfigHub-via-Argo) on the fast path.
		if anchor := collectArgoGitSource(ctx, obj, owner); anchor != nil {
			return anchor
		}
		if owner.Type == OwnerConfigHub {
			return collectFluxGitSource(ctx, obj, owner)
		}
		return nil
	case OwnerFlux:
		return collectFluxGitSource(ctx, obj, owner)
	default:
		return nil
	}
}

func collectArgoGitSource(ctx context.Context, obj *unstructured.Unstructured, owner Ownership) *GitSourceAnchor {
	tr := NewArgoTracer()
	if !tr.Available() {
		return nil
	}
	res, err := traceArgoForOwner(ctx, tr, obj, owner)
	if err != nil || res == nil || len(res.Chain) == 0 {
		return nil
	}
	return anchorFromChainRoot(res.Chain[0])
}

// traceArgoForOwner dispatches to the right Argo tracer entry point
// based on what's being traced:
//
//   - The Application itself → Trace(kind=Application, ...) — works directly
//   - A workload owned by an Argo Application → TraceApplication(owner.Name) —
//     uses the Application name resolved from the workload's labels/
//     annotations (Codex #446 round-4 fix)
//   - ConfigHub-managed resources delivered via Argo → same as above
func traceArgoForOwner(ctx context.Context, tr *ArgoTracer, obj *unstructured.Unstructured, owner Ownership) (*TraceResult, error) {
	if obj.GetKind() == "Application" {
		return tr.Trace(ctx, "Application", obj.GetName(), obj.GetNamespace())
	}
	// Owner name carries the Argo Application name when the workload was
	// detected as Argo-owned. For ConfigHub-via-Argo, the same applies —
	// the ownership detection resolves the upstream Application from
	// labels/annotations. Standalone-mode Trace would otherwise reject the
	// non-Application kind.
	if owner.Name != "" {
		return tr.TraceApplication(ctx, owner.Name)
	}
	// Last resort — let the tracer error tell the caller why we couldn't
	// resolve the anchor (so they can decide whether to surface the
	// failure or just record an INCONCLUSIVE receipt).
	return tr.Trace(ctx, obj.GetKind(), obj.GetName(), obj.GetNamespace())
}

func collectFluxGitSource(ctx context.Context, obj *unstructured.Unstructured, owner Ownership) *GitSourceAnchor {
	tr := NewFluxTracer()
	if !tr.Available() {
		return nil
	}
	res, err := traceFluxForOwner(ctx, tr, obj, owner)
	if err != nil || res == nil || len(res.Chain) == 0 {
		return nil
	}
	return anchorFromChainRoot(res.Chain[0])
}

// traceFluxForOwner dispatches Flux tracing similarly. Flux's Trace
// accepts Kustomization / HelmRelease kinds directly, but a workload
// owned by one must trace via the owner's name + kind. Falls back to
// the tracer's own Trace(kind, name, ns) for tracer-internal types
// (HelmRelease, Kustomization) when invoked on those directly.
func traceFluxForOwner(ctx context.Context, tr *FluxTracer, obj *unstructured.Unstructured, owner Ownership) (*TraceResult, error) {
	if obj.GetKind() == "Kustomization" || obj.GetKind() == "HelmRelease" {
		return tr.Trace(ctx, obj.GetKind(), obj.GetName(), obj.GetNamespace())
	}
	// For workloads owned by a Flux Kustomization / HelmRelease, use the
	// owner-aware tracer that already knows the right kind from the
	// ownership subType.
	if owner.Type == OwnerFlux && owner.Name != "" {
		return tr.TraceByOwnership(ctx, owner)
	}
	return tr.Trace(ctx, obj.GetKind(), obj.GetName(), obj.GetNamespace())
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
