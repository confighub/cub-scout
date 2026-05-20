// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func makeResource(managers ...string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{Object: map[string]interface{}{}}
	entries := make([]metav1.ManagedFieldsEntry, 0, len(managers))
	for _, m := range managers {
		entries = append(entries, metav1.ManagedFieldsEntry{Manager: m, Operation: metav1.ManagedFieldsOperationApply})
	}
	u.SetManagedFields(entries)
	return u
}

func TestAttributeFieldMutation_Argo(t *testing.T) {
	owner := Ownership{Type: OwnerArgo, SubType: "application"}

	t.Run("argocd-controller only", func(t *testing.T) {
		got := AttributeFieldMutation(makeResource(ManagerArgoCD), owner)
		if got.Cause != CauseControllerDrift {
			t.Errorf("Cause = %q, want %q", got.Cause, CauseControllerDrift)
		}
		if got.ManagerHint != ManagerArgoCD {
			t.Errorf("ManagerHint = %q, want %q", got.ManagerHint, ManagerArgoCD)
		}
	})

	t.Run("CSA migration default counts as controller", func(t *testing.T) {
		got := AttributeFieldMutation(makeResource(ManagerKubectlCSA), owner)
		if got.Cause != CauseControllerDrift {
			t.Errorf("Cause = %q, want %q (Argo CSA migration must be controller-drift)", got.Cause, CauseControllerDrift)
		}
		if got.ManagerHint != ManagerKubectlCSA {
			t.Errorf("ManagerHint = %q, want %q", got.ManagerHint, ManagerKubectlCSA)
		}
	})

	t.Run("controller + kubectl edit is manual-edit with kubectl hint", func(t *testing.T) {
		got := AttributeFieldMutation(makeResource(ManagerArgoCD, ManagerKubectlEdit), owner)
		if got.Cause != CauseManualEdit {
			t.Errorf("Cause = %q, want %q", got.Cause, CauseManualEdit)
		}
		if got.ManagerHint != ManagerKubectlEdit {
			t.Errorf("ManagerHint = %q, want %q (mixed case must surface interactive)", got.ManagerHint, ManagerKubectlEdit)
		}
		wantManagers := []string{ManagerArgoCD, ManagerKubectlEdit}
		if !reflect.DeepEqual(got.Managers, wantManagers) {
			t.Errorf("Managers = %v, want %v", got.Managers, wantManagers)
		}
	})
}

func TestAttributeFieldMutation_Native(t *testing.T) {
	owner := Ownership{Type: OwnerKubernetes}

	t.Run("CSA on native is manual-edit", func(t *testing.T) {
		got := AttributeFieldMutation(makeResource(ManagerKubectlCSA), owner)
		if got.Cause != CauseManualEdit {
			t.Errorf("Cause = %q, want %q (CSA on non-Argo must be manual)", got.Cause, CauseManualEdit)
		}
		if got.ManagerHint != ManagerKubectlCSA {
			t.Errorf("ManagerHint = %q, want %q", got.ManagerHint, ManagerKubectlCSA)
		}
	})

	t.Run("kubectl-edit is manual-edit", func(t *testing.T) {
		got := AttributeFieldMutation(makeResource(ManagerKubectlEdit), owner)
		if got.Cause != CauseManualEdit {
			t.Errorf("Cause = %q, want %q", got.Cause, CauseManualEdit)
		}
	})

	t.Run("bare kubectl SSA is manual-edit", func(t *testing.T) {
		got := AttributeFieldMutation(makeResource(ManagerKubectlSSA), owner)
		if got.Cause != CauseManualEdit {
			t.Errorf("Cause = %q, want %q", got.Cause, CauseManualEdit)
		}
	})
}

func TestAttributeFieldMutation_Flux(t *testing.T) {
	t.Run("kustomization with kustomize-controller", func(t *testing.T) {
		owner := Ownership{Type: OwnerFlux, SubType: "kustomization"}
		got := AttributeFieldMutation(makeResource(ManagerFluxKustomize), owner)
		if got.Cause != CauseControllerDrift {
			t.Errorf("Cause = %q, want %q", got.Cause, CauseControllerDrift)
		}
	})

	t.Run("helmrelease with helm-controller", func(t *testing.T) {
		owner := Ownership{Type: OwnerFlux, SubType: "helmrelease"}
		got := AttributeFieldMutation(makeResource(ManagerFluxHelm), owner)
		if got.Cause != CauseControllerDrift {
			t.Errorf("Cause = %q, want %q", got.Cause, CauseControllerDrift)
		}
	})

	t.Run("helmrelease with wrong controller subtype is unknown", func(t *testing.T) {
		owner := Ownership{Type: OwnerFlux, SubType: "helmrelease"}
		got := AttributeFieldMutation(makeResource(ManagerFluxKustomize), owner)
		if got.Cause != CauseUnknown {
			t.Errorf("Cause = %q, want %q (kustomize-controller does not manage HelmReleases)", got.Cause, CauseUnknown)
		}
	})

	t.Run("helmrelease with kustomize-controller + kubectl edit", func(t *testing.T) {
		owner := Ownership{Type: OwnerFlux, SubType: "helmrelease"}
		got := AttributeFieldMutation(makeResource(ManagerFluxKustomize, ManagerKubectlEdit), owner)
		// Kustomize-controller is not the expected controller for a
		// HelmRelease, but kubectl-edit is still interactive — manual-edit.
		if got.Cause != CauseManualEdit {
			t.Errorf("Cause = %q, want %q", got.Cause, CauseManualEdit)
		}
	})
}

