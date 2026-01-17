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
	OwnerKubernetes = "k8s"
	OwnerUnknown    = "unknown"
)

// DetectOwnership examines a resource and determines who manages it
func DetectOwnership(resource *unstructured.Unstructured) Ownership {
	labels := resource.GetLabels()
	annotations := resource.GetAnnotations()

	// Check for Flux ownership
	if ownership := detectFluxOwnership(labels, annotations); ownership.Type != "" {
		return ownership
	}

	// Check for Argo CD ownership
	if ownership := detectArgoOwnership(labels, annotations); ownership.Type != "" {
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

	// Check for Kubernetes native ownership (via OwnerReferences)
	if ownership := detectK8sOwnership(resource); ownership.Type != "" {
		return ownership
	}

	return Ownership{Type: OwnerUnknown}
}

func detectFluxOwnership(labels, annotations map[string]string) Ownership {
	// Flux Kustomization
	if name, ok := labels["kustomize.toolkit.fluxcd.io/name"]; ok {
		ns := labels["kustomize.toolkit.fluxcd.io/namespace"]
		return Ownership{
			Type:      OwnerFlux,
			SubType:   "kustomization",
			Name:      name,
			Namespace: ns,
		}
	}

	// Flux HelmRelease
	if name, ok := labels["helm.toolkit.fluxcd.io/name"]; ok {
		ns := labels["helm.toolkit.fluxcd.io/namespace"]
		return Ownership{
			Type:      OwnerFlux,
			SubType:   "helmrelease",
			Name:      name,
			Namespace: ns,
		}
	}

	return Ownership{}
}

func detectArgoOwnership(labels, annotations map[string]string) Ownership {
	// Argo CD Application
	if instance, ok := labels["app.kubernetes.io/instance"]; ok {
		if _, isArgo := labels["argocd.argoproj.io/instance"]; isArgo {
			return Ownership{
				Type:    OwnerArgo,
				SubType: "application",
				Name:    instance,
			}
		}
	}

	// Alternative: check annotation
	if tracking, ok := annotations["argocd.argoproj.io/tracking-id"]; ok {
		// Format: <app-name>:<group>/<kind>:<namespace>/<name>
		parts := strings.SplitN(tracking, ":", 2)
		if len(parts) > 0 {
			return Ownership{
				Type:    OwnerArgo,
				SubType: "application",
				Name:    parts[0],
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
			Type:    OwnerHelm,
			SubType: "release",
			Name:    name,
		}
	}

	// Legacy helm labels
	if release, ok := labels["helm.sh/chart"]; ok {
		name := labels["app.kubernetes.io/instance"]
		if name == "" {
			name = release
		}
		return Ownership{
			Type:    OwnerHelm,
			SubType: "release",
			Name:    name,
		}
	}

	return Ownership{}
}

func detectTerraformOwnership(labels, annotations map[string]string) Ownership {
	// Terraform Kubernetes provider
	if _, ok := annotations["app.terraform.io/run-id"]; ok {
		workspace := annotations["app.terraform.io/workspace-name"]
		return Ownership{
			Type:    OwnerTerraform,
			SubType: "workspace",
			Name:    workspace,
		}
	}

	// Alternative terraform markers
	if _, ok := labels["app.terraform.io/managed"]; ok {
		return Ownership{
			Type:    OwnerTerraform,
			SubType: "managed",
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
			Type:      OwnerConfigHub,
			SubType:   "unit",
			Name:      unit,
			Namespace: space,
		}
	}

	// Also check annotation (some resources may only have annotation)
	if unit, ok := annotations["confighub.com/UnitSlug"]; ok {
		space := annotations["confighub.com/SpaceName"]
		return Ownership{
			Type:      OwnerConfigHub,
			SubType:   "unit",
			Name:      unit,
			Namespace: space,
		}
	}

	return Ownership{}
}

func detectK8sOwnership(resource *unstructured.Unstructured) Ownership {
	owners := resource.GetOwnerReferences()
	if len(owners) == 0 {
		return Ownership{}
	}

	// Use the first owner reference
	owner := owners[0]
	return Ownership{
		Type:      OwnerKubernetes,
		SubType:   strings.ToLower(owner.Kind),
		Name:      owner.Name,
		Namespace: resource.GetNamespace(),
	}
}
