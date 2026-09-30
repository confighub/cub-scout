// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func makeResourceWithFieldsV1(entries ...metav1.ManagedFieldsEntry) *unstructured.Unstructured {
	u := &unstructured.Unstructured{Object: map[string]interface{}{}}
	u.SetManagedFields(entries)
	return u
}

func fieldsV1(rawJSON string) *metav1.FieldsV1 {
	return &metav1.FieldsV1{Raw: []byte(rawJSON)}
}

func TestAttributeFieldsByManagedFields_PerPathClassification(t *testing.T) {
	// Argo owns .spec.template.spec.containers; kubectl-edit owns .spec.replicas.
	// Per-path classification should split these.
	argoEntry := metav1.ManagedFieldsEntry{
		Manager:    ManagerArgoCD,
		Operation:  metav1.ManagedFieldsOperationApply,
		FieldsType: "FieldsV1",
		FieldsV1: fieldsV1(`{
			"f:spec":{
				"f:template":{
					"f:spec":{
						"f:containers":{}
					}
				}
			}
		}`),
	}
	editEntry := metav1.ManagedFieldsEntry{
		Manager:    ManagerKubectlEdit,
		Operation:  metav1.ManagedFieldsOperationUpdate,
		FieldsType: "FieldsV1",
		FieldsV1:   fieldsV1(`{"f:spec":{"f:replicas":{}}}`),
	}
	resource := makeResourceWithFieldsV1(argoEntry, editEntry)
	owner := Ownership{Type: OwnerArgo, SubType: "application"}

	_, byPath := AttributeFieldsByManagedFields(resource, owner)
	if len(byPath) == 0 {
		t.Fatalf("expected per-path map to be populated; got empty")
	}

	// .spec.replicas should be classified as manual-edit (kubectl-edit owns it).
	replicasAttr, ok := byPath[".spec.replicas"]
	if !ok {
		t.Errorf("missing .spec.replicas in byPath; keys = %v", keys(byPath))
	} else {
		if replicasAttr.Cause != CauseManualEdit {
			t.Errorf(".spec.replicas Cause = %q, want %q", replicasAttr.Cause, CauseManualEdit)
		}
		if replicasAttr.ManagerHint != ManagerKubectlEdit {
			t.Errorf(".spec.replicas ManagerHint = %q, want %q", replicasAttr.ManagerHint, ManagerKubectlEdit)
		}
	}

	// .spec.template.spec.containers should be classified as controller-drift (Argo owns it).
	containersAttr, ok := byPath[".spec.template.spec.containers"]
	if !ok {
		t.Errorf("missing .spec.template.spec.containers in byPath; keys = %v", keys(byPath))
	} else {
		if containersAttr.Cause != CauseControllerDrift {
			t.Errorf(".spec.template.spec.containers Cause = %q, want %q", containersAttr.Cause, CauseControllerDrift)
		}
		if containersAttr.ManagerHint != ManagerArgoCD {
			t.Errorf(".spec.template.spec.containers ManagerHint = %q, want %q", containersAttr.ManagerHint, ManagerArgoCD)
		}
	}
}

