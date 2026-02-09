// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"sort"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestForEachCanonicalWorkload_IncludesDeploymentAndStatefulSet(t *testing.T) {
	scheme := runtime.NewScheme()
	listKinds := map[schema.GroupVersionResource]string{
		{Group: "apps", Version: "v1", Resource: "deployments"}:  "DeploymentList",
		{Group: "apps", Version: "v1", Resource: "statefulsets"}: "StatefulSetList",
	}

	deployment := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "api",
				"namespace": "apps",
			},
		},
	}

	statefulSet := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "StatefulSet",
			"metadata": map[string]interface{}{
				"name":      "postgres",
				"namespace": "apps",
			},
		},
	}

	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, deployment, statefulSet)

	var got []string
	forEachCanonicalWorkload(context.Background(), client, "", func(workload *unstructured.Unstructured) {
		got = append(got, workload.GetKind()+"/"+workload.GetNamespace()+"/"+workload.GetName())
	})
	sort.Strings(got)

	want := []string{
		"Deployment/apps/api",
		"StatefulSet/apps/postgres",
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d workloads, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected workload at %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestForEachCanonicalWorkload_RespectsNamespaceFilter(t *testing.T) {
	scheme := runtime.NewScheme()
	listKinds := map[schema.GroupVersionResource]string{
		{Group: "apps", Version: "v1", Resource: "deployments"}:  "DeploymentList",
		{Group: "apps", Version: "v1", Resource: "statefulsets"}: "StatefulSetList",
	}

	deployment := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "api",
				"namespace": "apps",
			},
		},
	}
	statefulSet := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "StatefulSet",
			"metadata": map[string]interface{}{
				"name":      "postgres",
				"namespace": "databases",
			},
		},
	}

	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, deployment, statefulSet)

	var got []string
	forEachCanonicalWorkload(context.Background(), client, "apps", func(workload *unstructured.Unstructured) {
		got = append(got, workload.GetKind()+"/"+workload.GetNamespace()+"/"+workload.GetName())
	})

	if len(got) != 1 {
		t.Fatalf("expected 1 workload in namespace filter, got %d: %v", len(got), got)
	}
	if got[0] != "Deployment/apps/api" {
		t.Fatalf("unexpected workload from namespace filter: %s", got[0])
	}
}

func TestGetWorkloadReplicas_StatefulSetParity(t *testing.T) {
	tests := []struct {
		name   string
		obj    *unstructured.Unstructured
		wantD  int64
		wantR  int64
		wantOK bool
	}{
		{
			name:   "deployment ready",
			obj:    newWorkload("Deployment", "api", "apps", 3, 3),
			wantD:  3,
			wantR:  3,
			wantOK: true,
		},
		{
			name:   "statefulset ready",
			obj:    newWorkload("StatefulSet", "postgres", "apps", 2, 2),
			wantD:  2,
			wantR:  2,
			wantOK: true,
		},
		{
			name:   "statefulset not ready",
			obj:    newWorkload("StatefulSet", "postgres", "apps", 3, 1),
			wantD:  3,
			wantR:  1,
			wantOK: false,
		},
		{
			name: "deployment default replicas to one",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "defaulted",
						"namespace": "apps",
					},
					"status": map[string]interface{}{
						"readyReplicas": int64(1),
					},
				},
			},
			wantD:  1,
			wantR:  1,
			wantOK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desired, ready := getWorkloadReplicas(tt.obj)
			if desired != tt.wantD || ready != tt.wantR {
				t.Fatalf("getWorkloadReplicas() = (%d, %d), want (%d, %d)", desired, ready, tt.wantD, tt.wantR)
			}
			if got := isWorkloadReady(tt.obj); got != tt.wantOK {
				t.Fatalf("isWorkloadReady() = %v, want %v", got, tt.wantOK)
			}
		})
	}
}

func newWorkload(kind, name, namespace string, desired, ready int64) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       kind,
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"replicas": desired,
			},
			"status": map[string]interface{}{
				"readyReplicas": ready,
			},
		},
	}
}

func TestResolveArgoOwnershipLineage_AppOfAppsParent(t *testing.T) {
	scheme := runtime.NewScheme()
	listKinds := map[schema.GroupVersionResource]string{
		{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}: "ApplicationList",
	}

	childApp := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      "payments-prod",
				"namespace": "argocd",
				"labels": map[string]interface{}{
					"argocd.argoproj.io/instance": "root-app",
				},
			},
		},
	}
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, childApp)
	index := buildArgoLineageIndex(context.Background(), client)

	workload := &unstructured.Unstructured{}
	workload.SetLabels(map[string]string{"argocd.argoproj.io/instance": "payments-prod"})

	appName, appNamespace, lineage := resolveArgoOwnershipLineage(workload, index)
	if appName != "payments-prod" {
		t.Fatalf("expected app name payments-prod, got %q", appName)
	}
	if appNamespace != "argocd" {
		t.Fatalf("expected app namespace argocd, got %q", appNamespace)
	}
	if lineage != "application/root-app" {
		t.Fatalf("expected app-of-apps lineage, got %q", lineage)
	}
}

func TestResolveArgoOwnershipLineage_ApplicationSetParent(t *testing.T) {
	scheme := runtime.NewScheme()
	listKinds := map[schema.GroupVersionResource]string{
		{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}: "ApplicationList",
	}

	appFromSet := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      "checkout-prod",
				"namespace": "argocd",
			},
		},
	}
	appFromSet.SetOwnerReferences([]metav1.OwnerReference{
		{
			APIVersion: "argoproj.io/v1alpha1",
			Kind:       "ApplicationSet",
			Name:       "checkout-set",
			Controller: boolPtr(true),
		},
	})

	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, appFromSet)
	index := buildArgoLineageIndex(context.Background(), client)

	workload := &unstructured.Unstructured{}
	workload.SetLabels(map[string]string{"argocd.argoproj.io/instance": "checkout-prod"})

	appName, appNamespace, lineage := resolveArgoOwnershipLineage(workload, index)
	if appName != "checkout-prod" {
		t.Fatalf("expected app name checkout-prod, got %q", appName)
	}
	if appNamespace != "argocd" {
		t.Fatalf("expected app namespace argocd, got %q", appNamespace)
	}
	if lineage != "applicationset/checkout-set" {
		t.Fatalf("expected applicationset lineage, got %q", lineage)
	}
}

func TestResolveArgoOwnershipLineage_NoIndexMatchFallsBackToApplicationName(t *testing.T) {
	workload := &unstructured.Unstructured{}
	workload.SetLabels(map[string]string{"argocd.argoproj.io/instance": "billing-prod"})

	appName, appNamespace, lineage := resolveArgoOwnershipLineage(workload, newArgoLineageIndex())
	if appName != "billing-prod" {
		t.Fatalf("expected app name fallback, got %q", appName)
	}
	if appNamespace != "" {
		t.Fatalf("expected empty namespace fallback, got %q", appNamespace)
	}
	if lineage != "" {
		t.Fatalf("expected empty lineage fallback, got %q", lineage)
	}
}

func boolPtr(v bool) *bool {
	return &v
}
