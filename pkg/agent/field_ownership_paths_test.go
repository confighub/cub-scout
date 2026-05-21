// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
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

func keys(m map[string]FieldMutationAttribution) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
