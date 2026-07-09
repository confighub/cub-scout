// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Ownership detection tests - deterministic tests for v0.5 (#43)
//
// These tests verify ownership detection behavior as documented in:
// docs/reference/ownership-precedence.md
//
// Tests cover:
// - Individual owner types (Flux, ArgoCD, Sveltos, Modelplane, Helm, Terraform, ConfigHub, Crossplane, K8s)
// - Precedence rules when multiple signals are present
// - Unknown/ambiguous cases returning "unknown"
// - Edge cases (empty values, malformed annotations)
//
// See TestDetectOwnership_Priority for precedence verification.

package agent

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Helper to create an unstructured resource with labels and annotations
func newTestResource(namespace, name string, labels, annotations map[string]string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetNamespace(namespace)
	u.SetName(name)
	u.SetLabels(labels)
	u.SetAnnotations(annotations)
	return u
}

// Helper to create a resource with owner references
func newTestResourceWithOwners(namespace, name string, owners []metav1.OwnerReference) *unstructured.Unstructured {
	u := newTestResource(namespace, name, nil, nil)
	u.SetOwnerReferences(owners)
	return u
}

func TestDetectOwnership_Flux(t *testing.T) {
	tests := []struct {
		name        string
		labels      map[string]string
		annotations map[string]string
		apiVersion  string
		kind        string
		wantType    string
		wantSubType string
		wantName    string
		wantNS      string
		wantSource  string
	}{
		{
			name: "Flux Kustomization ownership",
			labels: map[string]string{
				"kustomize.toolkit.fluxcd.io/name":      "my-app",
				"kustomize.toolkit.fluxcd.io/namespace": "flux-system",
			},
			wantType:    OwnerFlux,
			wantSubType: "kustomization",
			wantName:    "my-app",
			wantNS:      "flux-system",
			wantSource:  "label:kustomize.toolkit.fluxcd.io/name",
		},
		{
			name: "Flux HelmRelease ownership",
			labels: map[string]string{
				"helm.toolkit.fluxcd.io/name":      "redis",
				"helm.toolkit.fluxcd.io/namespace": "default",
			},
			wantType:    OwnerFlux,
			wantSubType: "helmrelease",
			wantName:    "redis",
			wantNS:      "default",
			wantSource:  "label:helm.toolkit.fluxcd.io/name",
		},
		{
			name: "Flux Kustomization without namespace",
			labels: map[string]string{
				"kustomize.toolkit.fluxcd.io/name": "standalone-app",
			},
			wantType:    OwnerFlux,
			wantSubType: "kustomization",
			wantName:    "standalone-app",
			wantNS:      "",
			wantSource:  "label:kustomize.toolkit.fluxcd.io/name",
		},
		{
			name:        "Flux Operator aggregate resource by API group",
			apiVersion:  "fluxcd.controlplane.io/v1",
			kind:        "ResourceSet",
			wantType:    OwnerFlux,
			wantSubType: "resourceset",
			wantName:    "test-resource",
			wantNS:      "test-ns",
			wantSource:  "apiGroup:fluxcd.controlplane.io",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := newTestResource("test-ns", "test-resource", tt.labels, tt.annotations)
			resource.SetAPIVersion(tt.apiVersion)
			resource.SetKind(tt.kind)
			ownership := DetectOwnership(resource)

			if ownership.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", ownership.Type, tt.wantType)
			}
			if ownership.SubType != tt.wantSubType {
				t.Errorf("SubType = %q, want %q", ownership.SubType, tt.wantSubType)
			}
			if ownership.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", ownership.Name, tt.wantName)
			}
			if ownership.Namespace != tt.wantNS {
				t.Errorf("Namespace = %q, want %q", ownership.Namespace, tt.wantNS)
			}
			if ownership.Source != tt.wantSource {
				t.Errorf("Source = %q, want %q", ownership.Source, tt.wantSource)
			}
		})
	}
}

