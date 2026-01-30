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
