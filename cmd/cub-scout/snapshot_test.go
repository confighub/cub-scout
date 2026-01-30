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
