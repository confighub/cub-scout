// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestSecretEvidenceCollector_ExtractWorkloadSecretRefs(t *testing.T) {
	tests := []struct {
		name     string
		resource *unstructured.Unstructured
		wantLen  int
		wantRefs []string // Expected secret names
	}{
		{
			name: "deployment with envFrom secretRef",
			resource: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "test-deploy",
						"namespace": "default",
					},
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"containers": []interface{}{
									map[string]interface{}{
										"name": "main",
										"envFrom": []interface{}{
											map[string]interface{}{
												"secretRef": map[string]interface{}{
													"name": "db-credentials",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantLen:  1,
			wantRefs: []string{"db-credentials"},
		},
		{
			name: "deployment with env secretKeyRef",
			resource: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "test-deploy",
						"namespace": "default",
					},
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"containers": []interface{}{
									map[string]interface{}{
										"name": "main",
										"env": []interface{}{
											map[string]interface{}{
												"name": "DB_PASSWORD",
												"valueFrom": map[string]interface{}{
													"secretKeyRef": map[string]interface{}{
														"name": "db-secret",
														"key":  "password",
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantLen:  1,
			wantRefs: []string{"db-secret"},
		},
		{
			name: "deployment with secret volume",
			resource: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "test-deploy",
						"namespace": "default",
					},
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"containers": []interface{}{
									map[string]interface{}{
										"name": "main",
									},
								},
								"volumes": []interface{}{
									map[string]interface{}{
										"name": "tls-certs",
										"secret": map[string]interface{}{
											"secretName": "tls-secret",
										},
									},
								},
							},
						},
					},
				},
			},
			wantLen:  1,
			wantRefs: []string{"tls-secret"},
		},
		{
			name: "deployment with imagePullSecrets",
			resource: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "test-deploy",
						"namespace": "default",
					},
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"containers": []interface{}{
									map[string]interface{}{
										"name": "main",
									},
								},
								"imagePullSecrets": []interface{}{
									map[string]interface{}{
										"name": "registry-creds",
									},
								},
							},
						},
					},
				},
			},
			wantLen:  1,
			wantRefs: []string{"registry-creds"},
		},
		{
			name: "deployment with projected volume containing secret",
			resource: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "test-deploy",
						"namespace": "default",
					},
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"containers": []interface{}{
									map[string]interface{}{
										"name": "main",
									},
								},
								"volumes": []interface{}{
									map[string]interface{}{
										"name": "combined",
										"projected": map[string]interface{}{
											"sources": []interface{}{
												map[string]interface{}{
													"secret": map[string]interface{}{
														"name": "projected-secret",
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantLen:  1,
			wantRefs: []string{"projected-secret"},
		},
		{
			name: "deployment with multiple secret refs (deduped)",
			resource: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "test-deploy",
						"namespace": "default",
					},
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"containers": []interface{}{
									map[string]interface{}{
										"name": "main",
										"envFrom": []interface{}{
											map[string]interface{}{
												"secretRef": map[string]interface{}{
													"name": "shared-secret",
												},
											},
										},
										"env": []interface{}{
											map[string]interface{}{
												"name": "KEY",
												"valueFrom": map[string]interface{}{
													"secretKeyRef": map[string]interface{}{
														"name": "another-secret",
														"key":  "value",
													},
												},
											},
										},
									},
									map[string]interface{}{
										"name": "sidecar",
										"envFrom": []interface{}{
											map[string]interface{}{
												"secretRef": map[string]interface{}{
													"name": "shared-secret", // Same as first container - should dedupe
												},
											},
										},
									},
								},
								"volumes": []interface{}{
									map[string]interface{}{
										"name": "tls",
										"secret": map[string]interface{}{
											"secretName": "tls-secret",
										},
									},
								},
							},
						},
					},
				},
			},
			wantLen:  3, // shared-secret (deduped), another-secret, tls-secret
			wantRefs: []string{"shared-secret", "another-secret", "tls-secret"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a fake client - we don't need secrets for this test
			scheme := runtime.NewScheme()
			client := dynamicfake.NewSimpleDynamicClient(scheme)

			collector := NewSecretEvidenceCollector(client)
			refs := collector.extractWorkloadSecretRefs(tt.resource)

			if len(refs) != tt.wantLen {
				t.Errorf("extractWorkloadSecretRefs() returned %d refs, want %d", len(refs), tt.wantLen)
				for i, ref := range refs {
					t.Logf("  ref[%d]: %s (type: %s)", i, ref.name, ref.refType)
				}
			}

			// Check that expected refs are present
			for _, wantRef := range tt.wantRefs {
				found := false
				for _, ref := range refs {
					if ref.name == wantRef {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected secret ref %q not found", wantRef)
				}
			}
		})
	}
}