func TestAttributeFieldsByManagedFields_MixedOnSamePath(t *testing.T) {
	// Two managers both touch .spec.replicas — Argo via Apply, kubectl-edit via
	// Update. Per the algorithm, the mixed case resolves to manual-edit because
	// human involvement is the cause-changing signal.
	argoEntry := metav1.ManagedFieldsEntry{
		Manager:    ManagerArgoCD,
		Operation:  metav1.ManagedFieldsOperationApply,
		FieldsType: "FieldsV1",
		FieldsV1:   fieldsV1(`{"f:spec":{"f:replicas":{}}}`),
	}
	editEntry := metav1.ManagedFieldsEntry{
		Manager:    ManagerKubectlEdit,
		Operation:  metav1.ManagedFieldsOperationUpdate,
		FieldsType: "FieldsV1",
		FieldsV1:   fieldsV1(`{"f:spec":{"f:replicas":{}}}`),
	}
	resource := makeResourceWithFieldsV1(argoEntry, editEntry)
	owner := Ownership{Type: OwnerArgo, SubType: "application"}

	_, byPath := AttributeFieldsByManagedFields(resource, owner)
	replicasAttr, ok := byPath[".spec.replicas"]
	if !ok {
		t.Fatalf("missing .spec.replicas; keys = %v", keys(byPath))
	}
	if replicasAttr.Cause != CauseManualEdit {
		t.Errorf("Cause = %q, want %q (mixed must resolve to manual-edit)", replicasAttr.Cause, CauseManualEdit)
	}
	if replicasAttr.ManagerHint != ManagerKubectlEdit {
		t.Errorf("ManagerHint = %q, want %q", replicasAttr.ManagerHint, ManagerKubectlEdit)
	}
}

func TestAttributeFieldsByManagedFields_ResourceLevelAlsoReturned(t *testing.T) {
	resource := makeResource(ManagerArgoCD)
	owner := Ownership{Type: OwnerArgo, SubType: "application"}

	resourceLevel, _ := AttributeFieldsByManagedFields(resource, owner)
	if resourceLevel.Cause != CauseControllerDrift {
		t.Errorf("resourceLevel.Cause = %q, want %q", resourceLevel.Cause, CauseControllerDrift)
	}
	if resourceLevel.ManagerHint != ManagerArgoCD {
		t.Errorf("resourceLevel.ManagerHint = %q, want %q", resourceLevel.ManagerHint, ManagerArgoCD)
	}
}

func TestAttributeFieldsByManagedFields_NoFieldsV1ReturnsEmptyMap(t *testing.T) {
	// makeResource builds entries with no FieldsV1 — per-path map must be nil
	// or empty, while the resource-level rollup still works.
	resource := makeResource(ManagerArgoCD, ManagerKubectlEdit)
	owner := Ownership{Type: OwnerArgo, SubType: "application"}

	resourceLevel, byPath := AttributeFieldsByManagedFields(resource, owner)
	if resourceLevel.Cause != CauseManualEdit {
		t.Errorf("resourceLevel.Cause = %q, want %q", resourceLevel.Cause, CauseManualEdit)
	}
	if len(byPath) != 0 {
		t.Errorf("byPath = %v, want nil/empty when FieldsV1 missing", byPath)
	}
}

func TestAttributeFieldsByManagedFields_NilAndEmpty(t *testing.T) {
	t.Run("nil resource", func(t *testing.T) {
		resourceLevel, byPath := AttributeFieldsByManagedFields(nil, Ownership{Type: OwnerArgo})
		if resourceLevel.Cause != CauseUnknown {
			t.Errorf("resourceLevel.Cause = %q, want %q", resourceLevel.Cause, CauseUnknown)
		}
		if byPath != nil {
			t.Errorf("byPath = %v, want nil", byPath)
		}
	})

	t.Run("no managedFields", func(t *testing.T) {
		u := &unstructured.Unstructured{Object: map[string]interface{}{}}
		resourceLevel, byPath := AttributeFieldsByManagedFields(u, Ownership{Type: OwnerArgo})
		if resourceLevel.Cause != CauseUnknown {
			t.Errorf("resourceLevel.Cause = %q, want %q", resourceLevel.Cause, CauseUnknown)
		}
		if byPath != nil {
			t.Errorf("byPath = %v, want nil", byPath)
		}
	})

}

func TestAttributeFieldsByManagedFields_UnknownManagerSkipped(t *testing.T) {
	// Unknown manager strings should not produce per-path entries — parse don't guess.
	unknown := metav1.ManagedFieldsEntry{
		Manager:    "some-unknown-tool",
		Operation:  metav1.ManagedFieldsOperationApply,
		FieldsType: "FieldsV1",
		FieldsV1:   fieldsV1(`{"f:spec":{"f:replicas":{}}}`),
	}
	resource := makeResourceWithFieldsV1(unknown)
	_, byPath := AttributeFieldsByManagedFields(resource, Ownership{Type: OwnerArgo, SubType: "application"})
	if len(byPath) != 0 {
		t.Errorf("byPath = %v, want empty for unknown manager", byPath)
	}
}

