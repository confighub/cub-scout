// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

func TestBuildOwnsRelations(t *testing.T) {
	// Create test resources simulating: Deployment -> ReplicaSet -> Pod

	deploymentUID := types.UID("deploy-uid-123")
	replicaSetUID := types.UID("rs-uid-456")
	podUID := types.UID("pod-uid-789")

	// Deployment (no owner)
	deployment := unstructured.Unstructured{}
	deployment.SetUID(deploymentUID)
	deployment.SetName("backend")
	deployment.SetNamespace("prod")
	deployment.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "Deployment",
	})

	// ReplicaSet (owned by Deployment)
	replicaSet := unstructured.Unstructured{}
	replicaSet.SetUID(replicaSetUID)
	replicaSet.SetName("backend-xyz")
	replicaSet.SetNamespace("prod")
	replicaSet.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "ReplicaSet",
	})
	replicaSet.SetOwnerReferences([]metav1.OwnerReference{
		{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       "backend",
			UID:        deploymentUID,
		},
	})

	// Pod (owned by ReplicaSet)
	pod := unstructured.Unstructured{}
	pod.SetUID(podUID)
	pod.SetName("backend-xyz-abc")
	pod.SetNamespace("prod")
	pod.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Pod",
	})
	pod.SetOwnerReferences([]metav1.OwnerReference{
		{
			APIVersion: "apps/v1",
			Kind:       "ReplicaSet",
			Name:       "backend-xyz",
			UID:        replicaSetUID,
		},
	})

	items := []unstructured.Unstructured{deployment, replicaSet, pod}
	relations := buildOwnsRelations(items, "test-cluster")

	// Should have 2 relations:
	// 1. Deployment owns ReplicaSet
	// 2. ReplicaSet owns Pod
	if len(relations) != 2 {
		t.Errorf("expected 2 relations, got %d", len(relations))
	}

	// Check for Deployment -> ReplicaSet relation
	foundDeployToRS := false
	foundRSToPod := false

	for _, rel := range relations {
		if rel.Type != "owns" {
			t.Errorf("expected type 'owns', got %s", rel.Type)
		}

		// Deployment owns ReplicaSet
		if rel.From == "test-cluster/prod/apps/Deployment/backend" &&
			rel.To == "test-cluster/prod/apps/ReplicaSet/backend-xyz" {
			foundDeployToRS = true
		}

		// ReplicaSet owns Pod
		if rel.From == "test-cluster/prod/apps/ReplicaSet/backend-xyz" &&
			rel.To == "test-cluster/prod//Pod/backend-xyz-abc" {
			foundRSToPod = true
		}
	}

	if !foundDeployToRS {
		t.Error("missing Deployment -> ReplicaSet relation")
		t.Logf("relations: %+v", relations)
	}

	if !foundRSToPod {
		t.Error("missing ReplicaSet -> Pod relation")
		t.Logf("relations: %+v", relations)
	}
}

func TestBuildOwnsRelations_NoOwners(t *testing.T) {
	// Resource with no owner references should produce no relations
	deployment := unstructured.Unstructured{}
	deployment.SetUID("uid-123")
	deployment.SetName("standalone")
	deployment.SetNamespace("default")
	deployment.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "Deployment",
	})

	items := []unstructured.Unstructured{deployment}
	relations := buildOwnsRelations(items, "test-cluster")

	if len(relations) != 0 {
		t.Errorf("expected 0 relations, got %d", len(relations))
	}
}

