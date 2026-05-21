// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import "testing"

func TestIsControllerManagerFor(t *testing.T) {
	tests := []struct {
		name         string
		manager      string
		ownerType    string
		ownerSubType string
		want         bool
	}{
		// Argo CD — both the SSA-default and CSA-migration default count as
		// controller-drift when the owner is Argo.
		{"argo ssa", ManagerArgoCD, OwnerArgo, "application", true},
		{"argo csa migration", ManagerKubectlCSA, OwnerArgo, "application", true},

		// Flux — narrows by subtype.
		{"flux kustomize for kustomization", ManagerFluxKustomize, OwnerFlux, "kustomization", true},
		{"flux helm for helmrelease", ManagerFluxHelm, OwnerFlux, "helmrelease", true},
		{"flux source for gitrepository", ManagerFluxSource, OwnerFlux, "gitrepository", true},
		{"flux source for ocirepository", ManagerFluxSource, OwnerFlux, "ocirepository", true},
		{"flux source for bucket", ManagerFluxSource, OwnerFlux, "bucket", true},
		{"flux kustomize wrong subtype", ManagerFluxKustomize, OwnerFlux, "helmrelease", false},
		{"flux helm wrong subtype", ManagerFluxHelm, OwnerFlux, "kustomization", false},
		// Unknown Flux subtype accepts any Flux controller.
		{"flux unknown subtype accepts kustomize", ManagerFluxKustomize, OwnerFlux, "", true},
		{"flux unknown subtype accepts helm", ManagerFluxHelm, OwnerFlux, "", true},
		{"flux unknown subtype accepts source", ManagerFluxSource, OwnerFlux, "", true},

		// Helm direct.
		{"helm direct", ManagerHelm, OwnerHelm, "release", true},
		{"helm direct does not match flux helm controller", ManagerFluxHelm, OwnerHelm, "release", false},

		// Crossplane — exact for most, prefix for composed children.
		{"crossplane composite", ManagerCrossplaneComposite, OwnerCrossplane, "composite", true},
		{"crossplane composed prefix exact", ManagerCrossplaneComposedPrefix, OwnerCrossplane, "managed-resource", true},
		{"crossplane composed with hash suffix", "apiextensions.crossplane.io/composed-abcd1234ef567890", OwnerCrossplane, "managed-resource", true},
		{"crossplane composed with another hash", "apiextensions.crossplane.io/composed-deadbeef", OwnerCrossplane, "composite", true},
		{"crossplane claim", ManagerCrossplaneClaim, OwnerCrossplane, "claim", true},
		{"crossplane mrd", ManagerCrossplaneMRD, OwnerCrossplane, "managed-resource", true},
		{"crossplane ref resolver", ManagerCrossplaneRefResolver, OwnerCrossplane, "managed-resource", true},
		{"crossplane unrelated string", "apiextensions.crossplane.io/something-else", OwnerCrossplane, "claim", false},

		// kro.
		{"kro applyset", ManagerKroApplyset, OwnerKro, "instance", true},
		{"kro applyset-parent", ManagerKroApplysetParent, OwnerKro, "instance", true},
		{"kro labeller", ManagerKroLabeller, OwnerKro, "instance", true},
		{"kro unrelated string", "kro.run/other", OwnerKro, "instance", false},

		// ConfigHub units are delivered by GitOps controllers; accept either's strings.
		{"confighub via argo", ManagerArgoCD, OwnerConfigHub, "unit", true},
		{"confighub via argo csa migration", ManagerKubectlCSA, OwnerConfigHub, "unit", true},
		{"confighub via flux kustomize", ManagerFluxKustomize, OwnerConfigHub, "unit", true},
		{"confighub via flux helm", ManagerFluxHelm, OwnerConfigHub, "unit", true},
		{"confighub does not accept random controller", ManagerCrossplaneClaim, OwnerConfigHub, "unit", false},

		// Co-signal: kubectl-client-side-apply is only a controller string when
		// the owner is Argo or ConfigHub-delivered-via-Argo. Other owners treat
		// it as manual via IsInteractiveManager.
		{"csa for kubernetes-native is not controller", ManagerKubectlCSA, OwnerKubernetes, "", false},
		{"csa for helm is not controller", ManagerKubectlCSA, OwnerHelm, "release", false},
		{"csa for crossplane is not controller", ManagerKubectlCSA, OwnerCrossplane, "claim", false},
		{"csa for unknown owner is not controller", ManagerKubectlCSA, OwnerUnknown, "", false},

		// Empty strings + unknown manager.
		{"empty manager for argo", "", OwnerArgo, "application", false},
		{"empty manager for native", "", OwnerKubernetes, "", false},
		{"unknown manager for argo", "some-random-tool", OwnerArgo, "application", false},
		{"unknown manager for native", "some-random-tool", OwnerKubernetes, "", false},

		// Unknown owner type returns false for any manager.
		{"unknown owner with argo string", ManagerArgoCD, OwnerUnknown, "", false},
		{"empty owner type", ManagerArgoCD, "", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := IsControllerManagerFor(tc.manager, tc.ownerType, tc.ownerSubType)
			if got != tc.want {
				t.Errorf("IsControllerManagerFor(%q, %q, %q) = %v, want %v",
					tc.manager, tc.ownerType, tc.ownerSubType, got, tc.want)
			}
		})
	}
}

