// Package unit provides unit tests for ConfigHub Agent.
package unit

import (
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TestOwnershipDetection tests all 6 ownership types with table-driven tests.
func TestOwnershipDetection(t *testing.T) {
	tests := []struct {
		name        string
		labels      map[string]string
		annotations map[string]string
		owners      []metav1.OwnerReference
		wantType    string
		wantSubType string
		wantName    string
	}{
		// Flux Kustomization
		{
			name: "Flux Kustomization ownership",
			labels: map[string]string{
				"kustomize.toolkit.fluxcd.io/name":      "my-app",
				"kustomize.toolkit.fluxcd.io/namespace": "flux-system",
			},
			wantType:    agent.OwnerFlux,
			wantSubType: "kustomization",
			wantName:    "my-app",
		},
		{
			name: "Flux Kustomization without namespace",
			labels: map[string]string{
				"kustomize.toolkit.fluxcd.io/name": "podinfo",
			},
			wantType:    agent.OwnerFlux,
			wantSubType: "kustomization",
			wantName:    "podinfo",
		},

		// Flux HelmRelease
		{
			name: "Flux HelmRelease ownership",
			labels: map[string]string{
				"helm.toolkit.fluxcd.io/name":      "redis",
				"helm.toolkit.fluxcd.io/namespace": "flux-system",
			},
			wantType:    agent.OwnerFlux,
			wantSubType: "helmrelease",
			wantName:    "redis",
		},

		// Argo CD
		{
			name: "Argo CD Application via labels",
			labels: map[string]string{
				"app.kubernetes.io/instance":  "guestbook",
				"argocd.argoproj.io/instance": "guestbook",
			},
			wantType:    agent.OwnerArgo,
			wantSubType: "application",
			wantName:    "guestbook",
		},
		{
			name:   "Argo CD Application via tracking annotation",
			labels: map[string]string{},
			annotations: map[string]string{
				"argocd.argoproj.io/tracking-id": "myapp:apps/Deployment:default/nginx",
			},
			wantType:    agent.OwnerArgo,
			wantSubType: "application",
			wantName:    "myapp",
		},

		// Helm
		{
			name: "Helm release via managed-by",
			labels: map[string]string{
				"app.kubernetes.io/managed-by": "Helm",
				"app.kubernetes.io/instance":   "prometheus",
			},
			wantType:    agent.OwnerHelm,
			wantSubType: "release",
			wantName:    "prometheus",
		},
		{
			name: "Helm release via helm.sh/chart",
			labels: map[string]string{
				"helm.sh/chart":              "nginx-1.2.3",
				"app.kubernetes.io/instance": "nginx",
			},
			wantType:    agent.OwnerHelm,
			wantSubType: "release",
			wantName:    "nginx",
		},

		// Terraform
		{
			name:   "Terraform workspace via annotation",
			labels: map[string]string{},
			annotations: map[string]string{
				"app.terraform.io/run-id":         "run-12345",
				"app.terraform.io/workspace-name": "prod-infra",
			},
			wantType:    agent.OwnerTerraform,
			wantSubType: "workspace",
			wantName:    "prod-infra",
		},
		{
			name: "Terraform managed via label",
			labels: map[string]string{
				"app.terraform.io/managed": "true",
			},
			wantType:    agent.OwnerTerraform,
			wantSubType: "managed",
			wantName:    "",
		},

		// ConfigHub
		{
			name: "ConfigHub unit via label",
			labels: map[string]string{
				"confighub.com/UnitSlug": "backend",
			},
			annotations: map[string]string{
				"confighub.com/SpaceName":   "prod",
				"confighub.com/RevisionNum": "42",
			},
			wantType:    agent.OwnerConfigHub,
			wantSubType: "unit",
			wantName:    "backend",
		},
		{
			name:   "ConfigHub unit via annotation only",
			labels: map[string]string{},
			annotations: map[string]string{
				"confighub.com/UnitSlug":  "api-gateway",
				"confighub.com/SpaceName": "staging",
			},
			wantType:    agent.OwnerConfigHub,
			wantSubType: "unit",
			wantName:    "api-gateway",
		},

		// Kubernetes native (OwnerReferences)
		{
			name:   "Kubernetes native via OwnerReference",
			labels: map[string]string{},
			owners: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "ReplicaSet",
					Name:       "nginx-abc123",
				},
			},
			wantType:    agent.OwnerKubernetes,
			wantSubType: "replicaset",
			wantName:    "nginx-abc123",
		},

		// Unknown
		{
			name:     "Unknown ownership - no markers",
			labels:   map[string]string{},
			wantType: agent.OwnerUnknown,
		},
		{
			name: "Unknown ownership - irrelevant labels",
			labels: map[string]string{
				"app": "myapp",
				"env": "prod",
			},
			wantType: agent.OwnerUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build unstructured resource
			resource := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "test-resource",
						"namespace": "default",
					},
				},
			}

			// Set labels
			if tt.labels != nil {
				resource.SetLabels(tt.labels)
			}

			// Set annotations
			if tt.annotations != nil {
				resource.SetAnnotations(tt.annotations)
			}

			// Set owner references
			if tt.owners != nil {
				resource.SetOwnerReferences(tt.owners)
			}

			// Detect ownership
			ownership := agent.DetectOwnership(resource)

			// Assert
			assert.Equal(t, tt.wantType, ownership.Type, "Owner type mismatch")
			if tt.wantSubType != "" {
				assert.Equal(t, tt.wantSubType, ownership.SubType, "Owner subtype mismatch")
			}
			if tt.wantName != "" {
				assert.Equal(t, tt.wantName, ownership.Name, "Owner name mismatch")
			}
		})
	}
}

