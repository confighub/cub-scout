// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestProcessResourceWithLookup_AttachesModelplaneCrossplaneOwnerEvidence(t *testing.T) {
	obj := makeModelplaneCrossplaneWorkload()
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	entries := processResourceWithLookup(obj, gvr, "cluster-a", nil, map[string]int{}, mapApplicationSetLookup{
		byNamespacedName: map[string]int{},
		byName:           map[string]int{},
	})

	if len(entries) != 1 {
		t.Fatalf("entries len = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entry.Owner != "Modelplane" {
		t.Fatalf("owner = %q, want Modelplane", entry.Owner)
	}
	if entry.OwnerEvidence == nil {
		t.Fatalf("ownerEvidence missing: %+v", entry)
	}
	if entry.OwnerEvidence.Platform != agent.OwnerModelplane || entry.OwnerEvidence.Substrate != agent.OwnerCrossplane {
		t.Fatalf("ownerEvidence = %+v, want Modelplane/Crossplane", entry.OwnerEvidence)
	}
	if entry.OwnerEvidence.Composite != "x-qwen" {
		t.Fatalf("composite = %q, want x-qwen", entry.OwnerEvidence.Composite)
	}
}

func makeModelplaneCrossplaneWorkload() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion("apps/v1")
	obj.SetKind("Deployment")
	obj.SetNamespace("models")
	obj.SetName("qwen-engine")
	obj.SetLabels(map[string]string{
		"modelplane.ai/deployment":                "qwen",
		"crossplane.io/composite":                 "x-qwen",
		"crossplane.io/claim-name":                "qwen-claim",
		"crossplane.io/claim-namespace":           "models",
		"crossplane.io/composition-resource-name": "engine",
	})
	obj.SetManagedFields([]metav1.ManagedFieldsEntry{
		{Manager: "apiextensions.crossplane.io/composed-qwen"},
	})
	return obj
}