func TestDetectOwnership_Argo(t *testing.T) {
	tests := []struct {
		name        string
		labels      map[string]string
		annotations map[string]string
		wantType    string
		wantSubType string
		wantName    string
	}{
		{
			name: "Argo CD Application via labels",
			labels: map[string]string{
				"app.kubernetes.io/instance":  "payment-api",
				"argocd.argoproj.io/instance": "payment-api",
			},
			wantType:    OwnerArgo,
			wantSubType: "application",
			wantName:    "payment-api",
		},
		{
			name: "Argo CD - prefer argocd.argoproj.io/instance value",
			labels: map[string]string{
				"app.kubernetes.io/instance":  "generic-name",
				"argocd.argoproj.io/instance": "argo-specific-name",
			},
			wantType:    OwnerArgo,
			wantSubType: "application",
			wantName:    "argo-specific-name",
		},
		{
			name: "Argo CD - fall back to app.kubernetes.io/instance when argo label empty",
			labels: map[string]string{
				"app.kubernetes.io/instance":  "fallback-name",
				"argocd.argoproj.io/instance": "",
			},
			wantType:    OwnerArgo,
			wantSubType: "application",
			wantName:    "fallback-name",
		},
		{
			name: "Argo CD Application via tracking annotation",
			annotations: map[string]string{
				"argocd.argoproj.io/tracking-id": "guestbook:apps/Deployment:default/guestbook",
			},
			wantType:    OwnerArgo,
			wantSubType: "application",
			wantName:    "guestbook",
		},
		{
			name: "Argo CD tracking annotation with complex name",
			annotations: map[string]string{
				"argocd.argoproj.io/tracking-id": "my-complex-app-name:/apps/v1/Deployment:namespace/resource",
			},
			wantType:    OwnerArgo,
			wantSubType: "application",
			wantName:    "my-complex-app-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := newTestResource("test-ns", "test-resource", tt.labels, tt.annotations)
			ownership := DetectOwnership(resource)

			if ownership.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", ownership.Type, tt.wantType)
			}
			if ownership.SubType != tt.wantSubType {
				t.Errorf("SubType = %q, want %q", ownership.SubType, tt.wantSubType)
			}
			if ownership.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", ownership.Name, tt.wantName)
			}
		})
	}
}

func TestDetectOwnership_Helm(t *testing.T) {
	tests := []struct {
		name        string
		labels      map[string]string
		wantType    string
		wantSubType string
		wantName    string
	}{
		{
			name: "Helm release via managed-by label",
			labels: map[string]string{
				"app.kubernetes.io/managed-by": "Helm",
				"app.kubernetes.io/instance":   "my-redis",
			},
			wantType:    OwnerHelm,
			wantSubType: "release",
			wantName:    "my-redis",
		},
		{
			name: "Helm release via legacy helm.sh/chart label",
			labels: map[string]string{
				"helm.sh/chart":              "redis-17.0.0",
				"app.kubernetes.io/instance": "redis-ha",
			},
			wantType:    OwnerHelm,
			wantSubType: "release",
			wantName:    "redis-ha",
		},
		{
			name: "Helm release via helm.sh/chart without instance",
			labels: map[string]string{
				"helm.sh/chart": "nginx-1.0.0",
			},
			wantType:    OwnerHelm,
			wantSubType: "release",
			wantName:    "nginx-1.0.0", // Falls back to chart name
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := newTestResource("test-ns", "test-resource", tt.labels, nil)
			ownership := DetectOwnership(resource)

			if ownership.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", ownership.Type, tt.wantType)
			}
			if ownership.SubType != tt.wantSubType {
				t.Errorf("SubType = %q, want %q", ownership.SubType, tt.wantSubType)
			}
			if ownership.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", ownership.Name, tt.wantName)
			}
		})
	}
}

func TestDetectOwnership_Terraform(t *testing.T) {
	tests := []struct {
		name        string
		labels      map[string]string
		annotations map[string]string
		wantType    string
		wantSubType string
		wantName    string
	}{
		{
			name: "Terraform via run-id annotation",
			annotations: map[string]string{
				"app.terraform.io/run-id":         "run-abc123",
				"app.terraform.io/workspace-name": "production",
			},
			wantType:    OwnerTerraform,
			wantSubType: "workspace",
			wantName:    "production",
		},
		{
			name: "Terraform via managed label",
			labels: map[string]string{
				"app.terraform.io/managed": "true",
			},
			wantType:    OwnerTerraform,
			wantSubType: "managed",
			wantName:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := newTestResource("test-ns", "test-resource", tt.labels, tt.annotations)
			ownership := DetectOwnership(resource)

			if ownership.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", ownership.Type, tt.wantType)
			}
			if ownership.SubType != tt.wantSubType {
				t.Errorf("SubType = %q, want %q", ownership.SubType, tt.wantSubType)
			}
			if ownership.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", ownership.Name, tt.wantName)
			}
		})
	}
}

func TestDetectOwnership_ConfigHub(t *testing.T) {
	tests := []struct {
		name        string
		labels      map[string]string
		annotations map[string]string
		wantType    string
		wantSubType string
		wantName    string
		wantNS      string
	}{
		{
			name: "ConfigHub via label with annotation space",
			labels: map[string]string{
				"confighub.com/UnitSlug": "payment-api",
			},
			annotations: map[string]string{
				"confighub.com/SpaceName": "payments-team",
			},
			wantType:    OwnerConfigHub,
			wantSubType: "unit",
			wantName:    "payment-api",
			wantNS:      "payments-team",
		},
		{
			name: "ConfigHub via label with label space",
			labels: map[string]string{
				"confighub.com/UnitSlug":  "order-service",
				"confighub.com/SpaceName": "orders-team",
			},
			wantType:    OwnerConfigHub,
			wantSubType: "unit",
			wantName:    "order-service",
			wantNS:      "orders-team",
		},
		{
			name: "ConfigHub via annotation only",
			annotations: map[string]string{
				"confighub.com/UnitSlug":  "legacy-app",
				"confighub.com/SpaceName": "legacy-team",
			},
			wantType:    OwnerConfigHub,
			wantSubType: "unit",
			wantName:    "legacy-app",
			wantNS:      "legacy-team",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := newTestResource("test-ns", "test-resource", tt.labels, tt.annotations)
			ownership := DetectOwnership(resource)

			if ownership.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", ownership.Type, tt.wantType)
			}
			if ownership.SubType != tt.wantSubType {
				t.Errorf("SubType = %q, want %q", ownership.SubType, tt.wantSubType)
			}
			if ownership.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", ownership.Name, tt.wantName)
			}
			if ownership.Namespace != tt.wantNS {
				t.Errorf("Namespace = %q, want %q", ownership.Namespace, tt.wantNS)
			}
		})
	}
}