func TestAttributeFieldPathRetainsSortedManagersAndUnknownCoClaimant(t *testing.T) {
	resource := makeResourceWithFieldsV1(
		metav1.ManagedFieldsEntry{Manager: ManagerKubectlSet, FieldsType: "FieldsV1", FieldsV1: fieldsV1(`{"f:spec":{"f:image":{}}}`)},
		metav1.ManagedFieldsEntry{Manager: ManagerKubectlEdit, FieldsType: "FieldsV1", FieldsV1: fieldsV1(`{"f:spec":{"f:image":{}}}`)},
		metav1.ManagedFieldsEntry{Manager: "shell-writer", FieldsType: "FieldsV1", FieldsV1: fieldsV1(`{"f:spec":{"f:image":{}}}`)},
		metav1.ManagedFieldsEntry{Manager: ManagerKubectlEdit, FieldsType: "FieldsV1", FieldsV1: fieldsV1(`{"f:metadata":{"f:labels":{"f:app":{}}}}`)},
	)
	attr, ok, incomplete := AttributeFieldPath(resource, Ownership{Type: OwnerUnknown}, ".spec.image")
	if !ok {
		t.Fatal("expected exact path attribution")
	}
	if incomplete {
		t.Fatal("valid entries should be complete")
	}
	if attr.Cause != CauseUnknown {
		t.Fatalf("Cause = %q, want unknown when an unclassified manager co-claims the path", attr.Cause)
	}
	if !reflect.DeepEqual(attr.Managers, []string{"kubectl-edit", "kubectl-set", "shell-writer"}) {
		t.Fatalf("Managers = %v, want sorted exact-path managers", attr.Managers)
	}
	if _, ok, _ := AttributeFieldPath(resource, Ownership{Type: OwnerUnknown}, ".spec.replicas"); ok {
		t.Fatal("unrelated resource-level or label managers must not fall back into a missing path")
	}
}

func TestAttributeFieldPathDoesNotMergeSiblingListItemManagers(t *testing.T) {
	resource := makeResourceWithFieldsV1(
		metav1.ManagedFieldsEntry{Manager: ManagerKubectlSet, FieldsType: "FieldsV1", FieldsV1: fieldsV1(`{"f:spec":{"f:template":{"f:spec":{"f:containers":{"k:{\"name\":\"checkout\"}":{"f:image":{}}}}}}}`)},
		metav1.ManagedFieldsEntry{Manager: ManagerKubectlEdit, FieldsType: "FieldsV1", FieldsV1: fieldsV1(`{"f:spec":{"f:template":{"f:spec":{"f:containers":{"k:{\"name\":\"api\"}":{"f:image":{}}}}}}}`)},
	)
	attr, ok, incomplete := AttributeFieldPath(resource, Ownership{Type: OwnerUnknown}, `.spec.template.spec.containers[name="checkout"].image`)
	if !ok || incomplete || attr.Cause != CauseManualEdit || !reflect.DeepEqual(attr.Managers, []string{ManagerKubectlSet}) {
		t.Fatalf("checkout path attribution = (%+v, %v, %v), sibling manager must not bleed in", attr, ok, incomplete)
	}
}

