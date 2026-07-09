// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Owner type constants
const (
	OwnerFlux       = "flux"
	OwnerArgo       = "argo"
	OwnerHelm       = "helm"
	OwnerTerraform  = "terraform"
	OwnerConfigHub  = "confighub"
	OwnerSveltos    = "sveltos"
	OwnerModelplane = "modelplane"
	OwnerCrossplane = "crossplane"
	OwnerKro        = "kro"
	OwnerCustom     = "custom"
	OwnerKubernetes = "k8s"
	OwnerUnknown    = "unknown"
)

// DetectOwnership examines a resource and determines who manages it
func DetectOwnership(resource *unstructured.Unstructured) Ownership {
	labels := resource.GetLabels()
	annotations := resource.GetAnnotations()

	// Check for Flux ownership
	if ownership := detectFluxOwnership(labels, annotations, resource); ownership.Type != "" {
		return ownership
	}

	// Check for Argo CD ownership
	if ownership := detectArgoOwnership(labels, annotations); ownership.Type != "" {
		return ownership
	}

	// Check for Sveltos ownership before generic Helm/Crossplane markers.
	if ownership := detectSveltosOwnership(labels, annotations, resource); ownership.Type != "" {
		return ownership
	}

	// Check for Modelplane ownership before generic Helm/Crossplane markers.
	if ownership := detectModelplaneOwnership(labels, annotations, resource); ownership.Type != "" {
		return ownership
	}

	// Check for Helm ownership
	if ownership := detectHelmOwnership(labels, annotations); ownership.Type != "" {
		return ownership
	}

	// Check for Terraform ownership
	if ownership := detectTerraformOwnership(labels, annotations); ownership.Type != "" {
		return ownership
	}

	// Check for ConfigHub ownership
	if ownership := detectConfigHubOwnership(labels, annotations); ownership.Type != "" {
		return ownership
	}

	// Check for Crossplane system/control-plane ownership
	if ownership := detectCrossplaneSystemOwnership(resource); ownership.Type != "" {
		return ownership
	}

	// Check for Crossplane ownership
	if ownership := detectCrossplaneOwnership(labels, annotations, resource); ownership.Type != "" {
		return ownership
	}

	// Check for kro ownership
	if ownership := detectKroOwnership(labels, annotations, resource); ownership.Type != "" {
		return ownership
	}

	// Check for custom configured ownership detectors.
	// Built-ins intentionally run first to preserve contract precedence.
	if ownership := detectCustomOwnership(labels, annotations); ownership.Type != "" {
		return ownership
	}

	// Check for Kubernetes native ownership (via OwnerReferences)
	if ownership := detectK8sOwnership(resource); ownership.Type != "" {
		return ownership
	}

	return Ownership{Type: OwnerUnknown}
}

func detectFluxOwnership(labels, annotations map[string]string, resource *unstructured.Unstructured) Ownership {
	// Flux Kustomization
	if name, ok := labels["kustomize.toolkit.fluxcd.io/name"]; ok {
		ns := labels["kustomize.toolkit.fluxcd.io/namespace"]
		return Ownership{
			Type:       OwnerFlux,
			SubType:    "kustomization",
			Name:       name,
			Namespace:  ns,
			Source:     "label:kustomize.toolkit.fluxcd.io/name",
			Confidence: "high",
		}
	}

	// Flux HelmRelease
	if name, ok := labels["helm.toolkit.fluxcd.io/name"]; ok {
		ns := labels["helm.toolkit.fluxcd.io/namespace"]
		return Ownership{
			Type:       OwnerFlux,
			SubType:    "helmrelease",
			Name:       name,
			Namespace:  ns,
			Source:     "label:helm.toolkit.fluxcd.io/name",
			Confidence: "high",
		}
	}

	if resource != nil && fluxOperatorAPIGroup(resource.GetAPIVersion()) {
		return Ownership{
			Type:       OwnerFlux,
			SubType:    strings.ToLower(resource.GetKind()),
			Name:       resource.GetName(),
			Namespace:  resource.GetNamespace(),
			Source:     "apiGroup:fluxcd.controlplane.io",
			Confidence: "high",
		}
	}

	return Ownership{}
}

