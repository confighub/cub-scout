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
// If the XR/Claim objects are not present, the resolver still returns refs with Present=false.
func ResolveCrossplaneLineage(target *unstructured.Unstructured, objects []*unstructured.Unstructured) (*CrossplaneLineage, bool) {
	if target == nil {
		return nil, false
	}

	own := DetectOwnership(target)
	if own.Type != OwnerCrossplane {
		return nil, false
	}

	idx := newUnstructuredIndex(objects)
	lineage := &CrossplaneLineage{
		Managed: CrossplaneLineageNode{Ref: resourceRefFromUnstructured(target), Present: true},
	}

	// 1) Determine XR identity
	var xrRef ResourceRef
	var xrPresent bool

	// Prefer the Crossplane default composite label.
	if compName := target.GetLabels()["crossplane.io/composite"]; compName != "" {
		lineage.Evidence = append(lineage.Evidence, "label:crossplane.io/composite")
		// XR kind/group/version are not directly encoded in the label.
		// XRs use custom API groups defined by XRDs (e.g., database.example.org),
		// not crossplane.io. Try to find any object by name from provided objects.
		xrObj := idx.findByName(compName)
		if xrObj != nil {
			xrRef = resourceRefFromUnstructured(xrObj)
			xrPresent = true
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
				xrObj := idx.findByGVKNameNamespace(or.APIVersion, or.Kind, or.Name, "")
				if xrObj != nil {
					xrRef = resourceRefFromUnstructured(xrObj)
					xrPresent = true
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
		xrObj := idx.findByResourceRef(xrRef)
		if xrObj != nil {
			claimName = xrObj.GetLabels()["crossplane.io/claim-name"]
			claimNS = xrObj.GetLabels()["crossplane.io/claim-namespace"]
		}
	}
	if claimName != "" {
		lineage.Evidence = append(lineage.Evidence, "label:crossplane.io/claim-*")
		claimRef := ResourceRef{Kind: "Claim", Name: claimName, Namespace: claimNS}
		claimObj := idx.findByNameNamespace(claimName, claimNS)
		claimPresent := false
		if claimObj != nil {
			claimRef = resourceRefFromUnstructured(claimObj)
			claimPresent = true
		}
		lineage.Claim = &CrossplaneLineageNode{Ref: claimRef, Present: claimPresent}
	}

	return lineage, true
}

// unstructuredIndex provides simple deterministic lookups over a set of objects.
// It deliberately avoids discovery/pluralization so it can work with arbitrary CRDs.
type unstructuredIndex struct {
	byKey map[string]*unstructured.Unstructured
	all   []*unstructured.Unstructured
}

func newUnstructuredIndex(objects []*unstructured.Unstructured) *unstructuredIndex {
	idx := &unstructuredIndex{byKey: make(map[string]*unstructured.Unstructured), all: objects}
	for _, o := range objects {
		if o == nil {
			continue
		}
		key := idx.keyFor(o.GetAPIVersion(), o.GetKind(), o.GetName(), o.GetNamespace())
		idx.byKey[key] = o
	}
	return idx
}

func (i *unstructuredIndex) keyFor(apiVersion, kind, name, namespace string) string {
	return apiVersion + "|" + kind + "|" + namespace + "|" + name
}

func (i *unstructuredIndex) findByGVKNameNamespace(apiVersion, kind, name, namespace string) *unstructured.Unstructured {
	return i.byKey[i.keyFor(apiVersion, kind, name, namespace)]
}

func (i *unstructuredIndex) findByResourceRef(ref ResourceRef) *unstructured.Unstructured {
	// ResourceRef may not contain apiVersion/kind in all cases; fall back to name+namespace scan.
	if ref.Kind != "" && ref.Group != "" && ref.Version != "" {
		apiVersion := ref.Group + "/" + ref.Version
		if u := i.findByGVKNameNamespace(apiVersion, ref.Kind, ref.Name, ref.Namespace); u != nil {
			return u
		}
	}
	return i.findByNameNamespace(ref.Name, ref.Namespace)
}

func (i *unstructuredIndex) findByNameNamespace(name, namespace string) *unstructured.Unstructured {
	if name == "" {
		return nil
	}
	for _, o := range i.all {
		if o == nil {
			continue
		}
		if o.GetName() == name && o.GetNamespace() == namespace {
			return o
		}
	}
	return nil
}

func (i *unstructuredIndex) findByName(name string) *unstructured.Unstructured {
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