func TestDetectOwnership_Sveltos(t *testing.T) {
	tests := []struct {
		name           string
		apiVersion     string
		kind           string
		labels         map[string]string
		annotations    map[string]string
		owners         []metav1.OwnerReference
		wantType       string
		wantSubType    string
		wantName       string
		wantSource     string
		wantConfidence string
	}{
		{
			name: "Sveltos deployed resource via owner annotations",
			annotations: map[string]string{
				"projectsveltos.io/owner-kind":          "ClusterProfile",
				"projectsveltos.io/owner-name":          "config-to-production",
				"projectsveltos.io/owner-tier":          "100",
				"projectsveltos.io/reference-kind":      "ConfigMap",
				"projectsveltos.io/reference-name":      "webster-production",
				"projectsveltos.io/reference-namespace": "control-clusters-config",
				"projectsveltos.io/reference-tier":      "100",
			},
			wantType:       OwnerSveltos,
			wantSubType:    "clusterprofile",
			wantName:       "config-to-production",
			wantSource:     "annotation:projectsveltos.io/owner-kind/name",
			wantConfidence: "high",
		},
		{
			name: "Sveltos deployed resource annotation fallback",
			annotations: map[string]string{
				"projectsveltos.io/deployed-by-sveltos": "true",
			},
			wantType:       OwnerSveltos,
			wantSubType:    "deployed-resource",
			wantSource:     "annotation:projectsveltos.io/deployed-by-sveltos",
			wantConfidence: "medium",
		},
		{
			name:        "Sveltos ClusterProfile via API group",
			apiVersion:  "config.projectsveltos.io/v1beta1",
			kind:        "ClusterProfile",
			wantType:    OwnerSveltos,
			wantSubType: "clusterprofile",
			wantName:    "test-resource",
			wantSource:  "apiGroup:config.projectsveltos.io",
		},
		{
			name:        "Sveltos EventSource via API group",
			apiVersion:  "lib.projectsveltos.io/v1beta1",
			kind:        "EventSource",
			wantType:    OwnerSveltos,
			wantSubType: "eventsource",
			wantName:    "test-resource",
			wantSource:  "apiGroup:lib.projectsveltos.io",
		},
		{
			name: "Sveltos owner reference",
			owners: []metav1.OwnerReference{
				{
					APIVersion: "config.projectsveltos.io/v1beta1",
					Kind:       "ClusterSummary",
					Name:       "production-summary",
				},
			},
			wantType:       OwnerSveltos,
			wantSubType:    "clustersummary",
			wantName:       "production-summary",
			wantSource:     "ownerRef:config.projectsveltos.io/v1beta1",
			wantConfidence: "high",
		},
		{
			name: "Sveltos annotations take precedence over Helm chart labels",
			labels: map[string]string{
				"app.kubernetes.io/managed-by": "Helm",
				"app.kubernetes.io/instance":   "cert-manager",
			},
			annotations: map[string]string{
				"projectsveltos.io/owner-kind": "Profile",
				"projectsveltos.io/owner-name": "platform-addons",
			},
			wantType:    OwnerSveltos,
			wantSubType: "profile",
			wantName:    "platform-addons",
			wantSource:  "annotation:projectsveltos.io/owner-kind/name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resource *unstructured.Unstructured
			if len(tt.owners) > 0 {
				resource = newTestResourceWithOwners("test-ns", "test-resource", tt.owners)
				resource.SetLabels(tt.labels)
				resource.SetAnnotations(tt.annotations)
			} else {
				resource = newTestResource("test-ns", "test-resource", tt.labels, tt.annotations)
			}
			if tt.apiVersion != "" {
				resource.SetAPIVersion(tt.apiVersion)
			}
			if tt.kind != "" {
				resource.SetKind(tt.kind)
			}

			ownership := DetectOwnership(resource)

			if ownership.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", ownership.Type, tt.wantType)
			}
			if ownership.SubType != tt.wantSubType {
				t.Errorf("SubType = %q, want %q", ownership.SubType, tt.wantSubType)
			}
			if ownership.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", ownership.Name, tt.wantName)
			}
			if tt.wantSource != "" && ownership.Source != tt.wantSource {
				t.Errorf("Source = %q, want %q", ownership.Source, tt.wantSource)
			}
			if tt.wantConfidence != "" && ownership.Confidence != tt.wantConfidence {
				t.Errorf("Confidence = %q, want %q", ownership.Confidence, tt.wantConfidence)
			}
		})
	}
}

