// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestParseSourceRef_Kustomization(t *testing.T) {
	tests := []struct {
		name              string
		resource          *unstructured.Unstructured
		deployerNamespace string
		wantKey           string
		wantOK            bool
	}{
		{
			name: "basic GitRepository sourceRef",
			resource: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
					"kind":       "Kustomization",
					"metadata": map[string]interface{}{
						"name":      "app",
						"namespace": "flux-system",
					},
					"spec": map[string]interface{}{
						"sourceRef": map[string]interface{}{
							"kind": "GitRepository",
							"name": "my-repo",
						},
					},
				},
			},
			deployerNamespace: "flux-system",
			wantKey:           "GitRepository/flux-system/my-repo",
			wantOK:            true,
		},
		{
			name: "OCIRepository sourceRef",
			resource: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
					"kind":       "Kustomization",
					"metadata": map[string]interface{}{
						"name":      "app",
						"namespace": "flux-system",
					},
					"spec": map[string]interface{}{
						"sourceRef": map[string]interface{}{
							"kind": "OCIRepository",
							"name": "oci-manifests",
						},
					},
				},
			},
			deployerNamespace: "flux-system",
			wantKey:           "OCIRepository/flux-system/oci-manifests",
			wantOK:            true,
		},
		{
			name: "cross-namespace sourceRef",
			resource: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
					"kind":       "Kustomization",
					"metadata": map[string]interface{}{
						"name":      "team-app",
						"namespace": "team-alpha",
					},
					"spec": map[string]interface{}{
						"sourceRef": map[string]interface{}{
							"kind":      "OCIRepository",
							"name":      "shared-manifests",
							"namespace": "flux-system",
						},
					},
				},
			},
			deployerNamespace: "team-alpha",
			wantKey:           "OCIRepository/flux-system/shared-manifests",
			wantOK:            true,
		},
		{
			name: "missing sourceRef",
			resource: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
					"kind":       "Kustomization",
					"metadata": map[string]interface{}{
						"name":      "app",
						"namespace": "flux-system",
					},
					"spec": map[string]interface{}{},
				},
			},
			deployerNamespace: "flux-system",
			wantKey:           "",
			wantOK:            false,
		},
		{
			name: "missing name in sourceRef",
			resource: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
					"kind":       "Kustomization",
					"metadata": map[string]interface{}{
						"name":      "app",
						"namespace": "flux-system",
					},
					"spec": map[string]interface{}{
						"sourceRef": map[string]interface{}{
							"kind": "GitRepository",
						},
					},
				},
			},
			deployerNamespace: "flux-system",
			wantKey:           "",
			wantOK:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, ok := ParseSourceRef(tt.resource, tt.deployerNamespace)
			if ok != tt.wantOK {
				t.Errorf("ParseSourceRef() ok = %v, wantOK = %v", ok, tt.wantOK)
				return
			}
			if ok && ref.Key() != tt.wantKey {
				t.Errorf("ParseSourceRef().Key() = %v, wantKey = %v", ref.Key(), tt.wantKey)
			}
		})
	}
}

func TestParseSourceRef_HelmRelease(t *testing.T) {
	tests := []struct {
		name              string
		resource          *unstructured.Unstructured
		deployerNamespace string
		wantKey           string
		wantOK            bool
	}{
		{
			name: "HelmRelease with chartRef (OCIRepository)",
			resource: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "helm.toolkit.fluxcd.io/v2",
					"kind":       "HelmRelease",
					"metadata": map[string]interface{}{
						"name":      "app",
						"namespace": "default",
					},
					"spec": map[string]interface{}{
						"chartRef": map[string]interface{}{
							"kind": "OCIRepository",
							"name": "charts",
						},
					},
				},
			},
			deployerNamespace: "default",
			wantKey:           "OCIRepository/default/charts",
			wantOK:            true,
		},
		{
			name: "HelmRelease with chart.spec.sourceRef (HelmRepository)",
			resource: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "helm.toolkit.fluxcd.io/v2",
					"kind":       "HelmRelease",
					"metadata": map[string]interface{}{
						"name":      "app",
						"namespace": "default",
					},
					"spec": map[string]interface{}{
						"chart": map[string]interface{}{
							"spec": map[string]interface{}{
								"sourceRef": map[string]interface{}{
									"kind": "HelmRepository",
									"name": "bitnami",
								},
							},
						},
					},
				},
			},
			deployerNamespace: "default",
			wantKey:           "HelmRepository/default/bitnami",
			wantOK:            true,
		},
		{
			name: "HelmRelease with cross-namespace chartRef",
			resource: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "helm.toolkit.fluxcd.io/v2",
					"kind":       "HelmRelease",
					"metadata": map[string]interface{}{
						"name":      "app",
						"namespace": "production",
					},
					"spec": map[string]interface{}{
						"chartRef": map[string]interface{}{
							"kind":      "OCIRepository",
							"name":      "shared-charts",
							"namespace": "flux-system",
						},
					},
				},
			},
			deployerNamespace: "production",
			wantKey:           "OCIRepository/flux-system/shared-charts",
			wantOK:            true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, ok := ParseSourceRef(tt.resource, tt.deployerNamespace)
			if ok != tt.wantOK {
				t.Errorf("ParseSourceRef() ok = %v, wantOK = %v", ok, tt.wantOK)
				return
			}
			if ok && ref.Key() != tt.wantKey {
				t.Errorf("ParseSourceRef().Key() = %v, wantKey = %v", ref.Key(), tt.wantKey)
			}
		})
	}
}

