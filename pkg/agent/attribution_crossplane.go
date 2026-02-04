// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// BuildAttributionGraphFromCrossplaneLineage converts a CrossplaneLineage to an AttributionGraph.
// This is the bridge between the existing lineage resolver and the new attribution schema.
func BuildAttributionGraphFromCrossplaneLineage(lineage *CrossplaneLineage, bundleID string) *AttributionGraph {
	if lineage == nil {
		return nil
	}

	builder := NewAttributionGraphBuilder()
	if bundleID != "" {
		builder.SetSource(bundleID, time.Now(), "") // version set by caller if needed
	}

	// Add managed resource node (always present in a valid lineage)
	mrRef := resourceRefToAttributionRef(lineage.Managed.Ref)
	mrNode := AttributionNode{
		Type:    NodeMR,
		Ref:     mrRef,
		Present: lineage.Managed.Present,
	}
	builder.AddNode(mrNode)
	mrID := GenerateNodeID(mrNode.Type, mrNode.Ref)

	// Add composite resource (XR) node
	xrRef := resourceRefToAttributionRef(lineage.Composite.Ref)
	xrNode := AttributionNode{
		Type:    NodeXR,
		Ref:     xrRef,
		Present: lineage.Composite.Present,
	}
	builder.AddNode(xrNode)
	xrID := GenerateNodeID(xrNode.Type, xrNode.Ref)

	// Add edge: XR owns MR
	// Determine evidence from lineage.Evidence
	evidence := evidenceFromLineageEvidence(lineage.Evidence)
	builder.AddEdge(AttributionEdge{
		Type:     EdgeOwns,
		From:     xrID,
		To:       mrID,
		Evidence: evidence,
	})

	// Add claim node if present
	if lineage.Claim != nil {
		claimRef := resourceRefToAttributionRef(lineage.Claim.Ref)
		claimNode := AttributionNode{
			Type:    NodeClaim,
			Ref:     claimRef,
			Present: lineage.Claim.Present,
		}
		builder.AddNode(claimNode)
		claimID := GenerateNodeID(claimNode.Type, claimNode.Ref)

		// Add edge: Claim owns XR
		builder.AddEdge(AttributionEdge{
			Type:     EdgeOwns,
			From:     claimID,
			To:       xrID,
			Evidence: EvidenceClaimLabel,
		})
	}

	return builder.Build()
}

// resourceRefToAttributionRef converts a ResourceRef to an AttributionRef.
func resourceRefToAttributionRef(ref ResourceRef) AttributionRef {
	apiVersion := ""
	if ref.Group != "" && ref.Version != "" {
		apiVersion = ref.Group + "/" + ref.Version
	} else if ref.Version != "" {
		apiVersion = ref.Version
	}

	return AttributionRef{
		APIVersion: apiVersion,
		Kind:       ref.Kind,
		Namespace:  ref.Namespace,
		Name:       ref.Name,
		// UID is not available in ResourceRef, so we leave it empty
		// The node ID will fall back to canonical ref string
	}
}

// evidenceFromLineageEvidence maps CrossplaneLineage.Evidence strings to AttributionEvidence.
func evidenceFromLineageEvidence(evidence []string) AttributionEvidence {
	for _, e := range evidence {
		switch {
		case strings.HasPrefix(e, "label:crossplane.io/composite"):
			return EvidenceCompositeLabel
		case strings.HasPrefix(e, "ownerRef:"):
			return EvidenceOwnerReference
		case strings.HasPrefix(e, "label:crossplane.io/claim"):
			return EvidenceClaimLabel
		}
	}
	// Default to owner reference if no specific evidence found
	return EvidenceOwnerReference
}

// AttributionGraphForTarget builds an attribution graph for a target object.
// Returns nil if the target is not Crossplane-managed or lineage cannot be resolved.
func AttributionGraphForTarget(target *unstructured.Unstructured, objects []*unstructured.Unstructured, bundleID string) *AttributionGraph {
	if target == nil {
		return nil
	}

	lineage, ok := ResolveCrossplaneLineage(target, objects)
	if !ok || lineage == nil {
		return nil
	}

	return BuildAttributionGraphFromCrossplaneLineage(lineage, bundleID)
}