func TestDetectOwnership_Modelplane(t *testing.T) {
	tests := []struct {
		name        string
		apiVersion  string
		kind        string
		labels      map[string]string
		annotations map[string]string
		owners      []metav1.OwnerReference
		wantType    string
		wantSubType string
		wantName    string
		wantSource  string
	}{
		{
			name:        "Modelplane authored resource via API group",
			apiVersion:  "modelplane.ai/v1alpha1",
			kind:        "ModelDeployment",
			wantType:    OwnerModelplane,
			wantSubType: "modeldeployment",
			wantName:    "test-resource",
			wantSource:  "apiGroup:modelplane.ai",
		},
		{
			name:        "Modelplane infrastructure resource via API group",
			apiVersion:  "infrastructure.modelplane.ai/v1alpha1",
			kind:        "EKSCluster",
			wantType:    OwnerModelplane,
			wantSubType: "ekscluster",
			wantName:    "test-resource",
			wantSource:  "apiGroup:infrastructure.modelplane.ai",
		},
		{
			name: "Modelplane replica label takes precedence over Crossplane composition label",
			labels: map[string]string{
				"modelplane.ai/deployment":                "qwen-demo",
				"crossplane.io/composition-resource-name": "replica-0",
				"app.kubernetes.io/managed-by":            "Helm",
				"apiextensions.crossplane.io/composite":   "ignored",
				"crossplane.io/composite":                 "x-qwen-demo",
				"crossplane.io/claim-name":                "ignored-claim",
				"crossplane.io/claim-namespace":           "default",
			},
			annotations: map[string]string{
				"crossplane.io/composition-resource-name": "replica-0",
			},
			wantType:    OwnerModelplane,
			wantSubType: "modeldeployment",
			wantName:    "qwen-demo",
			wantSource:  "label:modelplane.ai/deployment",
		},
		{
			name: "Modelplane cache label",
			labels: map[string]string{
				"modelplane.ai/modelcache": "qwen-cache",
			},
			wantType:    OwnerModelplane,
			wantSubType: "modelcache",
			wantName:    "qwen-cache",
			wantSource:  "label:modelplane.ai/modelcache",
		},
		{
			name: "Modelplane release label takes precedence over Helm label",
			labels: map[string]string{
				"modelplane.ai/release":        "traefik",
				"app.kubernetes.io/managed-by": "Helm",
				"app.kubernetes.io/instance":   "traefik",
				"app.kubernetes.io/name":       "traefik",
				"app.kubernetes.io/part-of":    "modelplane",
			},
			wantType:    OwnerModelplane,
			wantSubType: "release",
			wantName:    "traefik",
			wantSource:  "label:modelplane.ai/release",
		},
		{
			name: "Modelplane owner reference",
			owners: []metav1.OwnerReference{
				{
					APIVersion: "modelplane.ai/v1alpha1",
					Kind:       "ModelReplica",
					Name:       "qwen-demo-0",
				},
			},
			wantType:    OwnerModelplane,
			wantSubType: "modelreplica",
			wantName:    "qwen-demo-0",
			wantSource:  "ownerRef:modelplane.ai/v1alpha1",
		},
		{
			name: "Broad Modelplane placement labels are not ownership by themselves",
			labels: map[string]string{
				"modelplane.ai/region": "us-east",
			},
			wantType: OwnerUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resource *unstructured.Unstructured
			if len(tt.owners) > 0 {
				resource = newTestResourceWithOwners("test-ns", "test-resource", tt.owners)
				resource.SetLabels(tt.labels)
				resource.SetAnnotations(tt.annotations)
			} else {
				resource = newTestResource("test-ns", "test-resource", tt.labels, tt.annotations)
			}
			if tt.apiVersion != "" {
				resource.SetAPIVersion(tt.apiVersion)
			}
			if tt.kind != "" {
				resource.SetKind(tt.kind)
			}

			ownership := DetectOwnership(resource)

			if ownership.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", ownership.Type, tt.wantType)
			}
			if tt.wantSubType != "" && ownership.SubType != tt.wantSubType {
				t.Errorf("SubType = %q, want %q", ownership.SubType, tt.wantSubType)
			}
			if tt.wantName != "" && ownership.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", ownership.Name, tt.wantName)
			}
			if tt.wantSource != "" && ownership.Source != tt.wantSource {
				t.Errorf("Source = %q, want %q", ownership.Source, tt.wantSource)
			}
		})
	}
}

