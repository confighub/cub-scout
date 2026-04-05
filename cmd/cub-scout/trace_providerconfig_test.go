// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/confighub/cub-scout/pkg/agent"
)

func TestNormalizeKind_ProviderConfig(t *testing.T) {
	tests := map[string]string{
		"providerconfig":  "ProviderConfig",
		"providerconfigs": "ProviderConfig",
		"ProviderConfig":  "ProviderConfig",
	}

	for input, want := range tests {
		if got := normalizeKind(input); got != want {
			t.Fatalf("normalizeKind(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestProviderConfigLocatorsFromAPIResourceLists(t *testing.T) {
	locators := providerConfigLocatorsFromAPIResourceLists([]*metav1.APIResourceList{
		{
			GroupVersion: "aws.upbound.io/v1beta1",
			APIResources: []metav1.APIResource{
				{Name: "providerconfigs", Kind: "ProviderConfig", Namespaced: false},
				{Name: "providerconfigs/status", Kind: "ProviderConfig", Namespaced: false},
			},
		},
		{
			GroupVersion: "pkg.crossplane.io/v1",
			APIResources: []metav1.APIResource{
				{Name: "providers", Kind: "Provider", Namespaced: false},
			},
		},
		{
			GroupVersion: "gcp.upbound.io/v1beta1",
			APIResources: []metav1.APIResource{
				{Name: "providerconfigs", Kind: "ProviderConfig", Namespaced: true},
			},
		},
	})

	if len(locators) != 2 {
		t.Fatalf("providerConfigLocatorsFromAPIResourceLists() returned %d locators, want 2", len(locators))
	}
	if locators[0].GVR.Group != "aws.upbound.io" || locators[0].Namespaced {
		t.Fatalf("first locator = %#v, want cluster-scoped aws.upbound.io ProviderConfig", locators[0])
	}
	if locators[1].GVR.Group != "gcp.upbound.io" || !locators[1].Namespaced {
		t.Fatalf("second locator = %#v, want namespaced gcp.upbound.io ProviderConfig", locators[1])
	}
}

func TestFetchResourceWithLocators_ClusterScopedProviderConfig(t *testing.T) {
	scheme := runtime.NewScheme()
	gvr := schema.GroupVersionResource{Group: "aws.upbound.io", Version: "v1beta1", Resource: "providerconfigs"}
	client := dynamicfake.NewSimpleDynamicClient(scheme, &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "aws.upbound.io/v1beta1",
			"kind":       "ProviderConfig",
			"metadata": map[string]interface{}{
				"name": "aws-provider",
			},
		},
	})

	got, err := fetchResourceWithLocators(context.Background(), client, "ProviderConfig", "aws-provider", "crossplane-system", []traceResourceLocator{
		{GVR: gvr, Namespaced: false},
	})
	if err != nil {
		t.Fatalf("fetchResourceWithLocators() error = %v", err)
	}
	if got.GetKind() != "ProviderConfig" || got.GetName() != "aws-provider" {
		t.Fatalf("fetchResourceWithLocators() returned unexpected object: kind=%q name=%q", got.GetKind(), got.GetName())
	}
	if got.GetNamespace() != "" {
		t.Fatalf("fetched ProviderConfig namespace = %q, want cluster-scoped empty namespace", got.GetNamespace())
	}
}

func TestBuildCrossplaneObservedTraceResult_ProviderConfigUsesClusterScope(t *testing.T) {
	result := buildCrossplaneObservedTraceResult("ProviderConfig", "aws-provider", "crossplane-system", &agent.Ownership{
		Type: agent.OwnerCrossplane,
	})

	if result.Object.Namespace != "" {
		t.Fatalf("result.Object.Namespace = %q, want empty string for cluster-scoped ProviderConfig", result.Object.Namespace)
	}
	if len(result.Chain) != 1 || result.Chain[0].Kind != "ProviderConfig" {
		t.Fatalf("unexpected synthetic trace chain: %#v", result.Chain)
	}
	if got := normalizeToolToOwner(result.Tool); got != "Crossplane" {
		t.Fatalf("normalizeToolToOwner(%q) = %q, want %q", result.Tool, got, "Crossplane")
	}
}

func TestOutputTraceHuman_CrossplaneObservedTraceResultShowsSecretEvidence(t *testing.T) {
	result := buildCrossplaneObservedTraceResult("ProviderConfig", "aws-provider", "crossplane-system", &agent.Ownership{
		Type: agent.OwnerCrossplane,
	})
	result.Secrets = &agent.SecretEvidenceResult{
		Resource: agent.ResourceRef{Kind: "ProviderConfig", Name: "aws-provider"},
		Secrets: []agent.SecretEvidence{
			{
				Name:       "provider-creds",
				Namespace:  "default",
				RefType:    agent.SecretRefTypeCrossplaneCredRef,
				Status:     agent.SecretStatusPresent,
				SecretType: "Opaque",
			},
		},
		Summary: agent.SecretEvidenceSummary{
			Total:   1,
			Present: 1,
		},
	}

	out := captureStdout(t, func() {
		if err := outputTraceHuman(result, nil); err != nil {
			t.Fatalf("outputTraceHuman() error = %v", err)
		}
	})

	if !strings.Contains(out, "Crossplane resource detected") {
		t.Fatalf("expected Crossplane degraded warning, got:\n%s", out)
	}
	if !strings.Contains(out, "Secret evidence") || !strings.Contains(out, "provider-creds") {
		t.Fatalf("expected secret evidence in output, got:\n%s", out)
	}
}