func TestBuildSelectsRelations(t *testing.T) {
	// Create a Service with selector and matching Pods

	// Service with selector app=backend
	service := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      "backend-svc",
				"namespace": "prod",
				"uid":       "svc-uid-123",
			},
			"spec": map[string]interface{}{
				"selector": map[string]interface{}{
					"app": "backend",
				},
			},
		},
	}

	// Pod that matches the selector
	matchingPod := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "backend-xyz-abc",
				"namespace": "prod",
				"uid":       "pod-uid-456",
				"labels": map[string]interface{}{
					"app": "backend",
				},
			},
		},
	}

	// Pod that doesn't match (different label)
	nonMatchingPod := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "frontend-xyz-abc",
				"namespace": "prod",
				"uid":       "pod-uid-789",
				"labels": map[string]interface{}{
					"app": "frontend",
				},
			},
		},
	}

	// Pod in different namespace (shouldn't match)
	differentNsPod := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "backend-other",
				"namespace": "staging",
				"uid":       "pod-uid-999",
				"labels": map[string]interface{}{
					"app": "backend",
				},
			},
		},
	}

	items := []unstructured.Unstructured{service, matchingPod, nonMatchingPod, differentNsPod}
	relations := buildSelectsRelations(items, "test-cluster")

	// Should have exactly 1 relation: Service -> matching Pod
	if len(relations) != 1 {
		t.Errorf("expected 1 relation, got %d", len(relations))
		t.Logf("relations: %+v", relations)
	}

	if len(relations) > 0 {
		rel := relations[0]
		if rel.Type != "selects" {
			t.Errorf("expected type 'selects', got %s", rel.Type)
		}
		if rel.From != "test-cluster/prod//Service/backend-svc" {
			t.Errorf("unexpected from: %s", rel.From)
		}
		if rel.To != "test-cluster/prod//Pod/backend-xyz-abc" {
			t.Errorf("unexpected to: %s", rel.To)
		}
	}
}

func TestBuildSelectsRelations_NoSelector(t *testing.T) {
	// Service without selector should produce no relations
	service := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      "external-svc",
				"namespace": "prod",
				"uid":       "svc-uid-123",
			},
			"spec": map[string]interface{}{
				// No selector - e.g., ExternalName service
			},
		},
	}

	pod := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "some-pod",
				"namespace": "prod",
				"uid":       "pod-uid-456",
				"labels": map[string]interface{}{
					"app": "backend",
				},
			},
		},
	}

	items := []unstructured.Unstructured{service, pod}
	relations := buildSelectsRelations(items, "test-cluster")

	if len(relations) != 0 {
		t.Errorf("expected 0 relations, got %d", len(relations))
	}
}

func TestBuildMountsRelations(t *testing.T) {
	// Pod that mounts a ConfigMap and Secret
	pod := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "app-xyz",
				"namespace": "prod",
				"uid":       "pod-uid-123",
			},
			"spec": map[string]interface{}{
				"volumes": []interface{}{
					map[string]interface{}{
						"name": "config-vol",
						"configMap": map[string]interface{}{
							"name": "app-config",
						},
					},
					map[string]interface{}{
						"name": "secret-vol",
						"secret": map[string]interface{}{
							"secretName": "app-secrets",
						},
					},
					map[string]interface{}{
						"name": "empty-vol",
						"emptyDir": map[string]interface{}{},
					},
				},
			},
		},
	}

	items := []unstructured.Unstructured{pod}
	relations := buildMountsRelations(items, "test-cluster")

	// Should have 2 relations: ConfigMap and Secret (not emptyDir)
	if len(relations) != 2 {
		t.Errorf("expected 2 relations, got %d", len(relations))
		t.Logf("relations: %+v", relations)
	}

	foundConfigMap := false
	foundSecret := false

	for _, rel := range relations {
		if rel.Type != "mounts" {
			t.Errorf("expected type 'mounts', got %s", rel.Type)
		}
		if rel.From != "test-cluster/prod//Pod/app-xyz" {
			t.Errorf("unexpected from: %s", rel.From)
		}
		if rel.To == "test-cluster/prod//ConfigMap/app-config" {
			foundConfigMap = true
		}
		if rel.To == "test-cluster/prod//Secret/app-secrets" {
			foundSecret = true
		}
	}

	if !foundConfigMap {
		t.Error("missing Pod -> ConfigMap relation")
	}
	if !foundSecret {
		t.Error("missing Pod -> Secret relation")
	}
}

func TestBuildMountsRelations_NoVolumes(t *testing.T) {
	// Pod without volumes should produce no relations
	pod := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "simple-pod",
				"namespace": "prod",
				"uid":       "pod-uid-123",
			},
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "main",
						"image": "nginx",
					},
				},
			},
		},
	}

	items := []unstructured.Unstructured{pod}
	relations := buildMountsRelations(items, "test-cluster")

	if len(relations) != 0 {
		t.Errorf("expected 0 relations, got %d", len(relations))
	}
}

