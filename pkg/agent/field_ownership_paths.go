// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"bytes"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/structured-merge-diff/v4/fieldpath"
)

// AttributeFieldsByManagedFields extends AttributeFieldMutation by computing
// per-field-path classifications when the K8s metadata.managedFields entries
// carry FieldsV1 data. Returns both the per-path map and the resource-level
// rollup (the latter matching AttributeFieldMutation).
//
// Keys in the returned map are canonical field-path strings as rendered by
// sigs.k8s.io/structured-merge-diff/v4/fieldpath.Path.String — for example
// ".spec.replicas" or ".spec.template.spec.containers[name=\"app\"].image".
//
// Per-path classification follows the same co-signal rule as the
// resource-level classifier:
//   - Any manager owning this path matches the expected owner's known
//     controller string → CauseControllerDrift.
//   - Only kubectl-*/bare kubectl owners on this path → CauseManualEdit.
//   - Both → CauseManualEdit (operator edited on top of controller).
//   - Otherwise → CauseUnknown.
//
// FieldsV1 entries that fail to decode are skipped silently; they do not
// produce a classification of their own (graceful degradation).
func AttributeFieldsByManagedFields(resource *unstructured.Unstructured, expectedOwner Ownership) (FieldMutationAttribution, map[string]FieldMutationAttribution) {
	resourceLevel := AttributeFieldMutation(resource, expectedOwner)
	if resource == nil {
		return resourceLevel, nil
	}
	entries := resource.GetManagedFields()
	if len(entries) == 0 {
		return resourceLevel, nil
	}

	type pathAccum struct {
		sawController   bool
		sawInteractive  bool
		controllerHint  string
		interactiveHint string
	}
	byPath := make(map[string]*pathAccum)

	for _, e := range entries {
		m := e.Manager
		if m == "" || e.FieldsV1 == nil || len(e.FieldsV1.Raw) == 0 {
			continue
		}
		set := &fieldpath.Set{}
		if err := set.FromJSON(bytes.NewReader(e.FieldsV1.Raw)); err != nil {
			continue
		}

		isController := IsControllerManagerFor(m, expectedOwner.Type, expectedOwner.SubType)
		isInteractive := !isController && IsInteractiveManager(m)
		if !isController && !isInteractive {
			continue
		}

		set.Iterate(func(p fieldpath.Path) {
			key := p.String()
			acc, ok := byPath[key]
			if !ok {
				acc = &pathAccum{}
				byPath[key] = acc
			}
			if isController {
				acc.sawController = true
				if acc.controllerHint == "" {
					acc.controllerHint = m
				}
			} else if isInteractive {
				acc.sawInteractive = true
				if acc.interactiveHint == "" {
					acc.interactiveHint = m
				}
			}
		})
	}

	if len(byPath) == 0 {
		return resourceLevel, nil
	}

	out := make(map[string]FieldMutationAttribution, len(byPath))
	for path, acc := range byPath {
		switch {
		case acc.sawController && acc.sawInteractive:
			out[path] = FieldMutationAttribution{
				Cause:       CauseManualEdit,
				ManagerHint: acc.interactiveHint,
			}
		case acc.sawController:
			out[path] = FieldMutationAttribution{
				Cause:       CauseControllerDrift,
				ManagerHint: acc.controllerHint,
			}
		case acc.sawInteractive:
			out[path] = FieldMutationAttribution{
				Cause:       CauseManualEdit,
				ManagerHint: acc.interactiveHint,
			}
		}
	}
	return resourceLevel, out
}