func fluxOperatorAPIGroup(apiVersion string) bool {
	group, _, ok := strings.Cut(apiVersion, "/")
	return ok && group == "fluxcd.controlplane.io"
}

func detectArgoOwnership(labels, annotations map[string]string) Ownership {
	// Argo CD Application - check for argocd.argoproj.io/instance label
	if argoInstance, isArgo := labels["argocd.argoproj.io/instance"]; isArgo {
		// Prefer the Argo-specific label value
		name := argoInstance
		source := "label:argocd.argoproj.io/instance"
		if name == "" {
			// Fall back to app.kubernetes.io/instance if Argo label is empty
			name = labels["app.kubernetes.io/instance"]
			source = "label:app.kubernetes.io/instance"
		}
		if name != "" {
			return Ownership{
				Type:       OwnerArgo,
				SubType:    "application",
				Name:       name,
				Source:     source,
				Confidence: "medium",
			}
		}
	}

	// Alternative: check tracking-id annotation
	if tracking, ok := annotations["argocd.argoproj.io/tracking-id"]; ok && tracking != "" {
		// Format: <app-name>:<group>/<kind>:<namespace>/<name>
		// Handle malformed tracking IDs gracefully
		parts := strings.SplitN(tracking, ":", 2)
		if len(parts) > 0 && parts[0] != "" {
			return Ownership{
				Type:       OwnerArgo,
				SubType:    "application",
				Name:       parts[0],
				Source:     "annotation:argocd.argoproj.io/tracking-id",
				Confidence: "medium",
			}
		}
	}

	return Ownership{}
}

func detectHelmOwnership(labels, annotations map[string]string) Ownership {
	// Helm release
	if release, ok := labels["app.kubernetes.io/managed-by"]; ok && release == "Helm" {
		name := labels["app.kubernetes.io/instance"]
		return Ownership{
			Type:       OwnerHelm,
			SubType:    "release",
			Name:       name,
			Source:     "label:app.kubernetes.io/managed-by=Helm",
			Confidence: "high",
		}
	}

	// Legacy helm labels
	if release, ok := labels["helm.sh/chart"]; ok {
		name := labels["app.kubernetes.io/instance"]
		if name == "" {
			name = release
		}
		return Ownership{
			Type:       OwnerHelm,
			SubType:    "release",
			Name:       name,
			Source:     "label:helm.sh/chart",
			Confidence: "high",
		}
	}

	return Ownership{}
}

func detectTerraformOwnership(labels, annotations map[string]string) Ownership {
	// Terraform Kubernetes provider
	if _, ok := annotations["app.terraform.io/run-id"]; ok {
		workspace := annotations["app.terraform.io/workspace-name"]
		return Ownership{
			Type:       OwnerTerraform,
			SubType:    "workspace",
			Name:       workspace,
			Source:     "annotation:app.terraform.io/run-id",
			Confidence: "high",
		}
	}

	// Alternative terraform markers
	if _, ok := labels["app.terraform.io/managed"]; ok {
		return Ownership{
			Type:       OwnerTerraform,
			SubType:    "managed",
			Source:     "label:app.terraform.io/managed",
			Confidence: "medium",
		}
	}

	return Ownership{}
}

func detectConfigHubOwnership(labels, annotations map[string]string) Ownership {
	// ConfigHub Unit - check both label and annotation
	// Label: confighub.com/UnitSlug
	// Annotations: confighub.com/SpaceName, confighub.com/SpaceID, confighub.com/RevisionNum
	if unit, ok := labels["confighub.com/UnitSlug"]; ok {
		space := annotations["confighub.com/SpaceName"]
		if space == "" {
			space = labels["confighub.com/SpaceName"]
		}
		return Ownership{
			Type:       OwnerConfigHub,
			SubType:    "unit",
			Name:       unit,
			Namespace:  space,
			Source:     "label:confighub.com/UnitSlug",
			Confidence: "high",
		}
	}

	// Also check annotation (some resources may only have annotation)
	if unit, ok := annotations["confighub.com/UnitSlug"]; ok {
		space := annotations["confighub.com/SpaceName"]
		return Ownership{
			Type:       OwnerConfigHub,
			SubType:    "unit",
			Name:       unit,
			Namespace:  space,
			Source:     "annotation:confighub.com/UnitSlug",
			Confidence: "high",
		}
	}

	return Ownership{}
}