func TestAttributeFieldPathDoesNotConfuseLabelAndImageOwners(t *testing.T) {
	resource := makeResourceWithFieldsV1(
		metav1.ManagedFieldsEntry{Manager: ManagerKubectlEdit, FieldsType: "FieldsV1", FieldsV1: fieldsV1(`{"f:metadata":{"f:labels":{"f:app":{}}}}`)},
		metav1.ManagedFieldsEntry{Manager: ManagerFluxKustomize, FieldsType: "FieldsV1", FieldsV1: fieldsV1(`{"f:spec":{"f:template":{"f:spec":{"f:containers":{"k:{\"name\":\"checkout\"}":{"f:image":{}}}}}}}`)},
	)
	attr, ok, incomplete := AttributeFieldPath(resource, Ownership{Type: OwnerFlux}, `.spec.template.spec.containers[name="checkout"].image`)
	if !ok || incomplete || attr.Cause != CauseControllerDrift || !reflect.DeepEqual(attr.Managers, []string{ManagerFluxKustomize}) {
		t.Fatalf("image attribution = (%+v, %v), want controller-only", attr, ok)
	}
}

func TestAttributeFieldPathSupportsUnknownCRDScalarFieldNames(t *testing.T) {
	resource := makeResourceWithFieldsV1(
		metav1.ManagedFieldsEntry{Manager: ManagerKubectlSet, FieldsType: "FieldsV1", FieldsV1: fieldsV1(`{"f:spec":{"f:environment":{}}}`)},
	)
	path := ".spec.environment"
	if err := ValidateCanonicalFieldPath(path); err != nil {
		t.Fatalf("valid CRD field path rejected: %v", err)
	}
	attr, ok, incomplete := AttributeFieldPath(resource, Ownership{Type: OwnerUnknown}, path)
	if !ok || incomplete || attr.Cause != CauseManualEdit || !reflect.DeepEqual(attr.Managers, []string{ManagerKubectlSet}) {
		t.Fatalf("CRD scalar path attribution = (%+v, %v, %v), want exact manager evidence", attr, ok, incomplete)
	}
}

func TestValidateCanonicalFieldPathAllowsWildcardInsideQuotedSelector(t *testing.T) {
	path := `.spec.template.spec.containers[name="checkout*"].image`
	if err := ValidateCanonicalFieldPath(path); err != nil {
		t.Fatalf("literal wildcard in quoted map key rejected: %v", err)
	}
}

func TestAttributeFieldPathUnknownForMalformedMissingAndWildcard(t *testing.T) {
	owner := Ownership{Type: OwnerFlux}
	tests := []struct {
		name       string
		resource   *unstructured.Unstructured
		path       string
		invalid    bool
		incomplete bool
	}{
		{name: "missing FieldsV1", resource: makeResourceWithFieldsV1(metav1.ManagedFieldsEntry{Manager: ManagerKubectlEdit}), path: ".spec.image", incomplete: true},
		{name: "path absent", resource: makeResourceWithFieldsV1(metav1.ManagedFieldsEntry{Manager: ManagerKubectlEdit, FieldsType: "FieldsV1", FieldsV1: fieldsV1(`{"f:metadata":{"f:labels":{"f:app":{}}}}`)}), path: ".spec.image"},
		{name: "wildcard", resource: makeResourceWithFieldsV1(), path: ".spec.template.spec.containers[*].image", invalid: true},
		{name: "unkeyed list path", resource: makeResourceWithFieldsV1(), path: ".spec.template.spec.containers.image"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			attr, ok, incomplete := AttributeFieldPath(tc.resource, owner, tc.path)
			if ok || attr.Cause != CauseUnknown {
				t.Fatalf("AttributeFieldPath = (%+v, %v), want unknown/unresolved", attr, ok)
			}
			if incomplete != tc.incomplete {
				t.Fatalf("incomplete = %v, want %v", incomplete, tc.incomplete)
			}
			if err := ValidateCanonicalFieldPath(tc.path); (err != nil) != tc.invalid {
				t.Fatalf("ValidateCanonicalFieldPath error = %v, invalid=%v", err, tc.invalid)
			}
		})
	}
}

