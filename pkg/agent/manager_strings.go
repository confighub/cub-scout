// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import "strings"

// K8s metadata.managedFields[].manager string constants, verified against
// upstream sources. Each constant's doc comment cites the source where the
// string is defined.
//
// The Attribution Layer (issue #435 / A1 issue #436) uses these constants
// together with ownership detection in ownership.go to classify a field
// mismatch as controller-drift, manual-edit, or unknown. See
// IsControllerManagerFor and IsInteractiveManager below.
//
// Strings are intentionally locked into named constants so that the
// verified enumeration is a single source of truth in the code, not free
// strings spread across call sites.

// --- GitOps / orchestration controller manager strings ---

// ManagerArgoCD is the field-manager used by Argo CD's application-controller
// for server-side apply. Source: argoproj/argo-cd `common.ArgoCDSSAManager`.
const ManagerArgoCD = "argocd-controller"

// ManagerFluxKustomize is the field-manager used by the Flux
// kustomize-controller. Source: fluxcd/kustomize-controller `controllerName`.
const ManagerFluxKustomize = "kustomize-controller"

// ManagerFluxHelm is the field-manager used by the Flux helm-controller.
// Source: fluxcd/helm-controller `controllerName`.
const ManagerFluxHelm = "helm-controller"

// ManagerFluxSource is the field-manager used by the Flux source-controller.
// Source: fluxcd/source-controller `controllerName`.
const ManagerFluxSource = "source-controller"

// ManagerHelm is the default field-manager used by the `helm` CLI for
// install/upgrade SSA patches. Source: helm/helm `kube.ManagedFieldsManager`
// default (`filepath.Base(os.Args[0])`). Third-party embedders (Flux
// helm-controller, terraform-provider-helm, etc.) override this with their
// own identity, so the bare string `helm` denotes a direct helm-binary
// invocation rather than a controller using the Helm library.
const ManagerHelm = "helm"

// ManagerCrossplaneComposite is the field-manager used by Crossplane's
// composite (XR) controller when writing fields on the XR itself. Source:
// crossplane/crossplane `FieldOwnerXR`.
const ManagerCrossplaneComposite = "apiextensions.crossplane.io/composite"

// ManagerCrossplaneComposedPrefix is the prefix of the field-manager used by
// the Crossplane composite controller when writing fields on composed
// children. The full string is `apiextensions.crossplane.io/composed-<hash>`
// where the suffix is per-XR. Match this constant by prefix only. Source:
// crossplane/crossplane `FieldOwnerComposedPrefix` + `ComposedFieldOwnerName`.
const ManagerCrossplaneComposedPrefix = "apiextensions.crossplane.io/composed"

// ManagerCrossplaneClaim is the field-manager used by the Crossplane claim
// SSA syncer. Source: crossplane/crossplane claim package `FieldOwnerXR`
// constant (note: symbol name in upstream is misleading; value is
// `.../claim`).
const ManagerCrossplaneClaim = "apiextensions.crossplane.io/claim"

// ManagerCrossplaneMRD is the field-manager used by the Crossplane MRD
// (managed-resource-definition) reconciler. Source: crossplane/crossplane
// `FieldOwnerMRD`.
const ManagerCrossplaneMRD = "apiextensions.crossplane.io/managed"

// ManagerCrossplaneRefResolver is the field-manager used by
// crossplane-runtime's reference resolver. Used by every provider that links
// via crossplane-runtime. Source: crossplane/crossplane-runtime
// `fieldOwnerAPISimpleRefResolver`.
const ManagerCrossplaneRefResolver = "managed.crossplane.io/api-simple-reference-resolver"

// ManagerKroApplyset is the field-manager used by kro for applyset-managed
// child resources. Source: kro-run/kro `applyset.FieldManager`.
const ManagerKroApplyset = "kro.run/applyset"

// ManagerKroApplysetParent is the field-manager used by kro for applyset
// metadata on the parent instance. Source: kro-run/kro
// `applyset.FieldManager + "-parent"`.
const ManagerKroApplysetParent = "kro.run/applyset-parent"

// ManagerKroLabeller is the field-manager used by kro for finalizer/label SSA
// on instances. Source: kro-run/kro `FieldManagerForLabeler`.
const ManagerKroLabeller = "kro.run/labeller"

// --- Interactive (kubectl) manager strings ---

// ManagerKubectlSSA is the field-manager used by `kubectl apply --server-side`.
// Source: kubernetes/kubectl `pkg/cmd/apply/apply.go` `fieldManagerServerSideApply`.
// The bare string `kubectl` is deliberate (back-compat with pre-SSA-flag kubectl).
const ManagerKubectlSSA = "kubectl"

// ManagerKubectlCSA is the field-manager used by `kubectl apply` (client-side,
// the default). Source: kubernetes/kubectl `FieldManagerClientSideApply`.
// Note: Argo CD's CSA migration uses the same string; the classifier
// disambiguates via the owner co-signal (see IsControllerManagerFor).
const ManagerKubectlCSA = "kubectl-client-side-apply"

// ManagerKubectlEdit is the field-manager used by `kubectl edit`.
// Source: kubernetes/kubectl `pkg/cmd/edit/edit.go`.
const ManagerKubectlEdit = "kubectl-edit"