func TestDetectOwnership_Crossplane(t *testing.T) {
	tests := []struct {
		name        string
		labels      map[string]string
		annotations map[string]string
		owners      []metav1.OwnerReference
		wantType    string
		wantSubType string
		wantName    string
		wantNS      string
	}{
		{
			name: "Crossplane Claim reference via labels",
			labels: map[string]string{
				"crossplane.io/claim-name":      "my-database",
				"crossplane.io/claim-namespace": "prod",
			},
			wantType:    OwnerCrossplane,
			wantSubType: "claim",
			wantName:    "my-database",
			wantNS:      "prod",
		},
		{
			name: "Crossplane Composite reference via label",
			labels: map[string]string{
				"crossplane.io/composite": "my-xr-abc123",
			},
			wantType:    OwnerCrossplane,
			wantSubType: "composite",
			wantName:    "my-xr-abc123",
			wantNS:      "",
		},
		{
			name: "Crossplane composition resource name via annotation",
			annotations: map[string]string{
				"crossplane.io/composition-resource-name": "rds-instance",
			},
			wantType:    OwnerCrossplane,
			wantSubType: "managed-resource",
			wantName:    "rds-instance",
			wantNS:      "",
		},
		{
			name: "Crossplane owner reference (crossplane.io API group)",
			owners: []metav1.OwnerReference{
				{
					APIVersion: "database.aws.crossplane.io/v1beta1",
					Kind:       "RDSInstance",
					Name:       "prod-db",
				},
			},
			wantType:    OwnerCrossplane,
			wantSubType: "rdsinstance",
			wantName:    "prod-db",
		},
		{
			name: "Crossplane owner reference (upbound.io API group)",
			owners: []metav1.OwnerReference{
				{
					APIVersion: "rds.aws.upbound.io/v1beta1",
					Kind:       "Instance",
					Name:       "staging-db",
				},
			},
			wantType:    OwnerCrossplane,
			wantSubType: "instance",
			wantName:    "staging-db",
		},
		{
			name: "Crossplane claim takes precedence over composite label",
			labels: map[string]string{
				"crossplane.io/claim-name":      "primary-claim",
				"crossplane.io/claim-namespace": "default",
				"crossplane.io/composite":       "some-xr",
			},
			wantType:    OwnerCrossplane,
			wantSubType: "claim",
			wantName:    "primary-claim",
			wantNS:      "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resource *unstructured.Unstructured
			if len(tt.owners) > 0 {
				resource = newTestResourceWithOwners("test-ns", "test-resource", tt.owners)
				resource.SetLabels(tt.labels)
				resource.SetAnnotations(tt.annotations)
			} else {
				resource = newTestResource("test-ns", "test-resource", tt.labels, tt.annotations)
			}
			ownership := DetectOwnership(resource)

			if ownership.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", ownership.Type, tt.wantType)
			}
			if ownership.SubType != tt.wantSubType {
				t.Errorf("SubType = %q, want %q", ownership.SubType, tt.wantSubType)
			}
			if ownership.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", ownership.Name, tt.wantName)
			}
			if tt.wantNS != "" && ownership.Namespace != tt.wantNS {
				t.Errorf("Namespace = %q, want %q", ownership.Namespace, tt.wantNS)
			}
		})
	}
}

func TestDetectOwnership_Kro(t *testing.T) {
	tests := []struct {
		name        string
		apiVersion  string
		kind        string
		labels      map[string]string
		annotations map[string]string
		owners      []metav1.OwnerReference
		wantType    string
		wantSubType string
		wantName    string
	}{
		{
			name:        "kro instance via API group",
			apiVersion:  "apps.kro.run/v1alpha1",
			kind:        "WebApp",
			wantType:    OwnerKro,
			wantSubType: "instance",
			wantName:    "test-resource",
		},
		{
			name:        "kro definition via API group",
			apiVersion:  "kro.run/v1alpha1",
			kind:        "ResourceGraphDefinition",
			wantType:    OwnerKro,
			wantSubType: "definition",
			wantName:    "test-resource",
		},
		{
			name: "kro metadata via label",
			labels: map[string]string{
				"kro.run/resource-graph-definition": "webapp-stack",
			},
			wantType:    OwnerKro,
			wantSubType: "instance",
			wantName:    "webapp-stack",
		},
		{
			name: "kro metadata via annotation",
			annotations: map[string]string{
				"kro.run/rgd": "payment-stack",
			},
			wantType:    OwnerKro,
			wantSubType: "instance",
			wantName:    "payment-stack",
		},
		{
			name: "kro owner reference to instance",
			owners: []metav1.OwnerReference{
				{
					APIVersion: "apps.kro.run/v1alpha1",
					Kind:       "WebApp",
					Name:       "checkout-prod",
				},
			},
			wantType:    OwnerKro,
			wantSubType: "instance",
			wantName:    "checkout-prod",
		},
		{
			name: "kro owner reference to definition",
			owners: []metav1.OwnerReference{
				{
					APIVersion: "kro.run/v1alpha1",
					Kind:       "ResourceGraphDefinition",
					Name:       "webapp-stack",
				},
			},
			wantType:    OwnerKro,
			wantSubType: "definition",
			wantName:    "webapp-stack",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resource *unstructured.Unstructured
			if len(tt.owners) > 0 {
				resource = newTestResourceWithOwners("test-ns", "test-resource", tt.owners)
				resource.SetLabels(tt.labels)
				resource.SetAnnotations(tt.annotations)
			} else {
				resource = newTestResource("test-ns", "test-resource", tt.labels, tt.annotations)
			}
			if tt.apiVersion != "" {
				resource.SetAPIVersion(tt.apiVersion)
			}
			if tt.kind != "" {
				resource.SetKind(tt.kind)
			}

			ownership := DetectOwnership(resource)

			if ownership.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", ownership.Type, tt.wantType)
			}
			if ownership.SubType != tt.wantSubType {
				t.Errorf("SubType = %q, want %q", ownership.SubType, tt.wantSubType)
			}
			if ownership.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", ownership.Name, tt.wantName)
			}
		})
	}
}

