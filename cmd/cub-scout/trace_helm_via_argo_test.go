// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestTryHelmViaArgoFallback_UsesTrackedApplicationAndAnnotatesTrace(t *testing.T) {
	ctx := context.Background()

	resource := newHelmManagedStatefulSet(
		"redis",
		"myapp-prod",
		"redis-18.6.1",
		"redis",
		"myapp-prod:apps/StatefulSet:myapp-prod/redis",
	)
	app := newArgoApplication(
		"myapp-prod",
		"argocd",
		"myapp-prod",
		[]map[string]string{
			{
				"kind":      "StatefulSet",
				"name":      "redis",
				"namespace": "myapp-prod",
			},
		},
	)

	dynClient := newHelmViaArgoTestDynamicClient(resource, app)
	helmResult := &agent.TraceResult{
		Object: agent.ResourceRef{
			Kind:      "Release",
			Name:      "redis",
			Namespace: "myapp-prod",
		},
		Tool:  "helm",
		Error: "Helm release 'redis' not found in namespace 'myapp-prod'",
	}
	ownership := &agent.Ownership{
		Type: agent.OwnerHelm,
		Name: "redis",
	}

	var gotAppName, gotAppNamespace string
	fallbackResult, fallbackOwnership, ok := tryHelmViaArgoFallback(
		ctx,
		dynClient,
		"StatefulSet",
		"redis",
		"myapp-prod",
		ownership,
		helmResult,
		func(_ context.Context, appName, appNamespace string) (*agent.TraceResult, error) {
			gotAppName = appName
			gotAppNamespace = appNamespace
			return &agent.TraceResult{
				Object: agent.ResourceRef{
					Kind:      "Application",
					Name:      appName,
					Namespace: appNamespace,
				},
				Tool:         "argocd",
				FullyManaged: true,
				Chain: []agent.ChainLink{
					{Kind: "Source", Name: "acme/myapp", Ready: true},
					{Kind: "Application", Name: appName, Namespace: appNamespace, Ready: true},
					{Kind: "StatefulSet", Name: "redis", Namespace: "myapp-prod", Ready: true},
				},
			}, nil
		},
	)
	if !ok {
		t.Fatalf("expected Helm-via-Argo fallback to be used")
	}
	if gotAppName != "myapp-prod" || gotAppNamespace != "argocd" {
		t.Fatalf("unexpected app traced: %s/%s", gotAppNamespace, gotAppName)
	}

	if fallbackOwnership == nil || fallbackOwnership.Type != agent.OwnerArgo {
		t.Fatalf("expected fallback ownership to be Argo, got %#v", fallbackOwnership)
	}
	if fallbackOwnership.Name != "myapp-prod" || fallbackOwnership.Namespace != "argocd" {
		t.Fatalf("unexpected fallback ownership app: %#v", fallbackOwnership)
	}

	if fallbackResult.Object.Kind != "StatefulSet" || fallbackResult.Object.Name != "redis" || fallbackResult.Object.Namespace != "myapp-prod" {
		t.Fatalf("fallback trace object should be original workload, got %#v", fallbackResult.Object)
	}
	if !strings.Contains(fallbackResult.Error, "Helm-via-ArgoCD") {
		t.Fatalf("expected explanatory Helm-via-Argo warning, got %q", fallbackResult.Error)
	}

	idxApp := traceChainIndex(fallbackResult.Chain, "Application", "myapp-prod", "argocd")
	idxChart := traceChainIndexByKind(fallbackResult.Chain, "HelmChart")
	idxWorkload := traceChainIndex(fallbackResult.Chain, "StatefulSet", "redis", "myapp-prod")

	if idxApp < 0 || idxChart < 0 || idxWorkload < 0 {
		t.Fatalf("expected Application, HelmChart, and workload links in chain: %#v", fallbackResult.Chain)
	}
	if idxChart != idxApp+1 {
		t.Fatalf("expected HelmChart directly after Application (app=%d, chart=%d)", idxApp, idxChart)
	}
	if idxWorkload <= idxChart {
		t.Fatalf("expected workload link after HelmChart (chart=%d, workload=%d)", idxChart, idxWorkload)
	}
	if fallbackResult.Chain[idxChart].Name != "redis" {
		t.Fatalf("expected HelmChart name redis, got %q", fallbackResult.Chain[idxChart].Name)
	}
	if fallbackResult.Chain[idxChart].Revision != "18.6.1" {
		t.Fatalf("expected HelmChart revision 18.6.1, got %q", fallbackResult.Chain[idxChart].Revision)
	}
	if !strings.Contains(strings.ToLower(fallbackResult.Chain[idxChart].Status), "template") {
		t.Fatalf("expected HelmChart status to mention template mode, got %q", fallbackResult.Chain[idxChart].Status)
	}
}