func TestAttributeFieldMutation_HelmDirect(t *testing.T) {
	owner := Ownership{Type: OwnerHelm, SubType: "release"}
	got := AttributeFieldMutation(makeResource(ManagerHelm), owner)
	if got.Cause != CauseControllerDrift {
		t.Errorf("Cause = %q, want %q", got.Cause, CauseControllerDrift)
	}
	if got.ManagerHint != ManagerHelm {
		t.Errorf("ManagerHint = %q, want %q", got.ManagerHint, ManagerHelm)
	}
}

func TestAttributeFieldMutation_Crossplane(t *testing.T) {
	t.Run("composite XR", func(t *testing.T) {
		owner := Ownership{Type: OwnerCrossplane, SubType: "composite"}
		got := AttributeFieldMutation(makeResource(ManagerCrossplaneComposite), owner)
		if got.Cause != CauseControllerDrift {
			t.Errorf("Cause = %q, want %q", got.Cause, CauseControllerDrift)
		}
	})

	t.Run("composed children prefix match", func(t *testing.T) {
		owner := Ownership{Type: OwnerCrossplane, SubType: "managed-resource"}
		got := AttributeFieldMutation(makeResource("apiextensions.crossplane.io/composed-9f8e7d6c5b4a"), owner)
		if got.Cause != CauseControllerDrift {
			t.Errorf("Cause = %q, want %q (prefix match for composed children must work)", got.Cause, CauseControllerDrift)
		}
		if got.ManagerHint != "apiextensions.crossplane.io/composed-9f8e7d6c5b4a" {
			t.Errorf("ManagerHint = %q, want full string with hash", got.ManagerHint)
		}
	})

	t.Run("claim", func(t *testing.T) {
		owner := Ownership{Type: OwnerCrossplane, SubType: "claim"}
		got := AttributeFieldMutation(makeResource(ManagerCrossplaneClaim), owner)
		if got.Cause != CauseControllerDrift {
			t.Errorf("Cause = %q, want %q", got.Cause, CauseControllerDrift)
		}
	})

	t.Run("provider via ref resolver", func(t *testing.T) {
		owner := Ownership{Type: OwnerCrossplane, SubType: "managed-resource"}
		got := AttributeFieldMutation(makeResource(ManagerCrossplaneRefResolver), owner)
		if got.Cause != CauseControllerDrift {
			t.Errorf("Cause = %q, want %q", got.Cause, CauseControllerDrift)
		}
	})
}

func TestAttributeFieldMutation_Kro(t *testing.T) {
	owner := Ownership{Type: OwnerKro, SubType: "instance"}
	got := AttributeFieldMutation(makeResource(ManagerKroApplyset), owner)
	if got.Cause != CauseControllerDrift {
		t.Errorf("Cause = %q, want %q", got.Cause, CauseControllerDrift)
	}
}

func TestAttributeFieldMutation_ConfigHub(t *testing.T) {
	owner := Ownership{Type: OwnerConfigHub, SubType: "unit"}

	t.Run("delivered via Argo", func(t *testing.T) {
		got := AttributeFieldMutation(makeResource(ManagerArgoCD), owner)
		if got.Cause != CauseControllerDrift {
			t.Errorf("Cause = %q, want %q", got.Cause, CauseControllerDrift)
		}
	})

	t.Run("delivered via Flux kustomize", func(t *testing.T) {
		got := AttributeFieldMutation(makeResource(ManagerFluxKustomize), owner)
		if got.Cause != CauseControllerDrift {
			t.Errorf("Cause = %q, want %q", got.Cause, CauseControllerDrift)
		}
	})

	t.Run("unrelated controller is not controller-drift", func(t *testing.T) {
		got := AttributeFieldMutation(makeResource(ManagerCrossplaneClaim), owner)
		if got.Cause != CauseUnknown {
			t.Errorf("Cause = %q, want %q (Crossplane manager doesn't deliver ConfigHub units)", got.Cause, CauseUnknown)
		}
	})
}