func TestIsInteractiveManager(t *testing.T) {
	tests := []struct {
		name    string
		manager string
		want    bool
	}{
		{"kubectl ssa bare", ManagerKubectlSSA, true},
		{"kubectl csa", ManagerKubectlCSA, true},
		{"kubectl edit", ManagerKubectlEdit, true},
		{"kubectl patch", ManagerKubectlPatch, true},
		{"kubectl create", ManagerKubectlCreate, true},
		{"kubectl replace", ManagerKubectlReplace, true},
		{"kubectl last-applied", ManagerKubectlLastApplied, true},

		// Controller strings are not interactive.
		{"argocd-controller is not interactive", ManagerArgoCD, false},
		{"kustomize-controller is not interactive", ManagerFluxKustomize, false},
		{"helm-controller is not interactive", ManagerFluxHelm, false},
		{"source-controller is not interactive", ManagerFluxSource, false},
		{"helm direct is not interactive", ManagerHelm, false},
		{"crossplane composite is not interactive", ManagerCrossplaneComposite, false},
		{"kro applyset is not interactive", ManagerKroApplyset, false},

		// Edge cases.
		{"empty manager", "", false},
		{"unknown manager", "some-random-tool", false},
		{"kubectl-like but unknown", "kubectl-fancy-new-thing", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := IsInteractiveManager(tc.manager)
			if got != tc.want {
				t.Errorf("IsInteractiveManager(%q) = %v, want %v", tc.manager, got, tc.want)
			}
		})
	}
}

// TestManagerStringCoSignalDisambiguates documents the key co-signal property
// for the kubectl-client-side-apply string: it appears as a controller string
// for Argo-owned resources (CSA migration) and as an interactive string for
// non-Argo owners. The classifier in field_ownership.go relies on this
// asymmetric behavior; if this test starts failing because both functions
// agree, the disambiguation is broken.
func TestManagerStringCoSignalDisambiguates(t *testing.T) {
	const csa = ManagerKubectlCSA

	if !IsControllerManagerFor(csa, OwnerArgo, "application") {
		t.Errorf("CSA must be a controller manager for Argo (CSA migration default)")
	}
	if !IsInteractiveManager(csa) {
		t.Errorf("CSA must remain in the interactive set for the classifier's mixed-case rule")
	}
	if IsControllerManagerFor(csa, OwnerKubernetes, "") {
		t.Errorf("CSA must NOT be a controller manager for native resources — that would misclassify kubectl apply as controller-drift")
	}
}
