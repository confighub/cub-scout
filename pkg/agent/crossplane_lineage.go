// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// CrossplaneLineageNode is a single node in a Crossplane lineage chain.
// Present indicates whether the referenced object was found in the supplied object set.
type CrossplaneLineageNode struct {
	Ref     ResourceRef `json:"ref"`
	Present bool        `json:"present"`
}

// CrossplaneLineage describes the XR-first platform lineage for a Crossplane-managed resource.
// The chain is: Managed (or composed) resource -> Composite Resource (XR) -> optional Claim.
//
// This resolver is intentionally Kubernetes-local and deterministic:
// it only uses fields present on objects (labels, annotations, ownerRefs).
// It does not call external APIs.
type CrossplaneLineage struct {
	Managed   CrossplaneLineageNode  `json:"managed"`
	Composite CrossplaneLineageNode  `json:"composite"`
	Claim     *CrossplaneLineageNode `json:"claim,omitempty"`

	// Evidence describes which signals were used to build the lineage.
	Evidence []string `json:"evidence,omitempty"`
}

// ResolveCrossplaneLineage builds a Crossplane lineage chain for the given target object.
//
// The resolver is XR-first:
// - If a composite label (crossplane.io/composite) is present, it is sufficient to identify the XR.
// - Claim labels (crossplane.io/claim-*) are optional enrichment.
// - OwnerReferences to *.crossplane.io / *.upbound.io groups are used when composite label is absent.
//
// The objects slice should include, at minimum, Crossplane XRs and (optionally) Claims.
// Missing, foreign-namespace or ambiguous parents remain Present=false.
// Label-only joins do not establish UID identity; ownerRef joins check UID when supplied.
//
// For batch operations, prefer ResolveCrossplaneLineageWithIndex to avoid O(n²) index rebuilds.
func ResolveCrossplaneLineage(target *unstructured.Unstructured, objects []*unstructured.Unstructured) (*CrossplaneLineage, bool) {
	return ResolveCrossplaneLineageWithIndex(target, NewUnstructuredIndex(objects))
}

// ResolveCrossplaneLineageWithIndex is like ResolveCrossplaneLineage but accepts a pre-built index.
// Use this for batch operations to avoid rebuilding the index for each call.
//
// Example:
//
//	idx := agent.NewUnstructuredIndex(objects)
//	for _, obj := range objects {
//	    lineage, ok := agent.ResolveCrossplaneLineageWithIndex(obj, idx)
//	    // ...
//	}
func ResolveCrossplaneLineageWithIndex(target *unstructured.Unstructured, idx *UnstructuredIndex) (*CrossplaneLineage, bool) {
	if target == nil {
		return nil, false
	}

	own := DetectOwnership(target)
	if own.Type != OwnerCrossplane {
		return nil, false
	}

	lineage := &CrossplaneLineage{
		Managed: CrossplaneLineageNode{Ref: resourceRefFromUnstructured(target), Present: true},
	}

	// 1) Determine XR identity
	var xrRef ResourceRef
	var xrPresent bool
	var xrObject *unstructured.Unstructured

	// Prefer the Crossplane default composite label.
	if compName := target.GetLabels()["crossplane.io/composite"]; compName != "" {
		lineage.Evidence = append(lineage.Evidence, "label:crossplane.io/composite")
		// XR kind/group/version are not directly encoded in the label.
		// XRs use custom API groups defined by XRDs (e.g., database.example.org),
		// not crossplane.io. Resolve only a unique candidate in a legal parent namespace.
		xrObj, ambiguous := crossplaneUniqueObject(idx, target, func(o *unstructured.Unstructured) bool {
			return o.GetName() == compName && (o.GetNamespace() == "" || o.GetNamespace() == target.GetNamespace())
		})
		if ambiguous {
			lineage.Evidence = append(lineage.Evidence, "xr:ambiguous")
		} else if xrObj == nil {
			lineage.Evidence = append(lineage.Evidence, "xr:unresolved")
		}
		if xrObj != nil {
			xrRef = resourceRefFromUnstructured(xrObj)
			xrPresent = true
			xrObject = xrObj
		} else {
			// Best-effort: unknown G/V/K, but preserve the name.
			xrRef = ResourceRef{Kind: "CompositeResource", Name: compName}
			xrPresent = false
		}
	} else {
		// Fall back to ownerRefs pointing to Crossplane API groups.
		for _, or := range target.GetOwnerReferences() {
			gv := strings.SplitN(or.APIVersion, "/", 2)
			group := gv[0]
			if strings.Contains(group, "crossplane.io") || strings.Contains(group, "upbound.io") {
				lineage.Evidence = append(lineage.Evidence, "ownerRef:"+or.APIVersion+"/"+or.Kind)
				xrRef = ResourceRef{Kind: or.Kind, Name: or.Name, Group: group}
				if len(gv) == 2 {
					xrRef.Version = gv[1]
				}
				xrObj, ambiguous := crossplaneUniqueObject(idx, target, func(o *unstructured.Unstructured) bool {
					return o.GetAPIVersion() == or.APIVersion && o.GetKind() == or.Kind && o.GetName() == or.Name &&
						(o.GetNamespace() == "" || o.GetNamespace() == target.GetNamespace()) &&
						(or.UID == "" || o.GetUID() == or.UID)
				})
				if ambiguous {
					lineage.Evidence = append(lineage.Evidence, "xr:ambiguous")
				} else if xrObj == nil {
					if or.UID != "" {
						lineage.Evidence = append(lineage.Evidence, "xr:owner_uid_not_observed")
					} else {
						lineage.Evidence = append(lineage.Evidence, "xr:unresolved")
					}
				}
				if xrObj != nil {
					xrRef = resourceRefFromUnstructured(xrObj)
					xrPresent = true
					xrObject = xrObj
				}
				break
			}
		}
	}

	if xrRef.Name == "" {
		// We know this is Crossplane-owned (DetectOwnership), but cannot identify the XR.
		lineage.Evidence = append(lineage.Evidence, "xr:unresolved")
		lineage.Composite = CrossplaneLineageNode{Ref: ResourceRef{Kind: "CompositeResource"}, Present: false}
		return lineage, true
	}
	lineage.Composite = CrossplaneLineageNode{Ref: xrRef, Present: xrPresent}

	// 2) Determine Claim (optional) from claim labels on the target or XR
	claimName := target.GetLabels()["crossplane.io/claim-name"]
	claimNS := target.GetLabels()["crossplane.io/claim-namespace"]
	if claimName == "" && xrPresent {
		// Prefer claim metadata from XR if available.
		xrObj := xrObject
		if xrObj != nil {
			claimName = xrObj.GetLabels()["crossplane.io/claim-name"]
			claimNS = xrObj.GetLabels()["crossplane.io/claim-namespace"]
		}
	}
	if claimName != "" {
		lineage.Evidence = append(lineage.Evidence, "label:crossplane.io/claim-*")
		claimRef := ResourceRef{Kind: "Claim", Name: claimName, Namespace: claimNS}
		claimObj, ambiguous := crossplaneUniqueObject(idx, target, func(o *unstructured.Unstructured) bool {
			return o.GetName() == claimName && o.GetNamespace() == claimNS
		})
		if ambiguous {
			lineage.Evidence = append(lineage.Evidence, "claim:ambiguous")
		}
		claimPresent := false
		if claimObj != nil {
			claimRef = resourceRefFromUnstructured(claimObj)
			claimPresent = true
		}
		lineage.Claim = &CrossplaneLineageNode{Ref: claimRef, Present: claimPresent}
	}

	return lineage, true
}