// ManagerKubectlPatch is the field-manager used by `kubectl patch`.
// Source: kubernetes/kubectl `pkg/cmd/patch/patch.go`.
const ManagerKubectlPatch = "kubectl-patch"

// ManagerKubectlCreate is the field-manager used by `kubectl create`.
// Source: kubernetes/kubectl `pkg/cmd/create/create.go`.
const ManagerKubectlCreate = "kubectl-create"

// ManagerKubectlReplace is the field-manager used by `kubectl replace`.
// Source: kubernetes/kubectl `pkg/cmd/replace/replace.go`.
const ManagerKubectlReplace = "kubectl-replace"

// ManagerKubectlLastApplied is the field-manager used by kubectl during the
// CSA-to-SSA migration on the `kubectl.kubernetes.io/last-applied-configuration`
// annotation. Source: kubernetes/kubectl `fieldManagerLastAppliedAnnotation`.
const ManagerKubectlLastApplied = "kubectl-last-applied"

// managerMatcher describes how a manager string is matched against an entry
// from metadata.managedFields. Most matches are exact; Crossplane composed
// children use a hashed suffix that requires prefix matching.
type managerMatcher struct {
	pattern  string
	isPrefix bool
}

func (m managerMatcher) matches(manager string) bool {
	if m.isPrefix {
		return strings.HasPrefix(manager, m.pattern)
	}
	return manager == m.pattern
}

// controllerManagersForOwner returns the set of manager-string matchers
// indicating the given owner is reconciling fields. The ownerSubType is
// honored when present to narrow Flux to the specific controller (kustomize
// vs helm vs source); when not, all three are accepted.
func controllerManagersForOwner(ownerType, ownerSubType string) []managerMatcher {
	switch ownerType {
	case OwnerArgo:
		return []managerMatcher{
			{pattern: ManagerArgoCD},
			{pattern: ManagerKubectlCSA}, // Argo CSA migration default
		}
	case OwnerFlux:
		switch ownerSubType {
		case "kustomization":
			return []managerMatcher{{pattern: ManagerFluxKustomize}}
		case "helmrelease":
			return []managerMatcher{{pattern: ManagerFluxHelm}}
		case "source", "gitrepository", "ocirepository", "helmrepository", "bucket":
			return []managerMatcher{{pattern: ManagerFluxSource}}
		default:
			return []managerMatcher{
				{pattern: ManagerFluxKustomize},
				{pattern: ManagerFluxHelm},
				{pattern: ManagerFluxSource},
			}
		}
	case OwnerHelm:
		return []managerMatcher{{pattern: ManagerHelm}}
	case OwnerCrossplane:
		return []managerMatcher{
			{pattern: ManagerCrossplaneComposite},
			{pattern: ManagerCrossplaneComposedPrefix, isPrefix: true},
			{pattern: ManagerCrossplaneClaim},
			{pattern: ManagerCrossplaneMRD},
			{pattern: ManagerCrossplaneRefResolver},
		}
	case OwnerKro:
		return []managerMatcher{
			{pattern: ManagerKroApplyset},
			{pattern: ManagerKroApplysetParent},
			{pattern: ManagerKroLabeller},
		}
	case OwnerConfigHub:
		// ConfigHub units are delivered by Argo or Flux controllers; accept
		// either's strings as controller-drift evidence.
		return []managerMatcher{
			{pattern: ManagerArgoCD},
			{pattern: ManagerKubectlCSA},
			{pattern: ManagerFluxKustomize},
			{pattern: ManagerFluxHelm},
		}
	default:
		return nil
	}
}

// interactiveManagers is the set of kubectl/CLI manager strings that indicate
// a manual edit when not accompanied by a controller co-signal.
var interactiveManagers = []managerMatcher{
	{pattern: ManagerKubectlSSA},
	{pattern: ManagerKubectlCSA},
	{pattern: ManagerKubectlEdit},
	{pattern: ManagerKubectlPatch},
	{pattern: ManagerKubectlCreate},
	{pattern: ManagerKubectlReplace},
	{pattern: ManagerKubectlLastApplied},
}

// IsControllerManagerFor returns true when the manager string indicates the
// GitOps/orchestration controller expected for the given owner is reconciling
// fields. The (ownerType, ownerSubType) co-signal handles overlapping strings:
// for example, `kubectl-client-side-apply` is Argo CD's CSA migration default,
// so an Argo-owned resource with that manager is controller-drift, not
// manual-edit.
//
// Returns false when:
//   - the owner type is unknown or unrecognized
//   - the manager string does not match any known controller pattern for the owner
func IsControllerManagerFor(manager, ownerType, ownerSubType string) bool {
	if manager == "" {
		return false
	}
	for _, m := range controllerManagersForOwner(ownerType, ownerSubType) {
		if m.matches(manager) {
			return true
		}
	}
	return false
}

// IsInteractiveManager returns true for manager strings that indicate a
// kubectl/CLI write. Callers classifying a field mismatch should check
// IsControllerManagerFor first when the resource has a known GitOps owner —
// see the package doc for the classifier co-signal rule.
func IsInteractiveManager(manager string) bool {
	if manager == "" {
		return false
	}
	for _, m := range interactiveManagers {
		if m.matches(manager) {
			return true
		}
	}
	return false
}