func detectSveltosOwnership(labels, annotations map[string]string, resource *unstructured.Unstructured) Ownership {
	if ownerKind, ownerName := annotations["projectsveltos.io/owner-kind"], annotations["projectsveltos.io/owner-name"]; ownerKind != "" && ownerName != "" {
		return Ownership{
			Type:       OwnerSveltos,
			SubType:    strings.ToLower(ownerKind),
			Name:       ownerName,
			Source:     "annotation:projectsveltos.io/owner-kind/name",
			Confidence: "high",
		}
	}

	if deployedBy := annotations["projectsveltos.io/deployed-by-sveltos"]; deployedBy != "" && !strings.EqualFold(deployedBy, "false") {
		return Ownership{
			Type:       OwnerSveltos,
			SubType:    "deployed-resource",
			Namespace:  resource.GetNamespace(),
			Source:     "annotation:projectsveltos.io/deployed-by-sveltos",
			Confidence: "medium",
		}
	}

	for _, owner := range resource.GetOwnerReferences() {
		if !isSveltosAPIVersion(owner.APIVersion) {
			continue
		}
		return Ownership{
			Type:       OwnerSveltos,
			SubType:    strings.ToLower(owner.Kind),
			Name:       owner.Name,
			Namespace:  resource.GetNamespace(),
			Source:     "ownerRef:" + owner.APIVersion,
			Confidence: "high",
		}
	}

	if apiVersion := resource.GetAPIVersion(); isSveltosAPIVersion(apiVersion) {
		return Ownership{
			Type:       OwnerSveltos,
			SubType:    strings.ToLower(resource.GetKind()),
			Name:       resource.GetName(),
			Namespace:  resource.GetNamespace(),
			Source:     "apiGroup:" + apiGroupFromVersion(apiVersion),
			Confidence: "high",
		}
	}

	return Ownership{}
}

func detectModelplaneOwnership(labels, annotations map[string]string, resource *unstructured.Unstructured) Ownership {
	_ = annotations

	for _, signal := range []struct {
		key        string
		subType    string
		confidence string
	}{
		{key: "modelplane.ai/deployment", subType: "modeldeployment", confidence: "high"},
		{key: "modelplane.ai/modelcache", subType: "modelcache", confidence: "high"},
		{key: "modelplane.ai/serving", subType: "modelreplica", confidence: "high"},
		{key: "modelplane.ai/workload", subType: "workload", confidence: "medium"},
		{key: "modelplane.ai/cluster", subType: "inferencecluster", confidence: "medium"},
		{key: "modelplane.ai/release", subType: "release", confidence: "medium"},
		{key: "modelplane.ai/resource", subType: "resource", confidence: "medium"},
		{key: "modelplane.ai/usage-consumer", subType: "usage-consumer", confidence: "medium"},
	} {
		if value := labels[signal.key]; value != "" {
			return Ownership{
				Type:       OwnerModelplane,
				SubType:    signal.subType,
				Name:       value,
				Namespace:  resource.GetNamespace(),
				Source:     "label:" + signal.key,
				Confidence: signal.confidence,
			}
		}
	}

	for _, owner := range resource.GetOwnerReferences() {
		if !isModelplaneAPIVersion(owner.APIVersion) {
			continue
		}
		return Ownership{
			Type:       OwnerModelplane,
			SubType:    strings.ToLower(owner.Kind),
			Name:       owner.Name,
			Namespace:  resource.GetNamespace(),
			Source:     "ownerRef:" + owner.APIVersion,
			Confidence: "high",
		}
	}

	if apiVersion := resource.GetAPIVersion(); isModelplaneAPIVersion(apiVersion) {
		return Ownership{
			Type:       OwnerModelplane,
			SubType:    strings.ToLower(resource.GetKind()),
			Name:       resource.GetName(),
			Namespace:  resource.GetNamespace(),
			Source:     "apiGroup:" + apiGroupFromVersion(apiVersion),
			Confidence: "high",
		}
	}

	return Ownership{}
}

