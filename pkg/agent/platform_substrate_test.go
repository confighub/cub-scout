// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestBuildModelplaneCrossplaneEvidence_ExplicitSignals(t *testing.T) {
	obj := &unstructured.Unstructured{}
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
	obj.SetAnnotations(map[string]string{
		"crossplane.io/composition-resource-name": "annotation-loses-to-label",
	})
	obj.SetManagedFields([]metav1.ManagedFieldsEntry{
		{Manager: "apiextensions.crossplane.io/composed-z-later"},
		{Manager: "apiextensions.crossplane.io/composed-a-first"},
	})
	ownership := DetectOwnership(obj)

	evidence, ok := BuildModelplaneCrossplaneEvidence(obj, ownership)
	if !ok {
		t.Fatal("expected Modelplane-on-Crossplane evidence")
	}
	if evidence.Platform != OwnerModelplane || evidence.Substrate != OwnerCrossplane {
		t.Fatalf("platform/substrate = %s/%s, want modelplane/crossplane", evidence.Platform, evidence.Substrate)
	}
	if evidence.Composite != "x-qwen" {
		t.Fatalf("composite = %q, want x-qwen", evidence.Composite)
	}
	if evidence.Claim == nil || evidence.Claim.Name != "qwen-claim" || evidence.Claim.Namespace != "models" {
		t.Fatalf("claim = %+v, want models/qwen-claim", evidence.Claim)
	}
	if evidence.CompositionResource != "engine" {
		t.Fatalf("compositionResource = %q, want engine", evidence.CompositionResource)
	}
	if evidence.FieldManager != "apiextensions.crossplane.io/composed-a-first" {
		t.Fatalf("fieldManager = %q, want deterministic first manager", evidence.FieldManager)
	}
	for _, want := range []string{
		"label:crossplane.io/composite",
		"label:crossplane.io/claim-name",
		"label:crossplane.io/claim-namespace",
		"label:crossplane.io/composition-resource-name",
		"managedFields:apiextensions.crossplane.io/composed-a-first",
	} {
		if !hasPlatformSubstrateSource(evidence.Sources, want) {
			t.Fatalf("sources = %+v, missing %q", evidence.Sources, want)
		}
	}
	for _, want := range []string{
		"Modelplane-on-Crossplane evidence",
		"composite=x-qwen",
		"claim=models/qwen-claim",
		"compositionResource=engine",
		"fieldManager=apiextensions.crossplane.io/composed-a-first",
	} {
		if !strings.Contains(evidence.Summary(), want) {
			t.Fatalf("summary %q missing %q", evidence.Summary(), want)
		}
	}
}

func TestBuildModelplaneCrossplaneEvidence_RequiresModelplaneOwner(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetKind("Deployment")
	obj.SetName("x")
	obj.SetLabels(map[string]string{
		"crossplane.io/composite": "x-qwen",
	})

	if evidence, ok := BuildModelplaneCrossplaneEvidence(obj, DetectOwnership(obj)); ok {
		t.Fatalf("Crossplane-only resource produced Modelplane evidence: %+v", evidence)
	}
}

func TestBuildModelplaneCrossplaneEvidence_RequiresCrossplaneSignal(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion("modelplane.ai/v1alpha1")
	obj.SetKind("ModelDeployment")
	obj.SetName("qwen")

	if evidence, ok := BuildModelplaneCrossplaneEvidence(obj, DetectOwnership(obj)); ok {
		t.Fatalf("Modelplane resource without Crossplane substrate signals produced evidence: %+v", evidence)
	}
}

func hasPlatformSubstrateSource(sources []string, want string) bool {
	for _, source := range sources {
		if source == want {
			return true
		}
	}
	return false
}
