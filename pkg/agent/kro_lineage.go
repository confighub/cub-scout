// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// KroLineageNode is a single node in a kro lineage chain.
// Present indicates whether the referenced object was found in the supplied object set.
type KroLineageNode struct {
	Ref     ResourceRef `json:"ref"`
	Present bool        `json:"present"`
}

// KroLineage describes platform lineage for a kro-managed resource.
// The chain is: managed resource -> kro instance -> optional ResourceGraphDefinition.
//
// This resolver is Kubernetes-local and deterministic:
// it only uses labels/annotations/ownerRefs already on objects.
type KroLineage struct {
	Managed    KroLineageNode  `json:"managed"`
	Instance   KroLineageNode  `json:"instance"`
	Definition *KroLineageNode `json:"definition,omitempty"`
	Evidence   []string        `json:"evidence,omitempty"`
}

// ResolveKroLineage builds a kro lineage chain for the given target object.
func ResolveKroLineage(target *unstructured.Unstructured, objects []*unstructured.Unstructured) (*KroLineage, bool) {
	return ResolveKroLineageWithIndex(target, NewUnstructuredIndex(objects))
}

// ResolveKroLineageWithIndex is like ResolveKroLineage but accepts a pre-built index.
func ResolveKroLineageWithIndex(target *unstructured.Unstructured, idx *UnstructuredIndex) (*KroLineage, bool) {
	if target == nil {
		return nil, false
	}

	own := DetectOwnership(target)
	if own.Type != OwnerKro {
		return nil, false
	}
	// ResourceGraphDefinition is a platform definition root, not a managed workload target.
	// Skip to avoid creating spurious self-groups in composition views.
	if own.SubType == "definition" {
		return nil, false
	}

	lineage := &KroLineage{
		Managed: KroLineageNode{Ref: resourceRefFromUnstructured(target), Present: true},
	}

	instanceRef, instancePresent, instanceObj := resolveKroInstanceRef(target, idx, &lineage.Evidence)
	if instanceRef.Name == "" {
		// Keep behavior conservative: if we cannot resolve an instance root,
		// still return a partial lineage to avoid false orphan assumptions.
		lineage.Evidence = append(lineage.Evidence, "instance:unresolved")
		lineage.Instance = KroLineageNode{
			Ref:     ResourceRef{Kind: "KroInstance", Name: own.Name, Namespace: target.GetNamespace()},
			Present: false,
		}
		return lineage, true
	}
	lineage.Instance = KroLineageNode{Ref: instanceRef, Present: instancePresent}

	defRef, defPresent := resolveKroDefinitionRef(target, instanceObj, idx, &lineage.Evidence)
	if defRef.Name != "" {
		lineage.Definition = &KroLineageNode{Ref: defRef, Present: defPresent}
	}

	return lineage, true
}

func resolveKroInstanceRef(target *unstructured.Unstructured, idx *UnstructuredIndex, evidence *[]string) (ResourceRef, bool, *unstructured.Unstructured) {
	// Primary: ownerRef to a kro instance.
	for _, or := range target.GetOwnerReferences() {
		if !isKroAPIVersion(or.APIVersion) || strings.EqualFold(or.Kind, "ResourceGraphDefinition") {
			continue
		}
		*evidence = append(*evidence, "ownerRef:"+or.APIVersion+"/"+or.Kind)
		ref := resourceRefFromOwnerRef(or, target.GetNamespace())
		obj := resolveOwnerRefObject(or, target.GetNamespace(), idx)
		if obj != nil {
			ref = resourceRefFromUnstructured(obj)
			return ref, true, obj
		}
		return ref, false, nil
	}

	// Fallback: if the target itself is a kro instance CR, treat it as the root.
	if isKroAPIVersion(target.GetAPIVersion()) && !strings.EqualFold(target.GetKind(), "ResourceGraphDefinition") {
		*evidence = append(*evidence, "self:apiGroup:"+strings.SplitN(target.GetAPIVersion(), "/", 2)[0])
		return resourceRefFromUnstructured(target), true, target
	}

	return ResourceRef{}, false, nil
}

func resolveKroDefinitionRef(target, instanceObj *unstructured.Unstructured, idx *UnstructuredIndex, evidence *[]string) (ResourceRef, bool) {
	// Metadata keys used by kro ecosystems to reference an RGD-like definition.
	metaKeys := []string{
		"kro.run/resource-graph-definition",
		"kro.run/resourcegraphdefinition",
		"kro.run/rgd",
		"kro.run/definition",
	}

	if ref, present, ok := findKroDefinitionByMetadata(target, idx, metaKeys, evidence); ok {
		return ref, present
	}
	if instanceObj != nil {
		if ref, present, ok := findKroDefinitionByMetadata(instanceObj, idx, metaKeys, evidence); ok {
			return ref, present
		}
	}

	// Fallback: ownerRef from instance -> ResourceGraphDefinition.
	ownerSource := target
	if instanceObj != nil {
		ownerSource = instanceObj
	}
	for _, or := range ownerSource.GetOwnerReferences() {
		if !isKroAPIVersion(or.APIVersion) || !strings.EqualFold(or.Kind, "ResourceGraphDefinition") {
			continue
		}
		*evidence = append(*evidence, "ownerRef:"+or.APIVersion+"/"+or.Kind)
		ref := resourceRefFromOwnerRef(or, "")
		if obj := resolveOwnerRefObject(or, "", idx); obj != nil {
			return resourceRefFromUnstructured(obj), true
		}
		return ref, false
	}

	return ResourceRef{}, false
}

func findKroDefinitionByMetadata(
	obj *unstructured.Unstructured,
	idx *UnstructuredIndex,
	keys []string,
	evidence *[]string,
) (ResourceRef, bool, bool) {
	if obj == nil {
		return ResourceRef{}, false, false
	}

	labels := obj.GetLabels()
	for _, key := range keys {
		if value := strings.TrimSpace(labels[key]); value != "" {
			*evidence = append(*evidence, "label:"+key)
			ref := ResourceRef{Kind: "ResourceGraphDefinition", Name: value}
			if found := idx.findByName(value); found != nil {
				return resourceRefFromUnstructured(found), true, true
			}
			return ref, false, true
		}
	}

	annotations := obj.GetAnnotations()
	for _, key := range keys {
		if value := strings.TrimSpace(annotations[key]); value != "" {
			*evidence = append(*evidence, "annotation:"+key)
			ref := ResourceRef{Kind: "ResourceGraphDefinition", Name: value}
			if found := idx.findByName(value); found != nil {
				return resourceRefFromUnstructured(found), true, true
			}
			return ref, false, true
		}
	}

	return ResourceRef{}, false, false
}

func resourceRefFromOwnerRef(or metav1.OwnerReference, namespace string) ResourceRef {
	ref := ResourceRef{
		Kind:      or.Kind,
		Name:      or.Name,
		Namespace: namespace,
	}
	parts := strings.SplitN(or.APIVersion, "/", 2)
	if len(parts) == 2 {
		ref.Group = parts[0]
		ref.Version = parts[1]
	} else {
		ref.Version = or.APIVersion
	}
	return ref
}

func resolveOwnerRefObject(or metav1.OwnerReference, namespace string, idx *UnstructuredIndex) *unstructured.Unstructured {
	obj := idx.findByGVKNameNamespace(or.APIVersion, or.Kind, or.Name, namespace)
	if obj != nil {
		return obj
	}
	// Fallback for cluster-scoped roots (e.g., definitions) or namespace drift.
	return idx.findByName(or.Name)
}