func detectCrossplaneSystemOwnership(resource *unstructured.Unstructured) Ownership {
	apiVersion := resource.GetAPIVersion()
	if apiVersion == "" {
		return Ownership{}
	}
	group := strings.SplitN(apiVersion, "/", 2)[0]

	// Crossplane control-plane / package manager API groups
	// These resources are "owned by Crossplane" even if not managed by GitOps.
	// Keeping this narrow avoids misclassifying provider-managed resources like *.aws.crossplane.io.
	if group == "pkg.crossplane.io" || group == "apiextensions.crossplane.io" {
		return Ownership{
			Type:       OwnerCrossplane,
			SubType:    "system",
			Name:       resource.GetName(),
			Namespace:  resource.GetNamespace(),
			Source:     "apiGroup:" + group,
			Confidence: "high",
		}
	}

	// Heuristic: Crossplane system namespace resources that are clearly part of Crossplane's control plane.
	// This is best-effort and intentionally conservative.
	if resource.GetNamespace() == "crossplane-system" && (strings.HasSuffix(group, ".crossplane.io") || strings.HasSuffix(group, ".upbound.io")) {
		kind := strings.ToLower(resource.GetKind())
		systemKinds := map[string]bool{
			"providerrevision":        true,
			"configurationrevision":   true,
			"functionrevision":        true,
			"provider":                true,
			"configuration":           true,
			"function":                true,
			"deploymentruntimeconfig": true,
		}
		if systemKinds[kind] {
			return Ownership{
				Type:       OwnerCrossplane,
				SubType:    "system",
				Name:       resource.GetName(),
				Namespace:  resource.GetNamespace(),
				Source:     "ns:crossplane-system kind:" + kind,
				Confidence: "medium",
			}
		}
	}

	return Ownership{}
}

func detectCrossplaneOwnership(labels, annotations map[string]string, resource *unstructured.Unstructured) Ownership {
	// Crossplane Claim reference (managed resource created from a Claim)
	if claimName, ok := labels["crossplane.io/claim-name"]; ok {
		claimNS := labels["crossplane.io/claim-namespace"]
		return Ownership{
			Type:       OwnerCrossplane,
			SubType:    "claim",
			Name:       claimName,
			Namespace:  claimNS,
			Source:     "label:crossplane.io/claim-name",
			Confidence: "high",
		}
	}

	// Crossplane Composite reference (managed resource created from XR)
	if composite, ok := labels["crossplane.io/composite"]; ok {
		return Ownership{
			Type:       OwnerCrossplane,
			SubType:    "composite",
			Name:       composite,
			Source:     "label:crossplane.io/composite",
			Confidence: "high",
		}
	}

	// Crossplane composition resource name annotation
	if compName, ok := annotations["crossplane.io/composition-resource-name"]; ok {
		return Ownership{
			Type:       OwnerCrossplane,
			SubType:    "managed-resource",
			Name:       compName,
			Source:     "annotation:crossplane.io/composition-resource-name",
			Confidence: "medium",
		}
	}

	// Check owner references for Crossplane XR types
	// Common Crossplane XR API groups: *.crossplane.io, *.upbound.io
	owners := resource.GetOwnerReferences()
	for _, owner := range owners {
		apiVersion := owner.APIVersion
		// Check if owner is a Crossplane resource
		if strings.Contains(apiVersion, "crossplane.io") || strings.Contains(apiVersion, "upbound.io") {
			return Ownership{
				Type:       OwnerCrossplane,
				SubType:    strings.ToLower(owner.Kind),
				Name:       owner.Name,
				Namespace:  resource.GetNamespace(),
				Source:     "ownerRef:" + apiVersion,
				Confidence: "high",
			}
		}
	}

	return Ownership{}
}