func TestTryHelmViaArgoFallback_DoesNotGuessWithAmbiguousApplications(t *testing.T) {
	ctx := context.Background()

	resource := newHelmManagedStatefulSet(
		"redis",
		"myapp-prod",
		"redis-18.6.1",
		"redis",
		"",
	)
	appA := newArgoApplication(
		"app-a",
		"argocd",
		"myapp-prod",
		[]map[string]string{
			{
				"kind":      "StatefulSet",
				"name":      "redis",
				"namespace": "myapp-prod",
			},
		},
	)
	appB := newArgoApplication(
		"app-b",
		"argocd",
		"myapp-prod",
		[]map[string]string{
			{
				"kind":      "StatefulSet",
				"name":      "redis",
				"namespace": "myapp-prod",
			},
		},
	)

	dynClient := newHelmViaArgoTestDynamicClient(resource, appA, appB)
	helmResult := &agent.TraceResult{
		Object: agent.ResourceRef{
			Kind:      "Release",
			Name:      "redis",
			Namespace: "myapp-prod",
		},
		Tool:  "helm",
		Error: "Helm release 'redis' not found in namespace 'myapp-prod'",
	}
	ownership := &agent.Ownership{Type: agent.OwnerHelm, Name: "redis"}

	traceCalled := false
	_, _, ok := tryHelmViaArgoFallback(
		ctx,
		dynClient,
		"StatefulSet",
		"redis",
		"myapp-prod",
		ownership,
		helmResult,
		func(_ context.Context, _, _ string) (*agent.TraceResult, error) {
			traceCalled = true
			return nil, nil
		},
	)
	if ok {
		t.Fatalf("expected fallback to skip ambiguous app matches")
	}
	if traceCalled {
		t.Fatalf("expected no Argo trace call when app ownership is ambiguous")
	}
}

func newHelmManagedStatefulSet(name, namespace, chartLabel, releaseName, trackingID string) *unstructured.Unstructured {
	labels := map[string]interface{}{
		"app.kubernetes.io/managed-by": "Helm",
		"app.kubernetes.io/instance":   releaseName,
	}
	if chartLabel != "" {
		labels["helm.sh/chart"] = chartLabel
	}

	annotations := map[string]interface{}{}
	if trackingID != "" {
		annotations["argocd.argoproj.io/tracking-id"] = trackingID
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "StatefulSet",
			"metadata": map[string]interface{}{
				"name":        name,
				"namespace":   namespace,
				"labels":      labels,
				"annotations": annotations,
			},
		},
	}
}

func newArgoApplication(name, namespace, destinationNamespace string, resources []map[string]string) *unstructured.Unstructured {
	statusResources := make([]interface{}, 0, len(resources))
	for _, res := range resources {
		statusResources = append(statusResources, map[string]interface{}{
			"kind":      res["kind"],
			"name":      res["name"],
			"namespace": res["namespace"],
		})
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"destination": map[string]interface{}{
					"namespace": destinationNamespace,
				},
			},
			"status": map[string]interface{}{
				"resources": statusResources,
			},
		},
	}
}

func newHelmViaArgoTestDynamicClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	listKinds := map[schema.GroupVersionResource]string{
		{Group: "apps", Version: "v1", Resource: "statefulsets"}:              "StatefulSetList",
		{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}: "ApplicationList",
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, objects...)
}

func traceChainIndex(chain []agent.ChainLink, kind, name, namespace string) int {
	for i, link := range chain {
		if link.Kind == kind && link.Name == name && link.Namespace == namespace {
			return i
		}
	}
	return -1
}

func traceChainIndexByKind(chain []agent.ChainLink, kind string) int {
	for i, link := range chain {
		if link.Kind == kind {
			return i
		}
	}
	return -1
}