func TestSourceRef_IsOCI(t *testing.T) {
	tests := []struct {
		name    string
		ref     SourceRef
		wantOCI bool
	}{
		{
			name:    "OCIRepository is OCI",
			ref:     SourceRef{Kind: "OCIRepository", Name: "test", Namespace: "default"},
			wantOCI: true,
		},
		{
			name:    "GitRepository is not OCI",
			ref:     SourceRef{Kind: "GitRepository", Name: "test", Namespace: "default"},
			wantOCI: false,
		},
		{
			name:    "HelmRepository is not OCI",
			ref:     SourceRef{Kind: "HelmRepository", Name: "test", Namespace: "default"},
			wantOCI: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ref.IsOCI(); got != tt.wantOCI {
				t.Errorf("SourceRef.IsOCI() = %v, want %v", got, tt.wantOCI)
			}
		})
	}
}

func TestParseDeployerRef(t *testing.T) {
	tests := []struct {
		name    string
		resource *unstructured.Unstructured
		wantKey string
		wantOK  bool
	}{
		{
			name: "Kustomization with sourceRef",
			resource: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
					"kind":       "Kustomization",
					"metadata": map[string]interface{}{
						"name":      "app-deploy",
						"namespace": "flux-system",
					},
					"spec": map[string]interface{}{
						"sourceRef": map[string]interface{}{
							"kind": "OCIRepository",
							"name": "manifests",
						},
					},
				},
			},
			wantKey: "Kustomization/flux-system/app-deploy",
			wantOK:  true,
		},
		{
			name: "HelmRelease with chartRef",
			resource: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "helm.toolkit.fluxcd.io/v2",
					"kind":       "HelmRelease",
					"metadata": map[string]interface{}{
						"name":      "nginx",
						"namespace": "default",
					},
					"spec": map[string]interface{}{
						"chartRef": map[string]interface{}{
							"kind": "OCIRepository",
							"name": "charts",
						},
					},
				},
			},
			wantKey: "HelmRelease/default/nginx",
			wantOK:  true,
		},
		{
			name: "Deployment is not a deployer",
			resource: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "app",
						"namespace": "default",
					},
				},
			},
			wantKey: "",
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, ok := ParseDeployerRef(tt.resource)
			if ok != tt.wantOK {
				t.Errorf("ParseDeployerRef() ok = %v, wantOK = %v", ok, tt.wantOK)
				return
			}
			if ok && ref.Key() != tt.wantKey {
				t.Errorf("ParseDeployerRef().Key() = %v, wantKey = %v", ref.Key(), tt.wantKey)
			}
		})
	}
}

func TestParseSourceRefKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantRef SourceRef
		wantOK  bool
	}{
		{
			name:    "valid key",
			key:     "OCIRepository/flux-system/manifests",
			wantRef: SourceRef{Kind: "OCIRepository", Namespace: "flux-system", Name: "manifests"},
			wantOK:  true,
		},
		{
			name:   "invalid key - missing parts",
			key:    "OCIRepository/manifests",
			wantOK: false,
		},
		{
			name:   "invalid key - empty",
			key:    "",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, ok := ParseSourceRefKey(tt.key)
			if ok != tt.wantOK {
				t.Errorf("ParseSourceRefKey() ok = %v, wantOK = %v", ok, tt.wantOK)
				return
			}
			if ok && ref != tt.wantRef {
				t.Errorf("ParseSourceRefKey() = %+v, want %+v", ref, tt.wantRef)
			}
		})
	}
}