func detectKroOwnership(labels, annotations map[string]string, resource *unstructured.Unstructured) Ownership {
	// Prefer explicit kro metadata when present.
	for key, value := range labels {
		if !strings.HasPrefix(key, "kro.run/") {
			continue
		}
		if value == "" {
			continue
		}
		return Ownership{
			Type:       OwnerKro,
			SubType:    "instance",
			Name:       value,
			Namespace:  resource.GetNamespace(),
			Source:     "label:" + key,
			Confidence: "high",
		}
	}
	for key, value := range annotations {
		if !strings.HasPrefix(key, "kro.run/") {
			continue
		}
		if value == "" {
			continue
		}
		return Ownership{
			Type:       OwnerKro,
			SubType:    "instance",
			Name:       value,
			Namespace:  resource.GetNamespace(),
			Source:     "annotation:" + key,
			Confidence: "high",
		}
	}

	// Owner references are the primary generated-resource linkage in kro.
	for _, owner := range resource.GetOwnerReferences() {
		if !isKroAPIVersion(owner.APIVersion) {
			continue
		}
		subType := "instance"
		if strings.EqualFold(owner.Kind, "ResourceGraphDefinition") {
			subType = "definition"
		}
		return Ownership{
			Type:       OwnerKro,
			SubType:    subType,
			Name:       owner.Name,
			Namespace:  resource.GetNamespace(),
			Source:     "ownerRef:" + owner.APIVersion,
			Confidence: "high",
		}
	}

	// kro definitions/instances can also be identified by API group.
	apiVersion := resource.GetAPIVersion()
	if isKroAPIVersion(apiVersion) {
		subType := "instance"
		if strings.EqualFold(resource.GetKind(), "ResourceGraphDefinition") {
			subType = "definition"
		}
		return Ownership{
			Type:       OwnerKro,
			SubType:    subType,
			Name:       resource.GetName(),
			Namespace:  resource.GetNamespace(),
			Source:     "apiGroup:" + strings.SplitN(apiVersion, "/", 2)[0],
			Confidence: "medium",
		}
	}

	return Ownership{}
}

func isKroAPIVersion(apiVersion string) bool {
	if apiVersion == "" {
		return false
	}
	group := strings.SplitN(apiVersion, "/", 2)[0]
	return strings.Contains(group, "kro.run")
}

func isSveltosAPIVersion(apiVersion string) bool {
	group := apiGroupFromVersion(apiVersion)
	return group == "config.projectsveltos.io" ||
		group == "lib.projectsveltos.io" ||
		group == "extension.projectsveltos.io"
}

func isModelplaneAPIVersion(apiVersion string) bool {
	group := apiGroupFromVersion(apiVersion)
	return group == "modelplane.ai" || group == "infrastructure.modelplane.ai"
}

func apiGroupFromVersion(apiVersion string) string {
	parts := strings.SplitN(apiVersion, "/", 2)
	if len(parts) != 2 {
		return ""
	}
	return parts[0]
}

func detectK8sOwnership(resource *unstructured.Unstructured) Ownership {
	owners := resource.GetOwnerReferences()
	if len(owners) == 0 {
		return Ownership{}
	}

	// Prefer an OwnerReference where Controller=true
	// Fall back to the first owner reference if none are marked controller
	var owner = owners[0]
	isController := false
	for _, o := range owners {
		if o.Controller != nil && *o.Controller {
			owner = o
			isController = true
			break
		}
	}

	source := "ownerRef"
	if isController {
		source = "ownerRef:controller"
	}

	return Ownership{
		Type:       OwnerKubernetes,
		SubType:    strings.ToLower(owner.Kind),
		Name:       owner.Name,
		Namespace:  resource.GetNamespace(),
		Source:     source,
		Confidence: "medium",
	}
}
