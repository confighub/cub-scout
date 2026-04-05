// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/confighub/cub-scout/pkg/agent"
)

func TestFormatSecretIssue_Missing(t *testing.T) {
	issue := SecretIssue{
		Resource:   "Deployment/nginx",
		Namespace:  "prod",
		SecretName: "db-creds",
		SecretNS:   "prod",
		RefType:    "envFrom.secretRef",
		Status:     "missing",
	}

	got := formatSecretIssue(issue)
	want := `✗ Deployment/nginx in prod: missing secret "db-creds" (envFrom.secretRef)`

	if got != want {
		t.Fatalf("formatSecretIssue() = %q, want %q", got, want)
	}
}

func TestFormatSecretIssue_Unreadable(t *testing.T) {
	issue := SecretIssue{
		Resource:   "HelmRelease/myapp",
		Namespace:  "default",
		SecretName: "helm-values",
		SecretNS:   "default",
		RefType:    "spec.valuesFrom",
		Status:     "unreadable",
	}

	got := formatSecretIssue(issue)
	want := `✗ HelmRelease/myapp in default: unreadable (RBAC) secret "helm-values" (spec.valuesFrom)`

	if got != want {
		t.Fatalf("formatSecretIssue() = %q, want %q", got, want)
	}
}

func TestFormatSecretIssue_CrossNamespace(t *testing.T) {
	issue := SecretIssue{
		Resource:   "Kustomization/infra",
		Namespace:  "flux-system",
		SecretName: "git-creds",
		SecretNS:   "secrets",
		RefType:    "spec.secretRef",
		Status:     "missing",
	}

	got := formatSecretIssue(issue)
	// Cross-namespace reference should show namespace/name
	want := `✗ Kustomization/infra in flux-system: missing secret "secrets/git-creds" (spec.secretRef)`

	if got != want {
		t.Fatalf("formatSecretIssue() = %q, want %q", got, want)
	}
}

func TestCollectSecretIssuesForResource_UnsupportedKind(t *testing.T) {
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClient(scheme)

	// ConfigMap is not a supported kind for secret collection
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "my-config",
				"namespace": "default",
			},
		},
	}

	issues := collectSecretIssuesForResource(context.Background(), client, resource)
	if issues != nil {
		t.Fatalf("collectSecretIssuesForResource() for ConfigMap should return nil, got %v", issues)
	}
}

