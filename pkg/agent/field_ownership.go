// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// FieldMutationCause classifies the cause of a field mutation observed on a
// live K8s resource, based on metadata.managedFields and the expected owner.
type FieldMutationCause string

const (
	// CauseControllerDrift indicates the resource's expected GitOps or
	// orchestration controller is reconciling fields. A mismatch with the
	// desired state is likely transient.
	CauseControllerDrift FieldMutationCause = "controller-drift"

	// CauseManualEdit indicates an interactive tool (kubectl-*) has written
	// fields on the resource. Also reported when both a controller and an
	// interactive tool have managed fields on the resource — the operator
	// edited on top of controller reconciliation.
	CauseManualEdit FieldMutationCause = "manual-edit"

	// CauseUnknown indicates the cause cannot be confidently determined,
	// either because managedFields is missing/empty or because the manager
	// strings present are not in the verified enumeration. Returning
	// unknown is preferred over guessing.
	CauseUnknown FieldMutationCause = "unknown"
)

// FieldMutationAttribution is the result of classifying a field mutation.
type FieldMutationAttribution struct {
	// Cause is the classified cause of the mutation.
	Cause FieldMutationCause `json:"cause"`

	// ManagerHint is a representative manager string for transparency. For
	// CauseControllerDrift it is the matched controller manager. For
	// CauseManualEdit it is the matched interactive manager (which is the
	// cause-changing signal even when mixed with a controller string). For
	// CauseUnknown it is the first unrecognized manager string if any.
	ManagerHint string `json:"managerHint,omitempty"`

	// Managers is the de-duplicated list of all manager strings observed on
	// the resource, in first-seen order. Provided for full transparency for
	// operators and downstream tooling that want the raw signal.
	Managers []string `json:"managers,omitempty"`
}

// AttributeFieldMutation classifies the cause of field mutations on a live
// K8s resource by reading metadata.managedFields and applying the
// expected-owner co-signal:
//
//   - Any managedFields entry whose manager matches the expected owner's
//     known controller string → CauseControllerDrift.
//   - Otherwise, any entry whose manager is a known interactive (kubectl-*)
//     string → CauseManualEdit.
//   - If both are seen → CauseManualEdit (operator edited on top of
//     controller reconciliation; surface the manual involvement).
//   - Otherwise → CauseUnknown.
//
// Missing or empty managedFields returns CauseUnknown. Manager strings not in
// the verified enumeration in manager_strings.go fall through to CauseUnknown
// — the function never guesses.
//
// expectedOwner is typically obtained via DetectOwnership on the same resource.
func AttributeFieldMutation(resource *unstructured.Unstructured, expectedOwner Ownership) FieldMutationAttribution {
	if resource == nil {
		return FieldMutationAttribution{Cause: CauseUnknown}
	}

	entries := resource.GetManagedFields()
	if len(entries) == 0 {
		return FieldMutationAttribution{Cause: CauseUnknown}
	}

	var (
		sawController   bool
		sawInteractive  bool
		controllerHint  string
		interactiveHint string
		managers        []string
	)
	seen := make(map[string]struct{})

	for _, e := range entries {
		m := e.Manager
		if m == "" {
			continue
		}
		if _, dup := seen[m]; !dup {
			seen[m] = struct{}{}
			managers = append(managers, m)
		}

		if IsControllerManagerFor(m, expectedOwner.Type, expectedOwner.SubType) {
			sawController = true
			if controllerHint == "" {
				controllerHint = m
			}
			continue
		}
		if IsInteractiveManager(m) {
			sawInteractive = true
			if interactiveHint == "" {
				interactiveHint = m
			}
		}
	}

	switch {
	case sawController && sawInteractive:
		return FieldMutationAttribution{
			Cause:       CauseManualEdit,
			ManagerHint: interactiveHint,
			Managers:    managers,
		}
	case sawController:
		return FieldMutationAttribution{
			Cause:       CauseControllerDrift,
			ManagerHint: controllerHint,
			Managers:    managers,
		}
	case sawInteractive:
		return FieldMutationAttribution{
			Cause:       CauseManualEdit,
			ManagerHint: interactiveHint,
			Managers:    managers,
		}
	default:
		hint := ""
		if len(managers) > 0 {
			hint = managers[0]
		}
		return FieldMutationAttribution{
			Cause:       CauseUnknown,
			ManagerHint: hint,
			Managers:    managers,
		}
	}
}
