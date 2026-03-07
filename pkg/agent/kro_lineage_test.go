// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestResolveKroLineage(t *testing.T) {
	instance := &unstructured.Unstructured{}
	instance.SetAPIVersion("apps.kro.run/v1alpha1")
	instance.SetKind("WebApp")
	instance.SetNamespace("prod")
	instance.SetName("checkout")
	instance.SetOwnerReferences([]metav1.OwnerReference{
		{APIVersion: "kro.run/v1alpha1", Kind: "ResourceGraphDefinition", Name: "webapp-stack"},
	})

	definition := &unstructured.Unstructured{}
	definition.SetAPIVersion("kro.run/v1alpha1")
	definition.SetKind("ResourceGraphDefinition")
	definition.SetName("webapp-stack")

	managed := &unstructured.Unstructured{}
	managed.SetAPIVersion("apps/v1")
	managed.SetKind("Deployment")
	managed.SetNamespace("prod")
	managed.SetName("checkout-api")
	managed.SetOwnerReferences([]metav1.OwnerReference{
		{APIVersion: "apps.kro.run/v1alpha1", Kind: "WebApp", Name: "checkout"},
	})

	objs := []*unstructured.Unstructured{managed, instance, definition}

	lineage, ok := ResolveKroLineage(managed, objs)
	if !ok || lineage == nil {
		t.Fatalf("ResolveKroLineage() = (%v, %v), want non-nil true", lineage, ok)
	}

	if got := lineage.Managed.Ref.Name; got != "checkout-api" {
		t.Fatalf("Managed.Ref.Name = %q, want checkout-api", got)
	}
	if got := lineage.Instance.Ref.Name; got != "checkout" {
		t.Fatalf("Instance.Ref.Name = %q, want checkout", got)
	}
	if !lineage.Instance.Present {
		t.Fatalf("Instance.Present = false, want true")
	}
	if lineage.Definition == nil {
		t.Fatalf("Definition = nil, want non-nil")
	}
	if got := lineage.Definition.Ref.Name; got != "webapp-stack" {
		t.Fatalf("Definition.Ref.Name = %q, want webapp-stack", got)
	}
	if !lineage.Definition.Present {
		t.Fatalf("Definition.Present = false, want true")
	}

	foundOwnerRefEvidence := false
	for _, e := range lineage.Evidence {
		if strings.HasPrefix(e, "ownerRef:apps.kro.run/") {
			foundOwnerRefEvidence = true
			break
		}
	}
	if !foundOwnerRefEvidence {
		t.Fatalf("evidence missing ownerRef signal: %#v", lineage.Evidence)
	}
}

func TestResolveKroLineage_PartialWhenInstanceMissing(t *testing.T) {
	managed := &unstructured.Unstructured{}
	managed.SetAPIVersion("apps/v1")
	managed.SetKind("Deployment")
	managed.SetNamespace("prod")
	managed.SetName("checkout-api")
	managed.SetOwnerReferences([]metav1.OwnerReference{
		{APIVersion: "apps.kro.run/v1alpha1", Kind: "WebApp", Name: "checkout"},
	})

	lineage, ok := ResolveKroLineage(managed, []*unstructured.Unstructured{managed})
	if !ok || lineage == nil {
		t.Fatalf("ResolveKroLineage() = (%v, %v), want non-nil true", lineage, ok)
	}

	if got := lineage.Instance.Ref.Name; got != "checkout" {
		t.Fatalf("Instance.Ref.Name = %q, want checkout", got)
	}
	if lineage.Instance.Present {
		t.Fatalf("Instance.Present = true, want false")
	}
}

func TestResolveKroLineage_NonKro(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion("apps/v1")
	obj.SetKind("Deployment")
	obj.SetNamespace("prod")
	obj.SetName("api")

	lineage, ok := ResolveKroLineage(obj, []*unstructured.Unstructured{obj})
	if ok {
		t.Fatalf("ResolveKroLineage() ok = true, want false (lineage=%#v)", lineage)
	}
}