func TestCollectSecretIssuesForResource_DeploymentWithMissingSecret(t *testing.T) {
	scheme := runtime.NewScheme()

	// Create a fake client with NO secrets (so all references are missing)
	client := dynamicfake.NewSimpleDynamicClient(scheme)

	// Create a deployment that references a secret
	deployment := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "nginx",
				"namespace": "prod",
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name": "nginx",
								"envFrom": []interface{}{
									map[string]interface{}{
										"secretRef": map[string]interface{}{
											"name": "db-creds",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	issues := collectSecretIssuesForResource(context.Background(), client, deployment)

	if len(issues) != 1 {
		t.Fatalf("collectSecretIssuesForResource() returned %d issues, want 1", len(issues))
	}

	issue := issues[0]
	if issue.SecretName != "db-creds" {
		t.Errorf("issue.SecretName = %q, want %q", issue.SecretName, "db-creds")
	}
	if issue.Status != "missing" {
		t.Errorf("issue.Status = %q, want %q", issue.Status, "missing")
	}
	if issue.Resource != "Deployment/nginx" {
		t.Errorf("issue.Resource = %q, want %q", issue.Resource, "Deployment/nginx")
	}
}

func TestCollectSecretIssuesForResource_DeploymentWithPresentSecret(t *testing.T) {
	scheme := runtime.NewScheme()

	// Create a fake client WITH the secret present
	secret := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "db-creds",
				"namespace": "prod",
			},
			"type": "Opaque",
		},
	}
	client := dynamicfake.NewSimpleDynamicClient(scheme, secret)

	// Create a deployment that references the secret
	deployment := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "nginx",
				"namespace": "prod",
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name": "nginx",
								"envFrom": []interface{}{
									map[string]interface{}{
										"secretRef": map[string]interface{}{
											"name": "db-creds",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	issues := collectSecretIssuesForResource(context.Background(), client, deployment)

	// Present secrets should NOT generate issues
	if len(issues) != 0 {
		t.Fatalf("collectSecretIssuesForResource() returned %d issues for present secret, want 0", len(issues))
	}
}

func TestCollectSecretIssuesForResource_DedupeMultipleReferences(t *testing.T) {
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClient(scheme)

	// Create a deployment that references the same secret multiple times
	deployment := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "app",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name": "app",
								"envFrom": []interface{}{
									map[string]interface{}{
										"secretRef": map[string]interface{}{
											"name": "shared-creds",
										},
									},
								},
							},
							map[string]interface{}{
								"name": "sidecar",
								"envFrom": []interface{}{
									map[string]interface{}{
										"secretRef": map[string]interface{}{
											"name": "shared-creds", // Same secret
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	issues := collectSecretIssuesForResource(context.Background(), client, deployment)

	// Should only report one issue for the deduplicated secret
	if len(issues) != 1 {
		t.Fatalf("collectSecretIssuesForResource() returned %d issues, want 1 (deduped)", len(issues))
	}
}

func TestCollectSecretIssuesForResource_FluxKustomization(t *testing.T) {
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClient(scheme)

	// Create a Kustomization that references a secret for decryption
	ks := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
			"kind":       "Kustomization",
			"metadata": map[string]interface{}{
				"name":      "infra",
				"namespace": "flux-system",
			},
			"spec": map[string]interface{}{
				"decryption": map[string]interface{}{
					"provider": "sops",
					"secretRef": map[string]interface{}{
						"name": "sops-age",
					},
				},
			},
		},
	}

	issues := collectSecretIssuesForResource(context.Background(), client, ks)

	if len(issues) != 1 {
		t.Fatalf("collectSecretIssuesForResource() for Kustomization returned %d issues, want 1", len(issues))
	}

	if issues[0].SecretName != "sops-age" {
		t.Errorf("issue.SecretName = %q, want %q", issues[0].SecretName, "sops-age")
	}
}

func TestSecretIssue_StatusTypes(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{"missing", "missing"},
		{"unreadable", "unreadable (RBAC)"},
		{"unresolved", "unresolved"},
	}

	for _, tt := range tests {
		issue := SecretIssue{
			Resource:   "Deployment/test",
			Namespace:  "default",
			SecretName: "test-secret",
			SecretNS:   "default",
			RefType:    "envFrom.secretRef",
			Status:     tt.status,
		}

		got := formatSecretIssue(issue)
		if !containsSubstring(got, tt.want) {
			t.Errorf("formatSecretIssue() with status %q should contain %q, got %q", tt.status, tt.want, got)
		}
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstringHelper(s, substr))
}

func containsSubstringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Verify that SecretStatusPresent from agent package is used correctly
func TestSecretStatusConstants(t *testing.T) {
	// Ensure we're using the correct constants from the agent package
	if agent.SecretStatusPresent != "present" {
		t.Errorf("agent.SecretStatusPresent = %q, want %q", agent.SecretStatusPresent, "present")
	}
	if agent.SecretStatusMissing != "missing" {
		t.Errorf("agent.SecretStatusMissing = %q, want %q", agent.SecretStatusMissing, "missing")
	}
	if agent.SecretStatusUnreadable != "unreadable" {
		t.Errorf("agent.SecretStatusUnreadable = %q, want %q", agent.SecretStatusUnreadable, "unreadable")
	}
	if agent.SecretStatusUnresolved != "unresolved" {
		t.Errorf("agent.SecretStatusUnresolved = %q, want %q", agent.SecretStatusUnresolved, "unresolved")
	}
}