func TestBuildReferencesRelations_EnvFrom(t *testing.T) {
	// Pod with envFrom referencing ConfigMap and Secret
	pod := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "app-xyz",
				"namespace": "prod",
				"uid":       "pod-uid-123",
			},
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "main",
						"image": "app:latest",
						"envFrom": []interface{}{
							map[string]interface{}{
								"configMapRef": map[string]interface{}{
									"name": "app-env",
								},
							},
							map[string]interface{}{
								"secretRef": map[string]interface{}{
									"name": "app-secrets",
								},
							},
						},
					},
				},
			},
		},
	}

	items := []unstructured.Unstructured{pod}
	relations := buildReferencesRelations(items, "test-cluster")

	// Should have 2 relations
	if len(relations) != 2 {
		t.Errorf("expected 2 relations, got %d", len(relations))
		t.Logf("relations: %+v", relations)
	}

	foundConfigMap := false
	foundSecret := false

	for _, rel := range relations {
		if rel.Type != "references" {
			t.Errorf("expected type 'references', got %s", rel.Type)
		}
		if rel.From != "test-cluster/prod//Pod/app-xyz" {
			t.Errorf("unexpected from: %s", rel.From)
		}
		if rel.To == "test-cluster/prod//ConfigMap/app-env" {
			foundConfigMap = true
		}
		if rel.To == "test-cluster/prod//Secret/app-secrets" {
			foundSecret = true
		}
	}

	if !foundConfigMap {
		t.Error("missing Pod -> ConfigMap relation")
	}
	if !foundSecret {
		t.Error("missing Pod -> Secret relation")
	}
}

func TestBuildReferencesRelations_EnvValueFrom(t *testing.T) {
	// Pod with env[].valueFrom referencing ConfigMap and Secret
	pod := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "app-xyz",
				"namespace": "prod",
				"uid":       "pod-uid-123",
			},
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "main",
						"image": "app:latest",
						"env": []interface{}{
							map[string]interface{}{
								"name": "DB_HOST",
								"valueFrom": map[string]interface{}{
									"configMapKeyRef": map[string]interface{}{
										"name": "db-config",
										"key":  "host",
									},
								},
							},
							map[string]interface{}{
								"name": "DB_PASSWORD",
								"valueFrom": map[string]interface{}{
									"secretKeyRef": map[string]interface{}{
										"name": "db-secrets",
										"key":  "password",
									},
								},
							},
							map[string]interface{}{
								"name":  "STATIC_VAR",
								"value": "static-value",
							},
						},
					},
				},
			},
		},
	}

	items := []unstructured.Unstructured{pod}
	relations := buildReferencesRelations(items, "test-cluster")

	// Should have 2 relations (static env var doesn't count)
	if len(relations) != 2 {
		t.Errorf("expected 2 relations, got %d", len(relations))
		t.Logf("relations: %+v", relations)
	}

	foundConfigMap := false
	foundSecret := false

	for _, rel := range relations {
		if rel.To == "test-cluster/prod//ConfigMap/db-config" {
			foundConfigMap = true
		}
		if rel.To == "test-cluster/prod//Secret/db-secrets" {
			foundSecret = true
		}
	}

	if !foundConfigMap {
		t.Error("missing Pod -> ConfigMap relation")
	}
	if !foundSecret {
		t.Error("missing Pod -> Secret relation")
	}
}

func TestBuildReferencesRelations_NoDuplicates(t *testing.T) {
	// Pod that references the same Secret multiple times should produce only one relation
	pod := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "app-xyz",
				"namespace": "prod",
				"uid":       "pod-uid-123",
			},
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "main",
						"image": "app:latest",
						"envFrom": []interface{}{
							map[string]interface{}{
								"secretRef": map[string]interface{}{
									"name": "shared-secrets",
								},
							},
						},
						"env": []interface{}{
							map[string]interface{}{
								"name": "SECRET_KEY",
								"valueFrom": map[string]interface{}{
									"secretKeyRef": map[string]interface{}{
										"name": "shared-secrets",
										"key":  "key",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	items := []unstructured.Unstructured{pod}
	relations := buildReferencesRelations(items, "test-cluster")

	// Should have only 1 relation (deduplicated)
	if len(relations) != 1 {
		t.Errorf("expected 1 relation (deduplicated), got %d", len(relations))
		t.Logf("relations: %+v", relations)
	}
}