func TestAttributeFieldMutation_EdgeCases(t *testing.T) {
	t.Run("nil resource", func(t *testing.T) {
		got := AttributeFieldMutation(nil, Ownership{Type: OwnerArgo})
		if got.Cause != CauseUnknown {
			t.Errorf("Cause = %q, want %q", got.Cause, CauseUnknown)
		}
		if got.ManagerHint != "" {
			t.Errorf("ManagerHint = %q, want empty", got.ManagerHint)
		}
		if len(got.Managers) != 0 {
			t.Errorf("Managers = %v, want empty", got.Managers)
		}
	})

	t.Run("no managedFields", func(t *testing.T) {
		u := &unstructured.Unstructured{Object: map[string]interface{}{}}
		got := AttributeFieldMutation(u, Ownership{Type: OwnerArgo})
		if got.Cause != CauseUnknown {
			t.Errorf("Cause = %q, want %q", got.Cause, CauseUnknown)
		}
	})

	t.Run("only unknown manager string", func(t *testing.T) {
		got := AttributeFieldMutation(makeResource("some-unrecognized-tool"), Ownership{Type: OwnerArgo})
		if got.Cause != CauseUnknown {
			t.Errorf("Cause = %q, want %q", got.Cause, CauseUnknown)
		}
		if got.ManagerHint != "some-unrecognized-tool" {
			t.Errorf("ManagerHint = %q, want %q (unrecognized hint surfaced for transparency)", got.ManagerHint, "some-unrecognized-tool")
		}
	})

	t.Run("empty manager entry skipped", func(t *testing.T) {
		got := AttributeFieldMutation(makeResource("", ManagerArgoCD), Ownership{Type: OwnerArgo})
		if got.Cause != CauseControllerDrift {
			t.Errorf("Cause = %q, want %q", got.Cause, CauseControllerDrift)
		}
		// Empty string must not appear in Managers.
		for _, m := range got.Managers {
			if m == "" {
				t.Errorf("Managers contains empty string: %v", got.Managers)
			}
		}
	})

	t.Run("duplicate managers deduplicated", func(t *testing.T) {
		got := AttributeFieldMutation(makeResource(ManagerArgoCD, ManagerArgoCD, ManagerArgoCD), Ownership{Type: OwnerArgo})
		if len(got.Managers) != 1 {
			t.Errorf("Managers = %v, want exactly 1 entry after dedup", got.Managers)
		}
	})

	t.Run("unknown owner with controller-looking string", func(t *testing.T) {
		// ArgoCD-controller string but ownership detection returned unknown.
		// The classifier shouldn't treat the string as controller-drift
		// without the expected-owner signal.
		got := AttributeFieldMutation(makeResource(ManagerArgoCD), Ownership{Type: OwnerUnknown})
		if got.Cause != CauseUnknown {
			t.Errorf("Cause = %q, want %q (cannot assume controller without owner signal)", got.Cause, CauseUnknown)
		}
	})

	t.Run("custom owner with no manager mapping", func(t *testing.T) {
		got := AttributeFieldMutation(makeResource(ManagerArgoCD), Ownership{Type: OwnerCustom})
		// No manager string is registered for OwnerCustom; the only thing the
		// classifier can do is fall through. argocd-controller is not in the
		// interactive set, so it goes to unknown.
		if got.Cause != CauseUnknown {
			t.Errorf("Cause = %q, want %q", got.Cause, CauseUnknown)
		}
	})
}

// TestAttributeFieldMutation_ManagerHintPrefersInteractiveWhenMixed locks in
// the mixed-case behavior: when both a controller and an interactive manager
// are present, the hint points at the interactive string because that's the
// cause-changing signal.
func TestAttributeFieldMutation_ManagerHintPrefersInteractiveWhenMixed(t *testing.T) {
	cases := []struct {
		name    string
		owner   Ownership
		mgrs    []string
		wantCause FieldMutationCause
		wantHint  string
	}{
		{
			name:      "argo + kubectl-edit",
			owner:     Ownership{Type: OwnerArgo, SubType: "application"},
			mgrs:      []string{ManagerArgoCD, ManagerKubectlEdit},
			wantCause: CauseManualEdit,
			wantHint:  ManagerKubectlEdit,
		},
		{
			name:      "flux kustomize + kubectl-patch",
			owner:     Ownership{Type: OwnerFlux, SubType: "kustomization"},
			mgrs:      []string{ManagerFluxKustomize, ManagerKubectlPatch},
			wantCause: CauseManualEdit,
			wantHint:  ManagerKubectlPatch,
		},
		{
			name:      "helm direct + kubectl-replace",
			owner:     Ownership{Type: OwnerHelm, SubType: "release"},
			mgrs:      []string{ManagerHelm, ManagerKubectlReplace},
			wantCause: CauseManualEdit,
			wantHint:  ManagerKubectlReplace,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := AttributeFieldMutation(makeResource(tc.mgrs...), tc.owner)
			if got.Cause != tc.wantCause {
				t.Errorf("Cause = %q, want %q", got.Cause, tc.wantCause)
			}
			if got.ManagerHint != tc.wantHint {
				t.Errorf("ManagerHint = %q, want %q", got.ManagerHint, tc.wantHint)
			}
		})
	}
}