func TestAttributeFieldPathMalformedEntryMakesObservedPathIncomplete(t *testing.T) {
	entries := []metav1.ManagedFieldsEntry{
		metav1.ManagedFieldsEntry{Manager: ManagerKubectlSet, FieldsType: "FieldsV1", FieldsV1: fieldsV1(`{"f:spec":{"f:image":{}}}`)},
		metav1.ManagedFieldsEntry{Manager: "maybe-another-owner", FieldsType: "FieldsV1", FieldsV1: fieldsV1(`{bad`)},
	}
	byPath, incomplete := fieldAttributionsByEntries(entries, Ownership{Type: OwnerUnknown}, true)
	attr, ok := byPath[".spec.image"]
	if !ok || !incomplete || attr.Cause != CauseManualEdit || !reflect.DeepEqual(attr.Managers, []string{ManagerKubectlSet}) {
		t.Fatalf("partial attribution = (%+v, %v, %v), want observed manager with incomplete evidence", attr, ok, incomplete)
	}
}

func TestAttributeFieldPathRetainsUnrecognizedOnlyManagerAsUnknown(t *testing.T) {
	entries := []metav1.ManagedFieldsEntry{
		{Manager: "vendor-writer", FieldsType: "FieldsV1", FieldsV1: fieldsV1(`{"f:spec":{"f:image":{}}}`)},
	}
	byPath, incomplete := fieldAttributionsByEntries(entries, Ownership{Type: OwnerFlux}, true)
	attr, ok := byPath[".spec.image"]
	if !ok || incomplete || attr.Cause != CauseUnknown || !reflect.DeepEqual(attr.Managers, []string{"vendor-writer"}) {
		t.Fatalf("unrecognized manager evidence = (%+v, %v), want unknown and retained manager", attr, incomplete)
	}
}

func TestAttributeFieldPathUnknownWhenControllerAndUnrecognizedManagerShare(t *testing.T) {
	resource := makeResourceWithFieldsV1(
		metav1.ManagedFieldsEntry{Manager: ManagerFluxKustomize, FieldsType: "FieldsV1", FieldsV1: fieldsV1(`{"f:spec":{"f:image":{}}}`)},
		metav1.ManagedFieldsEntry{Manager: "vendor-writer", FieldsType: "FieldsV1", FieldsV1: fieldsV1(`{"f:spec":{"f:image":{}}}`)},
	)
	attr, ok, incomplete := AttributeFieldPath(resource, Ownership{Type: OwnerFlux}, ".spec.image")
	if !ok || incomplete || attr.Cause != CauseUnknown || !reflect.DeepEqual(attr.Managers, []string{ManagerFluxKustomize, "vendor-writer"}) {
		t.Fatalf("controller plus unrecognized evidence = (%+v, %v, %v), want unknown with all managers", attr, ok, incomplete)
	}
}

func TestAttributeFieldPathSharedControllerAndManualManagersRemainUnknown(t *testing.T) {
	entries := []metav1.ManagedFieldsEntry{
		{Manager: ManagerFluxKustomize, FieldsType: "FieldsV1", FieldsV1: fieldsV1(`{"f:spec":{"f:image":{}}}`)},
		{Manager: ManagerKubectlSet, FieldsType: "FieldsV1", FieldsV1: fieldsV1(`{"f:spec":{"f:image":{}}}`)},
	}
	byPath, incomplete := fieldAttributionsByEntries(entries, Ownership{Type: OwnerFlux}, true)
	attr, ok := byPath[".spec.image"]
	if !ok || incomplete || attr.Cause != CauseUnknown || !reflect.DeepEqual(attr.Managers, []string{ManagerKubectlSet, ManagerFluxKustomize}) {
		t.Fatalf("shared manager evidence = (%+v, %v), want unknown with all managers", attr, incomplete)
	}
	legacyByPath, _ := fieldAttributionsByEntries(entries, Ownership{Type: OwnerFlux}, false)
	if legacyByPath[".spec.image"].Cause != CauseManualEdit {
		t.Fatalf("existing per-path rollup changed: %+v", legacyByPath[".spec.image"])
	}
}

func keys(m map[string]FieldMutationAttribution) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
