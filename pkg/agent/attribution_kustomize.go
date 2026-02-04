// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Package agent provides Kustomize overlay attribution for attribution-graph.v1.
//
// When a user provides --kustomize <dir>, we create an ownership edge from the
// kustomize overlay to the debug target. This is explicit user-provided context,
// not inferred from cluster state.
//
// Contract:
// - Overlay ownership requires explicit --kustomize flag (no guessing)
// - Path normalization is deterministic (no absolute path leakage)
// - Non-fatal on invalid paths (skip enrichment, bundle still written)
package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
)

// BuildKustomizeOverlayOwnership creates an attribution graph fragment representing
// kustomize overlay ownership of the debug target.
//
// The overlay node uses a deterministic ref based on the provided path:
// - Relative paths are kept as-is (normalized to slash form)
// - Absolute paths use basename + short hash to avoid leaking full paths
//
// The targetNodeType parameter specifies which node type to use for the target:
// - Use NodeMR if the target is a Crossplane managed resource
// - Use NodeK8sObject for generic Kubernetes objects
//
// Returns nil graph if kustomizeDir is empty.
func BuildKustomizeOverlayOwnership(kustomizeDir string, target AttributionRef, targetNodeType AttributionNodeType) (*AttributionGraph, error) {
	if kustomizeDir == "" {
		return nil, nil
	}

	builder := NewAttributionGraphBuilder()

	// Create overlay node with deterministic ref
	overlayRef := AttributionRef{
		Kind: "kustomize",
		Name: normalizeKustomizePath(kustomizeDir),
	}
	overlayNode := AttributionNode{
		Type:    NodeKustomizeOverlay,
		Ref:     overlayRef,
		Present: true,
	}
	builder.AddNode(overlayNode)

	// Create target node with specified type
	targetNode := AttributionNode{
		Type:    targetNodeType,
		Ref:     target,
		Present: true,
	}
	builder.AddNode(targetNode)

	// Create owns edge: overlay -> target
	overlayID := GenerateNodeID(overlayNode.Type, overlayNode.Ref)
	targetID := GenerateNodeID(targetNode.Type, targetNode.Ref)

	builder.AddEdge(AttributionEdge{
		Type:     EdgeOwns,
		From:     overlayID,
		To:       targetID,
		Evidence: EvidenceKustomizeOverlay,
	})

	return builder.Build(), nil
}

// normalizeKustomizePath creates a deterministic, safe path key from the input.
// - Relative paths: cleaned and converted to slash form
// - Absolute paths: basename + short hash (avoids leaking /Users/... paths)
func normalizeKustomizePath(path string) string {
	cleaned := filepath.Clean(path)

	if filepath.IsAbs(cleaned) {
		// Absolute path: use basename + hash to avoid leaking full path
		base := filepath.Base(cleaned)
		hash := shortHash(cleaned)
		return base + "#" + hash
	}

	// Relative path: convert to slash form for cross-platform consistency
	slashed := filepath.ToSlash(cleaned)

	// Strip leading "./" if present
	if strings.HasPrefix(slashed, "./") {
		slashed = slashed[2:]
	}

	return slashed
}

// shortHash returns a short (8 char) hash of the input string.
func shortHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:4])
}