func TestDetectOwnership_CrossplaneSystem(t *testing.T) {
	tests := []struct {
		name       string
		apiVersion string
		kind       string
		namespace  string
		resName    string
		wantSource string
	}{
		{
			name:       "pkg.crossplane.io group is classified as Crossplane system",
			apiVersion: "pkg.crossplane.io/v1",
			kind:       "ProviderRevision",
			namespace:  "crossplane-system",
			resName:    "provider-aws-1234abcd",
			wantSource: "apiGroup:pkg.crossplane.io",
		},
		{
			name:       "apiextensions.crossplane.io group is classified as Crossplane system",
			apiVersion: "apiextensions.crossplane.io/v1",
			kind:       "CompositeResourceDefinition",
			namespace:  "",
			resName:    "xpostgresqlinstances.database.example.org",
			wantSource: "apiGroup:apiextensions.crossplane.io",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": tt.apiVersion,
					"kind":       tt.kind,
					"metadata": map[string]interface{}{
						"name":      tt.resName,
						"namespace": tt.namespace,
					},
				},
			}

			own := DetectOwnership(u)
			if own.Type != OwnerCrossplane {
				t.Errorf("Type = %q, want %q", own.Type, OwnerCrossplane)
			}
			if own.SubType != "system" {
				t.Errorf("SubType = %q, want %q", own.SubType, "system")
			}
			if own.Name != tt.resName {
				t.Errorf("Name = %q, want %q", own.Name, tt.resName)
			}
			if tt.namespace != "" && own.Namespace != tt.namespace {
				t.Errorf("Namespace = %q, want %q", own.Namespace, tt.namespace)
			}
			if own.Source != tt.wantSource {
				t.Errorf("Source = %q, want %q", own.Source, tt.wantSource)
			}
		})
	}
}

func TestDetectOwnership_K8s(t *testing.T) {
	trueVal := true
	falseVal := false

	tests := []struct {
		name        string
		owners      []metav1.OwnerReference
		wantType    string
		wantSubType string
		wantName    string
	}{
		{
			name: "ReplicaSet owned by Deployment",
			owners: []metav1.OwnerReference{
				{
					Kind: "Deployment",
					Name: "my-deployment",
					UID:  "abc-123",
				},
			},
			wantType:    OwnerKubernetes,
			wantSubType: "deployment",
			wantName:    "my-deployment",
		},
		{
			name: "Pod owned by ReplicaSet",
			owners: []metav1.OwnerReference{
				{
					Kind: "ReplicaSet",
					Name: "my-deployment-abc123",
					UID:  "def-456",
				},
			},
			wantType:    OwnerKubernetes,
			wantSubType: "replicaset",
			wantName:    "my-deployment-abc123",
		},
		{
			name: "Pod owned by DaemonSet",
			owners: []metav1.OwnerReference{
				{
					Kind: "DaemonSet",
					Name: "kube-proxy",
					UID:  "ghi-789",
				},
			},
			wantType:    OwnerKubernetes,
			wantSubType: "daemonset",
			wantName:    "kube-proxy",
		},
		{
			name: "Multiple owners - prefer controller=true",
			owners: []metav1.OwnerReference{
				{
					Kind:       "Service",
					Name:       "not-controller",
					UID:        "svc-123",
					Controller: &falseVal,
				},
				{
					Kind:       "ReplicaSet",
					Name:       "the-controller",
					UID:        "rs-456",
					Controller: &trueVal,
				},
				{
					Kind: "ConfigMap",
					Name: "also-not-controller",
					UID:  "cm-789",
				},
			},
			wantType:    OwnerKubernetes,
			wantSubType: "replicaset",
			wantName:    "the-controller",
		},
		{
			name: "Multiple owners - none marked controller, use first",
			owners: []metav1.OwnerReference{
				{
					Kind: "Service",
					Name: "first-owner",
					UID:  "svc-123",
				},
				{
					Kind: "ConfigMap",
					Name: "second-owner",
					UID:  "cm-456",
				},
			},
			wantType:    OwnerKubernetes,
			wantSubType: "service",
			wantName:    "first-owner",
		},
		{
			name: "Multiple owners - controller=false on all, use first",
			owners: []metav1.OwnerReference{
				{
					Kind:       "Service",
					Name:       "first-not-controller",
					UID:        "svc-123",
					Controller: &falseVal,
				},
				{
					Kind:       "ConfigMap",
					Name:       "second-not-controller",
					UID:        "cm-456",
					Controller: &falseVal,
				},
			},
			wantType:    OwnerKubernetes,
			wantSubType: "service",
			wantName:    "first-not-controller",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := newTestResourceWithOwners("test-ns", "test-resource", tt.owners)
			ownership := DetectOwnership(resource)

			if ownership.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", ownership.Type, tt.wantType)
			}
			if ownership.SubType != tt.wantSubType {
				t.Errorf("SubType = %q, want %q", ownership.SubType, tt.wantSubType)
			}
			if ownership.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", ownership.Name, tt.wantName)
			}
		})
	}
}