// crossplaneUniqueObject refuses a name-only/type/UID join when more than one
// supplied object satisfies its evidence. Scope comes only from Kubernetes owner
// locality: a namespaced child may have a same-namespace or cluster parent. Labels
// do not identify parent GVK or scope, so conflicting candidates stay unresolved.
// Keep this separate from the generic index to avoid changing other resolvers.
func crossplaneUniqueObject(idx *UnstructuredIndex, target *unstructured.Unstructured, matches func(*unstructured.Unstructured) bool) (*unstructured.Unstructured, bool) {
	if idx == nil {
		return nil, false
	}
	var found *unstructured.Unstructured
	for _, object := range idx.all {
		if object == nil || (target != nil && object.GetAPIVersion() == target.GetAPIVersion() && object.GetKind() == target.GetKind() && object.GetNamespace() == target.GetNamespace() && object.GetName() == target.GetName()) || !matches(object) {
			continue
		}
		if found != nil {
			return nil, true
		}
		found = object
	}
	return found, false
}

// UnstructuredIndex provides simple deterministic lookups over a set of objects.
// It deliberately avoids discovery/pluralization so it can work with arbitrary CRDs.
//
// Exported to allow callers to build the index once and reuse it across multiple
// resolver invocations, avoiding O(n²) index rebuilds on large object sets.
type UnstructuredIndex struct {
	byKey map[string]*unstructured.Unstructured
	all   []*unstructured.Unstructured
}

// NewUnstructuredIndex builds an index over the given objects.
// Build this once and pass to ResolveCrossplaneLineageWithIndex for efficient batch operations.
func NewUnstructuredIndex(objects []*unstructured.Unstructured) *UnstructuredIndex {
	idx := &UnstructuredIndex{byKey: make(map[string]*unstructured.Unstructured), all: objects}
	for _, o := range objects {
		if o == nil {
			continue
		}
		key := idx.keyFor(o.GetAPIVersion(), o.GetKind(), o.GetName(), o.GetNamespace())
		idx.byKey[key] = o
	}
	return idx
}

// Len returns the number of indexed objects.
func (i *UnstructuredIndex) Len() int {
	return len(i.all)
}

func (i *UnstructuredIndex) keyFor(apiVersion, kind, name, namespace string) string {
	return apiVersion + "|" + kind + "|" + namespace + "|" + name
}

func (i *UnstructuredIndex) findByGVKNameNamespace(apiVersion, kind, name, namespace string) *unstructured.Unstructured {
	return i.byKey[i.keyFor(apiVersion, kind, name, namespace)]
}

func (i *UnstructuredIndex) findByName(name string) *unstructured.Unstructured {
	if name == "" {
		return nil
	}
	for _, o := range i.all {
		if o == nil {
			continue
		}
		if o.GetName() == name {
			return o
		}
	}
	return nil
}

func resourceRefFromUnstructured(u *unstructured.Unstructured) ResourceRef {
	ref := ResourceRef{Kind: u.GetKind(), Name: u.GetName(), Namespace: u.GetNamespace()}
	apiVersion := u.GetAPIVersion()
	if apiVersion == "" {
		return ref
	}
	if parts := strings.SplitN(apiVersion, "/", 2); len(parts) == 2 {
		ref.Group = parts[0]
		ref.Version = parts[1]
	} else {
		ref.Version = apiVersion
	}
	return ref
}