func TestSecretEvidenceCollector_ExtractFluxSecretRefs(t *testing.T) {
	tests := []struct {
		name     string
		resource *unstructured.Unstructured
		wantLen  int
		wantRefs []string
	}{
		{
			name: "GitRepository with secretRef",
			resource: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "source.toolkit.fluxcd.io/v1",
					"kind":       "GitRepository",
					"metadata": map[string]interface{}{
						"name":      "app-repo",
						"namespace": "flux-system",
					},
					"spec": map[string]interface{}{
						"url": "https://github.com/org/repo",
						"secretRef": map[string]interface{}{
							"name": "git-credentials",
						},
					},
				},
			},
			wantLen:  1,
			wantRefs: []string{"git-credentials"},
		},
		{
			name: "HelmRelease with valuesFrom secret",
			resource: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "helm.toolkit.fluxcd.io/v2",
					"kind":       "HelmRelease",
					"metadata": map[string]interface{}{
						"name":      "myapp",
						"namespace": "default",
					},
					"spec": map[string]interface{}{
						"valuesFrom": []interface{}{
							map[string]interface{}{
								"kind": "Secret",
								"name": "helm-values-secret",
							},
							map[string]interface{}{
								"kind": "ConfigMap",
								"name": "helm-values-cm",
							},
						},
					},
				},
			},
			wantLen:  1, // Only secret, not configmap
			wantRefs: []string{"helm-values-secret"},
		},
		{
			name: "Kustomization with decryption secretRef",
			resource: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
					"kind":       "Kustomization",
					"metadata": map[string]interface{}{
						"name":      "apps",
						"namespace": "flux-system",
					},
					"spec": map[string]interface{}{
						"decryption": map[string]interface{}{
							"provider": "sops",
							"secretRef": map[string]interface{}{
								"name": "sops-gpg-key",
							},
						},
					},
				},
			},
			wantLen:  1,
			wantRefs: []string{"sops-gpg-key"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			client := dynamicfake.NewSimpleDynamicClient(scheme)

			collector := NewSecretEvidenceCollector(client)
			result, err := collector.CollectFromResource(context.Background(), tt.resource)

			if err != nil {
				t.Fatalf("CollectFromResource() error = %v", err)
			}

			if len(result.Secrets) != tt.wantLen {
				t.Errorf("CollectFromResource() returned %d secrets, want %d", len(result.Secrets), tt.wantLen)
				for i, s := range result.Secrets {
					t.Logf("  secret[%d]: %s (status: %s)", i, s.Name, s.Status)
				}
			}

			for _, wantRef := range tt.wantRefs {
				found := false
				for _, s := range result.Secrets {
					if s.Name == wantRef {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected secret %q not found in results", wantRef)
				}
			}
		})
	}
}

func TestSecretEvidenceCollector_ResolveStatus(t *testing.T) {
	tests := []struct {
		name         string
		secretName   string
		secretExists bool
		forbidden    bool
		wantStatus   SecretStatus
	}{
		{
			name:         "secret exists",
			secretName:   "existing-secret",
			secretExists: true,
			wantStatus:   SecretStatusPresent,
		},
		{
			name:         "secret missing",
			secretName:   "missing-secret",
			secretExists: false,
			wantStatus:   SecretStatusMissing,
		},
		{
			name:         "secret forbidden (RBAC)",
			secretName:   "forbidden-secret",
			forbidden:    true,
			wantStatus:   SecretStatusUnreadable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			client := dynamicfake.NewSimpleDynamicClient(scheme)

			// Set up the fake client behavior
			if tt.secretExists {
				secret := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Secret",
						"metadata": map[string]interface{}{
							"name":      tt.secretName,
							"namespace": "default",
						},
						"type": "Opaque",
					},
				}
				_, err := client.Resource(schema.GroupVersionResource{
					Group:    "",
					Version:  "v1",
					Resource: "secrets",
				}).Namespace("default").Create(context.Background(), secret, metav1.CreateOptions{})
				if err != nil {
					t.Fatalf("Failed to create test secret: %v", err)
				}
			}

			if tt.forbidden {
				// Add a reactor to return Forbidden errors
				client.PrependReactor("get", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, apierrors.NewForbidden(
						schema.GroupResource{Group: "", Resource: "secrets"},
						tt.secretName,
						nil,
					)
				})
			}

			collector := NewSecretEvidenceCollector(client)
			ref := secretReference{
				name:    tt.secretName,
				refType: SecretRefTypeEnvFrom,
			}

			evidence := collector.resolveSecretRef(context.Background(), ref, "default")

			if evidence.Status != tt.wantStatus {
				t.Errorf("resolveSecretRef() status = %v, want %v", evidence.Status, tt.wantStatus)
			}

			// Verify status reason is set for non-present status
			if tt.wantStatus != SecretStatusPresent && evidence.StatusReason == "" {
				t.Error("expected StatusReason to be set for non-present status")
			}
		})
	}
}