func TestDetectOwnership_Unknown(t *testing.T) {
	tests := []struct {
		name        string
		labels      map[string]string
		annotations map[string]string
	}{
		{
			name:   "No labels or annotations",
			labels: nil,
		},
		{
			name: "Unrelated labels",
			labels: map[string]string{
				"app":         "my-app",
				"environment": "production",
			},
		},
		{
			name: "Partial Argo label (missing argocd.argoproj.io/instance)",
			labels: map[string]string{
				"app.kubernetes.io/instance": "my-app",
			},
		},
		{
			name: "Non-Helm managed-by",
			labels: map[string]string{
				"app.kubernetes.io/managed-by": "kustomize",
			},
		},
		{
			name: "Empty Argo tracking-id annotation",
			annotations: map[string]string{
				"argocd.argoproj.io/tracking-id": "",
			},
		},
		{
			name: "Malformed Argo tracking-id (starts with colon)",
			annotations: map[string]string{
				"argocd.argoproj.io/tracking-id": ":apps/Deployment:default/name",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := newTestResource("test-ns", "test-resource", tt.labels, tt.annotations)
			ownership := DetectOwnership(resource)

			if ownership.Type != OwnerUnknown {
				t.Errorf("Type = %q, want %q", ownership.Type, OwnerUnknown)
			}
		})
	}
}

func TestDetectOwnership_Priority(t *testing.T) {
	// Test that ownership detection priority is correct:
	// Flux > Argo > Helm > Terraform > ConfigHub > K8s

	t.Run("Flux takes precedence over Helm", func(t *testing.T) {
		resource := newTestResource("test-ns", "test", map[string]string{
			"kustomize.toolkit.fluxcd.io/name": "my-app",
			"app.kubernetes.io/managed-by":     "Helm",
		}, nil)
		ownership := DetectOwnership(resource)

		if ownership.Type != OwnerFlux {
			t.Errorf("Type = %q, want %q (Flux should take precedence)", ownership.Type, OwnerFlux)
		}
	})

	t.Run("Argo takes precedence over Helm", func(t *testing.T) {
		resource := newTestResource("test-ns", "test", map[string]string{
			"app.kubernetes.io/instance":   "my-app",
			"argocd.argoproj.io/instance":  "my-app",
			"app.kubernetes.io/managed-by": "Helm",
		}, nil)
		ownership := DetectOwnership(resource)

		if ownership.Type != OwnerArgo {
			t.Errorf("Type = %q, want %q (Argo should take precedence)", ownership.Type, OwnerArgo)
		}
	})

	t.Run("Flux takes precedence over Crossplane", func(t *testing.T) {
		resource := newTestResource("test-ns", "test", map[string]string{
			"kustomize.toolkit.fluxcd.io/name": "my-app",
			"crossplane.io/claim-name":         "my-claim",
		}, nil)
		ownership := DetectOwnership(resource)

		if ownership.Type != OwnerFlux {
			t.Errorf("Type = %q, want %q (Flux should take precedence)", ownership.Type, OwnerFlux)
		}
	})

	t.Run("Crossplane takes precedence over K8s ownerRef", func(t *testing.T) {
		resource := newTestResource("test-ns", "test", map[string]string{
			"crossplane.io/claim-name": "my-claim",
		}, nil)
		resource.SetOwnerReferences([]metav1.OwnerReference{
			{Kind: "ReplicaSet", Name: "some-rs"},
		})
		ownership := DetectOwnership(resource)

		if ownership.Type != OwnerCrossplane {
			t.Errorf("Type = %q, want %q (Crossplane should take precedence)", ownership.Type, OwnerCrossplane)
		}
	})

	t.Run("Flux takes precedence over kro", func(t *testing.T) {
		resource := newTestResource("test-ns", "test", map[string]string{
			"kustomize.toolkit.fluxcd.io/name": "my-app",
			"kro.run/rgd":                      "webapp-stack",
		}, nil)
		ownership := DetectOwnership(resource)

		if ownership.Type != OwnerFlux {
			t.Errorf("Type = %q, want %q (Flux should take precedence)", ownership.Type, OwnerFlux)
		}
	})

	t.Run("kro takes precedence over K8s ownerRef", func(t *testing.T) {
		resource := newTestResource("test-ns", "test", map[string]string{
			"kro.run/resource-graph-definition": "webapp-stack",
		}, nil)
		resource.SetOwnerReferences([]metav1.OwnerReference{
			{Kind: "ReplicaSet", Name: "some-rs"},
		})
		ownership := DetectOwnership(resource)

		if ownership.Type != OwnerKro {
			t.Errorf("Type = %q, want %q (kro should take precedence)", ownership.Type, OwnerKro)
		}
	})
}

