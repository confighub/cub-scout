// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// SourceRef represents a reference from a Flux deployer to its source.
// This is extracted from spec.sourceRef in Kustomization, HelmRelease, etc.
type SourceRef struct {
	// Kind is the source type: GitRepository, OCIRepository, HelmRepository, Bucket
	Kind string `json:"kind"`

	// Name is the source resource name
	Name string `json:"name"`

	// Namespace is the source resource namespace (may differ from deployer)
	Namespace string `json:"namespace"`
}

// Key returns the canonical key for this source reference: <kind>/<namespace>/<name>
func (s SourceRef) Key() string {
	return fmt.Sprintf("%s/%s/%s", s.Kind, s.Namespace, s.Name)
}

// String returns a human-readable representation
func (s SourceRef) String() string {
	return fmt.Sprintf("%s/%s in %s", s.Kind, s.Name, s.Namespace)
}

// IsValid returns true if all required fields are present
func (s SourceRef) IsValid() bool {
	return s.Kind != "" && s.Name != "" && s.Namespace != ""
}

// IsOCI returns true if this is an OCI-based source
func (s SourceRef) IsOCI() bool {
	return s.Kind == "OCIRepository"
}

// ParseSourceRef extracts the sourceRef from a Flux deployer resource.
// The deployerNamespace is used as default when sourceRef.namespace is not specified.
//
// Supports:
// - Kustomization (spec.sourceRef)
// - HelmRelease (spec.chartRef or spec.chart.spec.sourceRef)
func ParseSourceRef(resource *unstructured.Unstructured, deployerNamespace string) (SourceRef, bool) {
	if resource == nil {
		return SourceRef{}, false
	}

	kind := resource.GetKind()

	switch kind {
	case "Kustomization":
		return parseKustomizationSourceRef(resource, deployerNamespace)
	case "HelmRelease":
		return parseHelmReleaseSourceRef(resource, deployerNamespace)
	default:
		return SourceRef{}, false
	}
}

// parseKustomizationSourceRef extracts sourceRef from a Flux Kustomization
func parseKustomizationSourceRef(resource *unstructured.Unstructured, deployerNamespace string) (SourceRef, bool) {
	sourceKind, _, _ := unstructured.NestedString(resource.Object, "spec", "sourceRef", "kind")
	sourceName, _, _ := unstructured.NestedString(resource.Object, "spec", "sourceRef", "name")
	sourceNS, _, _ := unstructured.NestedString(resource.Object, "spec", "sourceRef", "namespace")

	if sourceKind == "" || sourceName == "" {
		return SourceRef{}, false
	}

	// Default namespace to deployer's namespace if not specified
	if sourceNS == "" {
		sourceNS = deployerNamespace
	}

	return SourceRef{
		Kind:      sourceKind,
		Name:      sourceName,
		Namespace: sourceNS,
	}, true
}

// parseHelmReleaseSourceRef extracts sourceRef from a Flux HelmRelease
// HelmRelease can have sourceRef in two places:
// 1. spec.chartRef (for OCIRepository sources)
// 2. spec.chart.spec.sourceRef (for HelmRepository sources)
func parseHelmReleaseSourceRef(resource *unstructured.Unstructured, deployerNamespace string) (SourceRef, bool) {
	// First, try spec.chartRef (OCIRepository, Flux v2.2+)
	chartRefKind, found1, _ := unstructured.NestedString(resource.Object, "spec", "chartRef", "kind")
	chartRefName, found2, _ := unstructured.NestedString(resource.Object, "spec", "chartRef", "name")
	chartRefNS, _, _ := unstructured.NestedString(resource.Object, "spec", "chartRef", "namespace")

	if found1 && found2 && chartRefKind != "" && chartRefName != "" {
		if chartRefNS == "" {
			chartRefNS = deployerNamespace
		}
		return SourceRef{
			Kind:      chartRefKind,
			Name:      chartRefName,
			Namespace: chartRefNS,
		}, true
	}

	// Fall back to spec.chart.spec.sourceRef (HelmRepository)
	sourceKind, _, _ := unstructured.NestedString(resource.Object, "spec", "chart", "spec", "sourceRef", "kind")
	sourceName, _, _ := unstructured.NestedString(resource.Object, "spec", "chart", "spec", "sourceRef", "name")
	sourceNS, _, _ := unstructured.NestedString(resource.Object, "spec", "chart", "spec", "sourceRef", "namespace")

	if sourceKind == "" || sourceName == "" {
		return SourceRef{}, false
	}

	if sourceNS == "" {
		sourceNS = deployerNamespace
	}

	return SourceRef{
		Kind:      sourceKind,
		Name:      sourceName,
		Namespace: sourceNS,
	}, true
}

// DeployerRef represents a Flux deployer (Kustomization, HelmRelease) that applies resources.
type DeployerRef struct {
	// Kind is the deployer type: Kustomization, HelmRelease
	Kind string `json:"kind"`

	// Name is the deployer resource name
	Name string `json:"name"`

	// Namespace is the deployer resource namespace
	Namespace string `json:"namespace"`

	// SourceRef is the source this deployer pulls from
	SourceRef *SourceRef `json:"sourceRef,omitempty"`
}

// Key returns the canonical key for this deployer: <kind>/<namespace>/<name>
func (d DeployerRef) Key() string {
	return fmt.Sprintf("%s/%s/%s", d.Kind, d.Namespace, d.Name)
}

// String returns a human-readable representation
func (d DeployerRef) String() string {
	return fmt.Sprintf("%s/%s in %s", d.Kind, d.Name, d.Namespace)
}

// ParseDeployerRef extracts deployer information from a Flux resource
func ParseDeployerRef(resource *unstructured.Unstructured) (DeployerRef, bool) {
	if resource == nil {
		return DeployerRef{}, false
	}

	kind := resource.GetKind()
	if kind != "Kustomization" && kind != "HelmRelease" {
		return DeployerRef{}, false
	}

	namespace := resource.GetNamespace()
	name := resource.GetName()

	ref := DeployerRef{
		Kind:      kind,
		Name:      name,
		Namespace: namespace,
	}

	// Parse the source reference
	if sourceRef, ok := ParseSourceRef(resource, namespace); ok {
		ref.SourceRef = &sourceRef
	}

	return ref, true
}

// LinkSourceToDeployer creates a canonical link key between source and deployer.
// Format: <source-key> -> <deployer-key>
func LinkSourceToDeployer(source SourceRef, deployer DeployerRef) string {
	return fmt.Sprintf("%s -> %s", source.Key(), deployer.Key())
}

// ParseSourceRefKey parses a canonical source key back to a SourceRef.
// Input format: <kind>/<namespace>/<name>
func ParseSourceRefKey(key string) (SourceRef, bool) {
	parts := strings.SplitN(key, "/", 3)
	if len(parts) != 3 {
		return SourceRef{}, false
	}
	return SourceRef{
		Kind:      parts[0],
		Namespace: parts[1],
		Name:      parts[2],
	}, true
}