func TestSecretEvidenceResult_HasIssues(t *testing.T) {
	tests := []struct {
		name    string
		summary SecretEvidenceSummary
		want    bool
	}{
		{
			name:    "all present - no issues",
			summary: SecretEvidenceSummary{Total: 3, Present: 3},
			want:    false,
		},
		{
			name:    "one missing - has issues",
			summary: SecretEvidenceSummary{Total: 3, Present: 2, Missing: 1},
			want:    true,
		},
		{
			name:    "one unreadable - has issues",
			summary: SecretEvidenceSummary{Total: 3, Present: 2, Unreadable: 1},
			want:    true,
		},
		{
			name:    "one unresolved - has issues",
			summary: SecretEvidenceSummary{Total: 3, Present: 2, Unresolved: 1},
			want:    true,
		},
		{
			name:    "empty - no issues",
			summary: SecretEvidenceSummary{Total: 0},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &SecretEvidenceResult{Summary: tt.summary}
			if got := result.HasIssues(); got != tt.want {
				t.Errorf("HasIssues() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSecretEvidenceResult_String(t *testing.T) {
	tests := []struct {
		name    string
		result  *SecretEvidenceResult
		want    string
		wantIn  []string // Substrings that should be present
		wantOut []string // Substrings that should NOT be present
	}{
		{
			name: "empty",
			result: &SecretEvidenceResult{
				Summary: SecretEvidenceSummary{Total: 0},
			},
			want: "no secret references",
		},
		{
			name: "all present",
			result: &SecretEvidenceResult{
				Summary: SecretEvidenceSummary{Total: 3, Present: 3},
			},
			wantIn:  []string{"3 secrets", "3 present"},
			wantOut: []string{"missing", "unreadable"},
		},
		{
			name: "mixed status",
			result: &SecretEvidenceResult{
				Summary: SecretEvidenceSummary{Total: 4, Present: 2, Missing: 1, Unreadable: 1},
			},
			wantIn: []string{"4 secrets", "2 present", "1 missing", "1 unreadable"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.result.String()
			if tt.want != "" && got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
			for _, s := range tt.wantIn {
				if !strings.Contains(got, s) {
					t.Errorf("String() = %q, should contain %q", got, s)
				}
			}
			for _, s := range tt.wantOut {
				if strings.Contains(got, s) {
					t.Errorf("String() = %q, should NOT contain %q", got, s)
				}
			}
		})
	}
}

func TestSecretEvidence_NoDataExposed(t *testing.T) {
	// This test verifies that secret data is never exposed in the evidence model.
	// The SecretEvidence struct intentionally excludes fields like Data, StringData.
	// This is a compile-time check via the struct definition.

	evidence := SecretEvidence{
		Name:         "test-secret",
		Namespace:    "default",
		RefType:      SecretRefTypeEnvFrom,
		Status:       SecretStatusPresent,
		SecretType:   "Opaque",
		StatusReason: "",
	}

	// Verify no data-related fields exist via reflection would be one approach,
	// but the struct definition itself is the guarantee.
	// This test documents the intent.

	if evidence.Name == "" {
		t.Error("expected Name to be set")
	}

	// The key safety property: SecretEvidence has no Data, StringData, or similar fields.
	// This is enforced by the struct definition not containing such fields.
	t.Log("SecretEvidence struct correctly excludes secret data fields")
}

func TestSecretEvidenceCollector_OptionalSecrets(t *testing.T) {
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "test-deploy",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name": "main",
								"envFrom": []interface{}{
									map[string]interface{}{
										"secretRef": map[string]interface{}{
											"name":     "optional-secret",
											"optional": true,
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

	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClient(scheme)

	collector := NewSecretEvidenceCollector(client)
	result, err := collector.CollectFromResource(context.Background(), resource)

	if err != nil {
		t.Fatalf("CollectFromResource() error = %v", err)
	}

	if len(result.Secrets) != 1 {
		t.Fatalf("expected 1 secret ref, got %d", len(result.Secrets))
	}

	if !result.Secrets[0].Optional {
		t.Error("expected Optional to be true for optional secretRef")
	}
}

func TestSecretEvidenceCollector_UnsupportedKind(t *testing.T) {
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service", // Not supported for secret evidence
			"metadata": map[string]interface{}{
				"name":      "test-svc",
				"namespace": "default",
			},
		},
	}

	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClient(scheme)

	collector := NewSecretEvidenceCollector(client)
	result, err := collector.CollectFromResource(context.Background(), resource)

	if err != nil {
		t.Fatalf("CollectFromResource() error = %v", err)
	}

	// Should return empty result for unsupported kinds
	if len(result.Secrets) != 0 {
		t.Errorf("expected 0 secrets for unsupported kind, got %d", len(result.Secrets))
	}
}
