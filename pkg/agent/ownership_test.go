// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

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
		wantType    string
		wantSubType string
		wantName    string
		wantNS      string
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
}

// Benchmark tests for ownership detection
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