// TestOwnershipPriority verifies that ownership detection follows correct priority.
// Priority: Flux > Argo > Helm > Terraform > ConfigHub > K8s > Unknown
func TestOwnershipPriority(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		wantType string
	}{
		{
			name: "Flux takes priority over Helm",
			labels: map[string]string{
				"kustomize.toolkit.fluxcd.io/name": "my-app",
				"app.kubernetes.io/managed-by":     "Helm",
			},
			wantType: agent.OwnerFlux,
		},
		{
			name: "Argo takes priority over Helm",
			labels: map[string]string{
				"app.kubernetes.io/instance":   "my-app",
				"argocd.argoproj.io/instance":  "my-app",
				"app.kubernetes.io/managed-by": "Helm",
			},
			wantType: agent.OwnerArgo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "test",
						"namespace": "default",
					},
				},
			}
			resource.SetLabels(tt.labels)

			ownership := agent.DetectOwnership(resource)
			require.Equal(t, tt.wantType, ownership.Type)
		})
	}
}

// TestFluxKustomizationLabels tests the specific Flux Kustomization label patterns.
func TestFluxKustomizationLabels(t *testing.T) {
	tests := []struct {
		name          string
		labels        map[string]string
		wantName      string
		wantNamespace string
	}{
		{
			name: "Standard Flux Kustomization labels",
			labels: map[string]string{
				"kustomize.toolkit.fluxcd.io/name":      "apps",
				"kustomize.toolkit.fluxcd.io/namespace": "flux-system",
			},
			wantName:      "apps",
			wantNamespace: "flux-system",
		},
		{
			name: "Flux Kustomization in custom namespace",
			labels: map[string]string{
				"kustomize.toolkit.fluxcd.io/name":      "monitoring",
				"kustomize.toolkit.fluxcd.io/namespace": "gitops",
			},
			wantName:      "monitoring",
			wantNamespace: "gitops",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "test",
						"namespace": "default",
					},
				},
			}
			resource.SetLabels(tt.labels)

			ownership := agent.DetectOwnership(resource)

			require.Equal(t, agent.OwnerFlux, ownership.Type)
			require.Equal(t, "kustomization", ownership.SubType)
			require.Equal(t, tt.wantName, ownership.Name)
			require.Equal(t, tt.wantNamespace, ownership.Namespace)
		})
	}
}

// TestConfigHubLabels tests the specific ConfigHub label patterns.
func TestConfigHubLabels(t *testing.T) {
	tests := []struct {
		name        string
		labels      map[string]string
		annotations map[string]string
		wantUnit    string
		wantSpace   string
	}{
		{
			name: "ConfigHub labels with annotation for space",
			labels: map[string]string{
				"confighub.com/UnitSlug": "payment-api",
			},
			annotations: map[string]string{
				"confighub.com/SpaceName":   "payments-prod",
				"confighub.com/SpaceID":     "550e8400-e29b-41d4-a716-446655440000",
				"confighub.com/RevisionNum": "127",
			},
			wantUnit:  "payment-api",
			wantSpace: "payments-prod",
		},
		{
			name: "ConfigHub with variant labels",
			labels: map[string]string{
				"confighub.com/UnitSlug":    "order-processor",
				"confighub.com/VariantSlug": "prod-east",
			},
			annotations: map[string]string{
				"confighub.com/SpaceName": "orders-prod",
			},
			wantUnit:  "order-processor",
			wantSpace: "orders-prod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "test",
						"namespace": "default",
					},
				},
			}
			resource.SetLabels(tt.labels)
			resource.SetAnnotations(tt.annotations)

			ownership := agent.DetectOwnership(resource)

			require.Equal(t, agent.OwnerConfigHub, ownership.Type)
			require.Equal(t, "unit", ownership.SubType)
			require.Equal(t, tt.wantUnit, ownership.Name)
			require.Equal(t, tt.wantSpace, ownership.Namespace)
		})
	}
}