// Benchmark tests for ownership detection
// TestDetectOwnership_BreakGlass verifies the ownership detection for
// the break-glass scenario: a Flux-managed resource alongside a Native
// orphan created via kubectl during an incident. This matches the fixture
// in examples/demos/break-glass.yaml.
func TestDetectOwnership_BreakGlass(t *testing.T) {
	t.Run("Flux-managed payment-api", func(t *testing.T) {
		resource := newTestResource("break-glass-demo", "payment-api", map[string]string{
			"app":                                   "payment-api",
			"kustomize.toolkit.fluxcd.io/name":      "payment-api",
			"kustomize.toolkit.fluxcd.io/namespace": "break-glass-demo",
		}, map[string]string{
			"confighub.com/revision":    "42",
			"confighub.com/deployed-by": "flux",
			"confighub.com/deployed-at": "2026-01-14T12:15:00Z",
		})
		ownership := DetectOwnership(resource)

		if ownership.Type != OwnerFlux {
			t.Errorf("payment-api: Type = %q, want %q", ownership.Type, OwnerFlux)
		}
		if ownership.SubType != "kustomization" {
			t.Errorf("payment-api: SubType = %q, want %q", ownership.SubType, "kustomization")
		}
		if ownership.Name != "payment-api" {
			t.Errorf("payment-api: Name = %q, want %q", ownership.Name, "payment-api")
		}
		if ownership.Confidence != "high" {
			t.Errorf("payment-api: Confidence = %q, want %q", ownership.Confidence, "high")
		}
	})

	t.Run("Native hotfix-cache (break-glass orphan)", func(t *testing.T) {
		// hotfix-cache has NO GitOps labels — only break-glass annotations
		resource := newTestResource("break-glass-demo", "hotfix-cache", map[string]string{
			"app": "hotfix-cache",
			// NO flux/argo/helm/terraform/confighub labels
		}, map[string]string{
			"break-glass/incident":   "INC-4521",
			"break-glass/applied-by": "admin",
			"break-glass/applied-at": "2026-01-14T14:23:00Z",
			"break-glass/reason":     "Emergency cache fix for payment processing failures",
		})
		ownership := DetectOwnership(resource)

		if ownership.Type != OwnerUnknown {
			t.Errorf("hotfix-cache: Type = %q, want %q (should be orphan/Native)", ownership.Type, OwnerUnknown)
		}
	})

	t.Run("Native hotfix-cache-config (break-glass ConfigMap)", func(t *testing.T) {
		resource := newTestResource("break-glass-demo", "hotfix-cache-config", map[string]string{
			"app": "hotfix-cache",
		}, map[string]string{
			"break-glass/incident":   "INC-4521",
			"break-glass/applied-by": "admin",
		})
		ownership := DetectOwnership(resource)

		if ownership.Type != OwnerUnknown {
			t.Errorf("hotfix-cache-config: Type = %q, want %q (should be orphan/Native)", ownership.Type, OwnerUnknown)
		}
	})

	t.Run("Break-glass annotations do not confer ownership", func(t *testing.T) {
		// Even with many annotations, if no GitOps label is present,
		// the resource must be classified as unknown (Native).
		resource := newTestResource("prod", "emergency-fix", map[string]string{
			"app":         "emergency-fix",
			"team":        "platform",
			"environment": "production",
		}, map[string]string{
			"break-glass/incident":                             "INC-9999",
			"break-glass/applied-by":                           "oncall",
			"kubectl.kubernetes.io/last-applied-configuration": "{}",
		})
		ownership := DetectOwnership(resource)

		if ownership.Type != OwnerUnknown {
			t.Errorf("emergency-fix: Type = %q, want %q", ownership.Type, OwnerUnknown)
		}
	})
}

func BenchmarkDetectOwnership_Flux(b *testing.B) {
	resource := newTestResource("test-ns", "test", map[string]string{
		"kustomize.toolkit.fluxcd.io/name":      "my-app",
		"kustomize.toolkit.fluxcd.io/namespace": "flux-system",
	}, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DetectOwnership(resource)
	}
}

func BenchmarkDetectOwnership_Unknown(b *testing.B) {
	// Worst case: check all ownership types before returning unknown
	resource := newTestResource("test-ns", "test", map[string]string{
		"app": "my-app",
	}, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DetectOwnership(resource)
	}
}
